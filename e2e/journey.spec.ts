import { execFileSync, spawn, type ChildProcess } from "node:child_process";
import { mkdtempSync, readFileSync, readdirSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { expect, test } from "@playwright/test";

const fumori = join(process.cwd(), "bin", "fumori");

function startServer(
  vault: string,
  state: string
): Promise<{ url: string; child: ChildProcess }> {
  const child = spawn(
    fumori,
    ["serve", "--vault", vault, "--state", state, "--port", "0"],
    { stdio: ["ignore", "pipe", "inherit"] }
  );
  return new Promise((resolve, reject) => {
    child.stdout!.on("data", (chunk: Buffer) => {
      const match = /at (http:\/\/\S+)/.exec(chunk.toString());
      if (match) resolve({ url: match[1]!, child });
    });
    child.once("exit", (code) => reject(new Error(`server exited ${code}`)));
  });
}

function stop(child: ChildProcess): Promise<void> {
  return new Promise((resolve) => {
    child.once("exit", () => resolve());
    child.kill("SIGTERM");
  });
}

test("write Today, capture memos, find notes, and survive a restart", async ({
  page,
  request
}) => {
  const root = mkdtempSync(join(tmpdir(), "fumori-e2e-"));
  const vault = join(root, "vault");
  const state = join(root, "state");
  execFileSync("git", ["init", "-q", vault]);
  execFileSync(fumori, [
    "bootstrap",
    "--vault",
    vault,
    "--timezone",
    "America/Los_Angeles"
  ]);
  let server = await startServer(vault, state);
  const saved = page.locator(".save-indicator[data-save-state='saved']");

  await page.goto(server.url);
  await expect(page).toHaveURL(/\/today$/);
  const editor = page.locator(
    "[data-testid='rich-editor'] [contenteditable='true']"
  );
  await editor.click();
  await page.keyboard.type("今天检查服务器备份。");
  await expect(saved).toBeVisible();

  await page.getByLabel("Quick capture").fill("想试一下旧手机做机器人摄像头");
  await page.getByRole("button", { name: "Capture" }).click();
  const memo = page.locator(".memo").first();
  await expect(memo).toContainText("旧手机");
  await memo.getByRole("button", { name: "Comment" }).click();
  await memo.getByLabel("Comment").fill("USB 供电即可");
  await memo.getByRole("button", { name: "Add" }).click();
  await expect(memo.locator(".memo-comment")).toContainText("USB 供电");

  await editor.click();
  await page.keyboard.press("End");
  await page.keyboard.type(" 顺便清理镜像。");
  await expect(saved).toBeVisible();

  const created = await (
    await request.post(`${server.url}/api/v1/notes`, {
      data: { context: "global" }
    })
  ).json();
  await request.put(`${server.url}/api/v1/notes/${created.id}`, {
    data: {
      format: "rich",
      baseRevision: created.revision,
      bodyMarkdown: "# ESP32-S3 摄像头小车\n\n延迟测试记录。"
    }
  });
  await page.goto(`${server.url}/search?q=${encodeURIComponent("摄像头")}`);
  const results = page.locator(".search-results");
  await expect(results).toContainText("ESP32-S3 摄像头小车");
  await expect(results).toContainText(/\d{4}-\d{2}-\d{2}/);

  await stop(server.child);
  const dailyDir = join(vault, "human", "daily");
  const daily = readdirSync(dailyDir, { recursive: true })
    .map(String)
    .find((file) => file.endsWith(".md"));
  const source = readFileSync(join(dailyDir, daily!), "utf8");
  expect(source).toContain("今天检查服务器备份。 顺便清理镜像。");
  expect(source).toMatch(/<!-- fumori:memo \{"id":"m_[0-9A-Z]{12}"/);
  expect(source).toMatch(/<!-- fumori:comment \{"id":"c_[0-9A-Z]{12}","parent":"m_/);
  expect(
    execFileSync("git", ["-C", vault, "status", "--porcelain"]).toString()
  ).toBe("");

  server = await startServer(vault, state);
  await page.goto(`${server.url}/today`);
  await expect(page.locator("[data-testid='rich-editor']")).toContainText(
    "顺便清理镜像"
  );
  await expect(page.locator(".memo-comment")).toContainText("USB 供电");
  await stop(server.child);
});

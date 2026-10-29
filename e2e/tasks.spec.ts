import { execFileSync, spawn, type ChildProcess } from "node:child_process";
import { mkdtempSync, readFileSync, readdirSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { expect, test } from "@playwright/test";

const calepin = join(process.cwd(), "bin", "calepin");

function plusDays(date: string, days: number): string {
  const d = new Date(`${date}T12:00:00Z`);
  d.setUTCDate(d.getUTCDate() + days);
  return d.toISOString().slice(0, 10);
}

test("run a routine and triage a captured task", async ({ page, request }) => {
  const root = mkdtempSync(join(tmpdir(), "calepin-tasks-"));
  const vault = join(root, "vault");
  execFileSync("git", ["init", "-q", vault]);
  execFileSync(calepin, ["bootstrap", "--vault", vault, "--timezone", "UTC"]);
  const child: ChildProcess = spawn(
    calepin,
    ["serve", "--vault", vault, "--state", join(root, "state"), "--port", "0"],
    { stdio: ["ignore", "pipe", "inherit"] }
  );
  const url = await new Promise<string>((resolve) =>
    child.stdout!.on("data", (chunk: Buffer) => {
      const match = /at (http:\/\/\S+)/.exec(chunk.toString());
      if (match) resolve(match[1]!);
    })
  );
  try {
    const createNote = async (data: object, body: string) => {
      const created = await (await request.post(`${url}/api/v1/notes`, { data })).json();
      await request.put(`${url}/api/v1/notes/${created.id}`, {
        data: { format: "rich", baseRevision: created.revision, bodyMarkdown: body }
      });
      return created.id as string;
    };
    await createNote({ context: "type", type: "task-list" }, "# 家庭服务器");
    const routine = await createNote(
      { context: "global" },
      "# 服务器例行检查\n\n## 检查流程\n\n- 查看备份状态"
    );
    const today = (await (await request.get(`${url}/api/v1/today`)).json()).date as string;

    await page.goto(`${url}/notes/${routine}`);
    const panel = page.getByRole("region", { name: "Task" });
    await panel.getByRole("button", { name: "Make this a task" }).click();
    await panel.getByLabel("Repeat", { exact: true }).selectOption("weekly");
    await panel.getByLabel("Task List").selectOption("家庭服务器");
    await panel.getByRole("button", { name: "Create task" }).click();
    await expect(panel).toContainText(`Next round ${today}`);

    await panel.getByLabel("Notes for this round").fill("备份正常，清理了旧镜像。");
    await panel.getByRole("button", { name: "Complete round" }).click();
    await expect(panel).toContainText(`Next round ${plusDays(today, 7)}`);
    await expect(panel.locator(".task-record")).toContainText("备份正常");
    await expect(page.locator("[data-testid='rich-editor']")).toContainText("查看备份状态");
    await expect(page.getByText("Protected Markdown")).toHaveCount(0);

    await page.goto(`${url}/tasks?list=${encodeURIComponent("家庭服务器")}`);
    await expect(page.locator(".task-row")).toContainText("服务器例行检查");

    await page.goto(`${url}/today`);
    await page.getByLabel("Quick capture").fill("买网线");
    await page.getByLabel("As task").check();
    await page.getByRole("button", { name: "Capture" }).click();
    await expect(page.locator(".memo")).toContainText("买网线");

    await page.goto(`${url}/tasks?filter=unlisted`);
    const row = page.locator(".task-row", { hasText: "买网线" });
    await expect(row).toBeVisible();
    const due = plusDays(today, 3);
    await row.getByLabel("Due date for 买网线").fill(due);
    await expect(row.getByLabel("Due date for 买网线")).toHaveValue(due);
    await row.getByLabel("List for 买网线").selectOption("家庭服务器");
    await expect(page.locator(".task-row", { hasText: "买网线" })).toHaveCount(0);
    await page.getByRole("button", { name: /家庭服务器/ }).click();
    await page.getByLabel("Complete 买网线").check();
    await page.getByRole("button", { name: /Done/ }).click();
    await expect(page.locator(".task-row", { hasText: "买网线" })).toBeVisible();

    const notes = join(vault, "human", "notes");
    const routineFile = readdirSync(notes).find((f) => f.startsWith("服务器例行检查"))!;
    const routineSource = readFileSync(join(notes, routineFile), "utf8");
    expect(routineSource).toMatch(/## History\n\n<!-- calepin:occurrence \{"id":"o_/);
    expect(routineSource).toContain("备份正常，清理了旧镜像。");
    const dailyDir = join(vault, "human", "daily");
    const daily = readdirSync(dailyDir, { recursive: true }).map(String).find((f) => f.endsWith(".md"))!;
    expect(readFileSync(join(dailyDir, daily), "utf8")).toMatch(
      /- \[x\] 买网线 <!-- calepin:task \{"id":"t_[0-9A-Z]{12}","list":"\[\[家庭服务器\]\]","due":"\d{4}-\d{2}-\d{2}","completed":/
    );
  } finally {
    child.kill("SIGTERM");
  }
});

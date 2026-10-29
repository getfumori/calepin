import { execFileSync, spawn } from "node:child_process";
import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { expect, test } from "@playwright/test";
import type { ZodType } from "zod";

import { appConfigSchema } from "../src/web/contracts/app-config";
import {
  noteConnectionsResponseSchema,
  wikilinkSuggestionListSchema
} from "../src/web/contracts/connections";
import { dailyNoteResponseSchema } from "../src/web/contracts/daily-note";
import {
  humanNoteDeletionImpactResponseSchema,
  humanNoteListResponseSchema,
  humanNoteResponseSchema
} from "../src/web/contracts/human-note";
import {
  organizationModelResponseSchema,
  savedViewListResponseSchema,
  savedViewResultResponseSchema,
  typeDefinitionListResponseSchema,
  typeResultResponseSchema
} from "../src/web/contracts/organization-model";
import { searchResponseSchema } from "../src/web/contracts/search";
import {
  dailyWriteResponseSchema,
  endpoints,
  taskCommandResponseSchema
} from "../src/web/contracts/api";

const calepin = join(process.cwd(), "bin", "calepin");

const flagView = `---
_schema: calepin.model.view
_version: 1
key: flagged
name: Flagged
query:
  filter:
    all:
      - {field: kind, operator: equals, value: standalone}
      - {not: {field: archived_flag, operator: equals, value: false}}
  group_by: state
  layout: table
---
`;

test("every Go API response satisfies the Web client contracts", async ({
  request
}) => {
  const root = mkdtempSync(join(tmpdir(), "calepin-contract-"));
  const vault = join(root, "vault");
  execFileSync("git", ["init", "-q", vault]);
  execFileSync(calepin, ["bootstrap", "--vault", vault, "--timezone", "UTC"]);
  writeFileSync(join(vault, ".calepin/model/views/flagged.md"), flagView);
  const child = spawn(
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
    const get = async (path: string, schema: ZodType) => {
      const response = await request.get(url + path);
      expect(response.ok(), path).toBe(true);
      const result = schema.safeParse(await response.json());
      expect(result.error?.issues ?? [], path).toEqual([]);
      return result.data as Record<string, unknown>;
    };

    await request.post(`${url}/api/v1/capture`, { data: { text: "备份 memo" } });
    const target = humanNoteResponseSchema.parse(
      await (await request.post(`${url}/api/v1/notes`, { data: { context: "wikilink", target: "Target" } })).json()
    );
    const source = humanNoteResponseSchema.parse(
      await (await request.post(`${url}/api/v1/notes`, { data: { context: "global" } })).json()
    );
    await request.put(`${url}/api/v1/notes/${source.id}`, {
      data: { format: "rich", baseRevision: source.revision, bodyMarkdown: "# 备份 Source\n\n[[Target]] [[Missing]]" }
    });

    for (const endpoint of endpoints) {
      if (endpoint.method === "get" && !endpoint.path.includes("{") && !endpoint.query?.q) {
        await get(endpoint.path, endpoint.response!);
      }
    }
    const made = await request.post(`${url}/api/v1/tasks/${target.id}/make`, {
      data: { scheduled: "2026-11-01", recurrence: { basis: "schedule", frequency: "weekly", interval: 1, weekdays: ["sun"] } }
    });
    expect(taskCommandResponseSchema.safeParse(await made.json()).success).toBe(true);
    const captured = await request.post(`${url}/api/v1/capture`, { data: { text: "买网线", task: true } });
    expect(dailyWriteResponseSchema.safeParse(await captured.json()).success).toBe(true);
    await get(`/api/v1/notes/${target.id}`, humanNoteResponseSchema);
    await get("/api/v1/tasks?status=all", endpoints.find((e) => e.path === "/api/v1/tasks")!.response!);
    await get("/api/v1/config", appConfigSchema);
    const today = await get("/api/v1/today", dailyNoteResponseSchema);
    await get(`/api/v1/daily/${today.date}`, dailyNoteResponseSchema);
    await get("/api/v1/daily/2020-01-01", dailyNoteResponseSchema);
    await get("/api/v1/notes", humanNoteListResponseSchema);
    await get("/api/v1/inbox", humanNoteListResponseSchema);
    await get("/api/v1/archive", humanNoteListResponseSchema);
    await get(`/api/v1/notes/${source.id}`, humanNoteResponseSchema);
    await get(`/api/v1/notes/${target.id}/deletion-impact`, humanNoteDeletionImpactResponseSchema);
    await get(`/api/v1/connections/${source.id}`, noteConnectionsResponseSchema);
    await get(`/api/v1/connections/${target.id}`, noteConnectionsResponseSchema);
    await get("/api/v1/wikilinks/suggestions?q=t", wikilinkSuggestionListSchema);
    await get("/api/v1/model", organizationModelResponseSchema);
    await get("/api/v1/types", typeDefinitionListResponseSchema);
    for (const key of ["note", "daily-note", "task-list"]) {
      await get(`/api/v1/types/${key}`, typeResultResponseSchema);
    }
    await get("/api/v1/views", savedViewListResponseSchema);
    await get("/api/v1/views/inbox", savedViewResultResponseSchema);
    const flagged = await get("/api/v1/views/flagged", savedViewResultResponseSchema);
    expect(JSON.stringify(flagged.query)).toContain('"value":false');
    const hits = (await get(`/api/v1/search?q=${encodeURIComponent("备份")}`, searchResponseSchema)) as unknown as unknown[];
    expect(hits.length).toBe(2);
  } finally {
    child.kill("SIGTERM");
  }
});

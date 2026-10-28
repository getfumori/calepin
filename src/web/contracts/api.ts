import { z, type ZodType } from "zod";

import { appConfigSchema } from "./app-config.js";
import {
  noteConnectionsResponseSchema,
  renameToTitleRequestSchema,
  wikilinkSuggestionListSchema
} from "./connections.js";
import {
  dailyNoteResponseSchema,
  saveDailyNoteRequestSchema
} from "./daily-note.js";
import {
  createHumanNoteRequestSchema,
  deleteHumanNoteRequestSchema,
  humanNoteDeletionImpactResponseSchema,
  humanNoteListResponseSchema,
  humanNoteResponseSchema,
  saveHumanNoteRequestSchema
} from "./human-note.js";
import {
  organizationModelResponseSchema,
  savedViewListResponseSchema,
  savedViewResultResponseSchema,
  typeDefinitionListResponseSchema,
  typeResultResponseSchema
} from "./organization-model.js";
import { searchResponseSchema } from "./search.js";
import { recurrenceSchema, taskListResponseSchema } from "./tasks.js";

export const checkpointResponseSchema = z.object({
  created: z.boolean(),
  commit: z.string().optional(),
  changedFiles: z.number().int().nonnegative()
});

export const diagnosticsResponseSchema = z.array(
  z.object({
    path: z.string(),
    revision: z.string(),
    problems: z.array(z.string())
  })
);

export const captureRequestSchema = z.object({
  text: z.string().min(1),
  task: z.boolean().optional(),
  list: z.string().optional()
});

export const commentRequestSchema = z.object({ text: z.string().min(1) });

export const dailyWriteResponseSchema = z.object({
  date: z.iso.date(),
  revision: z.string()
});

export const taskCommandRequestSchema = z.object({
  expectedOccurrence: z.string().optional(),
  baseRevision: z.string().optional(),
  notes: z.string().optional(),
  scheduled: z.string().optional(),
  due: z.string().optional(),
  list: z.string().optional(),
  recurrence: recurrenceSchema.optional()
});

export const taskCommandResponseSchema = z.object({
  id: z.string(),
  revision: z.string()
});

export const errorResponseSchema = z.object({
  error: z.string(),
  message: z.string().optional(),
  currentRevision: z.string().nullable().optional(),
  currentIncomingLinkCount: z.number().int().optional()
});

export type Endpoint = {
  method: "get" | "post" | "put" | "delete";
  path: string;
  summary: string;
  query?: Record<string, string>;
  request?: ZodType;
  response?: ZodType;
  status?: number;
  idempotent?: boolean;
};

const taskActions =
  "make, complete, skip, undo, cancel, reopen, pause, resume, update, or remove";

/** Every HTTP endpoint of the Vault Service. api/openapi.json is generated from this table. */
export const endpoints: Endpoint[] = [
  { method: "get", path: "/api/v1/config", summary: "Autosave and checkpoint settings", response: appConfigSchema },
  { method: "post", path: "/api/v1/checkpoint", summary: "Commit the work tree when it changed", response: checkpointResponseSchema },
  { method: "get", path: "/api/v1/diagnostics", summary: "Quarantined files and their problems", response: diagnosticsResponseSchema },
  { method: "get", path: "/api/v1/today", summary: "Today's Daily Note, real or not yet created", response: dailyNoteResponseSchema },
  { method: "get", path: "/api/v1/daily/{date}", summary: "The Daily Note for a date", response: dailyNoteResponseSchema },
  { method: "put", path: "/api/v1/daily/{date}", summary: "Save a Daily Note; the first save creates today's", request: saveDailyNoteRequestSchema, response: dailyNoteResponseSchema },
  { method: "post", path: "/api/v1/daily/{date}", summary: "Create the Daily Note for any date", response: dailyNoteResponseSchema, status: 201 },
  { method: "get", path: "/api/v1/notes", summary: "Active standalone Human Notes", response: humanNoteListResponseSchema },
  { method: "post", path: "/api/v1/notes", summary: "Create a Human Note", request: createHumanNoteRequestSchema, response: humanNoteResponseSchema, status: 201 },
  { method: "get", path: "/api/v1/notes/{id}", summary: "One Human Note", response: humanNoteResponseSchema },
  { method: "put", path: "/api/v1/notes/{id}", summary: "Save a Human Note against its base revision", request: saveHumanNoteRequestSchema, response: humanNoteResponseSchema },
  { method: "delete", path: "/api/v1/notes/{id}", summary: "Delete a note after confirming its incoming links", request: deleteHumanNoteRequestSchema, status: 204 },
  { method: "get", path: "/api/v1/notes/{id}/deletion-impact", summary: "Incoming links a delete would break", response: humanNoteDeletionImpactResponseSchema },
  { method: "post", path: "/api/v1/notes/{id}/rename-to-title", summary: "Rename the file to its title and rewrite links", request: renameToTitleRequestSchema, response: humanNoteResponseSchema },
  { method: "get", path: "/api/v1/connections/{id}", summary: "Links, backlinks, and relationships of a note", response: noteConnectionsResponseSchema },
  { method: "get", path: "/api/v1/wikilinks/suggestions", summary: "Link targets matching a prefix", query: { q: "Text to match" }, response: wikilinkSuggestionListSchema },
  { method: "get", path: "/api/v1/inbox", summary: "Captured standalone notes", response: humanNoteListResponseSchema },
  { method: "get", path: "/api/v1/archive", summary: "Archived standalone notes", response: humanNoteListResponseSchema },
  { method: "get", path: "/api/v1/model", summary: "The Organization Model", response: organizationModelResponseSchema },
  { method: "get", path: "/api/v1/types", summary: "Type definitions", response: typeDefinitionListResponseSchema },
  { method: "get", path: "/api/v1/types/{key}", summary: "Notes of one Type", response: typeResultResponseSchema },
  { method: "get", path: "/api/v1/views", summary: "Saved views", response: savedViewListResponseSchema },
  { method: "get", path: "/api/v1/views/{key}", summary: "Results of one Saved view", response: savedViewResultResponseSchema },
  {
    method: "get", path: "/api/v1/search", summary: "Lexical search in mixed Chinese and English",
    query: { q: "Query, at least two Han characters or one identifier", type: "Type key", list: "Task List title", from: "Earliest date", to: "Latest date" },
    response: searchResponseSchema
  },
  { method: "post", path: "/api/v1/capture", summary: "Append a Memo, or an Inline Task, to today's Daily Note", request: captureRequestSchema, response: dailyWriteResponseSchema, status: 201, idempotent: true },
  { method: "post", path: "/api/v1/memos/{id}/comments", summary: "Add a Memo comment", request: commentRequestSchema, response: dailyWriteResponseSchema, status: 201, idempotent: true },
  { method: "get", path: "/api/v1/tasks", summary: "Tasks; status defaults to open and paused", query: { list: "Task List title, or 'unlisted'", status: "open, paused, done, canceled, or all" }, response: taskListResponseSchema },
  { method: "post", path: "/api/v1/tasks/{id}/{action}", summary: `Run a task command: ${taskActions}`, request: taskCommandRequestSchema, response: taskCommandResponseSchema, idempotent: true }
];

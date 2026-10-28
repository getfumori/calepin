import { z } from "zod";

const dateSchema = z.iso.date();

export const recurrenceSchema = z.object({
  basis: z.enum(["schedule", "completion"]),
  frequency: z.enum(["daily", "weekly", "monthly"]),
  interval: z.number().int().positive(),
  weekdays: z.array(z.enum(["sun", "mon", "tue", "wed", "thu", "fri", "sat"])).optional(),
  day: z.number().int().min(1).max(31).optional()
});

export const occurrenceSchema = z.object({
  id: z.string().min(1),
  original: dateSchema,
  scheduled: dateSchema
});

export const taskStatusSchema = z.enum(["open", "paused", "done", "canceled"]);

export const pageTaskSchema = z.object({
  status: taskStatusSchema,
  list: z.string().optional(),
  scheduled: dateSchema.optional(),
  due: dateSchema.optional(),
  completed: z.string().optional(),
  timezone: z.string().optional(),
  occurrence: occurrenceSchema.optional(),
  recurrence: recurrenceSchema.optional()
});

export const occurrenceRecordSchema = z.object({
  id: z.string().min(1),
  original: dateSchema,
  outcome: z.enum(["completed", "skipped"]),
  at: z.string(),
  markdown: z.string()
});

export const taskViewSchema = z.object({
  id: z.string().min(1),
  kind: z.enum(["page", "inline"]),
  title: z.string(),
  status: taskStatusSchema,
  list: z.string().optional(),
  scheduled: dateSchema.optional(),
  due: dateSchema.optional(),
  recurring: z.boolean(),
  occurrence: occurrenceSchema.optional(),
  recurrence: recurrenceSchema.optional(),
  revision: z.string(),
  source: z.object({ id: z.string(), title: z.string(), url: z.string() }),
  overdue: z.boolean()
});

export const taskListResponseSchema = z.array(taskViewSchema);

export type Recurrence = z.infer<typeof recurrenceSchema>;
export type PageTask = z.infer<typeof pageTaskSchema>;
export type OccurrenceRecord = z.infer<typeof occurrenceRecordSchema>;
export type TaskView = z.infer<typeof taskViewSchema>;

/** The title of the Task List a `[[wikilink]]` reference names. */
export function listTitle(reference: string | undefined): string {
  return reference ? reference.replace(/^\[\[|\]\]$/g, "") : "";
}

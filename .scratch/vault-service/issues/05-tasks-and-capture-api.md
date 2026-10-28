# 05: Tasks and capture through the API

Status: resolved
Type: task
Blocked by: 04

## What

`internal/domain` task and capture rules, wired through the write path and
exposed by the API.

## Scope

- Recurrence engine as pure functions: `schedule` and `completion` bases,
  daily, weekly with weekdays, monthly with month-end clamping, collapse of
  missed rounds, pause and resume, rule edits.
- Commands with expected Occurrence ID, `If-Match`, and `Idempotency-Key`:
  `POST /tasks/{id}/occurrence/complete|skip|undo`; one-off Task
  complete and cancel; Inline Task check and uncheck, keeping `completed`
  consistent with the checkbox; set status, scheduled, due, and Task List.
- `POST /capture`: append a Memo to the owner's current Daily Note, creating
  it on first write; optional Inline Task capture with no Task List
  (Unlisted Tasks).
- `POST /memos/{id}/comments`: append to the Memo's original Daily Note.
- Queries: `GET /tasks` by Task List, status, scheduled range, and overdue;
  Unlisted Tasks; Inbox; Today context.

## Acceptance

- Weekly-on-Sunday Task completed on Monday keeps the Sunday Occurrence and
  schedules the next Sunday.
- The 2026-11-01 Occurrence completed on 2026-11-20 schedules 2026-11-22 and
  writes exactly one Occurrence record.
- Sending the same complete request twice, and retrying after a simulated
  crash, completes one round and writes one record.
- Undo removes only the latest record and restores the previous Occurrence;
  undo after the next Occurrence was modified is rejected.
- Capture at 23:30 owner time lands in that local date's Daily Note.
- A comment written days later is stored after its Memo in the original
  Daily Note with its own timestamp.
- Assigning a Task List to an Unlisted Task changes only the `task.list`
  field.
- Clock and time zone are injected; one test covers a midnight boundary.

## Comments

Done. Recurrence is in `internal/domain`, commands in `internal/vault/tasks.go`, routes in `internal/api/commands.go`. Monthly rules gained a `day` field so month-end rules do not drift. Today context has no dedicated endpoint; `/tasks` with Today's date covers it.

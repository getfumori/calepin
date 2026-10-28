# Tasks and Memos live in Markdown

Fumori manages tasks, including recurring tasks, and quick-capture Memos
itself instead of relying on separate apps, and stores both inside ordinary
notes rather than in their own stores. A Page Task is a `task` property on a
Human Note; an Inline Task is an explicitly marked checkbox; each Occurrence
record of a recurring Task is a structured block in that Task's body; a Memo
is an identified block in the Daily Note of its date. The motivating case is a
routine whose procedure, current schedule, and full history belong on one
note, and a capture box that never asks where a thought belongs.

## Considered Options

- A separate task store linked to notes: simpler queries, but splits each
  routine between a task and its note, which is the problem being solved.
- One file per Memo: simple identity, but floods the Vault with tiny files and
  loses the day as the natural reading unit.

## Consequences

- Identified blocks need explicit paired markers that survive arbitrary
  Markdown, are ignored inside code, and round-trip through the editor.
- Completing or skipping an Occurrence is a server action with a revision
  precondition and an idempotency key; clients and agents never compute the
  next Occurrence themselves.
- Plain checkboxes stay local checklists and never appear in Task views.

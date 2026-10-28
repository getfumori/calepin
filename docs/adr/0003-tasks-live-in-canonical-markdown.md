# Tasks live in canonical Markdown

Fumori manages tasks, including recurring tasks, instead of deferring to a
separate task application. This reverses the earlier boundary that Today
context must not turn the Vault into a task manager. Task state never gets its
own store: a Page Task is a `task` property on a Page, an Inline Task is an
explicitly marked checkbox in a body, and every Occurrence record of a
recurring task is a structured block in the Page body. The motivating case is a
routine whose procedure, current schedule, and complete execution history must
live on one Page rather than being split between a task app and a linked note.

## Consequences

- Git history plus canonical Markdown must be sufficient to reconstruct every
  task, its current Occurrence, and its full history; indexes stay disposable.
- Completing or skipping an Occurrence is a server-side business action with a
  revision precondition and an idempotency key. Clients and agents never
  compute the next Occurrence themselves.
- Plain checkboxes remain local checklists. Only explicitly marked tasks enter
  global task views.

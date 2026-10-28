# 03: Write path, recovery, and checkpoints

Status: open
Type: task
Blocked by: 02

## What

`internal/vault`, `internal/state`, and `fumori serve` with the
single-object write path, startup recovery, and Git checkpoints, exposed
through minimal note and Daily Note endpoints.

## Scope

- Runtime state directory and `state.db`; the process lock file.
- The mutation contract and single-object write path, steps 1 to 8 of the
  service design (index update is a no-op hook until issue 04).
- Write intents and idempotency records, 7-day retention.
- Startup sequence: verify repository, clear a stale `index.lock` safely,
  finish write intents, recovery checkpoint when dirty.
- Checkpoint loop (30-minute default, clean is a no-op), checkpoint on clean
  shutdown, `fumori checkpoint` and `POST /api/v1/checkpoint`.
- Rename of a Human Note with wikilink rewriting as a roll-forward
  transaction (journal, staging, recovery).
- Endpoints: `GET/PUT /notes/{id}`, `POST /notes`, `GET/PUT /daily/{date}`
  with `ETag`/`If-Match`; `GET /diagnostics`. A missing Daily Note is not
  created by `GET`.
- Quarantined files are readable and saveable as raw Markdown only.

## Acceptance

- A save against a stale revision returns a conflict and leaves the file
  untouched.
- One table-driven test injects a failure at each crash window in the service
  design through test hooks, restarts, and checks the documented state; a
  retry with the same idempotency key returns the original result exactly
  once.
- After a crash with unsaved-to-Git changes, restart creates one Recovery
  checkpoint and loses no saved content.
- A rename interrupted after the journal commit recovers to the complete
  renamed state.
- Editing an unknown frontmatter field through `PUT` with metadata leaves the
  rest of the file byte-identical.

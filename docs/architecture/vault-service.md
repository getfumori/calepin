# Vault Service Design

Status: accepted. This document designs the Go Vault Service: its module
boundaries, the write path, crash recovery, Git checkpoints, the Search
index lifecycle, and runtime state. It follows ADR-0001, ADR-0002, ADR-0004,
and ADR-0005, and implements the [Vault Format](vault-format.md). Terms follow
[`GLOSSARY.md`](../../GLOSSARY.md).

## Process and storage

One `calepin` binary serves one Vault per process in the first release. It owns
three storage areas:

| Area | Location | Contents | Backup |
| --- | --- | --- | --- |
| Vault repository | `--vault <path>` | Canonical Markdown, attachments, `.calepin/`, `.git/` | Required. Sufficient for all knowledge and task state. |
| Runtime state | `--state <path>`, default `$XDG_STATE_HOME/calepin/<vault-id>/` | `state.db`: write intents, transaction journals, idempotency records; later credentials | Optional. Loss forfeits in-flight deduplication only; later, credentials must be reissued. |
| Search index | `<state>/index/` | `index-<generation>.db` | Never. Rebuilt from the repository. |

Runtime state and the index never live inside the Vault repository, so a
clone of the repository never carries them.

The service is the only writer of a running Vault (Managed repository access).
It takes an exclusive lock file in the state directory at startup and refuses
to start if another process holds it.

## Module boundaries

```
cmd/calepin            CLI: bootstrap, serve, check, reindex, checkpoint
internal/format       Pure parse/edit/serialize of Vault format v1. No I/O.
internal/domain       Pure business rules: tasks, recurrence, memos, capture.
internal/vault        The Vault module: reads, writes, transactions, recovery.
internal/gitrepo      System Git CLI wrapper for checkpoints and status.
internal/index        Search index: projection, analyzer, queries.
internal/state        Runtime state store (state.db).
internal/api          HTTP handlers generated from the API schema.
internal/webassets    Embedded Web client build.
```

Rules:

- `format` and `domain` are pure: bytes and values in, bytes and values out,
  with the clock and ID generator passed in. They hold almost all business
  logic and are tested with fixtures and no file system. A future Go native
  client can import both.
- `vault` is the only module that touches Vault files, Git, and the write
  locks. Callers cannot write a file, commit, or update the index on their
  own; they submit a mutation and receive the outcome.
- `domain` never decides how bytes reach disk. It returns a *change*: for each
  affected object, the base revision it read and the new bytes.
- `api` translates HTTP into `vault` calls and never parses Markdown itself.

### The mutation contract

Every write, from autosave to "complete Occurrence", is a mutation:

```go
type Mutation struct {
    Kind           string            // e.g. "note.save", "occurrence.complete"
    IdempotencyKey string            // required for commands, optional for saves
    Targets        []ObjectRef       // objects read, each with expected revision
    Apply          func(ctx Snapshot) (Change, error) // pure; may run again
}
```

`vault` loads the targets, checks their revisions, calls `Apply` with parsed
objects, validates the result, and commits it through the single-object or
multi-object path below. `Apply` must be deterministic given its snapshot,
clock, and ID source, because recovery may re-run it.

## Single-object write path

Used by note saves, metadata edits, quick capture, comments, and task
commands; nearly all writes touch one file.

1. **Lock.** Take the object's write lock and a shared Vault barrier.
2. **Check.** Read current bytes, hash them, and compare with the expected
   Object working revision. A mismatch returns a Content conflict.
3. **Deduplicate.** If the request carries an idempotency key already
   recorded as completed for the same request fingerprint, return the recorded
   result without writing. A key reused with a different request is rejected.
4. **Compute.** Run `Apply`. Re-parse the new bytes with `format`; any parse
   error, duplicate block ID inside the file, or block ID already owned by
   another object rejects the write.
5. **Record intent.** Insert a write intent into `state.db`: key, object,
   base hash, target hash, and the response to return. Commit with fsync.
6. **Publish.** Write a temporary file beside the target, fsync it, rename it
   over the target, and fsync the directory.
7. **Index.** Update the object's rows in the Search index in one SQLite
   transaction, recording the new revision.
8. **Complete.** Mark the intent completed, which turns it into the
   idempotency record. Return the new revision and the index generation.

The response is sent only after step 7, so a client always reads its own
write through search and task views.

### Crash windows

| Crash after | State on restart | Recovery |
| --- | --- | --- |
| Step 4 | No intent, file unchanged | Nothing to do. A retry runs normally. |
| Step 5 | Pending intent, file at base hash | Discard the intent. A retry runs normally. |
| Step 6 | Pending intent, file at target hash | The write happened: mark the intent completed. A retry returns the recorded result. |
| Step 7 | Completed in file, index stale | Startup reconciliation re-indexes the file. |

A pending intent whose file matches neither hash means something else wrote
the file; the intent is discarded and the object is reported in diagnostics.
This closes the window in which content is committed but the deduplication
record is missing.

Idempotency records are kept for 7 days. Commands are also idempotent by
domain rules: completing an Occurrence names the expected Occurrence ID, so a
late retry after the record expired finds that Occurrence already recorded and
returns it instead of completing the next round.

## Multi-object transactions

Used by Memo extraction, rename with link rewriting, Task List deletion, and
later Change Proposals and schema migrations.

1. **Plan.** Run `Apply` against all targets to produce every new file,
   rename, and deletion.
2. **Barrier.** Take the exclusive Vault barrier, which waits for in-flight
   single-object writes and pauses new ones. Re-check every target revision.
3. **Stage.** Write every new file to `<state>/tx/<tx-id>/`, fsync, then write
   the journal: each operation with its path, base hash, and target hash.
   Commit the journal with fsync. This is the commit point.
4. **Apply.** Perform renames into the Vault in journal order, then
   deletions.
5. **Index and finish.** Update the index for every affected object, mark the
   journal completed, and request a Checkpoint commit describing the
   operation.

Recovery rolls forward: a committed journal is replayed, skipping operations
already at their target hash. A journal without the commit marker is
discarded with its staged files. If a file is at neither base nor target
hash, the service refuses writes and reports the conflict instead of
guessing.

Transactions roll forward rather than back. Rolling forward is
simpler and the staged files already hold the complete result.

## Git checkpoints

- The checkpoint loop runs every 30 minutes by default. If the canonical
  worktree is clean it does nothing; otherwise it takes the exclusive barrier
  only long enough to stage and commit.
- Additional checkpoints follow transactions, run at clean shutdown, and can
  be triggered manually.
- Git runs through the system CLI without a shell, with hooks, filters,
  credential helpers, and repository-level executable configuration
  disabled, and with a minimal environment.
- Startup sequence: verify the repository; clear a stale `index.lock` only when
  no Git child process of this service is alive; finish write intents and
  transaction journals; then, if the worktree is dirty, create a Recovery
  checkpoint.
- Commit messages are generated and content-free, for example
  `checkpoint: 12 files` or `transaction: extract memo`.

## Search index lifecycle

- The index file records its generation and a fingerprint of the schema,
  extractor, analyzer, dictionaries, and conversion data (ADR-0005).
- At startup the service hashes every canonical file and compares the hashes
  with the revisions recorded in the index. Changed, new, and deleted files
  are re-indexed incrementally. Hashing, not modification time, decides.
- If the fingerprint differs or the file is missing or corrupt, a full
  rebuild runs into a new generation file and is activated by atomic rename.
  Until it completes, search responses report that the index is rebuilding.
- Besides passage text, the index holds the structured projection used by
  task and view queries: objects, Tasks, current Occurrences, Occurrence
  records, Task List references, Memos, comments, and links.

## Validation and diagnostics

- **Vault-level errors fail startup**: an unreadable repository, an invalid
  Vault Manifest or Organization Model, or an unknown format version.
- **Object-level errors quarantine the file**: invalid YAML, broken block
  markers, duplicate IDs, or an invalid recurrence. A quarantined file is
  readable and editable through Raw Markdown so the user can fix it, but it
  contributes no Tasks or Memos and cannot receive task commands.
  Duplicate IDs quarantine every file that claims the ID.
- Diagnostics are exposed through the API and `calepin check`, which runs the
  same validation without starting the server.

Invalid files are quarantined while the Vault keeps serving, instead of
refusing to start. One broken note should not take the whole Vault offline, and
quarantine still prevents a wrong task action.

## Time

- The Vault Manifest stores the owner's IANA time zone. Daily Note dates,
  "today" for overdue checks, and default Task time zones use it.
- The domain receives the current instant from an injected clock, so
  recurrence and Daily Note rules are tested with fixed times, including
  around midnight and DST changes.

The owner time zone lives in the Vault Manifest, so it travels with the
repository.

## HTTP API

- Base path `/api/v1/`, JSON, one owner channel in the first release.
- Revisions use `ETag` and `If-Match`; commands use an `Idempotency-Key`
  header.
- Resource and command shape, for example:
  - `GET/PUT /notes/{id}`, `POST /notes`
  - `GET/PUT /daily/{date}`
  - `POST /capture` (Quick capture into today's Daily Note)
  - `POST /memos/{id}/comments`
  - `GET /tasks?list=&status=&scheduled_before=` and Unlisted Tasks
  - `POST /tasks/{id}/occurrence/complete|skip|undo`
  - `POST /sources/capture` (URL Capture result from a client)
  - `GET /search?q=&type=&list=&from=&to=`
  - `GET /diagnostics`, `POST /checkpoint`
- Lists return fields on request, filters, sorting, pagination, counts, and
  source IDs, so agents do not have to read whole files.

The API is defined once as an OpenAPI 3.1 document in `api/`, with
Go server stubs and TypeScript client types generated from it. This replaces
the Web client's hand-written contracts and keeps Go and TypeScript in step
(ADR-0002).

## First implementation slice

1. `internal/format` with the Vault Format fixtures as golden tests.
2. `calepin bootstrap` and `calepin check`.
3. The single-object write path, startup recovery, and checkpoints.
4. The index projection for notes, Daily Notes, Memos, and Tasks, then
   Chinese search.
5. Task commands with recurrence, end to end through the API.
6. Connecting the Web client.

The acceptance scenarios become integration tests against a real temporary
Git repository: unknown fields preserved, capture in the right Daily Note,
cross-day comments, Unlisted Task assignment, late completion of a recurring
Task, duplicate completion requests, stale writes, markers inside code
blocks, index rebuild equality, broken files, crash after save, mixed
Chinese search, and Memo extraction.

## Implementation notes

The first slice simplifies this design in four places. Each keeps the
design's guarantees and can be replaced without a format change.

- **One Vault-wide write lock** replaces per-object locks and the separate
  barrier. Writes are serial; reads take a shared lock.
- **The structured projection lives in memory.** Notes, links, Tasks, and
  Memos are rebuilt from the repository at startup. SQLite holds only the
  full-text Search index, so `GET /tasks` and views read the in-memory
  projection.
- **Write intents are recorded only for commands that carry an
  Idempotency-Key.** Editor saves are already safe to retry through their
  base revision, and a crash before the index update is repaired by the
  startup content-hash sync.
- **The HTTP contract is defined once in TypeScript.** The zod schemas in
  `src/web/contracts/` and the endpoint table in
  `src/web/contracts/api.ts` generate `api/openapi.json` with
  `pnpm run openapi`; `test/web/openapi.test.ts` fails when the committed
  document is stale, and `e2e/contracts.spec.ts` parses real Go responses
  with the same schemas. Go handlers are written by hand rather than
  generated; a future Go client can generate its types from the document.

The search dictionary is embedded by `internal/index` itself, so gse's
other dictionaries are dropped by the linker and the stripped binary is
about 20 MB.

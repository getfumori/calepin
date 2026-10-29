# Spec: Go Vault Service, first slice

Status: resolved

## Goal

Build the first working Go Vault Service: a single `calepin` binary that
bootstraps and serves one Git-backed Vault, edits notes and Daily Notes safely,
accepts Quick capture and Memo comments, runs Page Tasks with recurrence
through the HTTP API, searches mixed Chinese and English, and serves the
existing Vue Web client.

## Sources of truth

- Domain language: [`GLOSSARY.md`](../../GLOSSARY.md)
- Decisions: [`docs/adr/`](../../docs/adr/)
- File format and Task state machine:
  [`docs/architecture/vault-format.md`](../../docs/architecture/vault-format.md)
- Module boundaries, write path, recovery:
  [`docs/architecture/vault-service.md`](../../docs/architecture/vault-service.md)
- Search design:
  [`docs/research/chinese-full-text-search.md`](../../docs/research/chinese-full-text-search.md)

If an issue and these documents disagree, the documents win; fix the issue
or raise the conflict instead of silently diverging.

## Repository layout

- Go module `github.com/getfumori/fumori` at the repository root:
  `go.mod`, `cmd/calepin/`, `internal/...` as listed in the service design.
- API schema in `api/openapi.yaml`.
- The Web client stays in `src/web/`; its build is embedded into the binary.
- Go `1.26`, `CGO_ENABLED=0` for every build and test.

## In scope

- Vault format v1 parsing, editing, and serialization.
- `calepin bootstrap`, `calepin check`, `calepin serve`, `calepin reindex`,
  `calepin checkpoint`.
- Human Notes, Daily Notes, Memos, Memo comments, Page Tasks with
  Recurrence, one-off Inline Tasks, Task Lists, Unlisted Tasks, Inbox.
- Single-object writes, write intents, idempotency, crash recovery, Git
  checkpoints, quarantine diagnostics.
- Search index with structured projection and Chinese lexical search.
- OpenAPI contract and the Web client connected to the Go API.

## Out of scope for this slice

- Memo extraction and other multi-object transactions beyond what rename
  needs.
- Recurring Inline Tasks.
- URL Capture, Sources, browser extension.
- AI, agents and Agent API credentials, backup remotes.
- Multi-Vault, Vault import, restore, fork.
- Mobile layout and new Web UI for tasks; the Web client keeps its current
  note editing surface in this slice.

## Global acceptance

- `go test ./...` and `go vet ./...` pass with `CGO_ENABLED=0`.
- Integration tests run against real temporary Git repositories, never mocked
  storage.
- Restarting the service, or deleting the Search index and restarting,
  produces identical API results for notes, tasks, and search.
- No test or log contains note content outside test fixtures.

## Issues

1. [01-format-package](issues/01-format-package.md)
2. [02-bootstrap-and-check](issues/02-bootstrap-and-check.md)
3. [03-write-path-and-recovery](issues/03-write-path-and-recovery.md)
4. [04-search-index](issues/04-search-index.md)
5. [05-tasks-and-capture-api](issues/05-tasks-and-capture-api.md)
6. [06-openapi-and-web-client](issues/06-openapi-and-web-client.md)

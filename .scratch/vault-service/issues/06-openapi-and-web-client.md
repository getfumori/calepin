# 06: OpenAPI contract and Web client

Status: resolved
Type: task
Blocked by: 05

## What

Define the HTTP API once in `api/openapi.yaml`, generate Go server stubs and
TypeScript types from it, connect the Vue Web client, and embed its build in
the binary.

## Scope

- OpenAPI 3.1 document covering every endpoint from issues 03 to 05, with
  `ETag`, `If-Match`, `Idempotency-Key`, error and conflict shapes, and
  field selection and pagination for list endpoints.
- Code generation for Go handlers and TypeScript types, run by a single
  command and checked in CI for drift.
- Replace `src/web/contracts/` with the generated types; adapt the Web client
  to the Go endpoints without adding new UI.
- `internal/webassets`: embed the Vite build; `calepin serve` serves it with
  no-store for knowledge responses and the Ephemeral knowledge client rules.
- CI: Go tests with `CGO_ENABLED=0` on Linux and macOS, Web typecheck,
  tests, and build, then a binary smoke test.

## Acceptance

- A small set of end-to-end journeys against one built binary covers
  bootstrapping a Vault, creating, editing, linking, and searching notes, and
  editing Today. Web UI logic gets no new component unit tests.
- Changing the OpenAPI document without regenerating fails CI.
- The binary listens on `127.0.0.1` by default and warns when bound
  elsewhere, since the service has no authentication of its own.

## Comments

The Web client runs on the Go API, is embedded in the binary, and has a Memo stream with quick capture on Today. `e2e/contracts.spec.ts` checks every GET response against the zod contracts, and `e2e/journey.spec.ts` covers the main journey. Open: the OpenAPI document and code generation.

The Web client now has task screens: a Task panel on every Human Note (make a task, complete or skip a round with notes, history, undo, pause, Task List) and a `/tasks` view with Today, Unlisted, Task List, and Done filters. Human Note responses separate the editor text from Occurrence records, so routine notes stay editable in the rich editor. `e2e/tasks.spec.ts` covers the routine and triage journey.

`api/openapi.json` is generated from the zod contracts and the endpoint table in `src/web/contracts/api.ts`; a Web test fails when it is stale, and the e2e contract check walks the same table against the Go server. Go handlers are not generated; that is recorded in vault-service.md.

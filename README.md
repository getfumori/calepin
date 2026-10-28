# Fumori

Fumori is a private, self-hosted knowledge system: a forest grown from your
words.

It keeps notes, daily quick capture, tasks with recurring routines, and
captured Web content as plain Markdown in a Git repository on your own server.
Indexes are disposable, every client talks to one authoritative server, and
AI may read your notes but never silently rewrites them.

## Quick start

Requires Go 1.26, Node.js 24 with pnpm, and the system Git CLI.

```sh
corepack pnpm install
corepack pnpm run build:binary          # builds the Web client into bin/fumori

git init ~/fumori-vault
bin/fumori bootstrap --vault ~/fumori-vault --timezone America/Los_Angeles
bin/fumori serve --vault ~/fumori-vault  # http://127.0.0.1:3000
```

`fumori check --vault <path>` validates a Vault without serving it, and
`fumori reindex --vault <path>` rebuilds the Search index while the server is
stopped. Runtime state, including the Search index, lives outside the Vault in
`$XDG_STATE_HOME/fumori/<vault-id>/` unless `--state` is given.

The server binds to `127.0.0.1` and has no authentication or TLS of its own.
Expose it only on a trusted network or behind an authenticating gateway.

## What works today

- Today and Daily Notes, with a free-writing area and a Memo stream with quick
  capture and comments.
- Human Notes with Types, properties, relationships, wikilinks, backlinks,
  Inbox, Archive, Saved views, rename with link rewriting, and delete.
- Mixed Chinese and English search, including two-character words and model
  numbers.
- Tasks. Any note can become a one-off or recurring Page Task from its
  Task panel. Completing a round records your notes in the note's history and
  schedules the next round. Quick capture can add Inline Tasks, and the Tasks
  view groups them by Today, Unlisted, each Task List, and Done.
- Autosave with conflict handling, Git checkpoints every 30 minutes and on
  shutdown, and crash recovery.

## Repository layout

- `cmd/fumori/` and `internal/`: the Go Vault Service.
- `src/web/`: the TypeScript and Vue 3 Web client. Its build is embedded into
  the binary from `internal/webassets/dist/`.
- `e2e/`: browser journeys and API contract checks against the built binary.
- `GLOSSARY.md`, `docs/adr/`, and `docs/architecture/`: the domain language,
  decisions, Vault format, and service design.

## Development

```sh
CGO_ENABLED=0 go test ./...
corepack pnpm run typecheck
corepack pnpm test
corepack pnpm run test:e2e              # builds the binary, then runs Playwright
```

For Web development, run `bin/fumori serve` on port 3000 and
`corepack pnpm dev`; Vite proxies `/api` to the Go server.

## License

See [`LICENSE`](LICENSE).

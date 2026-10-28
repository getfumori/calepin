# Fumori

Fumori is a private, self-hosted knowledge system: a forest grown from your
words.

It keeps notes, daily quick capture, tasks with recurring routines, and
captured Web content as plain Markdown in a Git repository on your own server.
Indexes are disposable, every client talks to one authoritative server, and
AI may read your notes but never silently rewrites them.

## Status

Fumori is in design. The Go Vault Service has not been written yet. This
repository currently holds the design documents and the Vue Web client, whose
editor, autosave, and conflict handling will be connected to the Go API.

## Repository layout

- `src/web/`: the TypeScript and Vue 3 Web client.
- `test/web/`: Web client tests.
- `GLOSSARY.md`: the domain language.
- `docs/adr/`: architecture decisions.
- `docs/architecture/vault-format-draft.md`: the proposed canonical file
  format and Task state machine.
- `docs/research/`: primary-source research behind the decisions.
- `docs/product/brand.md` and `docs/brand/`: name, brand direction, and the
  logo concept.

## Web client development

Requires Node.js 24 and pnpm:

```sh
corepack pnpm install
corepack pnpm run typecheck
corepack pnpm test
corepack pnpm run build
```

## License

See [`LICENSE`](LICENSE).

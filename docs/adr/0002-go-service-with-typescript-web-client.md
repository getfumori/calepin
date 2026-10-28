# Go Vault Service with a TypeScript Web client

The Vault Service backend is written in Go, while the Web client remains
TypeScript and Vue 3. This supersedes the Node.js TypeScript server row of the
Accepted Architecture Baseline. Go gives a single self-contained server
binary, lower resident cost on a personal server, and code that a future Go
native client can reuse for Markdown parsing, Vault format rules, and API
types. The canonical Vault format, Git-centered history and recovery, the
modular-monolith shape, and server authority are unchanged; only the server
implementation language and packaging move.

## Consequences

- The Foundation TypeScript server and Vault Module are a reference
  implementation to be ported, not extended. Acceptance scenarios and
  fixtures carry over as the behavioral contract.
- The HTTP contract becomes the only shared boundary between Go and
  TypeScript, so it must be described in a language-neutral schema rather than
  shared Zod types.
- Distribution moves from an npm package to a single binary that embeds the
  prebuilt Web assets. The system Git CLI remains a runtime requirement.

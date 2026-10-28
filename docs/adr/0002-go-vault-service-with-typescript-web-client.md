# Go Vault Service with a TypeScript Web client

The Vault Service is written in Go and ships as one self-contained binary
that also serves the prebuilt Web client, which is written in TypeScript and
Vue 3. Go gives a small, fast server for a personal machine and lets a future
Go native client reuse the Markdown parsing, Vault format rules, and API
types. The system Git CLI is the only runtime dependency.

## Consequences

- The HTTP API is the only boundary shared by Go and TypeScript, so it is
  described in a language-neutral schema rather than shared type definitions.
- Clients, the CLI, and agents all use the same API and rules; none of them
  read or write Vault files directly.

# A Git-backed Markdown Vault is the only source of truth

A Vault is one Git repository on the user's own server. Every durable fact,
including note content, properties, Task state, Recurrence, Occurrence
records, Memos, and Sources, lives in Markdown files and attachments in that
repository, and Git provides history and recovery. Indexes and caches are
disposable and are rebuilt from the repository without changing it. One
server is authoritative: clients and agents talk to it over HTTP, and there is
no multi-device Vault synchronization, CRDT, or offline merge.

## Considered Options

- A database as the source of truth with Markdown export: faster queries, but
  gives up plain-file ownership and Git history.
- Local Vault copies synchronized between devices: offline use, but forces
  merge semantics onto task state and history that a personal tool does not
  need.

## Consequences

- Backing up the repository, plus Secret bindings, is a complete backup.
- Runtime state that is not knowledge, such as credentials and idempotency
  records, is stored and backed up separately and is never described as
  rebuildable from the index.
- Folders carry no meaning; identity, Type, and view membership come from file
  content.

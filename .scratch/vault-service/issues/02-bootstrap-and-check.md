# 02: Bootstrap and check

Status: resolved
Type: task
Blocked by: 01

## What

The `fumori` CLI skeleton with `bootstrap` and `check`, plus the Vault
Manifest and Core Model files.

## Scope

- `cmd/fumori` with subcommand dispatch and `--help`.
- Define in `vault-format.md` the format of `.fumori/vault.md` (identity,
  name, format version, owner IANA time zone) and the Core Model files
  under `.fumori/model/`: lifecycle, core properties, Types `note`,
  `daily-note`, `task-list`, and the Inbox Saved view.
- `fumori bootstrap --vault <path> [--name] [--timezone]`: requires an empty
  Git repository, writes the Manifest and Core Model, creates the reserved
  folders from F10, and makes the first commit. Defaults the time zone to
  the system zone.
- `fumori check --vault <path>`: full validation without starting a server.
  Vault-level errors exit non-zero; object-level problems are listed as
  quarantine diagnostics.
- `internal/gitrepo`: Git through the system CLI without a shell, with hooks,
  filters, and credential helpers disabled and a minimal environment.

## Acceptance

- Bootstrap on a non-empty or non-Git directory fails without writing.
- A bootstrapped Vault passes `check` with no diagnostics and has exactly one
  commit.
- `check` reports duplicate IDs (quarantining every claimant), broken YAML,
  broken markers, and an invalid recurrence, each with file and position.
- A repository-level Git hook in the Vault is never executed (test with a
  hook that would write a marker file).

## Comments

Done. `.fumori/vault.md` and `.fumori/model/` are written by `internal/model/core.go`; their format lives in that file rather than in vault-format.md. The hook test goes red when the `core.hooksPath` guard is removed.

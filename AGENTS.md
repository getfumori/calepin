## Agent skills

### Issue tracker

Issues and specs are tracked as local Markdown files under `.scratch/`. See `docs/agents/issue-tracker.md`.

### Domain docs

This is a single-context repository with `GLOSSARY.md` at the root and ADRs under `docs/adr/`. See `docs/agents/domain.md`.

### Testing

Keep the test suite small and tight: every test earns its place by guarding
behavior a user or the Vault format depends on.

- Test the common paths and the failure modes the design documents name.
  A rare edge case gets at most one test, or none when the code makes it
  obviously impossible.
- Each behavior is tested once, at the lowest level that can see it. Prefer
  one table-driven test over many near-identical tests.
- Unit tests run in milliseconds: no sleeps, real clocks, or network; use
  injected clocks and in-memory inputs. Integration tests use a temporary Git
  repository and finish in seconds.
- Web UI behavior is covered by a few end-to-end journeys rather than
  component unit tests. Each journey covers a distinct user flow; a new
  journey replaces or extends an existing one instead of repeating its steps.

### Repository language

All repository artifacts must be written in English, including source code,
identifiers, comments, user-facing copy, tests, documentation, commit messages,
and GitHub issues and pull requests. Agent-user conversation may use the
user's preferred language.

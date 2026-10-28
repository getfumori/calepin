# 01: Vault format package

Status: open
Type: task
Blocked by: none

## What

Implement `internal/format`: pure parsing, editing, and serialization of
Vault format v1, with no file system access.

## Scope

- Frontmatter: the YAML subset (F11), reserved envelope (F1), known fields
  with strict types (`type`, `state`, `tags`, `aliases`, `date`, `task`),
  and unknown fields preserved.
- Node-level frontmatter edits that change only the affected keys and keep
  key order, comments, and unknown fields. Reject anchors, aliases, tags, and
  duplicate keys.
- Body: H1 title extraction (F2); wikilinks in body and frontmatter (F3).
- Block markers (F5): `memo`, `comment`, `occurrence`, and Inline Task
  markers (F8), recognized only at the top level, never inside code, lists,
  or blockquotes. Report nested, unterminated, mismatched, unknown-kind,
  invalid-JSON, and duplicate-ID markers as diagnostics with positions.
- Block ID generation (F4) behind an injected random source.
- Typed results: `HumanNote`, `DailyNote` with free-form region and Memo
  stream (F6), `PageTask` with `Recurrence` and current `Occurrence`,
  `OccurrenceRecord`, `InlineTask`, `MemoComment`.
- Edit operations needed later: append Memo, append Memo comment after its
  Memo's comments, append Occurrence record under the history heading, set and
  replace `task` fields.

## Acceptance

- Every fixture in `vault-format.md` is a golden file under
  `internal/format/testdata/` and parses to the expected structure.
- Parse then serialize without edits is byte-identical for every fixture and
  for a few representative variants (CRLF, no trailing newline, a code fence
  containing markers).
- Editing one frontmatter field leaves every other byte of the file
  unchanged.
- A marker inside a fenced code block produces no Memo, Task, or comment.
- Diagnostics are covered by one table-driven test.

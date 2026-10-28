# Vault Format

Status: accepted. This document defines version 1 of the canonical Vault
format for Human Notes, Daily Notes, Memos, Tasks, and Occurrence records,
following ADR-0001 and ADR-0004. Changing any persistent syntax here requires
a new format version, never a silent rewrite of existing files. Terms follow
[`GLOSSARY.md`](../../GLOSSARY.md).

## Format rules

### F1. Reserved envelope

Every Durable Object carries the reserved fields `_id` (UUID), `_schema`,
`_version`, and `_created`. `_version` changes only when a file uses syntax an
older reader cannot interpret, and startup rejects a version it does not know
rather than rewriting it.

### F2. The H1 is the title

The first H1 is the title. Filenames evolve independently, and no frontmatter
`title` exists.

### F3. References are wikilinks

Frontmatter references, including `task.list`, use wikilinks such as
`"[[Homelab]]"`, matching relationship values and Tolaria. The index
resolves them to `_id`. Renaming a note is a managed operation that rewrites
affected wikilinks, so readable references do not cost stability.
An unresolved or ambiguous reference is a diagnostic, never a silent
re-categorization.

Alternative rejected: raw UUID references are robust but unreadable in
plain Markdown and Git diffs, and would introduce a second reference style.

### F4. Block identity

Durable Objects use UUIDs. Blocks (Memo, Memo comment, Inline Task, Occurrence record)
use a kind prefix and 12 random Crockford base32 characters, for example
`m_01J9Q4X7KD2M`. Block IDs are unique within a Vault; the index reports
duplicates. Moving content keeps IDs; copying generates new ones.

### F5. Block markers

Identified blocks are delimited by paired HTML comments:

```
<!-- fumori:memo {"id":"m_...","created":"2026-10-27T09:42:00-07:00"} -->
...ordinary Markdown...
<!-- /fumori:memo m_... -->
```

- A marker is recognized only as a top-level HTML block of the document. A
  marker inside a fenced code block, indented code, list, or blockquote is
  plain text.
- Metadata is one JSON object on one line. A literal `-->` inside a value is
  written as `--\u003e`.
- The end marker repeats the ID so a missing or misplaced end is detected
  rather than silently swallowing the rest of the file.
- Blocks do not nest. Nested, unterminated, or mismatched markers, unknown
  kinds, and invalid JSON make the file invalid for indexing; the file is
  reported and its last valid projection is not used to trigger task
  actions.
- The `ai:omit` redaction markers remain valid inside a block.

### F6. Daily Note regions

Free-form Today entries are all body content outside identified blocks. The
Memo stream is the sequence of top-level `memo` and `comment` blocks. Quick
capture appends a Memo at the end of the file. The UI shows free-form content
above the Memo stream; the file does not need a region marker.

A Memo comment is a sibling block with `parent` set to its Memo's ID, placed
after that Memo's existing comments, in the Memo's original Daily Note.

### F7. Page Task

A Human Note becomes a Page Task by carrying a `task` map in frontmatter. `task` is
an ordinary public property, not reserved envelope data.

### F8. Inline Task

An Inline Task is a list item whose first line ends with a task marker:

```
- [ ] Buy network cable <!-- fumori:task {"id":"t_...","list":"[[Homelab]]"} -->
```

The checkbox is the completion state. `completed` must agree with it; a
disagreement is a diagnostic offered as a repair, not guessed. Inline Tasks
do not recur in the first release.

### F9. Occurrence records

Each completed or skipped Occurrence appends an `occurrence` block at the
end of the recurring Task's body. Occurrence records always form the end of
the body under one `## History` heading, which is written with the first
record and removed with the last. The heading and records are layout owned
by the service: the editor edits only the text before them, and saving that
text keeps the records in place. The block, not the heading, is the
authority. Its id is the
Occurrence id, and its metadata records `original`, `scheduled` (the date
it was planned for when acted on, used by undo), `outcome` (`completed` or
`skipped`), and `at`.

Recurrence fields are `basis` (`schedule` or `completion`), `frequency`
(`daily`, `weekly`, or `monthly`), `interval` (at least 1), optional
`weekdays` (`sun` to `sat`) for weekly rules, and optional `day` (1 to 31)
for monthly rules, which defaults to the day of the first Occurrence.

### F10. Placement

Placement is non-semantic. Human Notes, Task Lists, and Page Tasks live in
`human/notes/`; Daily Notes in `human/daily/YYYY/`; Sources in
`sources/records/`; AI Knowledge in `knowledge/`; attachments in `assets/`.
The Vault Manifest and Organization Model live under `.fumori/`. There are no
per-kind folders such as `lists/` or `bookmarks/`.

### F11. Values

Dates are quoted `"YYYY-MM-DD"` local dates. Instants are quoted RFC 3339
strings with an offset. Recurring tasks carry an IANA `timezone`. The
accepted YAML subset is maps, sequences, and JSON-compatible scalars;
anchors, aliases, tags, and duplicate keys are rejected. Unknown fields and
comments are preserved through edits by changing only the affected
frontmatter nodes.

## Fixtures

### Task List

```markdown
---
_id: 7d0c8a52-3f1e-4b8e-9a51-0f5d2c1b6e01
_schema: fumori.note
_version: 1
_created: "2026-10-01T09:00:00-07:00"
type: task-list
state: organized
tags: []
aliases: []
---

# Homelab

Maintenance, backup, and hardware upgrades.
```

### One-off Page Task

```markdown
---
_id: 2b6f1d7e-9c44-4f0a-8e2d-6a1b3c5d7e90
_schema: fumori.note
_version: 1
_created: "2026-10-20T21:10:00-07:00"
type: project
state: organized
tags: [homelab]
aliases: []
task:
  status: open
  list: "[[Homelab]]"
  scheduled: "2026-11-01"
  due: "2026-11-10"
---

# Replace server disk

## Steps

- [ ] Verify backup
- [ ] Replace disk
- [ ] Restore data
```

The three checkboxes are a local checklist and are not global Tasks.

### Recurring Page Task with history

```markdown
---
_id: 9a3e5c71-0b2d-4e6f-8a19-c4d2e7f6a3b8
_schema: fumori.note
_version: 1
_created: "2026-10-01T09:00:00-07:00"
type: procedure
state: organized
tags: [homelab, maintenance]
aliases: []
task:
  status: open
  list: "[[Homelab]]"
  timezone: America/Los_Angeles
  occurrence:
    id: o_01J9R2B6W4TQ
    original: "2026-11-08"
    scheduled: "2026-11-08"
  recurrence:
    basis: schedule
    frequency: weekly
    interval: 1
    weekdays: [sun]
---

# Server routine check

## Procedure

- Check backup status
- Check disk space
- Check failed services

## History

<!-- fumori:occurrence {"id":"o_01J9P7M3D5XC","original":"2026-11-01","scheduled":"2026-11-01","outcome":"completed","at":"2026-11-02T09:30:00-08:00"} -->
### 2026-11-02

No time on Sunday; did it Monday. Backups fine, pruned old images.
<!-- /fumori:occurrence o_01J9P7M3D5XC -->
```

The Monday completion kept the Sunday Occurrence identity, and the next
Occurrence stays on the Sunday schedule.

### Daily Note with free writing, Memos, a comment, and an Inline Task

```markdown
---
_id: 4c1a9e2f-7b3d-4a6e-9f08-2d5c6b7a8e91
_schema: fumori.daily-note
_version: 1
_created: "2026-10-27T08:15:00-07:00"
type: daily-note
state: organized
tags: []
aliases: []
date: "2026-10-27"
---

# 2026-10-27

Slept badly. Plan: finish the backup script.

<!-- fumori:memo {"id":"m_01J9K3F8Q2VA","created":"2026-10-27T09:42:00-07:00"} -->
Try an old phone as the robot camera.

- [ ] Measure camera latency <!-- fumori:task {"id":"t_01J9K3G1N7HS"} -->
<!-- /fumori:memo m_01J9K3F8Q2VA -->

<!-- fumori:memo {"id":"m_01J9K5T2R8BD","created":"2026-10-27T15:30:00-07:00"} -->
Good write-up on phone vision for robots: [[Old phones as robot vision]]
<!-- /fumori:memo m_01J9K5T2R8BD -->

<!-- fumori:comment {"id":"c_01J9N1Z4H6PE","parent":"m_01J9K3F8Q2VA","created":"2026-10-29T10:15:00-07:00"} -->
USB power is enough; no battery needed.
<!-- /fumori:comment c_01J9N1Z4H6PE -->
```

The second Memo links to a Source created by client-side URL Capture.

## Task state machine

`task.status` is shared by one-off and recurring Tasks:

| Status | Meaning |
| --- | --- |
| `open` | Active. A recurring Task stays `open` across all Occurrences. |
| `paused` | Recurring only. No Occurrence is due or overdue while paused. |
| `done` | Finished. A one-off Task records `completed`; a recurring Task stops scheduling. |
| `canceled` | Abandoned. History is kept. |

A recurring Task has exactly one current Occurrence with a stable `id`, an
immutable `original` date, and a movable `scheduled` date. Postponing changes
only `scheduled`. Recurring Tasks are date-only in the first release, so DST
does not affect scheduling; `timezone` defines the local "today". Recurring
Tasks carry no `due`; `due` is for one-off Tasks.

Completing or skipping the current Occurrence appends one Occurrence record
and creates the next Occurrence:

- `basis: schedule`: the first rule date strictly after the later of
  `original` and the local action date. Missed rounds collapse into the one
  record; no skipped records are fabricated. Example: weekly on Sunday, the
  2026-11-01 Occurrence completed on 2026-11-20 makes the next one
  2026-11-22.
- `basis: completion`: the local action date plus the interval.
- Skip computes the next Occurrence exactly like completion and records
  `outcome: skipped`.

Other rules:

- Monthly dates beyond a month's length clamp to its last day; the rule's
  `day` keeps later months on the intended day.
- An Occurrence is overdue when `scheduled` is before the local today; it
  stays visible until acted on.
- Resuming a paused Task reschedules the current Occurrence to the first rule
  date on or after today (`schedule`) or to today (`completion`).
- Editing the recurrence rule leaves the current Occurrence unchanged; the
  new rule applies from the next one.
- Undo is allowed only for the most recent Occurrence record while the
  following Occurrence is still unmodified. It deletes that record and
  restores the previous Occurrence; Git history keeps the prior content.
- Complete, skip, and undo carry the expected Occurrence ID, the base Object
  working revision, and an idempotency key.

## Open validation

- Rich editor round-trip of block markers, which ADR-0003 requires before
  Memos can be edited in the rich surface.

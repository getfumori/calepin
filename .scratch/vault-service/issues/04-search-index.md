# 04: Search index and Chinese search

Status: open
Type: task
Blocked by: 03

## What

`internal/index`: the disposable Search index with the structured projection
and lexical search, following ADR-0005 and the research report.

## Scope

- `modernc.org/sqlite` FTS5, generation files, fingerprint, atomic
  activation of rebuilds, `fumori reindex`.
- Structured projection: objects, links and backlinks, Tasks, current
  Occurrences, Occurrence records, Task List references, Memos, comments,
  quarantine state.
- Indexed blocks: note sections, Memos, Memo comments, Occurrence records,
  each with owner, source range, and revision.
- Analyzer: NFKC, case folding, OpenCC `t2s`, gse with only the Simplified
  dictionary embedded, Han bigram fallback with verification, identifier
  tokens; snippets from original text through span maps.
- Startup reconciliation by content hash; per-save updates inside the write
  path.
- `GET /api/v1/search` with type, Task List, and date filters, correct
  ranking and pagination after filtering.

## Acceptance

- The research queries (备份, 服务器备份, 机器人, ESP32, ESP32-S3, C32,
  GNOME 远程登录, 摄像头延迟, 備份, full-width ESP32-S3, E_CONNRESET) return
  the expected fixture passages in the top results; `C320` and `ESP32S30`
  do not match `C32` and `ESP32`.
- A one-character Han query returns a clear "query too short" result.
- Deleting the index and restarting yields identical search and task
  results.
- Changing the analyzer fingerprint forces a full rebuild.
- The release binary builds with `CGO_ENABLED=0` and embeds no non-Chinese
  gse dictionaries; dictionary notices are included.

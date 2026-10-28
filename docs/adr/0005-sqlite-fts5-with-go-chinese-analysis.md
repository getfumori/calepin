# SQLite FTS5 with Go-side Chinese analysis

The Search index uses SQLite FTS5 through the CGO-free `modernc.org/sqlite`
driver, fed by a versioned Go analyzer: NFKC, case folding, and OpenCC `t2s`
script conversion, then gse dictionary segmentation of Han text plus a
separately weighted, verified Han bigram fallback and whole identifier tokens.
FTS5's built-in tokenizers cannot find two-character Chinese words, and this
stack keeps a single static binary while letting passage metadata, task
filters, and text rows change in one transaction. Evidence:
[`docs/research/chinese-full-text-search.md`](../research/chinese-full-text-search.md).

## Considered Options

- Bleve with gse: equal recall and the best synthetic ranking; the fallback if
  FTS5 ranking cannot be tuned on real notes.
- CGO drivers or gojieba: no recall gain that justifies the toolchain and
  runtime dictionary packaging.

## Consequences

- The index is updated after every save. Checkpoint-based freshness matters
  only for AI citations.
- Snippets come from original text through span maps, never from normalized
  or segmented text.
- Any change to the analyzer, dictionaries, conversion data, or query compiler
  forces a full rebuild.
- Queries need at least two Han characters or one complete identifier.

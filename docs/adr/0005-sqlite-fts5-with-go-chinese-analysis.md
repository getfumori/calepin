# SQLite FTS5 with Go-side Chinese analysis

Lexical retrieval uses SQLite FTS5 through the CGO-free `modernc.org/sqlite`
driver, fed by a versioned Go analyzer: NFKC, Unicode case folding, and
OpenCC `t2s` script conversion, then gse dictionary segmentation of Han text
plus a separately weighted, verified Han bigram fallback and whole identifier
tokens. FTS5's own `unicode61` and trigram tokenizers cannot find
two-character Chinese words, and the chosen stack keeps a single static binary
while letting passage metadata, task filters, and lexical rows change in one
SQLite transaction. The evidence is in
[`docs/research/chinese-full-text-search.md`](../research/chinese-full-text-search.md).

## Considered Options

- Bleve with gse: equally good recall and the best synthetic ranking, kept as
  the fallback if FTS5 ranking cannot be tuned on real notes.
- CGO drivers or gojieba: no recall gain that justifies the toolchain and
  runtime dictionary packaging.

## Consequences

- The projection is updated after every successful save, so owner search is
  current. Checkpoint-based freshness applies only to AI citations through the
  Retrieval watermark.
- Snippets are generated from original text through span maps, never from
  normalized or segmented text.
- Any change to the analyzer, dictionaries, conversion data, or query compiler
  changes the projection fingerprint and forces a full rebuild.
- Only the Simplified Chinese gse dictionary is embedded; its provenance and
  notices ship with the binary.
- Queries need at least two Han characters or one complete identifier.

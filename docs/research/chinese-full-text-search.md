# Chinese full-text search for the Go Vault Service

Calepin needs dependable lexical retrieval from Chinese Markdown mixed with technical English, while keeping its index disposable and its server easy to distribute. The evidence favors SQLite FTS5 through `modernc.org/sqlite`, with application-side Chinese segmentation, exact identifier tokens, and a separate bigram fallback. Plain `unicode61` and trigram alone fail the required retrieval behavior. This is a research recommendation, not an implemented backend or a replacement ADR.

## Inspection snapshot

- Inspected on 2026-10-27 in America/Los_Angeles, corresponding to 2026-10-28 UTC.
- Read the working copies of [AGENTS.md](../../AGENTS.md), [GLOSSARY.md](../../GLOSSARY.md), especially Core retrieval, Lexical retrieval, Indexed block, Memo, Memo comment, and Occurrence record, and [ADR 0002](../adr/0002-go-vault-service-with-typescript-web-client.md) before the investigation.
- Runtime was Go `1.26.5`, Linux `7.2.9-1-cachyos`, amd64, Intel N97 with four logical CPUs. No remote search service was used.
- All benchmark code, fixtures, executable probes, profiles, and raw results live outside the repository at `/tmp/fumori-search-bench`. Nothing was committed. The benchmark uses synthetic content, not the Primary Vault.

These are the exact resolved versions, not a claim that each remains the latest release:

| Component | Inspected and measured version | Role |
| --- | --- | --- |
| modernc.org/sqlite | `v1.61.0`, reporting SQLite `3.54.1` | Pure-Go FTS5 driver |
| mattn/go-sqlite3 | `v1.14.52`, reporting SQLite `3.53.4` | CGO comparison, `sqlite_fts5` build tag |
| ncruces/go-sqlite3 | `v0.35.6`, go-sqlite3-wasm/v6 `v6.3.35304`, reporting SQLite `3.53.4` | CGO-free translated-Wasm comparison |
| blevesearch/bleve/v2 | `v2.6.1` | Scorch index with explicit BM25 |
| go-ego/gse | `v1.1.0` | Simplified Chinese embedded dictionary, `CutSearch(text, true)` |
| yanyiwu/gojieba | `v1.4.7` | CppJieba search-mode comparison |
| longbridgeapp/opencc | `v0.3.13` | Embedded `t2s` conversion |
| golang.org/x/text | `v0.43.0` | NFKC normalization and Unicode case folding |

Unless marked as an observation, a library claim below comes from the linked primary source. Recommendations and proposed implementation behavior are labeled as design inferences.

## What the candidates actually provide

### SQLite FTS5

`unicode61` recognizes Unicode letter and number runs. It does not discover Chinese word boundaries inside those runs. Its default case handling is not a complete normalization policy. The trigram tokenizer indexes three-character substrings. FTS MATCH queries shorter than three Unicode characters cannot match its index. A LIKE fallback can scan, but does not supply the required indexed lexical ranking. `备份` has two characters; `机器人` has three. Trigram finding the latter does not establish two-character support. See the [FTS5 tokenizer documentation](https://sqlite.org/fts5.html#tokenizers) and [tokenizer implementation](https://github.com/sqlite/sqlite/blob/master/ext/fts5/fts5_tokenize.c).

FTS5 supplies weighted BM25, with better results having smaller scores, and supports SQL joins to metadata. Its built-in snippet and highlight functions operate on the indexed field's content and tokenizer positions. See the [FTS5 auxiliary functions](https://sqlite.org/fts5.html#auxiliary_functions).

Design inference. Segment in Go, serialize each resulting term as one atomic FTS token, and compile the query through the same analysis. Keep the original semantic passage and its Markdown source map separately. This avoids a C tokenizer bridge and lets structured metadata and lexical rows change in one SQLite transaction.

A separate original-text column alone does **not** make `snippet()` on a presegmented field return original text. Nor does attaching original text as external content repair incompatible token positions. FTS5 custom tokenizers report start and end byte offsets into their input. See the [FTS5 tokenizer interface](https://github.com/sqlite/sqlite/blob/master/ext/fts5/fts5.h) and [external-content requirements](https://sqlite.org/fts5.html#external_content_tables). Design inference. Generate snippets in Go from mapped original spans, or use a custom tokenizer that preserves those spans. Never display the token serialization as note text.

| Driver | Packaging and license | Assessment |
| --- | --- | --- |
| modernc | CGO-free; BSD-3-Clause. Its documentation requires a compatible pinned `modernc.org/libc`. | Preferred initial driver. The tested FTS5 build works with `CGO_ENABLED=0`. See [versioned documentation](https://pkg.go.dev/modernc.org/sqlite@v1.61.0). |
| mattn | CGO, C compiler, and FTS5 build tag; MIT. The bundled amalgamation does not require a SQLite server. | Viable, but these results do not justify its extra toolchain requirements. See [README](https://github.com/mattn/go-sqlite3/tree/v1.14.52) and [license](https://github.com/mattn/go-sqlite3/blob/v1.14.52/LICENSE). |
| ncruces | CGO-free; MIT. This version translates a Wasm SQLite build to Go with wasm2go. FTS5 requires registration on each connection. | Credible alternative, especially for Go custom tokenizers. Older wazero-runtime descriptions do not describe this measured version. See [versioned README](https://github.com/ncruces/go-sqlite3/blob/v0.35.6/README.md), [license](https://github.com/ncruces/go-sqlite3/blob/v0.35.6/LICENSE), and [FTS5 API](https://pkg.go.dev/github.com/ncruces/go-sqlite3/ext/fts5). |

SQLite itself is [public domain](https://sqlite.org/copyright.html). Wrapper and dependency licenses still apply. The ncruces README warns that its sandbox model uses more memory per connection; this investigation did not measure comparative resident memory.

### Bleve

Bleve's built-in CJK analyzer uses Unicode tokenization, width folding, lowercasing, and overlapping CJK bigrams. It is dictionary-free. Its default bigram filter does not emit every constituent unigram. See the [analyzer](https://github.com/blevesearch/bleve/blob/v2.6.1/analysis/lang/cjk/analyzer_cjk.go) and [bigram filter](https://github.com/blevesearch/bleve/blob/v2.6.1/analysis/lang/cjk/cjk_bigram.go).

Bleve defaults to TF-IDF. BM25 must be selected explicitly with `mapping.ScoringModel = "bm25"`; it has been available since v2.5.0. See the [versioned scoring documentation](https://github.com/blevesearch/bleve/blob/v2.6.1/docs/scoring.md). The tested lexical build succeeds with `CGO_ENABLED=0`. Bleve uses [Apache-2.0](https://github.com/blevesearch/bleve/blob/v2.6.1/LICENSE).

Bleve supports compound queries and pagination. Highlighting requires stored fields and term vectors. Those offsets refer to the analyzer's input, which may already differ from original Markdown after normalization or presegmentation. See [query documentation](https://blevesearch.com/docs/Query/), [highlighting requirements](https://blevesearch.com/docs/Highlight%20Matches%20in%20Results/), and [SearchRequest fields](https://github.com/blevesearch/bleve/blob/v2.6.1/search.go).

Design inference. Built-in CJK analysis still needs an explicit identifier policy. Splitting punctuation can make separate occurrences of `ESP32` and `S3` look like a compound-model query. The application identifier tokens prevent that ambiguity; the real fixture should include this additional decoy.

The benchmark uses built-in CJK and local whitespace-analyzer adapters for gse and gojieba. It does not use the archived [gse-bleve adapter](https://github.com/vcaesar/gse-bleve), whose [module file](https://github.com/vcaesar/gse-bleve/blob/main/go.mod) pins older dependencies. Design inference. A maintained local adapter is preferable to treating that adapter as a current integration guarantee.

### Segmentation and other pure-Go approaches

[gse](https://github.com/go-ego/gse/tree/v1.1.0) provides Go dictionary segmentation and embedded dictionaries under an [Apache-2.0 root license](https://github.com/go-ego/gse/blob/v1.1.0/LICENSE). [gojieba](https://github.com/yanyiwu/gojieba/tree/v1.4.7) wraps CppJieba through CGO under an [MIT root license](https://github.com/yanyiwu/gojieba/blob/v1.4.7/LICENSE). Its default dictionary lookup uses runtime paths. Compiling the library does not package those dictionaries into the executable.

Dictionary provenance needs its own record. gse's [dictionary README](https://github.com/go-ego/gse/blob/v1.1.0/data/dict/README.md) identifies Jieba data and additional corpus-derived files. The benchmark uses `s_1.txt`, not the added TF-IDF corpora. Do not interpret the root license as an independent provenance audit of every bundled data file. Record exact included files and retain their notices.

The additional pure-Go candidate is application-generated Han bigrams, alone or combined with dictionary terms, feeding FTS5 or Bleve. This separates the recall strategy from the storage engine. Design inference. It is more useful here than implementing a new postings index, BM25 scorer, persistence layer, and query language.

[Bluge](https://github.com/blugelabs/bluge) is another credible Go BM25 indexing library under Apache-2.0. It was screened through its official repository but not benchmarked. Design inference. It adds another backend without resolving the analyzer and source-offset work already tested here. The measured alternative in this report is the Go bigram/hybrid analyzer, not a claim about Bluge performance.

## Benchmark design and boundaries

The main fixture contains 480 passage documents, 120 synthetic source IDs, and 258,510 UTF-8 bytes across title, heading, tags, and body, about 0.247 MiB. Each of 12 topics has 40 variants. There are 120 units of each kind, section, Memo, comment, and Occurrence. Metadata includes a distinct object type, task-list reference, date, working revision, and source position. This is a passage-level fixture, not 480 complete imported Markdown notes. Source positions are synthetic integers, `i * 100`, rather than checked byte ranges. The benchmark does not validate Markdown extraction or canonical source mapping.

Topics cover server backups, phone backups, robotics, ESP32-S3, ESP32 sensors, C32, GNOME remote login, camera latency, errors, reading, logistics, and misleading model strings. Bodies include code fences, shell commands, YAML, C declarations, logs, and variable-length diary prose. Forty-eight passages use Traditional text; four S3 passages use full-width identifiers. Decoys include `ESP32S30`, `XC32`, `C320`, and disconnected `机器` and `器人`. Eight server-backup passages say `服务器的备份`, rather than the uninterrupted query string.

Relevance labels come from assigned topics, not the candidate tokenizer. `ESP32` includes the S3 family but excludes `ESP32S30`; `ESP32-S3` requires that exact model. Relevant title matches have gain 3, heading matches gain 2, and other relevant passages gain 1. Tags have gain 1 in this particular preference model. Recall is relevant returned passages divided by the complete labeled relevant set. P@5 counts relevant results among five positions, counting empty positions as misses. nDCG@5 uses gain `2^grade - 1`. These are controlled field-preference labels, not independent human relevance judgments.

All candidates receive NFKC, Unicode case folding, and OpenCC `t2s` before indexing and query analysis. Thus these are tests of complete configured pipelines, not claims that an engine supplies Traditional conversion by itself. Title, heading, tag, and body weights are 8, 4, 3, and 1. Weights are not numerically equivalent across engines.

SQLite batches the rebuild into one transaction, uses prepared inserts, one connection, WAL, and `synchronous=NORMAL`. Each update commits one replacement. Bleve uses Scorch, one rebuild batch, explicit BM25, stored fields, and term vectors. Both retain original body, source ID, working revision, and position. SQLite also stores its encoded fields; Bleve stores its analyzed-input fields. Byte totals are these configurations' closed index footprints, not a minimal or identical on-disk representation.

The dictionary configurations require every query term. The bigram configuration requires every generated gram. The hybrid indexes both streams and matches the dictionary conjunction OR the bigram conjunction. This recovers separated compounds while retaining dictionary-independent substring coverage. Terms may match across searchable fields. Baseline FTS queries quote whitespace-separated input groups; built-in Bleve CJK analyzes each group within a field. These semantic differences are intentional candidate settings and prevent attributing every result difference solely to the engine.

Five trials per configuration alternate forward and reverse candidate order. A second five-trial run alternates Bleve gojieba with Bleve gse. Every measured process must return valid JSON; errors abort the run. Early setup failures were corrected before these final datasets. Rebuild time includes normalizer and relevant dictionary initialization, analysis, index creation, commit, close/checkpoint, and reopen. It excludes Go compilation, synthetic-fixture generation, and Markdown parsing. Update time includes analysis and one committed unit replacement on an open index. It excludes canonical save, parsing, notifications, and subsequent validation searches. It is not an end-to-end save latency.

Every final trial checks a unique new term after replacement and its absence after restoration. The final dataset verification asserts both update and filter flags on every record; the runner itself aborts immediately on process or update failure. Reopen precedes the query evaluation. Filtering tests intersect object type `task`, unit kind `memo`, task list `work`, and a date lower bound. They compare the complete filtered IDs and order against independently filtering the unrestricted result sequence, then compare two three-result pages with that sequence. Bleve metadata clauses use zero boost so they cannot change lexical order. No filter or update checks failed.

The host had four CPUs. Initial load averages were 1.11, 0.41, and 0.22; compilation temporarily raised them. Timed trials began after compilation, with no remaining compiler process. Order alternation and the reported ranges expose remaining noise. These figures establish feasibility for these fixtures. They do not establish an engine-wide speed ranking or a production latency SLA.

## Recall and the first five results

Recall over the full returned set is:

| Query | Relevant units | unicode61 | Trigram | Dictionary | Go bigram / CJK | Hybrid |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `备份` | 80 | 12.5% | 0.0% | 100.0% | 100.0% | 100.0% |
| `服务器备份` | 40 | 20.0% | 80.0% | 100.0% | 80.0% | 100.0% |
| `机器人` | 40 | 25.0% | 100.0% | 100.0% | 100.0% | 100.0% |
| `ESP32` | 80 | 100.0% | 100.0% | 100.0% | 100.0% | 100.0% |
| `ESP32-S3` | 40 | 100.0% | 100.0% | 100.0% | 100.0% | 100.0% |
| `C32` | 40 | 100.0% | 100.0% | 100.0% | 100.0% | 100.0% |
| `GNOME 远程登录` | 40 | 25.0% | 100.0% | 100.0% | 100.0% | 100.0% |
| `摄像头延迟` | 40 | 25.0% | 100.0% | 100.0% | 100.0% | 100.0% |
| `备` | 80 | 0.0% | 0.0% | 0.0% | 0.0% | 0.0% |
| `備份` | 80 | 12.5% | 0.0% | 100.0% | 100.0% | 100.0% |
| `ＥＳＰ３２－Ｓ３` | 40 | 100.0% | 100.0% | 100.0% | 100.0% | 100.0% |
| `E_CONNRESET` | 40 | 100.0% | 100.0% | 100.0% | 100.0% | 100.0% |

The dictionary pipelines include FTS5 with gse through all three drivers, FTS5 with gojieba, and Bleve with gse or gojieba. Their relevant sets agree on this fixture. Every pipeline returned zero results for the optional one-character probe `备`. It is excluded from the aggregate quality table below and retained here as a visible limitation.

Trigram returned 160 hits for `ESP32`, only 80 relevant, and 80 for `C32`, only 40 relevant. Its `ESP32` P@5 was 0.4. Raw bigram and hybrid pipelines, including Bleve CJK, returned 50 `机器人` hits, with 40 relevant, because the disconnected-gram decoys pass conjunction matching. Their first five remained relevant. Perfect recall and P@5 therefore do not imply a clean complete result set.

The following macro averages cover the eleven queries other than the single-character probe:

| Configuration | Mean recall | Mean P@5 | Mean nDCG@5 |
| --- | ---: | ---: | ---: |
| FTS5 unicode61 / modernc | 0.564 | 1.000 | 0.532 |
| FTS5 trigram / modernc | 0.800 | 0.764 | 0.756 |
| FTS5 gse / modernc | 1.000 | 1.000 | 0.959 |
| FTS5 Go bigrams / modernc | 0.982 | 1.000 | 0.945 |
| FTS5 hybrid / modernc | 1.000 | 1.000 | 0.942 |
| Bleve CJK | 0.982 | 1.000 | 0.753 |
| Bleve gse | 1.000 | 1.000 | 1.000 |
| Bleve hybrid | 1.000 | 1.000 | 0.688 |
| FTS5 gse / mattn | 1.000 | 1.000 | 0.959 |
| FTS5 gojieba / mattn | 1.000 | 1.000 | 0.959 |
| FTS5 gse / ncruces | 1.000 | 1.000 | 0.959 |
| FTS5 hybrid / ncruces | 1.000 | 1.000 | 0.942 |
| Bleve gojieba | 1.000 | 1.000 | 1.000 |

These measurements favor dictionary analysis for the required examples. Naively adding and scoring overlapping dictionary and bigram streams does not automatically improve ranking. Bleve gse scored best under the fixture's field-preference labels; this does not prove superior ranking on real notes. An explanatory Bleve CJK run showed short, rare tag fields outranking longer title fields despite the chosen boosts. Store the explanation and tune the mapping rather than treating equal numeric boosts as equivalent relevance policies. The two-character failure of raw FTS tokenizers is decisive; the exact nDCG gap between tuned alternatives is not.

## Rebuild, updates, and index size

| Configuration | Rebuild ms | Update ms | Index MiB, median |
| --- | ---: | ---: | ---: |
| FTS5 unicode61 / modernc | 106.5 [103.6, 108.0] | 0.498 [0.493, 0.516] | 0.684 |
| FTS5 trigram / modernc | 115.2 [114.2, 116.0] | 0.580 [0.566, 0.584] | 1.035 |
| FTS5 gse / modernc | 1099.2 [1090.6, 1119.8] | 0.650 [0.642, 0.661] | 1.191 |
| FTS5 Go bigrams / modernc | 146.2 [145.0, 179.5] | 0.631 [0.593, 0.657] | 1.730 |
| FTS5 hybrid / modernc | 1126.5 [1122.4, 1187.5] | 0.707 [0.696, 0.724] | 2.234 |
| Bleve CJK | 237.2 [234.1, 242.6] | 2.335 [1.987, 2.444] | 1.846 |
| Bleve gse | 1152.7 [1135.2, 1249.1] | 1.987 [1.951, 2.449] | 1.447 |
| Bleve hybrid | 1255.9 [1227.5, 1285.2] | 2.274 [2.068, 2.565] | 2.734 |
| FTS5 gse / mattn | 1077.3 [1073.2, 1144.7] | 0.515 [0.513, 0.527] | 1.191 |
| FTS5 gojieba / mattn | 909.0 [899.4, 997.8] | 0.503 [0.493, 0.581] | 1.238 |
| FTS5 gse / ncruces | 1110.9 [1084.8, 1264.3] | 0.723 [0.708, 1.020] | 1.191 |
| FTS5 hybrid / ncruces | 1132.1 [1128.6, 1186.4] | 0.778 [0.762, 0.786] | 2.234 |
| Bleve gojieba | 996.3 [993.8, 1044.8] | 2.306 [1.990, 3.797] | 1.476 |

Values are medians with observed minimum and maximum in brackets. SQLite byte sizes were stable across the five runs. Bleve sizes varied slightly with segment persistence and merging. Size is measured after closing the index and checkpointing SQLite WAL; it excludes the canonical Markdown, attachments, dictionary data already embedded in the binary, and future production source-span mappings.

A second fixture contains 8,004 passage units and 26,253,148 bytes, **25.04 MiB**, using the same topics with 28 additional diary-prose lines per passage. Five interleaved trials each measured the selected modernc dictionary and hybrid paths. All filter and update checks passed. This repeats synthetic prose to test index capacity; it is not evidence of real-Vault ranking.

| Configuration | Rebuild seconds | Update ms | Closed index MiB |
| --- | ---: | ---: | ---: |
| FTS5 gse / modernc | 17.36 [17.29, 17.96] | 2.519 [2.478, 3.438] | 101.05 |
| FTS5 hybrid / modernc | 20.75 [20.42, 21.38] | 2.853 [2.804, 3.034] | 197.09 |

Use the observed ranges when judging feasibility. Do not use these runs to claim a small speed advantage between pipelines. The encoded hybrid index is about 7.9 times the indexed text size. It requires roughly 197 MiB of disposable disk before production span maps and active WAL, and should not be mistaken for a space-efficient final schema.

A separate CPU profile of the final 25 MiB hybrid rebuild excludes query evaluation, fixture generation, and dictionary initialization. It records 24.74 CPU-seconds over a 19.53-second collection interval. The main rebuild stack accounts for 19.40 CPU-seconds. Field analysis accounts for 64.7% of sampled CPU, with gse search segmentation at 26.2%, OpenCC conversion at 22.7%, and background garbage collection at 19.1%. These are cumulative, overlapping costs and must not be added. This identifies analysis and allocation as the main CPU limit in this implementation, rather than establishing a driver speed limit. The artifacts are `rebuild.cpu`, `profile-top.txt`, and `profile-cum.txt` in the scratch directory.

An earlier profile found redundant normalization in the segmented adapters. That work was removed before all final timing tables were regenerated. All 60 main-corpus quality records remained identical after the fix. The profiled run is excluded from the timing distributions.

## Executable and distribution cost

These are separate stripped Linux/amd64 executable probes built with `go build -buildvcs=false -ldflags='-s -w'`. The baseline calls `fmt.Println`. Library probes execute a representative API, so the linker must retain it. Deltas use the same baseline but are not additive because libraries share Go runtime and dependencies. They are measurements of probes, not the eventual Vault Service with embedded Web assets.

| Probe | Binary MiB | Delta from baseline MiB | CGO |
| --- | ---: | ---: | --- |
| base | 1.52 | 0.00 | No |
| modernc | 6.33 | 4.80 | No |
| mattn | 3.89 | 2.37 | Yes |
| ncruces | 8.50 | 6.98 | No |
| bleve | 13.88 | 12.36 | No |
| gse | 33.36 | 31.84 | No |
| jieba | 1.77 | 0.25 | Yes |
| opencc | 3.26 | 1.73 | No |
| gse-slim | 7.38 | 5.85 | No |
| chosen | 13.48 | 11.96 | No |

The default gse probe embeds dictionaries for other languages, including a roughly 23 MiB Japanese dictionary. A `ne` build plus an application-owned `//go:embed` of only `s_1.txt` reduces its probe substantially. The combined chosen probe includes modernc FTS5, gse with only that dictionary, OpenCC, NFKC, and case folding. Both builds use `CGO_ENABLED=0` and need no runtime dictionary paths. This follows the [gse embedding build files](https://github.com/go-ego/gse/blob/v1.1.0/dict_embed.go) and is verified by the executable probes.

The small gojieba executable is misleading if read alone. Its default dictionary directory occupies approximately 14 MiB on this machine, and `ldd` reports libstdc++, libgcc, libm, and libc dependencies. The mattn probe links libc and libm. Static CGO packaging may be possible, but was not tested. Neither CGO option improves fixture recall enough to justify taking that packaging work on now.

## Essential implementation and reproduction evidence

The scratch module includes `main.go`, build-tag-selected driver files, gse/gojieba helpers, `run.py`, `extra-trials.py`, `scale.py`, `summarize.py`, `go.mod`, and `go.sum`. Raw results are `results.jsonl`, `results-bleve-jieba.jsonl`, and `scale-results.jsonl`. Source snapshots and fixtures remain under that directory; they are not repository artifacts.

Representative corpus and query shapes are:

```go
type Doc struct {
	ID, Source, Revision, Kind, Type, TaskList, Date string
	Title, Heading, Tags, Body string
	Position, Topic, Gain int
}

// Body examples retain actual Markdown and code.
body := "每天执行服务器备份，使用 restic 保存快照。\n" +
	"```sh\nrestic backup /srv/data --tag nightly\n```\n"
query := "GNOME 远程登录"
// Relevance comes from topic labels, independently of analysis.
```

The benchmark normalizer and token transport use:

```go
text = cases.Fold().String(norm.NFKC.String(text))
text, err = traditionalToSimplified.Convert(text)
// Handle err. Apply this same pipeline to query text.

// Emit the full Latin identifier plus hyphen/underscore components.
identifier := regexp.MustCompile(`[a-z0-9]+(?:[-_][a-z0-9]+)*`)
// ESP32-S3 produces esp32-s3, esp32, s3.
// C320 remains c320, never c32.

// unicode61 must see each generated term as one atomic token.
func ftsTerm(term string) string {
	return "t" + hex.EncodeToString([]byte(term))
}
// 备份 becomes te5a487e4bbbd. ESP32-S3 becomes t65737033322d7333.
// Preserve repeated token occurrences; deduplicate only redundant query terms.
```

Dictionary search tokens and overlapping Han bigrams are generated separately. For `服务器备份`, the whole-run gram plan includes the cross-boundary gram `器备`. It cannot match `服务器的备份`. The dictionary conjunction supplies the alternate route. In the measured hybrid they share fields; the proposed implementation below separates their scoring weights.

The driver and ranking setup includes:

```go
// modernc registers the database/sql driver named sqlite.
db, err := sql.Open("sqlite", path)

// Current ncruces registers its optional FTS5 module per connection.
db, err := driver.Open(path, func(c *sqlite3.Conn) error {
	return fts5.Register(c)
})

// Bleve's default scoring is not BM25.
m := bleve.NewIndexMapping()
m.ScoringModel = "bm25"
m.DefaultAnalyzer = "cjk"
idx, err := bleve.NewUsing(path, m, "scorch", "boltdb", nil)
```

The measured SQLite query applies all constraints before pagination:

```sql
SELECT u.uid, bm25(ft, 8, 4, 3, 1) AS score
FROM ft JOIN units AS u ON u.id = ft.rowid
WHERE ft MATCH :compiled_literal_query
  AND u.type = 'task' AND u.kind = 'memo'
  AND u.tasklist = 'work' AND u.date >= '2026-10-10'
ORDER BY score, u.uid
LIMIT :page_size OFFSET :offset;
```

SQL parameters do not escape FTS query syntax. The query compiler escapes literal terms and supplies the Boolean operators itself, following [FTS5 string syntax](https://sqlite.org/fts5.html#fts5_strings).

A separate CGO-free ncruces custom-tokenizer proof uses `RegisterCustom` and `CreateTokenizer`. The tokenizer emits normalized token text with byte offsets into the original input. The actual output was:

```text
备份 => 今天為伺服器[備份]，檢查 ＥＳＰ３２－Ｓ３。
ESP32 => 今天為伺服器備份，檢查 [ＥＳＰ３２－Ｓ３]。
ESP32-S3 => 今天為伺服器備份，檢查 [ＥＳＰ３２－Ｓ３]。
```

This proves the current driver can highlight original Traditional and full-width text through a Go tokenizer. It does not prove a complete phrase-aware normalization/source-map implementation. The demonstration converts individual characters and is explicitly unsuitable as the production OpenCC adapter. Its time and size are not presented as a full native-tokenizer benchmark.

Representative commands are:

```sh
cd /tmp/fumori-search-bench
GOCACHE=/tmp/fumori-search-bench/go-cache CGO_ENABLED=0 \
	go build -buildvcs=false -o bench-modernc .
GOCACHE=/tmp/fumori-search-bench/go-cache CGO_ENABLED=0 \
	go build -buildvcs=false -tags ncruces -o bench-ncruces .
GOCACHE=/tmp/fumori-search-bench/go-cache \
	go build -buildvcs=false -tags 'mattn,sqlite_fts5,jieba' -o bench-mattn .
python3 run.py
python3 extra-trials.py
python3 scale.py
python3 summarize.py
```

The 480-unit JSON fixture SHA-256 is `5ac8c695e4f1be9b6ab1522d067d472f7010ef454f8d05f1b20c6b34bbf6dba6`. The query fixture is `08e8859c839dffb5c0b84cb01b92ac7d4ba7aae2c18f13a90b073f195cbb39b3`. The main benchmark source is `05666c0976d3aa5d158080495975c8bf06d245113a26c323245ede2da5e4cd54`, and `go.sum` is `aa2ab8f2b278f691732f3768bfb57b79592f4d72161e0a7da66a70f841ec2cda`. The separate profiling variant stops CPU collection before query evaluation. Scratch files are local evidence and can disappear with `/tmp`; the essential design and recorded results are retained here.

## Recommendation and implementation contract

Design inference. Choose modernc SQLite FTS5 as the initial storage/ranking engine, with a maintained Go analyzer around a pinned Simplified Chinese gse dictionary and Han bigrams. Keep dictionary and gram evidence separately weighted. Retain Bleve with dictionary analysis as the strongest measured alternative. Prefer ncruces if a complete original-offset custom tokenizer proves simpler than maintaining Go snippet mappings. Current measurements do not justify CGO.

This choice follows the repository's single-binary direction and gives metadata, task-list membership, original passages, and lexical rows one transactional lifecycle. For the presegmented design, modernc exposes the needed FTS5 functionality without registering an optional extension on each connection. The ncruces custom-tokenizer API is useful only if the implementation takes the original-offset route. It does not depend on a generic claim that SQLite is faster than Bleve. The synthetic ranking results do not support that claim. The Explain the Number principle led to repeated trials and a separate rebuild profile; Model the Domain led to passage documents with explicit source and filter metadata.

### Tokenization, normalization, and matching

Design inference. Use the following versioned pipeline:

1. Project Markdown-semantic passages, retaining inline code and code-block content while removing presentation syntax from the searchable text. Keep section boundaries, heading paths, Memo identities, comment parentage, and Occurrence identities. Keep canonical UTF-8 source ranges and a display-text-to-Markdown map.
2. Apply NFKC, then Unicode case folding, then pinned OpenCC `t2s` to a shadow representation and to the query. Preserve the original text. Use script conversion, not Taiwanese/Mainland vocabulary rewriting. Thus `備份` and `备份` are equivalent, but regional synonyms such as `伺服器` and `服务器` require an explicit future alias policy. OpenCC distinguishes script and regional conversions in its [official project](https://github.com/BYVoid/OpenCC) and the port's [configuration list](https://github.com/longbridgeapp/opencc/tree/v0.3.13).
3. Tokenize Han runs with gse search mode and also emit all overlapping Han bigrams. Preserve actual occurrences for term frequency. Keep Latin identifiers whole and add declared components. Never produce `C32` merely because it is a substring of `C320`. Require the compound token for an exact `ESP32-S3` query; allow `ESP32` to match its declared family component. Apply the same rules inside code.
4. Require all whitespace-separated query groups. Within a Han group, accept the dictionary conjunction or the bigram fallback. Give dictionary evidence more weight. Verify fallback candidates against normalized surface spans or valid dictionary boundaries before counting and paging, so disconnected grams do not become literal phrase matches. This verification and separately weighted layout are proposed refinements, not measurements of the naive hybrid above.
5. Set the initial public minimum to two Han characters or a complete supported identifier token. The one-character probe shows that the tested indexes cannot promise substring search for `备`. If one-character search becomes required, add an explicitly measured unigram field or bounded filtered scan; do not claim the existing pipeline supports it.
6. Build snippets from original display text and original span mappings. Normalization can expand or collapse characters and OpenCC can replace phrases, so map intervals rather than assuming equal byte lengths or converting one rune at a time. Escape output for HTML. Fall back to an unhighlighted original excerpt when alignment is uncertain, and return the canonical source range independently.

The pure-Go OpenCC port embeds configuration and dictionaries, but its version is a separate snapshot from upstream OpenCC. See its [source](https://github.com/longbridgeapp/opencc/blob/v0.3.13/opencc.go) and [Apache-2.0 license](https://github.com/longbridgeapp/opencc/blob/v0.3.13/LICENSE). Pin the conversion data as well as the code.

### Per-Vault schema sketch

This is a proposed layout, not the exact measured four-field fixture schema. Each row represents one Indexed block. A Memo comment remains a separate result pointing into the original Daily Note, even when its Memo has been extracted. An Occurrence record points into its Page Task. See the [domain definitions](../../GLOSSARY.md).

```sql
CREATE TABLE projection_meta (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
-- schema version, generation, tokenizer fingerprint, checkpoint watermark,
-- working-change state, SQLite version, and projection extraction version.

CREATE TABLE indexed_unit (
	rowid INTEGER PRIMARY KEY,
	unit_id TEXT NOT NULL UNIQUE,
	source_id TEXT NOT NULL,
	object_working_revision TEXT NOT NULL,
	unit_kind TEXT NOT NULL,
	object_type_id TEXT,
	parent_memo_id TEXT,
	occurrence_id TEXT,
	stable_block_id TEXT,
	event_date TEXT,
	heading_path TEXT NOT NULL,
	source_start_byte INTEGER NOT NULL,
	source_end_byte INTEGER NOT NULL,
	text_hash TEXT NOT NULL,
	original_text TEXT NOT NULL,
	normalized_text TEXT NOT NULL,
	span_map BLOB NOT NULL
);
CREATE INDEX unit_type_date ON indexed_unit(object_type_id, event_date);
CREATE INDEX unit_owner ON indexed_unit(source_id);

CREATE TABLE unit_task_list (
	unit_rowid INTEGER NOT NULL REFERENCES indexed_unit(rowid),
	task_list_id TEXT NOT NULL,
	PRIMARY KEY(unit_rowid, task_list_id)
);
CREATE INDEX list_units ON unit_task_list(task_list_id, unit_rowid);

CREATE VIRTUAL TABLE unit_fts USING fts5(
	title_words, heading_words, tag_words, body_words,
	title_grams, heading_grams, tag_grams, body_grams,
	tokenize='unicode61'
);
```

Design inference. Serialize each term atomically as in the benchmark. Start tuning separate word and gram weights against labeled real passages; the measured 8/4/3/1 word-field weights are only a starting point. The projection also needs Knowledge Space, retrieval-role, and policy metadata from Core retrieval. Those omitted columns do not disappear because the index is disposable.

Use `EXISTS` for task-list filtering to avoid duplicate rows from multivalued membership. Apply canonical type, date, task-list, policy, and fallback-verification predicates before `ORDER BY`, count, and `LIMIT`. Date semantics must identify which timestamp each unit exposes, including an Occurrence's scheduled versus actual time. Use stable `unit_id` as a score tiebreaker. Bind pagination to the projection generation and query/filter fingerprint. Reject or restart a cursor after the generation changes rather than mixing BM25 statistics from different generations.

Replace all changed units for one source, including obsolete units and task-list references, in one projection transaction. Rebuild in a separate database and atomically activate a completed generation. Markdown remains authoritative; the projection's deletion must not lose a Memo, comment, Occurrence, or citation identity.

There is an existing policy boundary to resolve before implementation. The requested benchmark measures an update after every save. [Retrieval watermark and Search refresh](../../GLOSSARY.md) currently describe checkpoint-based freshness and explicitly distinguish it from per-save indexing. The engine supports small committed per-save updates, but this report does not silently change that domain contract. If per-save projection updates are adopted, keep the checkpoint watermark distinct from working-revision freshness and update the contract through the normal design process.

Resolved after this report: [ADR 0005](../adr/0005-sqlite-fts5-with-go-chinese-analysis.md) adopts per-save index updates and keeps the Retrieval watermark only for AI citations.

### Fingerprint and rebuild rule

Design inference. Persist a fingerprint over projection-schema and Markdown-extractor versions, engine/scoring configuration, tokenizer code version, gse mode and HMM flag, included dictionary hashes, NFKC/case-fold implementation version, OpenCC code/config/data hashes, identifier rules, query compiler, and snippet-map format. Changing any analysis or position interpretation invalidates the old generation and triggers a full rebuild. Never mix old document analysis with new query analysis.

For this inspection, the gse `s_1.txt` SHA-256 is `2b3063ec552327520bee3c0c5819d6e131ab3db50a60b94641ec90f611c24bcd`. OpenCC `TSPhrases.txt` is `b2ef895dd4953b4bb77fc8ef8d26a2a9ca6d43a760ed9a1d767672cfafa6324f`; `TSCharacters.txt` is `6b5a0a799bea2bb22c001f635eaa3fc2904310f0c08addbff275477a80ecf09a`. Include the configuration and every additional loaded file in the actual fingerprint.

### Review attention

Independent review by `gpt-6-astra` found no blocking flaw in the research recommendation. Its evidence flags were resolved by making synthetic source positions explicit, retaining slim-binary size evidence, removing an unsupported storage-budget judgment, and verifying every final update/filter flag. The remaining limitations are the unimplemented refinements and real-Vault validation below.

### Open risks and required implementation validation

- The repeated-topic corpus proves specific capabilities, not general relevance. Validate a private, human-labeled sample of real notes with rare two-character terms, mixed identifiers, title/heading/tag preferences, and adjacent versus separated query groups. Compare dictionary-first ranking with the separately weighted fallback. Keep the disconnected-gram decoys.
- The proposed separate fields, fallback verification, and original span maps are not the naive measured hybrid. Measure their complete ranked-and-filtered query path. Avoid taking a limited engine page and filtering it afterward.
- The scale fixture repeats diary prose and is neither a 25 MiB real Vault nor a complete Markdown parser benchmark. Measure parse-to-visible-search and search latency on actual target hardware, including cold startup, resident memory, active WAL size, and source-map storage. Investigate contentless FTS storage if duplicated token text costs matter; no space savings from that change are claimed here.
- Validate original snippets through full-width text, case-fold expansions, combining characters, Traditional phrase conversion, repeated matches, Markdown links, and code fences. The native-offset demonstration covers only a narrow subset. Never use normalized byte offsets as original offsets.
- Test replacing and deleting complete notes, Memo extraction, later comments, Occurrence edits, heading changes, metadata-only updates, cancellation, crash/reopen, and rebuild activation. Prove changed tokenizer fingerprints force rebuilds. Check concurrent readers and analyzer use with Go's race detector.
- Confirm type/task-list/date filters preserve lexical order, counts, and all pages on the same generation. Test generation changes between page requests and zero-hit groups. Preserve the per-Vault and retrieval-role boundaries of Core retrieval.
- Retain provenance and notices for exactly the dictionaries shipped. Prove the release artifact runs with `CGO_ENABLED=0`, with no module cache, installed dictionaries, or writable index directory outside the chosen Vault runtime area.

The recommended implementation is SQLite FTS5 with modernc, dictionary-first Go analysis, exact identifier tokens, and a separately weighted, verified bigram fallback. Original-text snippets and generation-consistent filtering are part of that implementation, not optional follow-up features.

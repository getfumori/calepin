package index

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	_ "modernc.org/sqlite"

	"github.com/getfumori/fumori/internal/format"
	"github.com/getfumori/fumori/internal/vault"
)

// ErrQueryTooShort means the query has fewer than two Han characters and no
// complete identifier.
var ErrQueryTooShort = errors.New("query needs at least two Chinese characters or one complete word")

type Index struct {
	db *sql.DB
	a  *analyzer
}

const schema = `
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE docs (path TEXT PRIMARY KEY, revision TEXT NOT NULL);
CREATE VIRTUAL TABLE units USING fts5(
	path UNINDEXED, original UNINDEXED, normalized UNINDEXED,
	title_w, body_w, title_g, body_g,
	tokenize = "unicode61 remove_diacritics 0 tokenchars '-_.'"
);`

// Open opens the index in stateDir, rebuilding it from scratch when the
// fingerprint differs or the file is unreadable.
func Open(stateDir string) (vault.Index, error) {
	a, err := getAnalyzer()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(stateDir, "index")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "search.db")
	idx, err := openAt(path, a)
	if err == nil {
		return idx, nil
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		os.Remove(path + suffix)
	}
	return openAt(path, a)
}

func openAt(path string, a *analyzer) (*Index, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	var current string
	err = db.QueryRow(`SELECT value FROM meta WHERE key = 'fingerprint'`).Scan(&current)
	if err == nil && current == fingerprint {
		return &Index{db: db, a: a}, nil
	}
	var tables int
	db.QueryRow(`SELECT count(*) FROM sqlite_master`).Scan(&tables)
	if tables > 0 {
		db.Close()
		return nil, fmt.Errorf("index fingerprint changed")
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(`INSERT INTO meta VALUES ('fingerprint', ?)`, fingerprint); err != nil {
		db.Close()
		return nil, err
	}
	return &Index{db: db, a: a}, nil
}

func (x *Index) Close() error { return x.db.Close() }

// Sync re-indexes objects whose revision changed and drops removed ones.
func (x *Index) Sync(objects []*vault.Object) error {
	known := map[string]string{}
	rows, err := x.db.Query(`SELECT path, revision FROM docs`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var p, r string
		rows.Scan(&p, &r)
		known[p] = r
	}
	rows.Close()
	present := map[string]bool{}
	for _, o := range objects {
		present[o.Path] = true
		if known[o.Path] != o.Revision {
			if err := x.Put(o); err != nil {
				return err
			}
		}
	}
	for p := range known {
		if !present[p] {
			if err := x.Remove(p); err != nil {
				return err
			}
		}
	}
	return nil
}

func (x *Index) Remove(path string) error {
	tx, err := x.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM units WHERE path = ?`, path); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM docs WHERE path = ?`, path); err != nil {
		return err
	}
	return tx.Commit()
}

func (x *Index) Put(o *vault.Object) error {
	tx, err := x.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM units WHERE path = ?`, o.Path); err != nil {
		return err
	}
	title := o.Title
	if o.Kind == vault.KindDaily {
		title = o.Date
	}
	titleTerms := x.a.analyze(x.a.normalize(title + " " + strings.Join(o.Tags, " ") + " " + strings.Join(o.Aliases, " ")))
	for _, unit := range units(o) {
		normalized := x.a.normalize(unit)
		t := x.a.analyze(normalized)
		if _, err := tx.Exec(`INSERT INTO units (path, original, normalized, title_w, body_w, title_g, body_g) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			o.Path, unit, normalized, join(titleTerms.words), join(t.words), join(titleTerms.grams), join(t.grams)); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`INSERT OR REPLACE INTO docs VALUES (?, ?)`, o.Path, o.Revision); err != nil {
		return err
	}
	return tx.Commit()
}

var (
	headingLine = regexp.MustCompile(`(?m)^#{1,6}\s`)
	htmlComment = regexp.MustCompile(`<!--.*?-->`)
	linkBracket = strings.NewReplacer("[[", "", "]]", "")
)

// units splits an object into Indexed blocks: one per heading section of
// free text, and one per identified block.
func units(o *vault.Object) []string {
	body := o.Doc.Body()
	free, _ := format.SplitFreeform(body, o.Blocks)
	var out []string
	starts := headingLine.FindAllStringIndex(free, -1)
	bounds := []int{0}
	for _, s := range starts {
		bounds = append(bounds, s[0])
	}
	bounds = append(bounds, len(free))
	for i := 0; i+1 < len(bounds); i++ {
		if text := clean(free[bounds[i]:bounds[i+1]]); text != "" {
			out = append(out, text)
		}
	}
	for _, b := range o.Blocks {
		if text := clean(b.Content(body)); text != "" {
			out = append(out, text)
		}
	}
	if len(out) == 0 {
		out = append(out, "")
	}
	return out
}

func clean(s string) string {
	s = headingLine.ReplaceAllString(htmlComment.ReplaceAllString(s, ""), "")
	return strings.TrimSpace(linkBracket.Replace(s))
}

type group struct {
	expr  string
	han   []string
	words []string
}

func quote(term string) string { return `"` + strings.ReplaceAll(term, `"`, `""`) + `"` }

func phrase(terms []string) string {
	quoted := make([]string, len(terms))
	for i, t := range terms {
		quoted[i] = quote(t)
	}
	return strings.Join(quoted, " ")
}

func (x *Index) compile(query string) ([]group, error) {
	var groups []group
	for _, raw := range strings.Fields(x.a.normalize(query)) {
		t := x.a.analyze(raw)
		hanRunes := 0
		for _, run := range t.han {
			hanRunes += utf8.RuneCountInString(run)
		}
		var idents []string
		for _, w := range t.words {
			if !strings.ContainsFunc(w, isHan) {
				idents = append(idents, w)
			}
		}
		if hanRunes == 1 && len(idents) == 0 {
			return nil, ErrQueryTooShort
		}
		var parts []string
		if len(idents) > 0 {
			parts = append(parts, "{title_w body_w} : ("+phrase(idents[:1])+")")
		}
		var hanWords []string
		for _, w := range t.words {
			if strings.ContainsFunc(w, isHan) {
				hanWords = append(hanWords, w)
			}
		}
		if hanRunes > 0 {
			parts = append(parts, "({title_w body_w} : ("+phrase(hanWords)+") OR {title_g body_g} : ("+phrase(t.grams)+"))")
		}
		if len(parts) == 0 {
			continue
		}
		groups = append(groups, group{expr: strings.Join(parts, " AND "), han: t.han, words: append(hanWords, idents...)})
	}
	if len(groups) == 0 {
		return nil, ErrQueryTooShort
	}
	return groups, nil
}

// verified rejects bigram matches whose grams are not adjacent: each Han run
// must appear verbatim or as dictionary words in the passage.
func verified(normalized string, groups []group) bool {
	for _, g := range groups {
		for _, run := range g.han {
			if strings.Contains(normalized, run) {
				continue
			}
			for _, w := range g.words {
				if strings.Contains(run, w) && !strings.Contains(normalized, w) {
					return false
				}
			}
		}
	}
	return true
}

func (x *Index) Search(query string, limit int) ([]vault.Hit, error) {
	groups, err := x.compile(query)
	if err != nil {
		return nil, err
	}
	exprs := make([]string, len(groups))
	for i, g := range groups {
		exprs[i] = "(" + g.expr + ")"
	}
	rows, err := x.db.Query(`SELECT path, original, normalized FROM units WHERE units MATCH ?
		ORDER BY bm25(units, 0, 0, 0, 8.0, 1.0, 4.0, 0.5) LIMIT 500`, strings.Join(exprs, " AND "))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	var hits []vault.Hit
	for rows.Next() && len(hits) < limit {
		var path, original, normalized string
		if err := rows.Scan(&path, &original, &normalized); err != nil {
			return nil, err
		}
		if seen[path] || !verified(normalized, groups) {
			continue
		}
		seen[path] = true
		hits = append(hits, vault.Hit{Path: path, Snippet: snippet(original, normalized, groups)})
	}
	return hits, rows.Err()
}

// snippet cuts a window of original text around the first match. Analysis
// keeps one rune per rune for CJK and folded ASCII, so a rune offset found in
// the normalized text points at the same place in the original when the
// rune counts agree.
func snippet(original, normalized string, groups []group) string {
	orig, norm := []rune(original), []rune(normalized)
	at := -1
	for _, g := range groups {
		for _, needle := range append(append([]string{}, g.han...), g.words...) {
			if i := strings.Index(normalized, needle); i >= 0 {
				at = utf8.RuneCountInString(normalized[:i])
				break
			}
		}
		if at >= 0 {
			break
		}
	}
	if at < 0 || len(orig) != len(norm) {
		at = 0
	}
	start, end := max(0, at-40), min(len(orig), at+80)
	text := strings.Join(strings.Fields(string(orig[start:end])), " ")
	if start > 0 {
		text = "…" + text
	}
	if end < len(orig) {
		text += "…"
	}
	if text == "" {
		return "…"
	}
	return text
}

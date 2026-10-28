// Package index is the disposable Search index: SQLite FTS5 fed by a Go
// analyzer for mixed Chinese and English text (ADR-0005).
package index

import (
	_ "embed"
	"strings"
	"sync"
	"unicode"

	"github.com/go-ego/gse"
	"github.com/longbridgeapp/opencc"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// fingerprint changes whenever analysis or schema changes, forcing a rebuild.
const fingerprint = "v1-nfkc-fold-t2s-gse-zh_s-bigram-ident-1"

// zhDict is gse's Simplified Chinese dictionary (data/dict/zh/s_1.txt at
// gse v1.1.0, derived from jieba). Loading it from here instead of through
// gse's LoadDictEmbed leaves gse's other embedded dictionaries unreferenced,
// so the linker drops them and the binary shrinks by about 27 MB.
//
//go:embed dict/zh_s_1.txt
var zhDict string

type analyzer struct {
	seg  gse.Segmenter
	t2s  *opencc.OpenCC
	fold cases.Caser
}

var (
	sharedAnalyzer *analyzer
	analyzerOnce   sync.Once
	analyzerErr    error
	analyzerMu     sync.Mutex
)

func getAnalyzer() (*analyzer, error) {
	analyzerOnce.Do(func() {
		a := &analyzer{fold: cases.Fold()}
		a.seg.SkipLog = true
		if analyzerErr = a.seg.LoadDictStr(zhDict); analyzerErr != nil {
			return
		}
		a.t2s, analyzerErr = opencc.New("t2s")
		sharedAnalyzer = a
	})
	return sharedAnalyzer, analyzerErr
}

// normalize applies NFKC, case folding, and Traditional-to-Simplified script
// conversion, the same way for documents and queries.
func (a *analyzer) normalize(s string) string {
	s = a.fold.String(norm.NFKC.String(s))
	analyzerMu.Lock()
	defer analyzerMu.Unlock()
	if out, err := a.t2s.Convert(s); err == nil {
		return out
	}
	return s
}

// terms holds dictionary words and identifier tokens, and Han bigrams.
type terms struct {
	words []string
	grams []string
	han   []string
}

func isHan(r rune) bool { return unicode.Is(unicode.Han, r) }

func isIdent(r rune) bool {
	return !isHan(r) && (unicode.IsLetter(r) || unicode.IsDigit(r))
}

// analyze splits normalized text into Han runs and identifier runs.
func (a *analyzer) analyze(normalized string) terms {
	var t terms
	runes := []rune(normalized)
	for i := 0; i < len(runes); {
		switch {
		case isHan(runes[i]):
			j := i
			for j < len(runes) && isHan(runes[j]) {
				j++
			}
			run := string(runes[i:j])
			t.han = append(t.han, run)
			analyzerMu.Lock()
			for _, w := range a.seg.CutSearch(run, true) {
				if w = strings.TrimSpace(w); w != "" {
					t.words = append(t.words, w)
				}
			}
			analyzerMu.Unlock()
			if j-i == 1 {
				t.grams = append(t.grams, run)
			}
			for k := i; k+1 < j; k++ {
				t.grams = append(t.grams, string(runes[k:k+2]))
			}
			i = j
		case isIdent(runes[i]):
			j := i
			for j < len(runes) && (isIdent(runes[j]) || (strings.ContainsRune("-_.", runes[j]) && j+1 < len(runes) && isIdent(runes[j+1]))) {
				j++
			}
			token := string(runes[i:j])
			t.words = append(t.words, token)
			if strings.ContainsAny(token, "-_.") {
				t.words = append(t.words, strings.FieldsFunc(token, func(r rune) bool { return strings.ContainsRune("-_.", r) })...)
			}
			i = j
		default:
			i++
		}
	}
	return t
}

func join(tokens []string) string { return strings.Join(tokens, " ") }

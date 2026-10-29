package format

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// BlockKind names an identified block. The set is closed in format v1.
type BlockKind string

const (
	KindMemo       BlockKind = "memo"
	KindComment    BlockKind = "comment"
	KindOccurrence BlockKind = "occurrence"
)

var blockKinds = map[BlockKind]bool{KindMemo: true, KindComment: true, KindOccurrence: true}

// Block is one identified block in a body. Offsets are byte offsets into the
// body; End includes the end marker's newline.
type Block struct {
	Kind         BlockKind
	ID           string
	Meta         map[string]any
	Line         int
	Start, End   int
	ContentStart int
	ContentEnd   int
}

// Content returns the Markdown between the markers.
func (b Block) Content(body string) string {
	return strings.Trim(body[b.ContentStart:b.ContentEnd], "\n")
}

// MetaString returns a string metadata value.
func (b Block) MetaString(key string) string {
	s, _ := b.Meta[key].(string)
	return s
}

// Diagnostic is a problem that makes a file unusable for indexing.
type Diagnostic struct {
	Line    int
	Message string
}

func (d Diagnostic) String() string { return fmt.Sprintf("line %d: %s", d.Line, d.Message) }

var (
	markerPrefix = regexp.MustCompile(`^<!--\s*/?calepin:`)
	startMarker  = regexp.MustCompile(`^<!-- calepin:([a-z]+) (\{.*\}) -->$`)
	endMarker    = regexp.MustCompile(`^<!-- /calepin:([a-z]+) ([A-Za-z0-9_]+) -->$`)
)

// Blocks finds identified blocks. Markers count only at column zero outside
// fenced code; anything else that looks like a marker is reported.
func Blocks(body string) ([]Block, []Diagnostic) {
	var blocks []Block
	var diags []Diagnostic
	var open *Block
	seen := map[string]bool{}
	for i, l := range scanLines(body) {
		lineNo := i + 1
		if l.inFence || l.isFence || !markerPrefix.MatchString(l.text) {
			continue
		}
		if m := startMarker.FindStringSubmatch(l.text); m != nil {
			if open != nil {
				diags = append(diags, Diagnostic{lineNo, fmt.Sprintf("block %s starts inside unterminated block %s", m[1], open.ID)})
				open = nil
				continue
			}
			kind := BlockKind(m[1])
			if !blockKinds[kind] {
				diags = append(diags, Diagnostic{lineNo, fmt.Sprintf("unknown block kind %q", m[1])})
				continue
			}
			var meta map[string]any
			if err := json.Unmarshal([]byte(m[2]), &meta); err != nil {
				diags = append(diags, Diagnostic{lineNo, "block metadata is not valid JSON"})
				continue
			}
			id, _ := meta["id"].(string)
			if id == "" {
				diags = append(diags, Diagnostic{lineNo, "block metadata has no id"})
				continue
			}
			_, next, _ := cutLine(body, l.start)
			open = &Block{Kind: kind, ID: id, Meta: meta, Line: lineNo, Start: l.start, ContentStart: next}
			continue
		}
		if m := endMarker.FindStringSubmatch(l.text); m != nil {
			if open == nil {
				diags = append(diags, Diagnostic{lineNo, fmt.Sprintf("end marker for %s has no start", m[2])})
				continue
			}
			if string(open.Kind) != m[1] || open.ID != m[2] {
				diags = append(diags, Diagnostic{lineNo, fmt.Sprintf("end marker %s %s does not match open block %s %s", m[1], m[2], open.Kind, open.ID)})
				open = nil
				continue
			}
			_, next, _ := cutLine(body, l.start)
			open.ContentEnd = l.start
			open.End = next
			if seen[open.ID] {
				diags = append(diags, Diagnostic{open.Line, fmt.Sprintf("duplicate block id %s", open.ID)})
			}
			seen[open.ID] = true
			blocks = append(blocks, *open)
			open = nil
			continue
		}
		diags = append(diags, Diagnostic{lineNo, "malformed block marker"})
	}
	if open != nil {
		diags = append(diags, Diagnostic{open.Line, fmt.Sprintf("block %s is not terminated", open.ID)})
	}
	return blocks, diags
}

// KV is one ordered metadata entry.
type KV struct {
	Key   string
	Value any
}

func encodeMeta(meta []KV) string {
	var b strings.Builder
	b.WriteByte('{')
	for i, kv := range meta {
		if i > 0 {
			b.WriteByte(',')
		}
		key, _ := json.Marshal(kv.Key)
		value, err := json.Marshal(kv.Value)
		if err != nil {
			panic(err)
		}
		b.Write(key)
		b.WriteByte(':')
		b.Write(value)
	}
	b.WriteByte('}')
	return b.String()
}

// RenderBlock writes a complete block. meta must start with the id. Go's JSON
// encoder escapes '>' so a value can never close the comment early.
func RenderBlock(kind BlockKind, meta []KV, content string) string {
	id, _ := meta[0].Value.(string)
	return fmt.Sprintf("<!-- calepin:%s %s -->\n%s\n<!-- /calepin:%s %s -->\n",
		kind, encodeMeta(meta), strings.Trim(content, "\n"), kind, id)
}

// AppendBlock appends a rendered block at the end of body, separated by one
// blank line.
func AppendBlock(body, block string) string {
	return withTrailingBlankLine(body) + block
}

// InsertAt inserts a rendered block at offset, which must be a block end.
func InsertAt(body string, offset int, block string) string {
	return body[:offset] + "\n" + block + body[offset:]
}

func withTrailingBlankLine(body string) string {
	trimmed := strings.TrimRight(body, "\n")
	if trimmed == "" {
		return ""
	}
	return trimmed + "\n\n"
}

// SplitFreeform separates free-form text from identified blocks. Free text
// interleaved between blocks is gathered in order.
func SplitFreeform(body string, blocks []Block) (freeform string, blockText string) {
	var free, stream strings.Builder
	last := 0
	for _, block := range blocks {
		free.WriteString(body[last:block.Start])
		if stream.Len() > 0 {
			stream.WriteString("\n")
		}
		stream.WriteString(body[block.Start:block.End])
		last = block.End
	}
	free.WriteString(body[last:])
	return strings.Trim(free.String(), "\n"), stream.String()
}

// JoinFreeform rebuilds a body from free-form text and the block stream.
func JoinFreeform(freeform, blockText string) string {
	freeform = strings.Trim(freeform, "\n")
	switch {
	case blockText == "" && freeform == "":
		return ""
	case blockText == "":
		return freeform + "\n"
	case freeform == "":
		return blockText
	}
	return freeform + "\n\n" + blockText
}

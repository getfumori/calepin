package format

import (
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Quoted is a string that always encodes as a double-quoted YAML scalar, used
// for dates and instants so every YAML parser reads them as strings.
type Quoted string

func (q Quoted) MarshalYAML() (any, error) {
	return &yaml.Node{Kind: yaml.ScalarNode, Style: yaml.DoubleQuotedStyle, Value: string(q)}, nil
}

var fenceLine = regexp.MustCompile("^\\s*(```|~~~)")

// lines splits s into lines with their byte offsets. The newline is not part
// of the line text.
type line struct {
	start, end int
	text       string
	inFence    bool
	isFence    bool
}

func scanLines(s string) []line {
	var out []line
	fenced := false
	offset := 0
	for offset < len(s) || (offset == 0 && len(s) == 0) {
		end, next, _ := cutLine(s, offset)
		text := strings.TrimRight(s[offset:end], "\r")
		isFence := fenceLine.MatchString(text)
		l := line{start: offset, end: end, text: text, inFence: fenced && !isFence, isFence: isFence}
		if isFence {
			fenced = !fenced
		}
		out = append(out, l)
		if next == offset {
			break
		}
		offset = next
	}
	return out
}

var h1 = regexp.MustCompile(`^#\s+(.+?)\s*$`)

// Title returns the first H1 outside code, or "".
func Title(body string) string {
	for _, l := range scanLines(body) {
		if l.inFence || l.isFence {
			continue
		}
		if m := h1.FindStringSubmatch(l.text); m != nil {
			return strings.TrimSpace(m[1])
		}
	}
	return ""
}

// Wikilink is one [[target|label]] occurrence.
type Wikilink struct {
	Target         string
	Label          string
	SourceMarkdown string
}

var wikilinkPattern = regexp.MustCompile(`\[\[([^\]|\n]+?)(?:\|([^\]\n]+?))?\]\]`)
var inlineCode = regexp.MustCompile("`+[^`]*`+")

// Wikilinks returns links outside fenced and inline code.
func Wikilinks(markdown string) []Wikilink {
	var links []Wikilink
	for _, l := range scanLines(markdown) {
		if l.inFence || l.isFence {
			continue
		}
		for _, segment := range splitInlineCode(l.text) {
			if segment.code {
				continue
			}
			for _, m := range wikilinkPattern.FindAllStringSubmatch(segment.text, -1) {
				label := m[2]
				if label == "" {
					label = m[1]
				}
				links = append(links, Wikilink{
					Target:         strings.TrimSpace(m[1]),
					Label:          strings.TrimSpace(label),
					SourceMarkdown: m[0],
				})
			}
		}
	}
	return links
}

// RewriteWikilinks replaces the target of every link outside code for which
// rewrite returns true, keeping labels.
func RewriteWikilinks(markdown string, rewrite func(target string) bool, newTarget string) string {
	var b strings.Builder
	lines := scanLines(markdown)
	for i, l := range lines {
		if i > 0 {
			b.WriteString(markdown[lines[i-1].end:l.start])
		}
		raw := markdown[l.start:l.end]
		if l.inFence || l.isFence {
			b.WriteString(raw)
			continue
		}
		for _, segment := range splitInlineCode(raw) {
			if segment.code {
				b.WriteString(segment.text)
				continue
			}
			b.WriteString(wikilinkPattern.ReplaceAllStringFunc(segment.text, func(match string) string {
				m := wikilinkPattern.FindStringSubmatch(match)
				if !rewrite(strings.TrimSpace(m[1])) {
					return match
				}
				if m[2] != "" {
					return "[[" + newTarget + "|" + m[2] + "]]"
				}
				return "[[" + newTarget + "]]"
			}))
		}
	}
	if len(lines) > 0 {
		b.WriteString(markdown[lines[len(lines)-1].end:])
	}
	return b.String()
}

type segment struct {
	text string
	code bool
}

func splitInlineCode(s string) []segment {
	var out []segment
	last := 0
	for _, loc := range inlineCode.FindAllStringIndex(s, -1) {
		out = append(out, segment{text: s[last:loc[0]]}, segment{text: s[loc[0]:loc[1]], code: true})
		last = loc[1]
	}
	return append(out, segment{text: s[last:]})
}

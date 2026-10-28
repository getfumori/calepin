// Package format parses and edits Vault format v1 Markdown without touching
// the file system. Edits replace the smallest byte range they can, so bytes
// they do not own stay identical.
package format

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Document is one canonical Markdown file: YAML frontmatter and a body.
type Document struct {
	Source string
	// bodyStart is the byte offset where the body begins in Source.
	bodyStart int
	// closeStart is the byte offset of the closing "---" line.
	closeStart int
	root       *yaml.Node
}

var ErrNoFrontmatter = errors.New("file must begin with YAML frontmatter")

// Parse splits frontmatter and body and validates the accepted YAML subset.
func Parse(source string) (*Document, error) {
	first, rest, ok := cutLine(source, 0)
	if !ok || strings.TrimRight(source[:first], "\r") != "---" {
		return nil, ErrNoFrontmatter
	}
	offset := rest
	for offset < len(source) {
		end, next, _ := cutLine(source, offset)
		if strings.TrimRight(source[offset:end], "\r") == "---" {
			doc := &Document{Source: source, bodyStart: next, closeStart: offset}
			root, err := parseMapping(source[rest:offset])
			if err != nil {
				return nil, err
			}
			doc.root = root
			return doc, nil
		}
		offset = next
	}
	return nil, errors.New("frontmatter is not closed by a '---' line")
}

// cutLine returns the end of the line starting at offset (excluding the
// newline) and the offset of the next line.
func cutLine(s string, offset int) (end, next int, found bool) {
	i := strings.IndexByte(s[offset:], '\n')
	if i < 0 {
		return len(s), len(s), offset < len(s)
	}
	return offset + i, offset + i + 1, true
}

func parseMapping(source string) (*yaml.Node, error) {
	var file yaml.Node
	if err := yaml.Unmarshal([]byte(source), &file); err != nil {
		return nil, fmt.Errorf("invalid frontmatter: %w", err)
	}
	if len(file.Content) == 0 {
		return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}, nil
	}
	root := file.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, errors.New("frontmatter must be a YAML mapping")
	}
	if err := checkSubset(root); err != nil {
		return nil, err
	}
	return root, nil
}

func checkSubset(node *yaml.Node) error {
	if node.Kind == yaml.AliasNode || node.Anchor != "" {
		return fmt.Errorf("line %d: YAML anchors and aliases are not allowed", node.Line)
	}
	if node.Style&yaml.TaggedStyle != 0 {
		return fmt.Errorf("line %d: YAML tags are not allowed", node.Line)
	}
	if node.Kind == yaml.MappingNode {
		seen := map[string]bool{}
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i].Value
			if seen[key] {
				return fmt.Errorf("line %d: duplicate key %q", node.Content[i].Line, key)
			}
			seen[key] = true
		}
	}
	for _, child := range node.Content {
		if err := checkSubset(child); err != nil {
			return err
		}
	}
	return nil
}

// Body returns the Markdown after the frontmatter.
func (d *Document) Body() string { return d.Source[d.bodyStart:] }

// Frontmatter returns the frontmatter block including both "---" lines.
func (d *Document) Frontmatter() string { return d.Source[:d.bodyStart] }

// WithBody returns the source with the body replaced.
func (d *Document) WithBody(body string) string { return d.Source[:d.bodyStart] + body }

// Node returns the value node for a top-level key, or nil.
func (d *Document) Node(key string) *yaml.Node {
	for i := 0; i < len(d.root.Content); i += 2 {
		if d.root.Content[i].Value == key {
			return d.root.Content[i+1]
		}
	}
	return nil
}

// Keys returns top-level keys in file order.
func (d *Document) Keys() []string {
	keys := make([]string, 0, len(d.root.Content)/2)
	for i := 0; i < len(d.root.Content); i += 2 {
		keys = append(keys, d.root.Content[i].Value)
	}
	return keys
}

// Decode decodes a top-level key into v. A missing key leaves v untouched and
// reports false.
func (d *Document) Decode(key string, v any) (bool, error) {
	node := d.Node(key)
	if node == nil {
		return false, nil
	}
	if err := node.Decode(v); err != nil {
		return true, fmt.Errorf("field %q: %w", key, err)
	}
	return true, nil
}

// String returns a top-level scalar string value.
func (d *Document) String(key string) (string, bool) {
	node := d.Node(key)
	if node == nil || node.Kind != yaml.ScalarNode || node.Tag == "!!null" {
		return "", false
	}
	return node.Value, true
}

// keyRange returns the byte range in Source that a top-level key occupies:
// from its first line through the line before the next top-level key.
func (d *Document) keyRange(key string) (start, end int, ok bool) {
	lineOffsets := d.frontmatterLineOffsets()
	for i := 0; i < len(d.root.Content); i += 2 {
		if d.root.Content[i].Value != key {
			continue
		}
		start = lineOffsets[d.root.Content[i].Line]
		end = d.closeStart
		if i+2 < len(d.root.Content) {
			end = lineOffsets[d.root.Content[i+2].Line]
		}
		return start, end, true
	}
	return 0, 0, false
}

// frontmatterLineOffsets maps a 1-based YAML line number to its byte offset
// in Source. YAML line 1 is the line after the opening "---".
func (d *Document) frontmatterLineOffsets() map[int]int {
	offsets := map[int]int{}
	_, offset, _ := cutLine(d.Source, 0)
	for line := 1; offset <= d.closeStart; line++ {
		offsets[line] = offset
		_, next, _ := cutLine(d.Source, offset)
		if next == offset {
			break
		}
		offset = next
	}
	return offsets
}

// Set returns the source with one top-level key set to value. Other keys keep
// their bytes; a new key is appended before the closing "---".
func (d *Document) Set(key string, value any) (string, error) {
	encoded, err := encodeKey(key, value)
	if err != nil {
		return "", err
	}
	if start, end, ok := d.keyRange(key); ok {
		return d.Source[:start] + encoded + d.Source[end:], nil
	}
	return d.Source[:d.closeStart] + encoded + d.Source[d.closeStart:], nil
}

// Delete returns the source without a top-level key.
func (d *Document) Delete(key string) string {
	if start, end, ok := d.keyRange(key); ok {
		return d.Source[:start] + d.Source[end:]
	}
	return d.Source
}

// Edit applies Set and Delete operations in order, reparsing between them.
// A nil value deletes the key.
func Edit(source string, changes []Change) (string, error) {
	for _, change := range changes {
		doc, err := Parse(source)
		if err != nil {
			return "", err
		}
		if change.Value == nil {
			source = doc.Delete(change.Key)
			continue
		}
		if source, err = doc.Set(change.Key, change.Value); err != nil {
			return "", err
		}
	}
	return source, nil
}

// Change is one frontmatter edit for Edit.
type Change struct {
	Key   string
	Value any
}

// Null encodes an explicit YAML null, distinct from deleting the key.
type Null struct{}

func encodeKey(key string, value any) (string, error) {
	var valueNode yaml.Node
	if _, isNull := value.(Null); isNull {
		valueNode = yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
	} else if err := valueNode.Encode(value); err != nil {
		return "", err
	}
	flowScalarSequences(&valueNode)
	mapping := yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: key},
		&valueNode,
	}}
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(&mapping); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// flowScalarSequences writes short lists such as tags as [a, b], matching
// the hand-written style of the format fixtures.
func flowScalarSequences(node *yaml.Node) {
	if node.Kind == yaml.SequenceNode {
		scalars := true
		for _, child := range node.Content {
			if child.Kind != yaml.ScalarNode {
				scalars = false
			}
		}
		if scalars {
			node.Style = yaml.FlowStyle
		}
	}
	for _, child := range node.Content {
		flowScalarSequences(child)
	}
}

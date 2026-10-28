package format

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func fixtures(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile("../../docs/architecture/vault-format.md")
	if err != nil {
		t.Fatal(err)
	}
	section := string(raw)[strings.Index(string(raw), "## Fixtures"):]
	section = section[:strings.Index(section, "\n## Task state machine")]
	out := map[string]string{}
	re := regexp.MustCompile("(?s)### ([^\n]+)\n\n```markdown\n(.*?)\n```")
	for _, m := range re.FindAllStringSubmatch(section, -1) {
		out[m[1]] = m[2] + "\n"
	}
	if len(out) != 4 {
		t.Fatalf("expected 4 fixtures in vault-format.md, found %d", len(out))
	}
	return out
}

func TestFormatFixtures(t *testing.T) {
	fx := fixtures(t)
	for name, source := range fx {
		doc, err := Parse(source)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if _, diags := Blocks(doc.Body()); len(diags) > 0 {
			t.Fatalf("%s: %v", name, diags)
		}
		if _, err := doc.PageTask(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}

	recurring, _ := Parse(fx["Recurring Page Task with history"])
	task, _ := recurring.PageTask()
	if !task.Recurring() || task.Occurrence.ID != "o_01J9R2B6W4TQ" || task.Recurrence.Weekdays[0] != "sun" {
		t.Fatalf("recurring task decoded wrong: %+v", task)
	}
	blocks, _ := Blocks(recurring.Body())
	if len(blocks) != 1 || blocks[0].Kind != KindOccurrence || blocks[0].MetaString("outcome") != "completed" {
		t.Fatalf("occurrence record decoded wrong: %+v", blocks)
	}

	daily, _ := Parse(fx["Daily Note with free writing, Memos, a comment, and an Inline Task"])
	blocks, _ = Blocks(daily.Body())
	kinds := []BlockKind{}
	for _, b := range blocks {
		kinds = append(kinds, b.Kind)
	}
	if strings.Join(toStrings(kinds), ",") != "memo,memo,comment" || blocks[2].MetaString("parent") != "m_01J9K3F8Q2VA" {
		t.Fatalf("memo stream decoded wrong: %v", kinds)
	}
	inline, _ := InlineTasks(daily.Body())
	if len(inline) != 1 || inline[0].ID != "t_01J9K3G1N7HS" || inline[0].Checked {
		t.Fatalf("inline task decoded wrong: %+v", inline)
	}
	free, _ := SplitFreeform(daily.Body(), blocks)
	if free != "# 2026-10-27\n\nSlept badly. Plan: finish the backup script." {
		t.Fatalf("free-form region wrong: %q", free)
	}

	oneOff, _ := Parse(fx["One-off Page Task"])
	if tasks, _ := InlineTasks(oneOff.Body()); len(tasks) != 0 {
		t.Fatal("plain checkboxes must not become tasks")
	}
	if Title(oneOff.Body()) != "Replace server disk" {
		t.Fatalf("title: %q", Title(oneOff.Body()))
	}
}

func toStrings(kinds []BlockKind) []string {
	out := make([]string, len(kinds))
	for i, k := range kinds {
		out[i] = string(k)
	}
	return out
}

func TestEditKeepsUnrelatedBytes(t *testing.T) {
	source := "---\n_id: x\n# keep this comment\ncustom:\n  nested: [1, 2]  # trailing\ntags: [a]\nstate: captured\n---\n\n# Title\r\nbody without newline"
	doc, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	out, err := doc.Set("tags", []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(source, "tags: [a]\n", "tags: [a, b]\n", 1)
	if out != want {
		t.Fatalf("got\n%q\nwant\n%q", out, want)
	}
	out, _ = Edit(source, []Change{{Key: "state", Value: "organized"}, {Key: "new", Value: Quoted("2026-01-01")}})
	if !strings.Contains(out, "state: organized\nnew: \"2026-01-01\"\n---\n") || !strings.HasPrefix(out, source[:strings.Index(source, "state:")]) {
		t.Fatalf("edit changed unrelated bytes: %q", out)
	}
}

func TestRejectedFrontmatterAndBodyDiagnostics(t *testing.T) {
	frontmatter := map[string]string{
		"anchor":    "---\na: &x 1\nb: *x\n---\n",
		"tag":       "---\na: !custom 1\n---\n",
		"duplicate": "---\na: 1\na: 2\n---\n",
		"unclosed":  "---\na: 1\n",
		"no fm":     "# Title\n",
	}
	for name, source := range frontmatter {
		if _, err := Parse(source); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	bodies := map[string]string{
		"unterminated": "<!-- fumori:memo {\"id\":\"m_1\"} -->\ntext\n",
		"mismatch":     "<!-- fumori:memo {\"id\":\"m_1\"} -->\n<!-- /fumori:memo m_2 -->\n",
		"nested":       "<!-- fumori:memo {\"id\":\"m_1\"} -->\n<!-- fumori:memo {\"id\":\"m_2\"} -->\n",
		"bad json":     "<!-- fumori:memo {id} -->\n",
		"unknown kind": "<!-- fumori:poll {\"id\":\"p_1\"} -->\n",
		"duplicate":    "<!-- fumori:memo {\"id\":\"m_1\"} -->\na\n<!-- /fumori:memo m_1 -->\n<!-- fumori:memo {\"id\":\"m_1\"} -->\nb\n<!-- /fumori:memo m_1 -->\n",
		"stray end":    "<!-- /fumori:memo m_1 -->\n",
	}
	for name, body := range bodies {
		if _, diags := Blocks(body); len(diags) == 0 {
			t.Errorf("%s: expected a diagnostic", name)
		}
	}
}

func TestMarkersInsideCodeAreText(t *testing.T) {
	body := "```\n<!-- fumori:memo {\"id\":\"m_1\"} -->\n- [ ] x <!-- fumori:task {\"id\":\"t_1\"} -->\n```\n"
	blocks, diags := Blocks(body)
	tasks, taskDiags := InlineTasks(body)
	if len(blocks)+len(diags)+len(tasks)+len(taskDiags) != 0 {
		t.Fatal("markers inside a code fence must be ignored")
	}
}

func TestRenderedBlocksRoundTrip(t *testing.T) {
	body := AppendBlock("# 2026-10-27\n", RenderBlock(KindMemo, []KV{{"id", "m_1"}, {"note", "a --> b"}}, "hello"))
	blocks, diags := Blocks(body)
	if len(diags) != 0 || len(blocks) != 1 || blocks[0].MetaString("note") != "a --> b" || blocks[0].Content(body) != "hello" {
		t.Fatalf("round trip failed: %q %v", body, diags)
	}
	free, stream := SplitFreeform(body, blocks)
	if JoinFreeform(free, stream) != body {
		t.Fatalf("free-form split does not rejoin: %q", JoinFreeform(free, stream))
	}
}

func TestWikilinksSkipCode(t *testing.T) {
	body := "See [[A]] and [[B|bee]] `[[C]]`\n```\n[[D]]\n```\n"
	links := Wikilinks(body)
	if len(links) != 2 || links[1].Label != "bee" {
		t.Fatalf("links: %+v", links)
	}
	out := RewriteWikilinks(body, func(target string) bool { return target == "B" }, "Bee")
	if out != strings.Replace(body, "[[B|bee]]", "[[Bee|bee]]", 1) {
		t.Fatalf("rewrite: %q", out)
	}
}

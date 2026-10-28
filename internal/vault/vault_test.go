package vault

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getfumori/fumori/internal/format"
)

var testNow = time.Date(2026, 10, 27, 23, 30, 0, 0, time.FixedZone("PDT", -7*3600))

// newVault bootstraps a real Git repository and opens it with a fixed clock.
func newVault(t *testing.T) *Vault {
	t.Helper()
	root := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	if _, err := Bootstrap(context.Background(), root, "Test", "America/Los_Angeles", testNow); err != nil {
		t.Fatal(err)
	}
	return openVault(t, root, t.TempDir())
}

func openVault(t *testing.T, root, state string) *Vault {
	t.Helper()
	v, err := Open(context.Background(), Options{Root: root, StateDir: state, Clock: func() time.Time { return testNow }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { v.Close() })
	return v
}

func gitLog(t *testing.T, root string) []string {
	out, err := exec.Command("git", "-C", root, "log", "--format=%s").Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n")
}

func TestBootstrapCheckAndHooks(t *testing.T) {
	root := t.TempDir()
	exec.Command("git", "init", "-q", root).Run()
	hook := filepath.Join(root, ".git", "hooks", "post-commit")
	os.WriteFile(hook, []byte("#!/bin/sh\ntouch "+filepath.Join(root, "HOOK_RAN")+"\n"), 0o755)
	if _, err := Bootstrap(context.Background(), root, "Test", "America/Los_Angeles", testNow); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "HOOK_RAN")); err == nil {
		t.Fatal("a repository hook ran")
	}
	if log := gitLog(t, root); len(log) != 1 {
		t.Fatalf("bootstrap should make one commit, got %v", log)
	}
	if diags, err := Check(root); err != nil || len(diags) != 0 {
		t.Fatalf("fresh vault should check clean: %v %v", diags, err)
	}
	if _, err := Bootstrap(context.Background(), root, "Again", "America/Los_Angeles", testNow); err == nil {
		t.Fatal("bootstrap must refuse a repository with commits")
	}

	os.WriteFile(filepath.Join(root, "human/notes/broken.md"), []byte("---\na: [\n---\n"), 0o644)
	if diags, _ := Check(root); len(diags) != 1 || diags[0].Path != "human/notes/broken.md" {
		t.Fatalf("broken file should be quarantined: %+v", diags)
	}
}

func mustNote(t *testing.T, v *Vault, title, body string) *Object {
	t.Helper()
	o, err := v.CreateNote("note", title)
	if err != nil {
		t.Fatal(err)
	}
	o, err = v.SaveNote(o.ID, NoteSave{Format: "rich", BaseRevision: o.Revision, BodyMarkdown: body})
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func TestNoteEditing(t *testing.T) {
	v := newVault(t)
	o, _ := v.CreateNote("note", "")
	if !strings.HasSuffix(o.Path, "note-"+o.ID+".md") {
		t.Fatalf("untitled note should use a temporary filename: %s", o.Path)
	}
	o, err := v.SaveNote(o.ID, NoteSave{Format: "rich", BaseRevision: o.Revision, BodyMarkdown: "# 服务器 Setup\n\nbody"})
	if err != nil || o.Path != "human/notes/服务器-setup.md" {
		t.Fatalf("first H1 should rename once: %v %v", o, err)
	}
	if _, err := v.SaveNote(o.ID, NoteSave{Format: "rich", BaseRevision: "stale", BodyMarkdown: "x"}); err == nil {
		t.Fatal("stale save must fail")
	}

	raw := strings.Replace(o.Source, "aliases: []\n", "aliases: []\ncustom_field: {keep: [1, 2]}  # mine\n", 1)
	o, err = v.SaveNote(o.ID, NoteSave{Format: "raw", BaseRevision: o.Revision, Source: raw})
	if err != nil {
		t.Fatal(err)
	}
	typ := "note"
	o, err = v.SaveNote(o.ID, NoteSave{Format: "metadata", BaseRevision: o.Revision, Type: &typ, State: "organized", Tags: []string{"homelab"}, Aliases: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(o.Source, "custom_field: {keep: [1, 2]}  # mine\n") || !strings.Contains(o.Source, "tags: [homelab]\n") {
		t.Fatalf("metadata edit must keep unknown fields byte for byte:\n%s", o.Source)
	}

	linker := mustNote(t, v, "Linker", "# Linker\n\nSee [[服务器 Setup]].")
	if _, n, _ := v.DeletionImpact(o.ID); n != 1 {
		t.Fatalf("expected one incoming link, got %d", n)
	}
	o, err = v.SaveNote(o.ID, NoteSave{Format: "rich", BaseRevision: o.Revision, BodyMarkdown: "# Server Setup\n\nbody"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.RenameToTitle(context.Background(), o.ID, o.Revision); err != nil {
		t.Fatal(err)
	}
	linker, _ = v.Note(linker.ID)
	if !strings.Contains(linker.Source, "[[Server Setup]]") {
		t.Fatalf("rename must rewrite links: %s", linker.Source)
	}
	o, _ = v.Note(o.ID)
	if o.Path != "human/notes/server-setup.md" {
		t.Fatalf("renamed path: %s", o.Path)
	}
}

func TestDailyCaptureAndComments(t *testing.T) {
	v := newVault(t)
	if v.Today() != "2026-10-27" {
		t.Fatalf("23:30 owner time must be 2026-10-27, got %s", v.Today())
	}
	if _, err := v.SaveDaily("2026-10-20", NoteSave{Format: "rich", BodyMarkdown: "x"}); err != ErrExplicitCreation {
		t.Fatalf("past date needs explicit creation: %v", err)
	}
	if _, err := v.Capture("", "旧手机当摄像头", false, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Capture("", "测试延迟", true, ""); err != nil {
		t.Fatal(err)
	}
	view, _ := v.Daily("2026-10-27")
	memo := view.Object.Blocks[0].ID
	if _, err := v.Comment("", memo, "USB 供电即可"); err != nil {
		t.Fatal(err)
	}
	view, _ = v.Daily("2026-10-27")
	o := view.Object
	if len(o.Blocks) != 3 || o.Blocks[1].Kind != "comment" || o.Blocks[1].MetaString("parent") != memo {
		t.Fatalf("comment must follow its memo: %+v", o.Blocks)
	}
	if len(o.InlineTasks) != 1 || len(v.Tasks("unlisted", "")) != 1 {
		t.Fatal("task capture must create one Unlisted Inline Task")
	}
	saved, err := v.SaveDaily("2026-10-27", NoteSave{Format: "rich", BaseRevision: o.Revision, BodyMarkdown: "睡得不好。"})
	if err != nil {
		t.Fatal(err)
	}
	free, _ := DailyParts(saved)
	if free != "睡得不好。" || len(saved.Blocks) != 3 {
		t.Fatalf("rich save must replace free text and keep the memo stream: %q %d", free, len(saved.Blocks))
	}
}

func TestRecurringTaskCommands(t *testing.T) {
	v := newVault(t)
	o := mustNote(t, v, "Server check", "# Server check\n\n## Procedure\n\n- check backups")
	first := "2026-11-01"
	weekly := &format.Recurrence{Basis: format.BasisSchedule, Frequency: format.Weekly, Interval: 1, Weekdays: []string{"sun"}}
	if _, err := v.TaskAction(TaskCommand{TaskID: o.ID, Action: "make", Fingerprint: "m", Scheduled: &first, Recurrence: weekly}); err != nil {
		t.Fatal(err)
	}
	o, _ = v.Note(o.ID)
	occurrence := o.Task.Occurrence.ID

	testNow = time.Date(2026, 11, 2, 9, 30, 0, 0, time.FixedZone("PST", -8*3600))
	defer func() { testNow = time.Date(2026, 10, 27, 23, 30, 0, 0, time.FixedZone("PDT", -7*3600)) }()
	complete := TaskCommand{TaskID: o.ID, Action: "complete", ExpectedOccurrence: occurrence, IdempotencyKey: "k1", Fingerprint: "f1", Notes: "周一补做"}
	firstResponse, err := v.TaskAction(complete)
	if err != nil {
		t.Fatal(err)
	}
	again, err := v.TaskAction(complete)
	if err != nil || string(again) != string(firstResponse) {
		t.Fatalf("same key must replay: %v", err)
	}
	complete.IdempotencyKey = "k2"
	if _, err := v.TaskAction(complete); err != nil {
		t.Fatalf("a retry for an already recorded occurrence succeeds: %v", err)
	}
	o, _ = v.Note(o.ID)
	if len(o.Blocks) != 1 || o.Task.Occurrence.Original != "2026-11-08" {
		t.Fatalf("late completion: %d records, next %s", len(o.Blocks), o.Task.Occurrence.Original)
	}

	free, _ := NoteParts(o)
	if strings.Contains(free, "History") || strings.Contains(free, "fumori:") {
		t.Fatalf("the editor body must not contain history: %q", free)
	}
	o, err = v.SaveNote(o.ID, NoteSave{Format: "rich", BaseRevision: o.Revision, BodyMarkdown: free + "\n- check disks"})
	if err != nil || len(o.Blocks) != 1 || !strings.HasSuffix(strings.TrimSpace(o.Source), "<!-- /fumori:occurrence "+occurrence+" -->") {
		t.Fatalf("a rich save must keep the history at the end: %v\n%s", err, o.Source)
	}

	if _, err := v.TaskAction(TaskCommand{TaskID: o.ID, Action: "undo", ExpectedOccurrence: o.Task.Occurrence.ID, Fingerprint: "u"}); err != nil {
		t.Fatal(err)
	}
	o, _ = v.Note(o.ID)
	if o.Task.Occurrence.ID != occurrence || strings.Contains(o.Source, "fumori:occurrence") || strings.Contains(o.Source, "## History") {
		t.Fatalf("undo must restore the previous occurrence:\n%s", o.Source)
	}
}

func TestCrashRecovery(t *testing.T) {
	errCrash := errors.New("crash")
	for _, point := range []string{"after-intent", "after-publish"} {
		root := t.TempDir()
		exec.Command("git", "init", "-q", root).Run()
		Bootstrap(context.Background(), root, "Test", "America/Los_Angeles", testNow)
		state := t.TempDir()
		v := openVault(t, root, state)
		crashHook = func(p string) error {
			if p == point {
				return errCrash
			}
			return nil
		}
		if _, err := v.Capture("key", "hello", false, ""); err != errCrash {
			t.Fatalf("%s: expected injected crash, got %v", point, err)
		}
		crashHook = nil
		v.Close()
		v = openVault(t, root, state)
		if _, err := v.Capture("key", "hello", false, ""); err != nil {
			t.Fatalf("%s: retry: %v", point, err)
		}
		view, _ := v.Daily(v.Today())
		if len(view.Object.Blocks) != 1 {
			t.Fatalf("%s: retry must capture exactly once, got %d memos", point, len(view.Object.Blocks))
		}
		if log := gitLog(t, root); !strings.HasPrefix(log[0], "recovery checkpoint") && point == "after-publish" {
			t.Fatalf("%s: saved content must be sealed by a recovery checkpoint: %v", point, log)
		}
	}

	v := newVault(t)
	o := mustNote(t, v, "Old", "# New Title\n\nx")
	crashHook = func(p string) error {
		if p == "after-journal" {
			return errCrash
		}
		return nil
	}
	if _, err := v.RenameToTitle(context.Background(), o.ID, o.Revision); err != errCrash {
		t.Fatalf("expected crash during rename, got %v", err)
	}
	crashHook = nil
	root, state := v.Root, v.StateDir
	v.Close()
	v = openVault(t, root, state)
	if o, _ = v.Note(o.ID); o.Path != "human/notes/new-title.md" {
		t.Fatalf("rename must roll forward after the journal commit: %s", o.Path)
	}
}

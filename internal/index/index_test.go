package index

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/getfumori/fumori/internal/vault"
)

func openTestVault(t *testing.T) (*vault.Vault, string, string) {
	t.Helper()
	root, state := t.TempDir(), t.TempDir()
	exec.Command("git", "init", "-q", root).Run()
	now := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	if _, err := vault.Bootstrap(context.Background(), root, "Test", "America/Los_Angeles", now); err != nil {
		t.Fatal(err)
	}
	v, err := vault.Open(context.Background(), vault.Options{Root: root, StateDir: state, OpenIndex: Open, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { v.Close() })
	return v, root, state
}

func addNote(t *testing.T, v *vault.Vault, title, body string) {
	t.Helper()
	o, err := v.CreateNote("note", title)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.SaveNote(o.ID, vault.NoteSave{Format: "rich", BaseRevision: o.Revision, BodyMarkdown: "# " + title + "\n\n" + body}); err != nil {
		t.Fatal(err)
	}
}

var corpus = map[string]string{
	"服务器备份策略":      "每周检查服务器备份，确认 restic 快照完整。",
	"服务器的日常维护":     "清理磁盘，然后做服务器的备份检查。",
	"手机照片备份":       "把手机里的照片备份到 NAS。",
	"機器人視覺":        "用舊手機做機器人的攝像頭，測試延遲。",
	"机器学习笔记":       "机器 学习 与 器人 无关",
	"ESP32-S3 小车":  "ESP32-S3 驱动摄像头，摄像头延迟大约 120ms。",
	"ESP32 传感器":    "ESP32 读取温湿度传感器。",
	"ESP32S30 假型号": "这是一个不存在的 ESP32S30 型号。",
	"C32 电容":       "主板上的 C32 电容需要更换。",
	"C320 电阻":      "C320 是另一个元件。",
	"GNOME 远程登录":   "在 Fedora 上配置 GNOME 远程登录。",
	"网络错误":         "日志里出现 E_CONNRESET，然后重试。",
	"代码":           "```\nfunc backup() {}\n```\n读书笔记。",
}

func TestChineseSearch(t *testing.T) {
	v, _, _ := openTestVault(t)
	for title, body := range corpus {
		addNote(t, v, title, body)
	}
	cases := map[string]struct {
		want    []string
		exclude []string
	}{
		"备份":          {want: []string{"服务器备份策略", "服务器的日常维护", "手机照片备份"}},
		"服务器备份":       {want: []string{"服务器备份策略", "服务器的日常维护"}, exclude: []string{"手机照片备份"}},
		"机器人":         {want: []string{"機器人視覺"}, exclude: []string{"机器学习笔记"}},
		"ESP32":       {want: []string{"ESP32-S3 小车", "ESP32 传感器"}, exclude: []string{"ESP32S30 假型号"}},
		"ESP32-S3":    {want: []string{"ESP32-S3 小车"}, exclude: []string{"ESP32 传感器"}},
		"ＥＳＰ３２－Ｓ３":    {want: []string{"ESP32-S3 小车"}},
		"C32":         {want: []string{"C32 电容"}, exclude: []string{"C320 电阻"}},
		"GNOME 远程登录":  {want: []string{"GNOME 远程登录"}},
		"摄像头延迟":       {want: []string{"ESP32-S3 小车", "機器人視覺"}},
		"備份":          {want: []string{"服务器备份策略", "手机照片备份"}},
		"E_CONNRESET": {want: []string{"网络错误"}},
	}
	for query, c := range cases {
		results, err := v.Search(query, vault.SearchFilter{})
		if err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		got := map[string]bool{}
		for _, r := range results {
			got[r.Object.Title] = true
			if r.Snippet == "" || strings.Contains(r.Snippet, "calepin:") {
				t.Errorf("%s: bad snippet %q", query, r.Snippet)
			}
		}
		for _, w := range c.want {
			if !got[w] {
				t.Errorf("%s: missing %s (got %v)", query, w, got)
			}
		}
		for _, x := range c.exclude {
			if got[x] {
				t.Errorf("%s: should not match %s", query, x)
			}
		}
	}
	if _, err := v.Search("备", vault.SearchFilter{}); err != ErrQueryTooShort {
		t.Fatalf("single character query: %v", err)
	}
}

func TestIndexRebuildsFromVault(t *testing.T) {
	v, root, state := openTestVault(t)
	addNote(t, v, "服务器备份策略", "restic 快照")
	before, _ := v.Search("备份", vault.SearchFilter{})
	v.Close()
	if err := exec.Command("rm", "-rf", state+"/index").Run(); err != nil {
		t.Fatal(err)
	}
	reopened, err := vault.Open(context.Background(), vault.Options{Root: root, StateDir: state, OpenIndex: Open})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	after, _ := reopened.Search("备份", vault.SearchFilter{})
	if len(before) != 1 || len(after) != 1 || before[0].Object.Path != after[0].Object.Path {
		t.Fatalf("rebuild changed results: %v vs %v", before, after)
	}
}

func TestSearchFilters(t *testing.T) {
	v, _, _ := openTestVault(t)
	addNote(t, v, "服务器备份策略", "restic 快照")
	list, err := v.CreateNote("task-list", "家庭服务器")
	if err != nil {
		t.Fatal(err)
	}
	list, _ = v.SaveNote(list.ID, vault.NoteSave{Format: "rich", BaseRevision: list.Revision, BodyMarkdown: "# 家庭服务器\n\n备份相关事项"})
	if _, err := v.Capture("", "检查备份", true, "[[家庭服务器]]"); err != nil {
		t.Fatal(err)
	}
	titles := func(f vault.SearchFilter) []string {
		results, err := v.Search("备份", f)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, r := range results {
			out = append(out, r.Object.Title)
		}
		return out
	}
	if got := titles(vault.SearchFilter{}); len(got) != 3 {
		t.Fatalf("unfiltered: %v", got)
	}
	if got := titles(vault.SearchFilter{Type: "task-list"}); len(got) != 1 || got[0] != "家庭服务器" {
		t.Fatalf("type filter: %v", got)
	}
	if got := titles(vault.SearchFilter{List: "家庭服务器"}); len(got) != 1 || got[0] != "2026-10-27" {
		t.Fatalf("list filter: %v", got)
	}
	if got := titles(vault.SearchFilter{From: "2026-10-28"}); len(got) != 0 {
		t.Fatalf("date filter: %v", got)
	}
}

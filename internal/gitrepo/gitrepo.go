// Package gitrepo runs the system Git CLI against a Vault repository without
// a shell and without letting repository configuration run code.
package gitrepo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Repo struct{ Root string }

// safety disables hooks, fsmonitor, and pagers and fixes the commit identity.
var safety = []string{
	"-c", "core.hooksPath=/dev/null",
	"-c", "core.fsmonitor=false",
	"-c", "core.pager=cat",
	"-c", "commit.gpgSign=false",
	"-c", "user.name=Calepin",
	"-c", "user.email=calepin@localhost",
}

func (r Repo) Run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append(append([]string{}, safety...), args...)...)
	cmd.Dir = r.Root
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + r.Root,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
		"LC_ALL=C",
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// IsEmptyRepository reports whether root is a Git work tree with no commits
// and no files besides .git.
func (r Repo) IsEmptyRepository(ctx context.Context) error {
	if _, err := r.Run(ctx, "rev-parse", "--is-inside-work-tree"); err != nil {
		return errors.New("not a Git repository")
	}
	if _, err := r.Run(ctx, "rev-parse", "--verify", "HEAD"); err == nil {
		return errors.New("repository already has commits")
	}
	entries, err := os.ReadDir(r.Root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() != ".git" {
			return errors.New("repository work tree is not empty")
		}
	}
	return nil
}

// ClearStaleIndexLock removes .git/index.lock left by a crashed Git child.
// The caller guarantees no Git process of this service is running.
func (r Repo) ClearStaleIndexLock() error {
	err := os.Remove(filepath.Join(r.Root, ".git", "index.lock"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Dirty reports whether the work tree has uncommitted changes.
func (r Repo) Dirty(ctx context.Context) (int, error) {
	out, err := r.Run(ctx, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return 0, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return 0, nil
	}
	return len(strings.Split(out, "\n")), nil
}

// Commit stages everything and commits. It returns "" when the tree is clean.
func (r Repo) Commit(ctx context.Context, message string) (string, int, error) {
	changed, err := r.Dirty(ctx)
	if err != nil || changed == 0 {
		return "", 0, err
	}
	if _, err := r.Run(ctx, "add", "--all"); err != nil {
		return "", 0, err
	}
	if _, err := r.Run(ctx, "commit", "--quiet", "--no-verify", "-m", message); err != nil {
		return "", 0, err
	}
	sha, err := r.Run(ctx, "rev-parse", "HEAD")
	return strings.TrimSpace(sha), changed, err
}

package vault

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/getfumori/fumori/internal/gitrepo"
	"github.com/getfumori/fumori/internal/model"
)

var reservedDirs = []string{"human/notes", "human/daily", "sources/records", "knowledge", "assets"}

// Bootstrap turns an empty Git repository into a Blank Vault and commits it.
func Bootstrap(ctx context.Context, root, name, timezone string, now time.Time) (string, error) {
	repo := gitrepo.Repo{Root: root}
	if err := repo.IsEmptyRepository(ctx); err != nil {
		return "", err
	}
	location, err := time.LoadLocation(timezone)
	if err != nil || timezone == "" || timezone == "Local" {
		return "", fmt.Errorf("timezone must be an IANA time zone such as America/Los_Angeles")
	}
	v := &Vault{random: rand.Reader}
	id := v.newUUID()
	files := model.CoreFiles(id, name, timezone, now.In(location).Format(time.RFC3339))
	for _, dir := range reservedDirs {
		files[dir+"/.gitkeep"] = ""
	}
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err
		}
		if err := writeDurable(path, content); err != nil {
			return "", err
		}
	}
	if _, _, err := repo.Commit(ctx, "bootstrap: create vault"); err != nil {
		return "", err
	}
	return id, nil
}

// Check validates a Vault without opening it for service.
func Check(root string) ([]Quarantined, error) {
	if _, err := model.LoadManifest(root); err != nil {
		return nil, err
	}
	m, err := model.Load(root)
	if err != nil {
		return nil, err
	}
	v := &Vault{Root: root, Model: m}
	if err := v.scan(); err != nil {
		return nil, err
	}
	return v.Diagnostics(), nil
}

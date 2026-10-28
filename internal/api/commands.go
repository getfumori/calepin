package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/getfumori/fumori/internal/format"
	"github.com/getfumori/fumori/internal/vault"
)

// readCommand reads a command body and fingerprints it so a reused
// Idempotency-Key with a different request is rejected.
func readCommand(r *http.Request, v any) (key, fingerprint string, err error) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return "", "", err
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, v); err != nil {
			return "", "", badRequest{"request body is not valid JSON"}
		}
	}
	sum := sha256.Sum256(append([]byte(r.Method+" "+r.URL.Path+"\n"), raw...))
	return r.Header.Get("Idempotency-Key"), hex.EncodeToString(sum[:]), nil
}

func rawJSON(w http.ResponseWriter, status int, body []byte) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(body)
	return nil
}

func (s *server) capture(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Text string `json:"text"`
		Task bool   `json:"task"`
		List string `json:"list"`
	}
	key, _, err := readCommand(r, &req)
	if err != nil {
		return err
	}
	body, err := s.v.Capture(key, req.Text, req.Task, req.List)
	if err != nil {
		return err
	}
	return rawJSON(w, http.StatusCreated, body)
}

func (s *server) comment(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Text string `json:"text"`
	}
	key, _, err := readCommand(r, &req)
	if err != nil {
		return err
	}
	body, err := s.v.Comment(key, r.PathValue("id"), req.Text)
	if err != nil {
		return err
	}
	return rawJSON(w, http.StatusCreated, body)
}

func (s *server) tasks(w http.ResponseWriter, r *http.Request) error {
	tasks := s.v.Tasks(r.URL.Query().Get("list"), r.URL.Query().Get("status"))
	if tasks == nil {
		tasks = []vault.TaskView{}
	}
	return ok(w, tasks)
}

func (s *server) taskAction(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		ExpectedOccurrence string             `json:"expectedOccurrence"`
		BaseRevision       string             `json:"baseRevision"`
		Notes              string             `json:"notes"`
		Scheduled          *string            `json:"scheduled"`
		Due                *string            `json:"due"`
		List               *string            `json:"list"`
		Recurrence         *format.Recurrence `json:"recurrence"`
	}
	key, fingerprint, err := readCommand(r, &req)
	if err != nil {
		return err
	}
	if base := strings.Trim(r.Header.Get("If-Match"), `"`); base != "" {
		req.BaseRevision = base
	}
	body, err := s.v.TaskAction(vault.TaskCommand{
		TaskID: r.PathValue("id"), Action: r.PathValue("action"), ExpectedOccurrence: req.ExpectedOccurrence,
		BaseRevision: req.BaseRevision, IdempotencyKey: key, Fingerprint: fingerprint, Notes: req.Notes,
		Scheduled: req.Scheduled, Due: req.Due, List: req.List, Recurrence: req.Recurrence,
	})
	if err != nil {
		return err
	}
	return rawJSON(w, http.StatusOK, body)
}

// webHandler serves built assets and falls back to index.html so client
// routes such as /notes/{id} load the app.
func webHandler(web fs.FS) http.Handler {
	files := http.FileServerFS(web)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/today", http.StatusFound)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			if _, err := fs.Stat(web, strings.TrimPrefix(path.Clean(r.URL.Path), "/")); err == nil {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				files.ServeHTTP(w, r)
				return
			}
			http.NotFound(w, r)
			return
		}
		index, err := fs.ReadFile(web, "index.html")
		if err != nil {
			http.Error(w, "the Web client is not built into this binary", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(index)
	})
}

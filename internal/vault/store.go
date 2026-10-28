package vault

import (
	"database/sql"
	"errors"
	"time"

	_ "modernc.org/sqlite"
)

// store is runtime state outside the Vault repository. Losing it loses only
// in-flight deduplication; it never holds knowledge.
type store struct{ db *sql.DB }

const intentRetention = 7 * 24 * time.Hour

func openStore(path string) (*store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS intents (
		key TEXT PRIMARY KEY,
		fingerprint TEXT NOT NULL,
		path TEXT NOT NULL,
		base TEXT NOT NULL,
		target TEXT NOT NULL,
		response BLOB NOT NULL,
		status TEXT NOT NULL CHECK (status IN ('pending', 'completed')),
		created INTEGER NOT NULL
	)`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return &store{db: db}, nil
}

func (s *store) close() error { return s.db.Close() }

type intent struct {
	fingerprint string
	response    []byte
	status      string
}

var errNoIntent = errors.New("no intent")

func (s *store) lookup(key string) (intent, error) {
	var in intent
	err := s.db.QueryRow(`SELECT fingerprint, response, status FROM intents WHERE key = ?`, key).
		Scan(&in.fingerprint, &in.response, &in.status)
	if errors.Is(err, sql.ErrNoRows) {
		return in, errNoIntent
	}
	return in, err
}

func (s *store) begin(key, fingerprint, path, base, target string, response []byte) error {
	_, err := s.db.Exec(`INSERT INTO intents (key, fingerprint, path, base, target, response, status, created)
		VALUES (?, ?, ?, ?, ?, ?, 'pending', ?)`, key, fingerprint, path, base, target, response, time.Now().Unix())
	return err
}

func (s *store) complete(key string) error {
	_, err := s.db.Exec(`UPDATE intents SET status = 'completed' WHERE key = ?`, key)
	return err
}

func (s *store) discard(key string) error {
	_, err := s.db.Exec(`DELETE FROM intents WHERE key = ?`, key)
	return err
}

// settleIntents resolves intents left pending by a crash. A file already at
// the target revision means the write happened; anything else means it did
// not, so the intent is discarded and a retry runs normally.
func (s *store) settleIntents(current func(path string) string) error {
	if _, err := s.db.Exec(`DELETE FROM intents WHERE created < ?`, time.Now().Add(-intentRetention).Unix()); err != nil {
		return err
	}
	rows, err := s.db.Query(`SELECT key, path, target FROM intents WHERE status = 'pending'`)
	if err != nil {
		return err
	}
	type pending struct{ key, path, target string }
	var all []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.key, &p.path, &p.target); err != nil {
			rows.Close()
			return err
		}
		all = append(all, p)
	}
	rows.Close()
	for _, p := range all {
		if current(p.path) == p.target {
			err = s.complete(p.key)
		} else {
			err = s.discard(p.key)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

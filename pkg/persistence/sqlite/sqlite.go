//

//

package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/session"
)

// ============================================================================
// DDL
// ============================================================================

const (
	ddlEvents = `
		CREATE TABLE IF NOT EXISTS session_events (
			session_id   TEXT NOT NULL,
			seq          INTEGER NOT NULL,
			event_type   TEXT NOT NULL,
			ts           DATETIME NOT NULL,
			data         BLOB NOT NULL,
			text_content TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (session_id, seq)
		);
	`
	ddlFTS = `
		CREATE VIRTUAL TABLE IF NOT EXISTS session_events_fts
			USING fts5(session_id UNINDEXED, seq UNINDEXED, text_content,
				tokenize = 'trigram');
	`
)

// ============================================================================
// Store
// ============================================================================

type Store struct {
	db *sql.DB

	fts bool
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec(ddlEvents); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("persistence/sqlite: init events table: %w", err)
	}
	st := &Store{db: db}

	if _, err := db.Exec(ddlFTS); err == nil {
		st.fts = true
	} else {

		st.fts = false
	}
	return st, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) FTSEnabled() bool { return s.fts }

func (s *Store) FTSUnavailable() bool { return !s.fts }

// ============================================================================

// ============================================================================

func extractText(ev session.SessionEvent) string {
	switch d := ev.Data.(type) {
	case session.UserMessageData:
		return d.Content
	case session.AssistantMessageData:
		return d.Content
	case session.SessionTitleData:
		return d.Title
	default:
		return ""
	}
}

// ============================================================================

// ============================================================================

func (s *Store) Append(ctx context.Context, sessionID brand.SessionID, ev session.SessionEvent) error {
	return s.AppendBatch(ctx, sessionID, []session.SessionEvent{ev})
}

func (s *Store) AppendBatch(ctx context.Context, sessionID brand.SessionID, events []session.SessionEvent) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR REPLACE INTO session_events(session_id, seq, event_type, ts, data, text_content)
		VALUES(?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	var ftsStmt *sql.Stmt
	if s.fts {
		ftsStmt, err = tx.PrepareContext(ctx, `
			INSERT INTO session_events_fts(session_id, seq, text_content)
			VALUES(?, ?, ?)`)
		if err != nil {
			return err
		}
	}

	for _, ev := range events {
		raw, err := ev.MarshalJSON()
		if err != nil {
			return err
		}
		txt := extractText(ev)
		if _, err := stmt.ExecContext(ctx, sessionID.Raw(), ev.Seq, string(ev.Type), ev.Time, raw, txt); err != nil {
			return err
		}
		if s.fts {
			if _, err := ftsStmt.ExecContext(ctx, sessionID.Raw(), ev.Seq, txt); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// ============================================================================

// ============================================================================

func (s *Store) Load(ctx context.Context, sessionID brand.SessionID) ([]session.SessionEvent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT data FROM session_events WHERE session_id = ? ORDER BY seq`, sessionID.Raw())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []session.SessionEvent
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var ev session.SessionEvent
		if err := ev.UnmarshalJSON(raw); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

func (s *Store) Count(ctx context.Context, sessionID brand.SessionID) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM session_events WHERE session_id = ?`, sessionID.Raw()).Scan(&n)
	return n, err
}

func (s *Store) ListSessions(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT session_id FROM session_events ORDER BY session_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Store) Delete(ctx context.Context, sessionID brand.SessionID) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM session_events WHERE session_id = ?`, sessionID.Raw()); err != nil {
		return err
	}
	if s.fts {
		if _, err := tx.ExecContext(ctx, `DELETE FROM session_events_fts WHERE session_id = ?`, sessionID.Raw()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ============================================================================

// ============================================================================

type Match struct {
	SessionID string `json:"sessionId"`
	Seq       uint64 `json:"seq"`
	Text      string `json:"text,omitempty"`
}

func (s *Store) Search(ctx context.Context, query string) ([]Match, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if !s.fts || len([]rune(query)) < 3 {
		return s.searchLike(ctx, query)
	}
	return s.searchFTS(ctx, query)
}

func (s *Store) searchFTS(ctx context.Context, query string) ([]Match, error) {

	term := `"` + strings.ReplaceAll(query, `"`, `""`) + `"`
	rows, err := s.db.QueryContext(ctx, `
		SELECT f.session_id, f.seq, f.text_content
		FROM session_events_fts f
		WHERE session_events_fts MATCH ?
		ORDER BY f.session_id, f.seq`, term)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Match
	for rows.Next() {
		var m Match
		if err := rows.Scan(&m.SessionID, &m.Seq, &m.Text); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) searchLike(ctx context.Context, query string) ([]Match, error) {
	pattern := "%" + query + "%"
	rows, err := s.db.QueryContext(ctx, `
		SELECT session_id, seq, text_content FROM session_events
		WHERE text_content LIKE ?
		ORDER BY session_id, seq`, pattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Match
	for rows.Next() {
		var m Match
		if err := rows.Scan(&m.SessionID, &m.Seq, &m.Text); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

package threads

import (
	"database/sql"
	"encoding/json"
	"fmt"

	_ "modernc.org/sqlite"
)

// Store is a SQLite-backed dataset for the db command. It is pure-Go
// (modernc.org/sqlite), so the binary stays cgo-free.
type Store struct {
	db *sql.DB
}

// OpenStore opens (creating if needed) a SQLite dataset at path and ensures the
// posts table exists.
func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, codeErr(ExitGeneric, "open db: %v", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the underlying database.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS posts (
	id          TEXT PRIMARY KEY,
	shortcode   TEXT,
	username    TEXT,
	user_id     TEXT,
	text        TEXT,
	media_type  TEXT,
	media_urls  TEXT,
	permalink   TEXT,
	timestamp   TEXT,
	like_count  INTEGER,
	reply_count INTEGER,
	repost_count INTEGER,
	quote_count INTEGER,
	is_reply    INTEGER,
	fetched_at  TEXT
);`
	if _, err := s.db.Exec(schema); err != nil {
		return codeErr(ExitGeneric, "migrate: %v", err)
	}
	return nil
}

// PutPost upserts one post row.
func (s *Store) PutPost(p Post) error {
	media, _ := json.Marshal(p.MediaURLs)
	const q = `
INSERT INTO posts (id, shortcode, username, user_id, text, media_type, media_urls,
	permalink, timestamp, like_count, reply_count, repost_count, quote_count, is_reply, fetched_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
	like_count=excluded.like_count, reply_count=excluded.reply_count,
	repost_count=excluded.repost_count, quote_count=excluded.quote_count,
	fetched_at=excluded.fetched_at;`
	_, err := s.db.Exec(q, p.ID, p.Shortcode, p.Username, p.UserID, p.Text, p.MediaType,
		string(media), p.Permalink, p.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
		p.LikeCount, p.ReplyCount, p.RepostCount, p.QuoteCount, boolToInt(p.IsReply),
		p.FetchedAt.Format("2006-01-02T15:04:05Z07:00"))
	if err != nil {
		return codeErr(ExitGeneric, "put post: %v", err)
	}
	return nil
}

// Query runs an arbitrary SQL query and returns column names plus rows of
// string-rendered values.
func (s *Store) Query(query string) ([]string, [][]string, error) {
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, nil, codeErr(ExitUsage, "query: %v", err)
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		return nil, nil, codeErr(ExitGeneric, "columns: %v", err)
	}
	var out [][]string
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, nil, codeErr(ExitGeneric, "scan: %v", err)
		}
		rec := make([]string, len(cols))
		for i, v := range vals {
			rec[i] = renderCell(v)
		}
		out = append(out, rec)
	}
	return cols, out, rows.Err()
}

func renderCell(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case []byte:
		return string(x)
	default:
		return fmt.Sprintf("%v", x)
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

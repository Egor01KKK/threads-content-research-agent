package research

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Store persists topic research in SQLite. It uses the same pure-Go SQLite
// driver as threads.Store but keeps research relationships in namespaced,
// additive tables so existing th db datasets remain usable.
type Store struct {
	db *sql.DB
}

// OpenStore opens a research database and applies additive migrations.
func OpenStore(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("research database path is required")
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open research database: %w", err)
	}
	s := &Store{db: db}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable research foreign keys: %w", err)
	}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the underlying SQLite connection.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS research_runs (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    topic        TEXT NOT NULL,
    started_at   TEXT NOT NULL,
    completed_at TEXT,
    query_count  INTEGER NOT NULL DEFAULT 0,
    status       TEXT NOT NULL DEFAULT 'completed',
    error_message TEXT
);

CREATE TABLE IF NOT EXISTS research_queries (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id         INTEGER NOT NULL REFERENCES research_runs(id) ON DELETE CASCADE,
    query          TEXT NOT NULL,
    position       INTEGER NOT NULL,
    searched_at    TEXT NOT NULL,
    result_count   INTEGER NOT NULL DEFAULT 0,
    unique_new_posts INTEGER NOT NULL DEFAULT 0,
    relevant_count INTEGER NOT NULL DEFAULT 0,
    adjacent_count INTEGER NOT NULL DEFAULT 0,
    irrelevant_count INTEGER NOT NULL DEFAULT 0,
    uncertain_count INTEGER NOT NULL DEFAULT 0,
    precision REAL NOT NULL DEFAULT 0,
    contributes_to_conclusions INTEGER NOT NULL DEFAULT 0,
    error_message  TEXT,
    UNIQUE(run_id, query)
);

CREATE TABLE IF NOT EXISTS research_posts (
    id              TEXT PRIMARY KEY,
    shortcode       TEXT,
    url             TEXT,
    text            TEXT,
    author_username TEXT,
    published_at    TEXT,
    like_count      INTEGER,
    reply_count     INTEGER,
    repost_count    INTEGER,
    quote_count     INTEGER,
    view_count      INTEGER,
    detected_topic  TEXT,
    first_seen_at   TEXT NOT NULL,
    last_seen_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS research_post_queries (
    run_id        INTEGER NOT NULL REFERENCES research_runs(id) ON DELETE CASCADE,
    query_id      INTEGER NOT NULL REFERENCES research_queries(id) ON DELETE CASCADE,
    post_id       TEXT NOT NULL REFERENCES research_posts(id) ON DELETE CASCADE,
    discovered_at TEXT NOT NULL,
    PRIMARY KEY(run_id, query_id, post_id)
);

CREATE TABLE IF NOT EXISTS research_post_relevance (
    run_id           INTEGER NOT NULL REFERENCES research_runs(id) ON DELETE CASCADE,
    post_id          TEXT NOT NULL REFERENCES research_posts(id) ON DELETE CASCADE,
    relevance_score  REAL NOT NULL,
    relevance_label  TEXT NOT NULL,
    relevance_reasons TEXT NOT NULL DEFAULT '[]',
    assessed_at      TEXT NOT NULL,
    PRIMARY KEY(run_id, post_id)
);

CREATE TABLE IF NOT EXISTS research_authors (
    username        TEXT PRIMARY KEY,
    name            TEXT,
    bio             TEXT,
    follower_count  INTEGER,
    following_count INTEGER,
    verified        INTEGER,
    profile_url     TEXT,
    updated_at      TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS research_baseline_posts (
    run_id       INTEGER NOT NULL REFERENCES research_runs(id) ON DELETE CASCADE,
    username     TEXT NOT NULL REFERENCES research_authors(username) ON DELETE CASCADE,
    post_id      TEXT NOT NULL REFERENCES research_posts(id) ON DELETE CASCADE,
    captured_at  TEXT NOT NULL,
    PRIMARY KEY(run_id, username, post_id)
);

CREATE TABLE IF NOT EXISTS research_author_snapshots (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id          INTEGER REFERENCES research_runs(id) ON DELETE CASCADE,
    username        TEXT NOT NULL REFERENCES research_authors(username) ON DELETE CASCADE,
    captured_at     TEXT NOT NULL,
    name            TEXT,
    bio             TEXT,
    follower_count  INTEGER,
    following_count INTEGER,
    verified        INTEGER,
    profile_url     TEXT
);

CREATE TABLE IF NOT EXISTS research_post_snapshots (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id       INTEGER REFERENCES research_runs(id) ON DELETE CASCADE,
    post_id      TEXT NOT NULL REFERENCES research_posts(id) ON DELETE CASCADE,
    captured_at  TEXT NOT NULL,
    like_count   INTEGER,
    reply_count  INTEGER,
    repost_count INTEGER,
    quote_count  INTEGER,
    view_count   INTEGER
);

CREATE TABLE IF NOT EXISTS research_classifications (
	 id             INTEGER PRIMARY KEY AUTOINCREMENT,
	 post_id        TEXT NOT NULL REFERENCES research_posts(id) ON DELETE CASCADE,
	 provider       TEXT NOT NULL,
	 model          TEXT,
	 schema_version TEXT NOT NULL DEFAULT 'legacy',
	 prompt_version TEXT NOT NULL DEFAULT 'legacy',
	 classified_at  TEXT NOT NULL,
	 payload_json   TEXT NOT NULL,
	 mechanics_json TEXT NOT NULL DEFAULT '{}',
	 performance_json TEXT NOT NULL DEFAULT '{}',
	 raw_response   TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS research_classification_runs (
	 run_id           INTEGER NOT NULL REFERENCES research_runs(id) ON DELETE CASCADE,
	 classification_id INTEGER NOT NULL REFERENCES research_classifications(id) ON DELETE CASCADE,
	 linked_at        TEXT NOT NULL,
	 PRIMARY KEY(run_id, classification_id)
);

CREATE TABLE IF NOT EXISTS research_deep_runs (
    run_id          INTEGER PRIMARY KEY REFERENCES research_runs(id) ON DELETE CASCADE,
    mode            TEXT NOT NULL,
    objective       TEXT NOT NULL,
    anonymous       INTEGER NOT NULL DEFAULT 1,
    stage           TEXT NOT NULL DEFAULT 'anchors',
    status          TEXT NOT NULL DEFAULT 'running',
    checkpoint_json TEXT NOT NULL DEFAULT '{}',
    updated_at      TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS research_deep_anchor_probes (
    run_id      INTEGER NOT NULL REFERENCES research_runs(id) ON DELETE CASCADE,
    query       TEXT NOT NULL,
    wave        TEXT NOT NULL,
    position    INTEGER NOT NULL,
    status      TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    PRIMARY KEY(run_id, query, wave)
);

CREATE TABLE IF NOT EXISTS research_deep_profiles (
    run_id      INTEGER NOT NULL REFERENCES research_runs(id) ON DELETE CASCADE,
    username    TEXT NOT NULL,
    status      TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    PRIMARY KEY(run_id, username)
);

CREATE TABLE IF NOT EXISTS research_deep_posts (
    run_id      INTEGER NOT NULL REFERENCES research_runs(id) ON DELETE CASCADE,
    post_id     TEXT NOT NULL REFERENCES research_posts(id) ON DELETE CASCADE,
    stage       TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    PRIMARY KEY(run_id, post_id)
);

CREATE TABLE IF NOT EXISTS research_deep_provenance (
    run_id       INTEGER NOT NULL REFERENCES research_runs(id) ON DELETE CASCADE,
    post_id      TEXT NOT NULL REFERENCES research_posts(id) ON DELETE CASCADE,
    edge_key     TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    observed_at  TEXT NOT NULL,
    PRIMARY KEY(run_id, post_id, edge_key)
);

CREATE TABLE IF NOT EXISTS research_deep_secondary_anchors (
    run_id      INTEGER NOT NULL REFERENCES research_runs(id) ON DELETE CASCADE,
    phrase      TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    PRIMARY KEY(run_id, phrase)
);

CREATE TABLE IF NOT EXISTS research_deep_replies (
    run_id      INTEGER NOT NULL REFERENCES research_runs(id) ON DELETE CASCADE,
    reply_id    TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    observed_at TEXT NOT NULL,
    PRIMARY KEY(run_id, reply_id)
);

CREATE TABLE IF NOT EXISTS research_deep_checkpoints (
    run_id      INTEGER NOT NULL REFERENCES research_runs(id) ON DELETE CASCADE,
    stage       TEXT NOT NULL,
    checkpoint_key TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    PRIMARY KEY(run_id, stage, checkpoint_key)
);

`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("migrate research database: %w", err)
	}
	// Legacy rows have no trustworthy terminal marker. Keep them visibly
	// historical/partial instead of upgrading them to an unqualified success.
	if err := s.addColumn("research_runs", "status", "TEXT NOT NULL DEFAULT 'completed_with_warnings'"); err != nil {
		return fmt.Errorf("migrate research run status: %w", err)
	}
	if err := s.addColumn("research_runs", "error_message", "TEXT"); err != nil {
		return fmt.Errorf("migrate research run error: %w", err)
	}
	if err := s.addColumn("research_author_snapshots", "run_id", "INTEGER REFERENCES research_runs(id) ON DELETE CASCADE"); err != nil {
		return fmt.Errorf("migrate research author snapshot run: %w", err)
	}
	if err := s.addColumn("research_post_snapshots", "run_id", "INTEGER REFERENCES research_runs(id) ON DELETE CASCADE"); err != nil {
		return fmt.Errorf("migrate research post snapshot run: %w", err)
	}
	if err := s.addColumn("research_queries", "relevant_count", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("migrate research query relevant count: %w", err)
	}
	if err := s.addColumn("research_queries", "adjacent_count", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("migrate research query adjacent count: %w", err)
	}
	if err := s.addColumn("research_queries", "irrelevant_count", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("migrate research query irrelevant count: %w", err)
	}
	if err := s.addColumn("research_queries", "uncertain_count", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("migrate research query uncertain count: %w", err)
	}
	if err := s.addColumn("research_queries", "precision", "REAL NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("migrate research query precision: %w", err)
	}
	if err := s.addColumn("research_queries", "contributes_to_conclusions", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("migrate research query contribution: %w", err)
	}
	if err := s.addColumn("research_classifications", "schema_version", "TEXT NOT NULL DEFAULT 'legacy'"); err != nil {
		return fmt.Errorf("migrate classification schema version: %w", err)
	}
	if err := s.addColumn("research_classifications", "prompt_version", "TEXT NOT NULL DEFAULT 'legacy'"); err != nil {
		return fmt.Errorf("migrate classification prompt version: %w", err)
	}
	if err := s.addColumn("research_classifications", "mechanics_json", "TEXT NOT NULL DEFAULT '{}'"); err != nil {
		return fmt.Errorf("migrate classification mechanics: %w", err)
	}
	if err := s.addColumn("research_classifications", "performance_json", "TEXT NOT NULL DEFAULT '{}'"); err != nil {
		return fmt.Errorf("migrate classification performance: %w", err)
	}
	if err := s.addColumn("research_classifications", "raw_response", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("migrate classification raw response: %w", err)
	}
	if _, err := s.db.Exec(`
CREATE INDEX IF NOT EXISTS idx_research_queries_run ON research_queries(run_id);
CREATE INDEX IF NOT EXISTS idx_research_post_queries_post ON research_post_queries(post_id);
CREATE INDEX IF NOT EXISTS idx_research_post_relevance_run_label ON research_post_relevance(run_id, relevance_label);
CREATE INDEX IF NOT EXISTS idx_research_posts_author ON research_posts(author_username);
CREATE INDEX IF NOT EXISTS idx_research_post_snapshots_post ON research_post_snapshots(post_id);
CREATE INDEX IF NOT EXISTS idx_research_author_snapshots_author ON research_author_snapshots(username);
CREATE INDEX IF NOT EXISTS idx_research_post_snapshots_run ON research_post_snapshots(run_id);
CREATE INDEX IF NOT EXISTS idx_research_author_snapshots_run ON research_author_snapshots(run_id);
CREATE INDEX IF NOT EXISTS idx_research_baseline_posts_author ON research_baseline_posts(username);
CREATE INDEX IF NOT EXISTS idx_research_classifications_cache ON research_classifications(post_id, provider, model, schema_version, prompt_version);
CREATE INDEX IF NOT EXISTS idx_research_classification_runs_run ON research_classification_runs(run_id);
CREATE INDEX IF NOT EXISTS idx_research_deep_posts_run ON research_deep_posts(run_id);
CREATE INDEX IF NOT EXISTS idx_research_deep_provenance_post ON research_deep_provenance(post_id);
CREATE INDEX IF NOT EXISTS idx_research_deep_profiles_status ON research_deep_profiles(run_id, status);
CREATE INDEX IF NOT EXISTS idx_research_deep_replies_root ON research_deep_replies(run_id);`); err != nil {
		return fmt.Errorf("migrate research indexes: %w", err)
	}
	return nil
}

// addColumn upgrades databases created by the first MVP without touching
// existing evidence. All identifiers passed here are compile-time schema
// names, never user input.
func (s *Store) addColumn(table, column, definition string) error {
	rows, err := s.db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	var found bool
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if name == column {
			found = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = s.db.Exec("ALTER TABLE " + table + " ADD COLUMN " + column + " " + definition)
	return err
}

// StartRun creates a research run and returns its SQLite ID.
func (s *Store) StartRun(topic string, startedAt time.Time, queryCount int) (int64, error) {
	result, err := s.db.Exec(
		`INSERT INTO research_runs (topic, started_at, query_count, status) VALUES (?, ?, ?, ?)`,
		topic, formatTime(startedAt), queryCount, RunStatusRunning,
	)
	if err != nil {
		return 0, fmt.Errorf("start research run: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read research run id: %w", err)
	}
	return id, nil
}

// AddQuery records one expanded search query and returns its ID.
func (s *Store) AddQuery(runID int64, query string, position int, searchedAt time.Time) (int64, error) {
	result, err := s.db.Exec(
		`INSERT INTO research_queries (run_id, query, position, searched_at) VALUES (?, ?, ?, ?)`,
		runID, query, position, formatTime(searchedAt),
	)
	if err != nil {
		return 0, fmt.Errorf("add research query %q: %w", query, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read research query id: %w", err)
	}
	return id, nil
}

// FinishQuery stores result/error counts for one search query.
func (s *Store) FinishQuery(queryID int64, resultCount, uniqueNewPosts int, queryErr string) error {
	var errValue any
	if queryErr != "" {
		errValue = queryErr
	}
	_, err := s.db.Exec(
		`UPDATE research_queries SET result_count = ?, unique_new_posts = ?, error_message = ? WHERE id = ?`,
		resultCount, uniqueNewPosts, errValue, queryID,
	)
	if err != nil {
		return fmt.Errorf("finish research query: %w", err)
	}
	return nil
}

// SaveQueryQuality persists the deterministic relevance breakdown for one
// search query. It is additive to the original result/error counters so old
// research databases remain readable.
func (s *Store) SaveQueryQuality(queryID int64, report QueryReport) error {
	if report.ResultCount < 0 || report.Relevant < 0 || report.Adjacent < 0 ||
		report.Irrelevant < 0 || report.Uncertain < 0 {
		return fmt.Errorf("query relevance counts cannot be negative")
	}
	if math.IsNaN(report.Precision) || math.IsInf(report.Precision, 0) || report.Precision < 0 || report.Precision > 1 {
		return fmt.Errorf("query relevance precision must be between 0 and 1")
	}
	_, err := s.db.Exec(`
UPDATE research_queries SET
    relevant_count = ?,
    adjacent_count = ?,
    irrelevant_count = ?,
    uncertain_count = ?,
    precision = ?,
    contributes_to_conclusions = ?
WHERE id = ?`,
		report.Relevant,
		report.Adjacent,
		report.Irrelevant,
		report.Uncertain,
		report.Precision,
		boolInt(report.ContributesToConclusions),
		queryID,
	)
	if err != nil {
		return fmt.Errorf("save research query quality: %w", err)
	}
	return nil
}

// SavePostRelevance stores topic-scoped relevance separately from the
// canonical post row. The same post may be relevant to one research topic and
// irrelevant to another, so this relation is keyed by run and post.
func (s *Store) SavePostRelevance(runID int64, postID string, assessment RelevanceAssessment, assessedAt time.Time) error {
	if strings.TrimSpace(postID) == "" {
		return fmt.Errorf("cannot store relevance without a post id")
	}
	if !validRelevanceLabel(assessment.Label) {
		return fmt.Errorf("invalid relevance label %q", assessment.Label)
	}
	if math.IsNaN(assessment.Score) || math.IsInf(assessment.Score, 0) || assessment.Score < 0 || assessment.Score > 1 {
		return fmt.Errorf("relevance score must be between 0 and 1")
	}
	reasons, err := json.Marshal(uniqueSortedStrings(assessment.Reasons))
	if err != nil {
		return fmt.Errorf("marshal relevance reasons for %s: %w", postID, err)
	}
	_, err = s.db.Exec(`
INSERT INTO research_post_relevance (
    run_id, post_id, relevance_score, relevance_label, relevance_reasons, assessed_at
) VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(run_id, post_id) DO UPDATE SET
    relevance_score = excluded.relevance_score,
    relevance_label = excluded.relevance_label,
    relevance_reasons = excluded.relevance_reasons,
    assessed_at = excluded.assessed_at`,
		runID, postID, assessment.Score, assessment.Label, string(reasons), formatTime(assessedAt),
	)
	if err != nil {
		return fmt.Errorf("save relevance for research post %s: %w", postID, err)
	}
	return nil
}

func validRelevanceLabel(label RelevanceLabel) bool {
	switch label {
	case RelevanceRelevant, RelevanceAdjacent, RelevanceIrrelevant, RelevanceUncertain:
		return true
	default:
		return false
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// FinishRun marks a research run with an explicit terminal status.
func (s *Store) FinishRun(runID int64, completedAt time.Time, status RunStatus, runErr any) error {
	if status == RunStatusRunning || !status.valid() {
		return fmt.Errorf("invalid terminal research run status %q", status)
	}
	var storedErr any
	if status == RunStatusFailed {
		storedErr = runErr
	}
	_, err := s.db.Exec(
		`UPDATE research_runs SET completed_at = ?, status = ?, error_message = ? WHERE id = ?`,
		formatTime(completedAt), status, storedErr, runID,
	)
	if err != nil {
		return fmt.Errorf("finish research run: %w", err)
	}
	return nil
}

// UpsertPost stores one canonical post, retaining the first-seen timestamp and
// filling missing fields when later searches expose them.
func (s *Store) UpsertPost(post Post) error {
	if strings.TrimSpace(post.ID) == "" {
		return fmt.Errorf("cannot store a post without an id")
	}
	_, err := s.db.Exec(`
INSERT INTO research_posts (
    id, shortcode, url, text, author_username, published_at,
    like_count, reply_count, repost_count, quote_count, view_count,
    detected_topic, first_seen_at, last_seen_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    shortcode       = COALESCE(NULLIF(excluded.shortcode, ''), research_posts.shortcode),
    url             = COALESCE(NULLIF(excluded.url, ''), research_posts.url),
    text            = COALESCE(NULLIF(excluded.text, ''), research_posts.text),
    author_username = COALESCE(NULLIF(excluded.author_username, ''), research_posts.author_username),
    published_at    = COALESCE(excluded.published_at, research_posts.published_at),
    like_count      = COALESCE(excluded.like_count, research_posts.like_count),
    reply_count     = COALESCE(excluded.reply_count, research_posts.reply_count),
    repost_count    = COALESCE(excluded.repost_count, research_posts.repost_count),
    quote_count     = COALESCE(excluded.quote_count, research_posts.quote_count),
    view_count      = COALESCE(excluded.view_count, research_posts.view_count),
    detected_topic  = COALESCE(NULLIF(excluded.detected_topic, ''), research_posts.detected_topic),
    last_seen_at    = excluded.last_seen_at`,
		post.ID,
		post.Shortcode,
		post.URL,
		post.Text,
		post.AuthorUsername,
		nullableTime(post.PublishedAt),
		nullableInt(post.Likes),
		nullableInt(post.Replies),
		nullableInt(post.Reposts),
		nullableInt(post.Quotes),
		nullableInt(post.Views),
		post.DetectedTopic,
		formatTime(post.FirstSeenAt),
		formatTime(post.LastSeenAt),
	)
	if err != nil {
		return fmt.Errorf("upsert research post %s: %w", post.ID, err)
	}
	return nil
}

// LinkPostToQuery preserves every query that found a post in a run.
func (s *Store) LinkPostToQuery(runID, queryID int64, postID string, discoveredAt time.Time) error {
	_, err := s.db.Exec(`
INSERT INTO research_post_queries (run_id, query_id, post_id, discovered_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(run_id, query_id, post_id) DO NOTHING`,
		runID, queryID, postID, formatTime(discoveredAt),
	)
	if err != nil {
		return fmt.Errorf("link research post %s to query: %w", postID, err)
	}
	return nil
}

// EnsureAuthor creates or updates the canonical author row without appending a
// profile snapshot. The pipeline uses this for authors that were discovered in
// search but were outside the bounded metadata lookup.
func (s *Store) EnsureAuthor(author Author, updatedAt time.Time) error {
	username := normalizeAuthor(author.Username)
	if username == "" {
		return fmt.Errorf("cannot store an author without a username")
	}
	author.Username = username
	_, err := s.db.Exec(`
INSERT INTO research_authors (
    username, name, bio, follower_count, following_count, verified, profile_url, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(username) DO UPDATE SET
    name            = COALESCE(NULLIF(excluded.name, ''), research_authors.name),
    bio             = COALESCE(NULLIF(excluded.bio, ''), research_authors.bio),
    follower_count  = COALESCE(excluded.follower_count, research_authors.follower_count),
    following_count = COALESCE(excluded.following_count, research_authors.following_count),
    verified        = COALESCE(excluded.verified, research_authors.verified),
    profile_url     = COALESCE(NULLIF(excluded.profile_url, ''), research_authors.profile_url),
    updated_at      = excluded.updated_at`,
		author.Username,
		author.Name,
		author.Bio,
		nullableInt(author.FollowerCount),
		nullableInt(author.FollowingCount),
		nullableBool(author.Verified),
		author.ProfileURL,
		formatTime(updatedAt),
	)
	if err != nil {
		return fmt.Errorf("upsert research author %s: %w", username, err)
	}
	return nil
}

// UpsertAuthor stores the latest known profile metadata and appends a snapshot
// tied to the run that observed it.
func (s *Store) UpsertAuthor(runID int64, author Author, capturedAt time.Time) error {
	if err := s.EnsureAuthor(author, capturedAt); err != nil {
		return err
	}
	username := normalizeAuthor(author.Username)
	author.Username = username
	_, err := s.db.Exec(`
INSERT INTO research_author_snapshots (
	run_id, username, captured_at, name, bio, follower_count, following_count, verified, profile_url
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		runID,
		author.Username,
		formatTime(capturedAt),
		author.Name,
		author.Bio,
		nullableInt(author.FollowerCount),
		nullableInt(author.FollowingCount),
		nullableBool(author.Verified),
		author.ProfileURL,
	)
	if err != nil {
		return fmt.Errorf("snapshot research author %s: %w", username, err)
	}
	return nil
}

// LinkBaselinePost records that a recent author-feed post was used as a
// baseline observation, without linking it to a topic search query.
func (s *Store) LinkBaselinePost(runID int64, username, postID string, capturedAt time.Time) error {
	username = normalizeAuthor(username)
	if username == "" || strings.TrimSpace(postID) == "" {
		return fmt.Errorf("baseline post and author are required")
	}
	_, err := s.db.Exec(`
INSERT INTO research_baseline_posts (run_id, username, post_id, captured_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(run_id, username, post_id) DO NOTHING`,
		runID, username, postID, formatTime(capturedAt),
	)
	if err != nil {
		return fmt.Errorf("link baseline research post %s: %w", postID, err)
	}
	return nil
}

// RecordPostSnapshot stores the metric state observed during a run.
func (s *Store) RecordPostSnapshot(runID int64, post Post, capturedAt time.Time) error {
	_, err := s.db.Exec(`
INSERT INTO research_post_snapshots (
	run_id, post_id, captured_at, like_count, reply_count, repost_count, quote_count, view_count
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		runID,
		post.ID,
		formatTime(capturedAt),
		nullableInt(post.Likes),
		nullableInt(post.Replies),
		nullableInt(post.Reposts),
		nullableInt(post.Quotes),
		nullableInt(post.Views),
	)
	if err != nil {
		return fmt.Errorf("snapshot research post %s: %w", post.ID, err)
	}
	return nil
}

// FindClassification returns the latest classification for the full semantic
// cache key. A changed provider, model, prompt, or schema never reuses an old
// result accidentally.
func (s *Store) FindClassification(postID, provider, model, schemaVersion, promptVersion string) (*ClassificationRecord, error) {
	var (
		record          ClassificationRecord
		classifiedAt    string
		payloadJSON     string
		mechanicsJSON   string
		performanceJSON string
	)
	err := s.db.QueryRow(`
SELECT id, post_id, provider, COALESCE(model, ''), schema_version, prompt_version,
       classified_at, payload_json, mechanics_json, performance_json, raw_response
FROM research_classifications
WHERE post_id = ? AND provider = ? AND COALESCE(model, '') = ?
  AND schema_version = ? AND prompt_version = ?
ORDER BY classified_at DESC, id DESC
LIMIT 1`, postID, provider, model, schemaVersion, promptVersion).Scan(
		&record.ID, &record.PostID, &record.Provider, &record.Model, &record.SchemaVersion,
		&record.PromptVersion, &classifiedAt, &payloadJSON, &mechanicsJSON, &performanceJSON, &record.RawResponse,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find cached classification %s: %w", postID, err)
	}
	record.ClassifiedAt = parseStoredTime(classifiedAt)
	if err := json.Unmarshal([]byte(payloadJSON), &record.Analysis); err != nil {
		return nil, fmt.Errorf("decode cached classification %s: %w", postID, err)
	}
	if mechanicsJSON != "" && mechanicsJSON != "{}" {
		if err := json.Unmarshal([]byte(mechanicsJSON), &record.Analysis.Mechanics); err != nil {
			return nil, fmt.Errorf("decode cached mechanics %s: %w", postID, err)
		}
	}
	if performanceJSON != "" && performanceJSON != "{}" {
		if err := json.Unmarshal([]byte(performanceJSON), &record.Performance); err != nil {
			return nil, fmt.Errorf("decode cached performance %s: %w", postID, err)
		}
	}
	if record.PostID == "" {
		record.PostID = postID
	}
	return &record, nil
}

// SaveClassification stores a validated semantic result and links it to the
// current run. Existing cache rows are reused rather than duplicated.
func (s *Store) SaveClassification(runID int64, record ClassificationRecord) error {
	if runID <= 0 || strings.TrimSpace(record.PostID) == "" || strings.TrimSpace(record.Provider) == "" ||
		strings.TrimSpace(record.Model) == "" || strings.TrimSpace(record.SchemaVersion) == "" || strings.TrimSpace(record.PromptVersion) == "" {
		return errors.New("run, post, provider, model, schema version, and prompt version are required for classification")
	}
	if err := ValidatePostAnalysis(&record.Analysis); err != nil {
		return fmt.Errorf("validate classification %s: %w", record.PostID, err)
	}
	if record.Analysis.PostID != record.PostID {
		return fmt.Errorf("classification post_id %q does not match record post_id %q", record.Analysis.PostID, record.PostID)
	}
	cached, err := s.FindClassification(record.PostID, record.Provider, record.Model, record.SchemaVersion, record.PromptVersion)
	// A malformed historical cache row is treated as a miss so a successful
	// retry can append a repaired version without deleting the old evidence.
	// Operational failures still surface from the INSERT below.
	if err != nil {
		cached = nil
	}
	if cached != nil {
		return s.linkClassification(runID, cached.ID, record.ClassifiedAt)
	}
	if record.ClassifiedAt.IsZero() {
		record.ClassifiedAt = time.Now().UTC()
	}
	payload, err := json.Marshal(record.Analysis)
	if err != nil {
		return fmt.Errorf("marshal classification %s: %w", record.PostID, err)
	}
	mechanics, err := json.Marshal(record.Analysis.Mechanics)
	if err != nil {
		return fmt.Errorf("marshal mechanics %s: %w", record.PostID, err)
	}
	performance, err := json.Marshal(record.Performance)
	if err != nil {
		return fmt.Errorf("marshal performance %s: %w", record.PostID, err)
	}
	result, err := s.db.Exec(`
INSERT INTO research_classifications (
    post_id, provider, model, schema_version, prompt_version, classified_at,
    payload_json, mechanics_json, performance_json, raw_response
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.PostID, record.Provider, record.Model, record.SchemaVersion, record.PromptVersion,
		formatTime(record.ClassifiedAt), string(payload), string(mechanics), string(performance), record.RawResponse,
	)
	if err != nil {
		return fmt.Errorf("store classification %s: %w", record.PostID, err)
	}
	classificationID, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("read classification id %s: %w", record.PostID, err)
	}
	return s.linkClassification(runID, classificationID, record.ClassifiedAt)
}

func (s *Store) linkClassification(runID, classificationID int64, linkedAt time.Time) error {
	if linkedAt.IsZero() {
		linkedAt = time.Now().UTC()
	}
	_, err := s.db.Exec(`
INSERT INTO research_classification_runs (run_id, classification_id, linked_at)
VALUES (?, ?, ?)
ON CONFLICT(run_id, classification_id) DO NOTHING`, runID, classificationID, formatTime(linkedAt))
	if err != nil {
		return fmt.Errorf("link classification %d to run %d: %w", classificationID, runID, err)
	}
	return nil
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return time.Unix(0, 0).UTC().Format(time.RFC3339Nano)
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func parseStoredTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return formatTime(value)
}

func nullableInt(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableBool(value *bool) any {
	if value == nil {
		return nil
	}
	if *value {
		return 1
	}
	return 0
}

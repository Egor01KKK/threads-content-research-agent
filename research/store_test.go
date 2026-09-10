package research

import (
	"database/sql"
	"testing"
	"time"
)

func TestStoreDeduplicatesPostsAndPreservesNullMetrics(t *testing.T) {
	store, err := OpenStore(t.TempDir() + "/research.db")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	runID, err := store.StartRun("topic", now, 2)
	if err != nil {
		t.Fatal(err)
	}
	firstQuery, err := store.AddQuery(runID, "topic", 0, now)
	if err != nil {
		t.Fatal(err)
	}
	secondQuery, err := store.AddQuery(runID, "topic tools", 1, now)
	if err != nil {
		t.Fatal(err)
	}

	post := Post{
		ID:             "post-1",
		URL:            "https://www.threads.com/@ada/post/ABC123",
		AuthorUsername: "ada",
		Text:           "hello",
		Likes:          nil,
		Replies:        intPtr(0),
		DetectedTopic:  "topic",
		FirstSeenAt:    now,
		LastSeenAt:     now,
	}
	if err := store.UpsertPost(post); err != nil {
		t.Fatal(err)
	}
	if err := store.LinkPostToQuery(runID, firstQuery, post.ID, now); err != nil {
		t.Fatal(err)
	}
	post.Likes = intPtr(12)
	if err := store.UpsertPost(post); err != nil {
		t.Fatal(err)
	}
	if err := store.LinkPostToQuery(runID, secondQuery, post.ID, now); err != nil {
		t.Fatal(err)
	}
	if err := store.LinkPostToQuery(runID, secondQuery, post.ID, now); err != nil {
		t.Fatal(err)
	}

	var postCount, hitCount int
	if err := store.db.QueryRow(`SELECT count(*) FROM research_posts`).Scan(&postCount); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT count(*) FROM research_post_queries`).Scan(&hitCount); err != nil {
		t.Fatal(err)
	}
	if postCount != 1 || hitCount != 2 {
		t.Fatalf("post count=%d hit count=%d; want 1 and 2", postCount, hitCount)
	}

	var likes, replies sql.NullInt64
	if err := store.db.QueryRow(`SELECT like_count, reply_count FROM research_posts WHERE id = ?`, post.ID).Scan(&likes, &replies); err != nil {
		t.Fatal(err)
	}
	if !likes.Valid || likes.Int64 != 12 {
		t.Errorf("likes = %+v, want latest known 12", likes)
	}
	if !replies.Valid || replies.Int64 != 0 {
		t.Errorf("replies = %+v, want exposed zero", replies)
	}

	unknown := Post{ID: "post-unknown", FirstSeenAt: now, LastSeenAt: now}
	if err := store.UpsertPost(unknown); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT like_count FROM research_posts WHERE id = ?`, unknown.ID).Scan(&likes); err != nil {
		t.Fatal(err)
	}
	if likes.Valid {
		t.Errorf("unknown like count = %+v, want SQL NULL", likes)
	}
}

func TestStorePersistsTopicScopedRelevanceAndQueryQuality(t *testing.T) {
	store, err := OpenStore(t.TempDir() + "/research.db")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	now := time.Now().UTC()
	runID, err := store.StartRun("finding clients for freelancers", now, 1)
	if err != nil {
		t.Fatal(err)
	}
	queryID, err := store.AddQuery(runID, "find clients", 0, now)
	if err != nil {
		t.Fatal(err)
	}
	post := Post{ID: "post-1", Text: "Need clients for web design", FirstSeenAt: now, LastSeenAt: now}
	if err := store.UpsertPost(post); err != nil {
		t.Fatal(err)
	}
	assessment := RelevanceAssessment{Score: 0.9, Label: RelevanceRelevant, Reasons: []string{"client_family", "acquisition_intent"}}
	if err := store.SavePostRelevance(runID, post.ID, assessment, now); err != nil {
		t.Fatal(err)
	}
	quality := QueryReport{Query: "find clients", ResultCount: 4, Relevant: 2, Adjacent: 1, Irrelevant: 1, Precision: 0.5, ContributesToConclusions: true}
	if err := store.SaveQueryQuality(queryID, quality); err != nil {
		t.Fatal(err)
	}

	var score float64
	var label, reasons string
	if err := store.db.QueryRow(`SELECT relevance_score, relevance_label, relevance_reasons FROM research_post_relevance WHERE run_id = ? AND post_id = ?`, runID, post.ID).Scan(&score, &label, &reasons); err != nil {
		t.Fatal(err)
	}
	if score != assessment.Score || label != string(assessment.Label) || reasons != `["acquisition_intent","client_family"]` {
		t.Errorf("stored relevance = score %v label %q reasons %s", score, label, reasons)
	}
	var relevant, adjacent, irrelevant, uncertain, contributes int
	var precision float64
	if err := store.db.QueryRow(`SELECT relevant_count, adjacent_count, irrelevant_count, uncertain_count, precision, contributes_to_conclusions FROM research_queries WHERE id = ?`, queryID).Scan(&relevant, &adjacent, &irrelevant, &uncertain, &precision, &contributes); err != nil {
		t.Fatal(err)
	}
	if relevant != 2 || adjacent != 1 || irrelevant != 1 || uncertain != 0 || precision != 0.5 || contributes != 1 {
		t.Errorf("stored query quality = %d/%d/%d/%d precision=%v contributes=%d", relevant, adjacent, irrelevant, uncertain, precision, contributes)
	}
}

func TestStoreAuthorSnapshotPreservesFalseAndNull(t *testing.T) {
	store, err := OpenStore(t.TempDir() + "/research.db")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	verified := false
	runID, err := store.StartRun("topic", time.Now(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAuthor(runID, Author{Username: "ada", FollowingCount: intPtr(0), Verified: &verified}, time.Now()); err != nil {
		t.Fatal(err)
	}
	var following, followers, storedVerified sql.NullInt64
	if err := store.db.QueryRow(`SELECT following_count, follower_count, verified FROM research_authors WHERE username = 'ada'`).Scan(&following, &followers, &storedVerified); err != nil {
		t.Fatal(err)
	}
	if !following.Valid || following.Int64 != 0 {
		t.Errorf("following = %+v, want exposed zero", following)
	}
	if followers.Valid {
		t.Errorf("followers = %+v, want NULL", followers)
	}
	if !storedVerified.Valid || storedVerified.Int64 != 0 {
		t.Errorf("verified = %+v, want false/0", storedVerified)
	}
}

func TestStoreRunStatusAndSnapshotLinkage(t *testing.T) {
	store, err := OpenStore(t.TempDir() + "/research.db")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	now := time.Now().UTC()
	runID, err := store.StartRun("topic", now, 1)
	if err != nil {
		t.Fatal(err)
	}
	var status string
	if err := store.db.QueryRow(`SELECT status FROM research_runs WHERE id = ?`, runID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(RunStatusRunning) {
		t.Fatalf("initial status = %q, want running", status)
	}
	verified := false
	post := Post{ID: "post-1", Likes: intPtr(4), FirstSeenAt: now, LastSeenAt: now}
	if err := store.UpsertAuthor(runID, Author{Username: "ada", Verified: &verified}, now); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertPost(post); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordPostSnapshot(runID, post, now); err != nil {
		t.Fatal(err)
	}
	var authorRun, postRun sql.NullInt64
	if err := store.db.QueryRow(`SELECT run_id FROM research_author_snapshots WHERE username = 'ada'`).Scan(&authorRun); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT run_id FROM research_post_snapshots WHERE post_id = 'post-1'`).Scan(&postRun); err != nil {
		t.Fatal(err)
	}
	if !authorRun.Valid || authorRun.Int64 != runID || !postRun.Valid || postRun.Int64 != runID {
		t.Errorf("snapshot linkage author=%+v post=%+v run=%d", authorRun, postRun, runID)
	}
	if err := store.FinishRun(runID, now, RunStatusCompletedWithWarns, "partial collection"); err != nil {
		t.Fatal(err)
	}
	var errorMessage sql.NullString
	if err := store.db.QueryRow(`SELECT status, error_message FROM research_runs WHERE id = ?`, runID).Scan(&status, &errorMessage); err != nil {
		t.Fatal(err)
	}
	if status != string(RunStatusCompletedWithWarns) || errorMessage.Valid {
		t.Errorf("warning status=%q error=%+v; warning runs should not be failures", status, errorMessage)
	}
}

func TestStoreClassificationCacheIsVersionedAndLinkedToRuns(t *testing.T) {
	store, err := OpenStore(t.TempDir() + "/research.db")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	now := time.Now().UTC()
	runOne, err := store.StartRun("topic", now, 1)
	if err != nil {
		t.Fatal(err)
	}
	runTwo, err := store.StartRun("topic", now, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertPost(Post{ID: "post-1", Text: "text", FirstSeenAt: now, LastSeenAt: now}); err != nil {
		t.Fatal(err)
	}
	first := testPostAnalysis("post-1")
	if err := store.SaveClassification(runOne, ClassificationRecord{
		PostID: "post-1", Provider: "fake", Model: "model", SchemaVersion: "schema-v1", PromptVersion: "prompt-v1",
		ClassifiedAt: now, Analysis: first, Performance: PerformanceContext{Engagement: 7}, RawResponse: `{"raw":1}`,
	}); err != nil {
		t.Fatal(err)
	}
	second := first
	second.ContentType = "opinion"
	if err := store.SaveClassification(runTwo, ClassificationRecord{
		PostID: "post-1", Provider: "fake", Model: "model", SchemaVersion: "schema-v1", PromptVersion: "prompt-v1",
		ClassifiedAt: now.Add(time.Minute), Analysis: second, RawResponse: `{"raw":2}`,
	}); err != nil {
		t.Fatal(err)
	}
	cached, err := store.FindClassification("post-1", "fake", "model", "schema-v1", "prompt-v1")
	if err != nil {
		t.Fatal(err)
	}
	if cached == nil || cached.Analysis.ContentType != "educational" || cached.RawResponse != `{"raw":1}` {
		t.Errorf("cached record = %+v; same version should not be overwritten", cached)
	}
	var rows, links int
	if err := store.db.QueryRow(`SELECT count(*) FROM research_classifications`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT count(*) FROM research_classification_runs`).Scan(&links); err != nil {
		t.Fatal(err)
	}
	if rows != 1 || links != 2 {
		t.Errorf("cache rows=%d run links=%d; want 1 and 2", rows, links)
	}
	if err := store.SaveClassification(runTwo, ClassificationRecord{
		PostID: "post-1", Provider: "fake", Model: "model", SchemaVersion: "schema-v2", PromptVersion: "prompt-v1",
		ClassifiedAt: now.Add(2 * time.Minute), Analysis: second, RawResponse: `{"raw":3}`,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT count(*) FROM research_classifications`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 2 {
		t.Errorf("schema version change did not invalidate cache: rows=%d", rows)
	}
}

func TestStoreMigratesFirstMVPSchema(t *testing.T) {
	path := t.TempDir() + "/legacy.db"
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE research_runs (id INTEGER PRIMARY KEY AUTOINCREMENT, topic TEXT NOT NULL, started_at TEXT NOT NULL, completed_at TEXT, query_count INTEGER NOT NULL DEFAULT 0);
CREATE TABLE research_authors (username TEXT PRIMARY KEY);
CREATE TABLE research_posts (id TEXT PRIMARY KEY, author_username TEXT, first_seen_at TEXT NOT NULL, last_seen_at TEXT NOT NULL);
CREATE TABLE research_author_snapshots (id INTEGER PRIMARY KEY AUTOINCREMENT, username TEXT NOT NULL, captured_at TEXT NOT NULL);
CREATE TABLE research_post_snapshots (id INTEGER PRIMARY KEY AUTOINCREMENT, post_id TEXT NOT NULL, captured_at TEXT NOT NULL);
INSERT INTO research_runs (topic, started_at, query_count) VALUES ('legacy topic', '2026-09-08T00:00:00Z', 1);`)
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	assertColumn := func(table, want string) {
		t.Helper()
		rows, err := store.db.Query("PRAGMA table_info(" + table + ")")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = rows.Close() }()
		found := false
		for rows.Next() {
			var cid, notNull, pk int
			var name, typ string
			var defaultValue any
			if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
				t.Fatal(err)
			}
			if name == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%s.%s was not added by migration", table, want)
		}
	}
	assertColumn("research_runs", "status")
	assertColumn("research_runs", "error_message")
	assertColumn("research_author_snapshots", "run_id")
	assertColumn("research_post_snapshots", "run_id")
	assertColumn("research_queries", "relevant_count")
	assertColumn("research_queries", "precision")
	var relevanceTable string
	if err := store.db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'research_post_relevance'`).Scan(&relevanceTable); err != nil {
		t.Fatal(err)
	}
	if relevanceTable != "research_post_relevance" {
		t.Errorf("relevance table = %q", relevanceTable)
	}
	var legacyStatus string
	if err := store.db.QueryRow(`SELECT status FROM research_runs WHERE topic = 'legacy topic'`).Scan(&legacyStatus); err != nil {
		t.Fatal(err)
	}
	if legacyStatus != string(RunStatusCompletedWithWarns) {
		t.Errorf("legacy run status = %q, want completed_with_warnings", legacyStatus)
	}
}

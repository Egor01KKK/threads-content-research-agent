package research

import (
	"context"
	"errors"
	"iter"
	"strings"
	"testing"
	"time"

	"github.com/Egor01KKK/threads-content-research-agent/threads"
)

type fixedExpander struct {
	queries []string
}

func (e fixedExpander) Expand(context.Context, string) ([]string, error) {
	return append([]string(nil), e.queries...), nil
}

type fakeCollector struct {
	searches       map[string][]threads.SearchResult
	searchErrors   map[string]error
	profiles       map[string]*threads.Profile
	profileErrors  map[string]error
	baselines      map[string][]threads.Post
	baselineErrors map[string]error
	searched       []string
	profiled       []string
	baselined      []string
}

func (f *fakeCollector) Search(_ context.Context, query string, _ int) iter.Seq2[threads.SearchResult, error] {
	return func(yield func(threads.SearchResult, error) bool) {
		f.searched = append(f.searched, query)
		for _, result := range f.searches[query] {
			if !yield(result, nil) {
				return
			}
		}
		if err := f.searchErrors[query]; err != nil {
			yield(threads.SearchResult{}, err)
		}
	}
}

func (f *fakeCollector) Profile(_ context.Context, username string) (*threads.Profile, error) {
	f.profiled = append(f.profiled, username)
	if err := f.profileErrors[username]; err != nil {
		return nil, err
	}
	return f.profiles[username], nil
}

func (f *fakeCollector) ProfilePosts(_ context.Context, username string, limit int) iter.Seq2[threads.Post, error] {
	return func(yield func(threads.Post, error) bool) {
		f.baselined = append(f.baselined, username)
		n := 0
		for _, post := range f.baselines[username] {
			if !yield(post, nil) {
				return
			}
			n++
			if limit > 0 && n >= limit {
				break
			}
		}
		if err := f.baselineErrors[username]; err != nil {
			yield(threads.Post{}, err)
		}
	}
}

func TestRunDeduplicatesAndBoundsProfileLookups(t *testing.T) {
	collector := &fakeCollector{
		searches: map[string][]threads.SearchResult{
			"one": {
				searchResult("p1", "small", 20, "https://www.threads.com/@small/post/P1"),
				searchResult("p2", "large", 80, "https://www.threads.com/@large/post/P2"),
			},
			"two": {
				searchResult("p1", "small", 20, "https://www.threads.com/@small/post/P1"),
				searchResult("p3", "small", 10, "https://www.threads.com/@small/post/P3"),
			},
			"three": {},
		},
		searchErrors: map[string]error{"three": errors.New("stale search query")},
		profiles: map[string]*threads.Profile{
			"small": {
				Username: "small", Name: "Small Creator", FollowerCount: 100,
				FollowerCountAvailable: true, FollowingCount: 0, FollowingCountAvailable: true,
				IsVerified: false, VerifiedAvailable: true,
				URL: "https://www.threads.com/@small",
			},
			"large": {
				Username: "large", FollowerCount: 100000, FollowerCountAvailable: true,
				URL: "https://www.threads.com/@large",
			},
		},
		profileErrors: map[string]error{},
	}
	store, err := OpenStore(t.TempDir() + "/research.db")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	cfg := DefaultConfig()
	cfg.Topic = "topic"
	cfg.PerQueryLimit = 5
	cfg.ProfileLimit = 1
	cfg.TopPosts = 2
	cfg.TopAuthors = 2
	report, err := Run(context.Background(), cfg, collector, fixedExpander{queries: []string{"one", "two", "three"}}, store)
	if err != nil {
		t.Fatal(err)
	}
	if report.PostsAnalyzed != 3 || report.AuthorsDiscovered != 2 {
		t.Errorf("report counts = posts %d authors %d", report.PostsAnalyzed, report.AuthorsDiscovered)
	}
	if len(collector.profiled) != 1 || report.ProfilesAttempted != 1 {
		t.Errorf("profile lookups = %v, attempts=%d; want one", collector.profiled, report.ProfilesAttempted)
	}
	if report.ProfilesWithKnownFollowers != 1 || report.Coverage.Followers.Known != 1 {
		t.Errorf("follower coverage = profiles=%d coverage=%+v", report.ProfilesWithKnownFollowers, report.Coverage.Followers)
	}
	if len(report.TopPosts) != 2 || report.TopPosts[0].Post.URL == "" {
		t.Errorf("top posts = %+v", report.TopPosts)
	}
	if len(report.TopOutperformingPosts) != 0 {
		t.Errorf("outperforming posts without a baseline = %+v", report.TopOutperformingPosts)
	}
	if len(report.TopAuthors) != 2 {
		t.Errorf("top authors = %+v", report.TopAuthors)
	}
	if report.Status != RunStatusCompletedWithWarns {
		t.Errorf("status = %q, want completed_with_warnings", report.Status)
	}
	if report.FollowerRelativeRanking != "partial" {
		t.Errorf("follower-relative ranking status = %q, want partial", report.FollowerRelativeRanking)
	}
	if report.Collection.QueriesAttempted != 3 || report.Collection.QueriesSuccessful != 2 || report.Collection.QueriesWithResults != 2 ||
		report.Collection.RawPostsCollected != 4 || report.Collection.UniquePosts != 3 || report.Collection.UniqueAuthors != 2 {
		t.Errorf("collection coverage = %+v", report.Collection)
	}
	foundWarning := false
	for _, warning := range report.Warnings {
		if strings.Contains(warning, "stale search query") {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Errorf("query failure warning missing: %v", report.Warnings)
	}

	var postCount, hitCount, queryCount int
	if err := store.db.QueryRow(`SELECT count(*) FROM research_posts`).Scan(&postCount); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT count(*) FROM research_post_queries`).Scan(&hitCount); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT count(*) FROM research_queries WHERE error_message IS NOT NULL`).Scan(&queryCount); err != nil {
		t.Fatal(err)
	}
	if postCount != 3 || hitCount != 4 || queryCount != 1 {
		t.Errorf("stored posts=%d hits=%d failed_queries=%d", postCount, hitCount, queryCount)
	}
}

func TestRussianResearchProbesAllCandidatesAndDeepensOnlyUsefulQueries(t *testing.T) {
	collector := &fakeCollector{
		searches: map[string][]threads.SearchResult{
			"q1": {searchResult("p1", "owner-one", 10, "https://www.threads.com/@owner-one/post/P1")},
			"q2": {searchResult("p2", "owner-two", 20, "https://www.threads.com/@owner-two/post/P2")},
			"q3": {searchResult("p3", "owner-three", 30, "https://www.threads.com/@owner-three/post/P3")},
			"q4": {},
		},
		profiles:       map[string]*threads.Profile{},
		profileErrors:  map[string]error{},
		baselines:      map[string][]threads.Post{},
		baselineErrors: map[string]error{},
	}
	collector.searches["q1"][0].Text = "У меня свой магазин. Веду заявки в Excel, они теряются, кто посоветует CRM?"
	collector.searches["q3"][0].Text = "У нас в агентстве все задачи в чатах, сотрудники забывают передавать заявки. Как связать CRM и Telegram?"
	collector.searches["q2"][0].Text = "У меня свой бизнес, расскажите чем занимаетесь."

	store, err := OpenStore(t.TempDir() + "/research.db")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	cfg := DefaultConfig()
	cfg.Topic = "проблемы малого бизнеса которые можно решить автоматизацией и IT"
	cfg.MaxQueries = 4
	cfg.PerQueryLimit = 5
	cfg.ProfileLimit = 0
	cfg.BaselineAuthorLimit = 0
	cfg.TopPosts = 5
	cfg.TopAuthors = 5
	report, err := Run(context.Background(), cfg, collector, fixedExpander{queries: []string{"q1", "q2", "q3", "q4"}}, store)
	if err != nil {
		t.Fatal(err)
	}
	if len(collector.searched) != 6 {
		t.Fatalf("search calls = %v, want four probes plus two deep passes", collector.searched)
	}
	if report.Collection.RawPostsCollected != 5 || report.Collection.UniquePosts != 3 {
		t.Errorf("collection coverage = %+v, want five raw and three unique", report.Collection)
	}
	if len(report.Queries) != 4 {
		t.Fatalf("query reports = %d, want four", len(report.Queries))
	}
	if !report.Queries[0].SelectedForDeep || report.Queries[1].SelectedForDeep || !report.Queries[2].SelectedForDeep || report.Queries[3].SelectedForDeep {
		t.Errorf("deep selection = %+v", report.Queries)
	}
	if report.Queries[0].ProbeResultCount != 1 || report.Queries[0].DeepResultCount != 1 || report.Queries[0].ProbeUniqueContribution != 1 || report.Queries[0].DeepUniqueContribution != 0 {
		t.Errorf("q1 probe/deep accounting = %+v", report.Queries[0])
	}
	if report.Queries[1].ProbePainPosts != 0 || report.Queries[1].ProbeOwnerLikelyPosts != 1 {
		t.Errorf("owner-only q2 should be probe evidence but not pain evidence: %+v", report.Queries[1])
	}
	if report.BusinessPainCounts.GenuinePainPosts != 2 || report.BusinessPainCounts.ITActionablePainPosts != 2 {
		t.Errorf("business pain counts = %+v", report.BusinessPainCounts)
	}
}

func TestRunFiltersIrrelevantPostsBeforeEnrichmentAndFinalRanking(t *testing.T) {
	collector := &fakeCollector{
		searches: map[string][]threads.SearchResult{
			"clients": {
				{ID: "relevant", Username: "good", Permalink: "https://threads/relevant", Text: "Need clients for web design", LikeCount: intPtr(2)},
				{ID: "irrelevant", Username: "noise", Permalink: "https://threads/irrelevant", Text: "Fancy Francine", LikeCount: intPtr(100000)},
				{ID: "adjacent", Username: "adjacent", Permalink: "https://threads/adjacent", Text: "Client call", LikeCount: intPtr(50000)},
			},
		},
		profiles: map[string]*threads.Profile{
			"good":     {Username: "good", FollowerCount: 1000, FollowerCountAvailable: true},
			"noise":    {Username: "noise", FollowerCount: 1000, FollowerCountAvailable: true},
			"adjacent": {Username: "adjacent", FollowerCount: 1000, FollowerCountAvailable: true},
		},
		baselines: map[string][]threads.Post{
			"good": {threadPost("good-b1", "good", 1), threadPost("good-b2", "good", 2), threadPost("good-b3", "good", 3)},
		},
		profileErrors: map[string]error{}, baselineErrors: map[string]error{},
	}
	store, err := OpenStore(t.TempDir() + "/research.db")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	cfg := DefaultConfig()
	cfg.Topic = "finding clients for freelancers"
	cfg.MaxQueries = 1
	cfg.PerQueryLimit = 3
	cfg.ProfileLimit = 1
	cfg.BaselineAuthorLimit = 1
	cfg.BaselinePostLimit = 3
	cfg.MinBaselinePosts = 3
	cfg.TopPosts = 3
	cfg.TopAuthors = 1
	report, err := Run(context.Background(), cfg, collector, fixedExpander{queries: []string{"clients"}}, store)
	if err != nil {
		t.Fatal(err)
	}
	if len(collector.profiled) != 1 || collector.profiled[0] != "good" {
		t.Errorf("profile enrichment consumed by wrong author: %v", collector.profiled)
	}
	if len(collector.baselined) != 1 || collector.baselined[0] != "good" {
		t.Errorf("baseline enrichment consumed by wrong author: %v", collector.baselined)
	}
	if report.Collection.RelevantPosts != 1 || report.Collection.AdjacentPosts != 1 || report.Collection.IrrelevantPosts != 1 || report.Collection.RelevantAuthors != 1 {
		t.Errorf("relevance coverage = %+v", report.Collection)
	}
	if report.Queries[0].Relevant != 1 || report.Queries[0].Adjacent != 1 || report.Queries[0].Irrelevant != 1 || report.Queries[0].Precision != 1.0/3.0 || !report.Queries[0].ContributesToConclusions {
		t.Errorf("query quality = %+v", report.Queries[0])
	}
	if len(report.TopPosts) != 1 || report.TopPosts[0].Post.ID != "relevant" {
		t.Errorf("topic ranking leaked noise: %+v", report.TopPosts)
	}
	foundIrrelevant := false
	for _, ranked := range report.RawTopPosts {
		if ranked.Post.ID == "irrelevant" {
			foundIrrelevant = true
		}
	}
	if len(report.RawTopPosts) != 3 || !foundIrrelevant {
		t.Errorf("raw diagnostic ranking missing noisy high performer: %+v", report.RawTopPosts)
	}
	if report.TopPosts[0].RelativePerformance == nil || *report.TopPosts[0].RelativePerformance <= 0 {
		t.Errorf("relevant post lost baseline performance: %+v", report.TopPosts[0])
	}
	var storedLabel string
	if err := store.db.QueryRow(`SELECT relevance_label FROM research_post_relevance WHERE run_id = ? AND post_id = 'irrelevant'`, report.RunID).Scan(&storedLabel); err != nil {
		t.Fatal(err)
	}
	if storedLabel != string(RelevanceIrrelevant) {
		t.Errorf("stored irrelevant label = %q", storedLabel)
	}
}

func TestProfileSelectionDoesNotFavorRawEngagement(t *testing.T) {
	posts := []Post{
		{ID: "small", AuthorUsername: "small", Likes: intPtr(1), PublishedAt: time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)},
		{ID: "large", AuthorUsername: "large", Likes: intPtr(1000), PublishedAt: time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)},
	}
	candidates := selectProfileCandidates(posts, 1)
	if len(candidates) != 1 || candidates[0] != "small" {
		t.Fatalf("profile candidates = %v, want recent small creator first", candidates)
	}
}

func TestRunUsesIndependentBaselineAndKeepsItOutOfTopicEvidence(t *testing.T) {
	collector := &fakeCollector{
		searches: map[string][]threads.SearchResult{
			"topic": {searchResult("evidence", "creator", 100, "https://www.threads.com/@creator/post/EVIDENCE")},
		},
		profiles: map[string]*threads.Profile{
			"creator": {Username: "creator", FollowerCount: 1000, FollowerCountAvailable: true, URL: "https://www.threads.com/@creator"},
		},
		baselines: map[string][]threads.Post{
			"creator": {
				threadPost("baseline-1", "creator", 10),
				threadPost("baseline-2", "creator", 20),
				threadPost("baseline-3", "creator", 30),
			},
		},
		profileErrors:  map[string]error{},
		baselineErrors: map[string]error{},
	}
	store, err := OpenStore(t.TempDir() + "/research.db")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	cfg := DefaultConfig()
	cfg.Topic = "topic"
	cfg.PerQueryLimit = 1
	cfg.MaxQueries = 1
	cfg.ProfileLimit = 10
	cfg.BaselineAuthorLimit = 1
	cfg.BaselinePostLimit = 3
	cfg.MinBaselinePosts = 3
	cfg.TopPosts = 1
	cfg.TopAuthors = 1
	report, err := Run(context.Background(), cfg, collector, fixedExpander{queries: []string{"topic"}}, store)
	if err != nil {
		t.Fatal(err)
	}
	if len(collector.baselined) != 1 || collector.baselined[0] != "creator" {
		t.Errorf("baseline fetches = %v", collector.baselined)
	}
	if len(report.TopPosts) != 1 || report.TopPosts[0].BaselineEngagement == nil || *report.TopPosts[0].BaselineEngagement != 20 {
		t.Errorf("post baseline = %+v, want independent median 20", report.TopPosts)
	}
	if report.TopPosts[0].RelativePerformance == nil || *report.TopPosts[0].RelativePerformance != 5 {
		t.Errorf("post outperformance = %+v, want 5x", report.TopPosts[0].RelativePerformance)
	}
	if report.Baseline.AuthorsWithReliableData != 1 || report.Baseline.PostsCollected != 3 {
		t.Errorf("baseline coverage = %+v", report.Baseline)
	}
	if report.PostsAnalyzed != 1 || len(report.TopAuthors) != 1 {
		t.Errorf("baseline posts leaked into evidence: posts=%d authors=%d", report.PostsAnalyzed, len(report.TopAuthors))
	}
	var baselineLinks, topicLinks int
	if err := store.db.QueryRow(`SELECT count(*) FROM research_baseline_posts`).Scan(&baselineLinks); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT count(*) FROM research_post_queries`).Scan(&topicLinks); err != nil {
		t.Fatal(err)
	}
	if baselineLinks != 3 || topicLinks != 1 {
		t.Errorf("baseline links=%d topic links=%d", baselineLinks, topicLinks)
	}
}

func TestRunMarksContextFailureAsFailed(t *testing.T) {
	collector := &fakeCollector{
		searches:     map[string][]threads.SearchResult{"topic": {}},
		searchErrors: map[string]error{"topic": context.Canceled},
		profiles:     map[string]*threads.Profile{}, profileErrors: map[string]error{},
		baselines: map[string][]threads.Post{}, baselineErrors: map[string]error{},
	}
	store, err := OpenStore(t.TempDir() + "/research.db")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	cfg := DefaultConfig()
	cfg.Topic = "topic"
	cfg.MaxQueries = 1
	report, err := Run(context.Background(), cfg, collector, fixedExpander{queries: []string{"topic"}}, store)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("run error = %v, want context canceled", err)
	}
	if report.Status != RunStatusFailed {
		t.Errorf("report status = %q, want failed", report.Status)
	}
	var status string
	var completedAt, errorMessage string
	if err := store.db.QueryRow(`SELECT status, completed_at, error_message FROM research_runs WHERE id = ?`, report.RunID).Scan(&status, &completedAt, &errorMessage); err != nil {
		t.Fatal(err)
	}
	if status != string(RunStatusFailed) || completedAt == "" || errorMessage == "" {
		t.Errorf("stored run status=%q completed_at=%q error=%q", status, completedAt, errorMessage)
	}
}

func TestRunMarksAllSearchFailuresAsFailed(t *testing.T) {
	collector := &fakeCollector{
		searches:     map[string][]threads.SearchResult{"topic": {}},
		searchErrors: map[string]error{"topic": errors.New("network unavailable")},
		profiles:     map[string]*threads.Profile{}, profileErrors: map[string]error{},
		baselines: map[string][]threads.Post{}, baselineErrors: map[string]error{},
	}
	store, err := OpenStore(t.TempDir() + "/research.db")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	cfg := DefaultConfig()
	cfg.Topic = "topic"
	cfg.MaxQueries = 1
	report, err := Run(context.Background(), cfg, collector, fixedExpander{queries: []string{"topic"}}, store)
	if err == nil || !strings.Contains(err.Error(), "all research queries failed") {
		t.Fatalf("run error = %v, want all-query failure", err)
	}
	if report.Status != RunStatusFailed {
		t.Errorf("report status = %q, want failed", report.Status)
	}
	var status string
	if err := store.db.QueryRow(`SELECT status FROM research_runs WHERE id = ?`, report.RunID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(RunStatusFailed) {
		t.Errorf("stored status = %q, want failed", status)
	}
}

func searchResult(id, username string, likes int64, permalink string) threads.SearchResult {
	return threads.SearchResult{
		ID: id, Username: username, Permalink: permalink, Text: "topic evidence " + id,
		LikeCount: intPtr(likes), Timestamp: time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC),
	}
}

func threadPost(id, username string, likes int64) threads.Post {
	return threads.Post{
		ID: id, Username: username, Shortcode: id, Permalink: "https://www.threads.com/@" + username + "/post/" + id,
		LikeCount: likes, LikeCountAvailable: true,
		Timestamp: time.Date(2026, 9, 8, 9, 0, 0, 0, time.UTC),
	}
}

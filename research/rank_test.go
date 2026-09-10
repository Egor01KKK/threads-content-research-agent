package research

import "testing"

func TestRankUsesWeightedMetricsAndNullableRate(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Topic = "automation"
	authors := map[string]Author{
		"small":   {Username: "small", FollowerCount: intPtr(100)},
		"large":   {Username: "large", FollowerCount: intPtr(100000)},
		"unknown": {Username: "unknown"},
	}
	posts := []Post{
		{ID: "weighted", AuthorUsername: "small", Likes: intPtr(10), Replies: intPtr(2), Reposts: intPtr(1), Quotes: intPtr(1)},
		{ID: "large-post", AuthorUsername: "large", Likes: intPtr(100)},
		{ID: "unknown-post", AuthorUsername: "unknown", Likes: intPtr(5)},
	}
	queries := map[string][]string{"weighted": {"automation"}}
	ranked, _ := Rank(posts, authors, queries, BaselineData{}, cfg)
	byID := map[string]RankedPost{}
	for _, post := range ranked {
		byID[post.Post.ID] = post
	}
	if got := byID["weighted"].Engagement; got != 20 {
		t.Errorf("weighted engagement = %v, want 20", got)
	}
	if got := byID["weighted"].EngagementRate; got == nil || *got != 0.2 {
		t.Errorf("weighted engagement rate = %v, want 0.2", got)
	}
	if byID["unknown-post"].EngagementRate != nil {
		t.Error("unknown follower count must produce nil engagement rate")
	}
	if byID["weighted"].KnownMetricCount != 4 || byID["weighted"].MetricCoverage.Percent != 80 {
		t.Errorf("metric coverage = %+v, want 4/5", byID["weighted"].MetricCoverage)
	}
	if byID["weighted"].ScoringCoverage >= 1 {
		t.Errorf("missing baseline should be visible in scoring coverage: %v", byID["weighted"].ScoringCoverage)
	}
}

func TestRankDoesNotInventRateOrOutperformanceForUnknownEngagement(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Topic = "topic"
	baseline := BaselineData{
		PostsByAuthor: map[string][]Post{
			"creator": {{ID: "recent", Likes: intPtr(10)}},
		},
		MinPosts: 1,
	}
	ranked, _ := Rank(
		[]Post{{ID: "unknown", AuthorUsername: "creator"}},
		map[string]Author{"creator": {Username: "creator", FollowerCount: intPtr(100)}},
		nil,
		baseline,
		cfg,
	)
	if len(ranked) != 1 {
		t.Fatalf("ranked posts = %d, want 1", len(ranked))
	}
	if ranked[0].EngagementRate != nil || ranked[0].RelativePerformance != nil {
		t.Errorf("unknown engagement produced derived values: %+v", ranked[0])
	}
	if ranked[0].MetricCoverage.Known != 0 || ranked[0].ScoringCoverage != 0 {
		t.Errorf("unknown engagement coverage = %+v", ranked[0])
	}
}

func TestRankUsesIndependentRecentAuthorBaseline(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Topic = "topic"
	posts := []Post{
		{ID: "low", AuthorUsername: "creator", Likes: intPtr(10)},
		{ID: "high", AuthorUsername: "creator", Likes: intPtr(100)},
	}
	baseline := BaselineData{
		PostsByAuthor: map[string][]Post{
			"creator": {{ID: "recent-1", Likes: intPtr(10)}, {ID: "recent-2", Likes: intPtr(20)}, {ID: "recent-3", Likes: intPtr(30)}},
		},
		MinPosts: 3,
	}
	ranked, authors := Rank(posts, map[string]Author{"creator": {Username: "creator"}}, nil, baseline, cfg)
	byID := map[string]RankedPost{}
	for _, post := range ranked {
		byID[post.Post.ID] = post
	}
	if byID["low"].BaselineEngagement == nil || *byID["low"].BaselineEngagement != 20 {
		t.Errorf("low baseline = %v, want 20", byID["low"].BaselineEngagement)
	}
	if byID["high"].RelativePerformance == nil || *byID["high"].RelativePerformance != 5 {
		t.Errorf("high outperformance = %v, want 5", byID["high"].RelativePerformance)
	}
	if byID["low"].RelativePerformance == nil || *byID["low"].RelativePerformance != 0.5 {
		t.Errorf("low outperformance = %v, want 0.5", byID["low"].RelativePerformance)
	}
	if byID["high"].BaselineConfidence != baselineUsable || byID["high"].BaselinePostCount != 3 {
		t.Errorf("baseline metadata = confidence %q count %d", byID["high"].BaselineConfidence, byID["high"].BaselinePostCount)
	}
	if len(authors) != 1 || authors[0].MedianRelativePerformance == nil || *authors[0].MedianRelativePerformance == 1 {
		t.Errorf("author outperformance = %+v; it must not be constant by construction", authors)
	}
}

func TestRankMarksInsufficientBaselineLimited(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Topic = "topic"
	ranked, _ := Rank(
		[]Post{{ID: "post", AuthorUsername: "creator", Likes: intPtr(100)}},
		map[string]Author{"creator": {Username: "creator"}},
		nil,
		BaselineData{PostsByAuthor: map[string][]Post{"creator": {{ID: "recent", Likes: intPtr(10)}}}, MinPosts: 3},
		cfg,
	)
	if len(ranked) != 1 || ranked[0].RelativePerformance == nil || *ranked[0].RelativePerformance != 10 {
		t.Errorf("limited baseline outperformance = %+v, want 10x", ranked)
	}
	if ranked[0].BaselineConfidence != baselineLimited {
		t.Errorf("confidence = %q, want limited", ranked[0].BaselineConfidence)
	}
}

func TestRankMarksZeroBaselineUnavailable(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Topic = "topic"
	ranked, _ := Rank(
		[]Post{{ID: "post", AuthorUsername: "creator", Likes: intPtr(10)}},
		map[string]Author{"creator": {Username: "creator"}},
		nil,
		BaselineData{PostsByAuthor: map[string][]Post{"creator": {{ID: "recent", Likes: intPtr(0)}}}, MinPosts: 1},
		cfg,
	)
	if len(ranked) != 1 || ranked[0].RelativePerformance != nil || ranked[0].BaselineConfidence != baselineUnavailable {
		t.Errorf("zero baseline metadata = %+v; want unavailable outperformance", ranked)
	}
}

func TestRankHighlightsSmallCreatorAfterFollowerEnrichment(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Topic = "topic"
	posts := []Post{
		{ID: "small-post", AuthorUsername: "small", Likes: intPtr(50)},
		{ID: "large-post", AuthorUsername: "large", Likes: intPtr(500)},
	}
	authors := map[string]Author{
		"small": {Username: "small", FollowerCount: intPtr(100)},
		"large": {Username: "large", FollowerCount: intPtr(100000)},
	}
	baseline := BaselineData{PostsByAuthor: map[string][]Post{
		"small": {{ID: "small-recent-1", Likes: intPtr(10)}, {ID: "small-recent-2", Likes: intPtr(10)}, {ID: "small-recent-3", Likes: intPtr(10)}},
		"large": {{ID: "large-recent-1", Likes: intPtr(1000)}, {ID: "large-recent-2", Likes: intPtr(1000)}, {ID: "large-recent-3", Likes: intPtr(1000)}},
	}, MinPosts: 3}
	_, rankedAuthors := Rank(posts, authors, nil, baseline, cfg)
	if len(rankedAuthors) != 2 || rankedAuthors[0].Author.Username != "small" {
		t.Errorf("creator ranking = %+v; small creator should lead on rate", rankedAuthors)
	}
}

func TestRankPenalizesIncompleteScoringCoverageWithoutImputingMetrics(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Topic = "topic"
	posts := []Post{
		{ID: "partial", AuthorUsername: "creator", Likes: intPtr(10)},
		{ID: "full", AuthorUsername: "creator", Likes: intPtr(10), Replies: intPtr(0), Reposts: intPtr(0), Quotes: intPtr(0)},
	}
	baseline := BaselineData{PostsByAuthor: map[string][]Post{
		"creator": {{ID: "recent-1", Likes: intPtr(10), Replies: intPtr(0), Reposts: intPtr(0), Quotes: intPtr(0)}, {ID: "recent-2", Likes: intPtr(10), Replies: intPtr(0), Reposts: intPtr(0), Quotes: intPtr(0)}, {ID: "recent-3", Likes: intPtr(10), Replies: intPtr(0), Reposts: intPtr(0), Quotes: intPtr(0)}},
	}, MinPosts: 3}
	ranked, _ := Rank(posts, map[string]Author{"creator": {Username: "creator", FollowerCount: intPtr(100)}}, nil, baseline, cfg)
	byID := map[string]RankedPost{}
	for _, post := range ranked {
		byID[post.Post.ID] = post
	}
	if byID["partial"].MetricCoverage.Known != 1 || byID["full"].MetricCoverage.Known != 4 {
		t.Errorf("coverage was not preserved: partial=%+v full=%+v", byID["partial"].MetricCoverage, byID["full"].MetricCoverage)
	}
	if byID["partial"].ScoringCoverage >= byID["full"].ScoringCoverage || byID["partial"].RankScore >= byID["full"].RankScore {
		t.Errorf("incomplete evidence was not discounted: partial=%+v full=%+v", byID["partial"], byID["full"])
	}
}

func intPtr(value int64) *int64 { return &value }

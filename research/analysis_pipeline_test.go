package research

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Egor01KKK/threads-content-research-agent/threads"
)

type fakeAnalysisProvider struct {
	calls   int
	fail    error
	failAt  int
	partial bool
}

func (p *fakeAnalysisProvider) Name() string  { return "fake" }
func (p *fakeAnalysisProvider) Model() string { return "fixture-model" }

func (p *fakeAnalysisProvider) AnalyzePosts(_ context.Context, inputs []AnalysisInput) (AnalysisBatch, error) {
	p.calls++
	if p.fail != nil && (p.failAt == 0 || p.calls == p.failAt) {
		return AnalysisBatch{}, p.fail
	}
	if p.partial && len(inputs) > 1 {
		inputs = inputs[:1]
	}
	analyses := make([]PostAnalysis, 0, len(inputs))
	for _, input := range inputs {
		analysis := testPostAnalysis(input.PostID)
		analysis.Mechanics = WritingMechanics{CharacterCount: 999} // pipeline must replace model-side mechanics
		analyses = append(analyses, analysis)
	}
	return AnalysisBatch{
		Analyses: analyses, RawResponse: `{"fixture":true}`,
		Usage: AnalysisUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
	}, nil
}

func (p *fakeAnalysisProvider) AnalyzeAggregatePatterns(context.Context, AggregatePatternInput) (AggregatePatternResult, error) {
	return AggregatePatternResult{Summary: "fixture"}, nil
}

func TestRunWithAnalysisBatchesStoresPerformanceAndReusesVersionedCache(t *testing.T) {
	collector := analysisFixtureCollector()
	store, err := OpenStore(t.TempDir() + "/research.db")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	provider := &fakeAnalysisProvider{}
	cfg := analysisFixtureConfig()
	var firstPlan AnalysisPlan
	first, err := RunWithAnalysis(context.Background(), cfg, collector, fixedExpander{queries: []string{"topic"}}, store, AnalysisOptions{
		Enabled: true, Provider: provider, PostsLimit: 10, BatchSize: 2, BeforeAnalyze: func(plan AnalysisPlan) { firstPlan = plan },
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 2 || firstPlan.PostsEligible != 3 || firstPlan.PostsToSend != 3 || firstPlan.BatchCount != 2 {
		t.Errorf("first analysis calls/plan = %d/%+v", provider.calls, firstPlan)
	}
	if first.Analysis.Coverage.PostsClassified != 3 || first.Analysis.Coverage.PostsWithReliableBaseline != 3 {
		t.Errorf("first analysis coverage = %+v", first.Analysis.Coverage)
	}
	if first.Analysis.Classifications[0].Analysis.Mechanics.CharacterCount == 999 {
		t.Error("provider mechanics were not replaced by deterministic mechanics")
	}
	if first.Analysis.Classifications[0].Text == "" {
		t.Error("source post text was not retained for manual audit")
	}
	if first.Analysis.Plan.ActualInputTokens != 20 || first.Analysis.Plan.ActualOutputTokens != 10 || first.Analysis.Plan.ActualTotalTokens != 30 {
		t.Errorf("actual token usage = %+v", first.Analysis.Plan)
	}

	var stored, linked int
	if err := store.db.QueryRow(`SELECT count(*) FROM research_classifications`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT count(*) FROM research_classification_runs WHERE run_id = ?`, first.RunID).Scan(&linked); err != nil {
		t.Fatal(err)
	}
	if stored != 3 || linked != 3 {
		t.Errorf("classification storage = rows %d links %d", stored, linked)
	}
	var raw, schema, prompt string
	if err := store.db.QueryRow(`SELECT raw_response, schema_version, prompt_version FROM research_classifications LIMIT 1`).Scan(&raw, &schema, &prompt); err != nil {
		t.Fatal(err)
	}
	if raw == "" || schema != AnalysisSchemaVersion || prompt != AnalysisPromptVersion {
		t.Errorf("stored metadata raw=%q schema=%q prompt=%q", raw, schema, prompt)
	}

	var secondPlan AnalysisPlan
	second, err := RunWithAnalysis(context.Background(), cfg, collector, fixedExpander{queries: []string{"topic"}}, store, AnalysisOptions{
		Enabled: true, Provider: provider, PostsLimit: 10, BatchSize: 2, BeforeAnalyze: func(plan AnalysisPlan) { secondPlan = plan },
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 2 || secondPlan.PostsCached != 3 || secondPlan.PostsToSend != 0 || secondPlan.BatchCount != 0 {
		t.Errorf("cache was not reused: calls=%d plan=%+v", provider.calls, secondPlan)
	}
	if second.Analysis.Coverage.PostsClassified != 3 {
		t.Errorf("cached analysis coverage = %+v", second.Analysis.Coverage)
	}
	if err := store.db.QueryRow(`SELECT count(*) FROM research_classifications`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 3 {
		t.Errorf("cache created duplicate rows: %d", stored)
	}
}

func TestRunWithAnalysisPreservesCompletedBatchesAndResumesAfterFailure(t *testing.T) {
	collector := analysisFixtureCollector()
	store, err := OpenStore(t.TempDir() + "/research.db")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	provider := &fakeAnalysisProvider{fail: errors.New("rate limited"), failAt: 2}

	first, runErr := RunWithAnalysis(context.Background(), analysisFixtureConfig(), collector, fixedExpander{queries: []string{"topic"}}, store, AnalysisOptions{
		Enabled: true, Provider: provider, PostsLimit: 10, BatchSize: 2,
	})
	if runErr != nil {
		t.Fatalf("partial provider failure should preserve usable run: %v", runErr)
	}
	if first.Status != RunStatusCompletedWithWarns || first.Analysis.Coverage.PostsClassified != 2 {
		t.Errorf("first partial run = status %q coverage %+v", first.Status, first.Analysis.Coverage)
	}
	var stored int
	if err := store.db.QueryRow(`SELECT count(*) FROM research_classifications`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 2 {
		t.Fatalf("completed batch was not persisted: %d classifications", stored)
	}

	provider.fail = nil
	provider.failAt = 0
	second, runErr := RunWithAnalysis(context.Background(), analysisFixtureConfig(), collector, fixedExpander{queries: []string{"topic"}}, store, AnalysisOptions{
		Enabled: true, Provider: provider, PostsLimit: 10, BatchSize: 2,
	})
	if runErr != nil {
		t.Fatal(runErr)
	}
	if second.Analysis.Plan.PostsCached != 2 || second.Analysis.Plan.PostsToSend != 1 || second.Analysis.Coverage.PostsClassified != 3 {
		t.Errorf("resume plan/coverage = %+v / %+v", second.Analysis.Plan, second.Analysis.Coverage)
	}
	if err := store.db.QueryRow(`SELECT count(*) FROM research_classifications`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 3 {
		t.Errorf("resume created duplicate classification rows: %d", stored)
	}
}

func TestRunWithAnalysisMarksInterruptedProviderRunFailedAfterPersistingPriorBatches(t *testing.T) {
	collector := analysisFixtureCollector()
	store, err := OpenStore(t.TempDir() + "/research.db")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	provider := &fakeAnalysisProvider{fail: context.Canceled, failAt: 2}

	report, runErr := RunWithAnalysis(context.Background(), analysisFixtureConfig(), collector, fixedExpander{queries: []string{"topic"}}, store, AnalysisOptions{
		Enabled: true, Provider: provider, PostsLimit: 10, BatchSize: 2,
	})
	if !errors.Is(runErr, context.Canceled) {
		t.Fatalf("run error = %v, want context canceled", runErr)
	}
	if report.Status != RunStatusFailed || report.Analysis.Coverage.PostsClassified != 2 {
		t.Errorf("interrupted run = status %q coverage %+v", report.Status, report.Analysis.Coverage)
	}
	var stored int
	if err := store.db.QueryRow(`SELECT count(*) FROM research_classifications`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 2 {
		t.Errorf("prior successful batch was not preserved after interruption: %d", stored)
	}
}

func TestRunWithAnalysisKeepsPartialAndFailedProviderRunsUsable(t *testing.T) {
	for _, test := range []struct {
		name        string
		provider    *fakeAnalysisProvider
		wantClass   int
		wantWarning string
	}{
		{name: "partial", provider: &fakeAnalysisProvider{partial: true}, wantClass: 1, wantWarning: "partial"},
		{name: "failed", provider: &fakeAnalysisProvider{fail: errors.New("provider unavailable")}, wantClass: 0, wantWarning: "batch 1/1 failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			collector := analysisFixtureCollector()
			store, err := OpenStore(t.TempDir() + "/research.db")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = store.Close() }()
			report, runErr := RunWithAnalysis(context.Background(), analysisFixtureConfig(), collector, fixedExpander{queries: []string{"topic"}}, store, AnalysisOptions{
				Enabled: true, Provider: test.provider, PostsLimit: 10, BatchSize: 10,
			})
			if runErr != nil {
				t.Fatalf("run error = %v; provider failure must be non-fatal", runErr)
			}
			if report.PostsAnalyzed != 3 || report.Analysis.Coverage.PostsClassified != test.wantClass {
				t.Errorf("usable collection/analysis = posts %d coverage %+v", report.PostsAnalyzed, report.Analysis.Coverage)
			}
			found := false
			for _, warning := range report.Warnings {
				if strings.Contains(warning, test.wantWarning) {
					found = true
				}
			}
			if !found {
				t.Errorf("warning %q missing from %v", test.wantWarning, report.Warnings)
			}
		})
	}
}

func analysisFixtureConfig() Config {
	cfg := DefaultConfig()
	cfg.Topic = "topic"
	cfg.PerQueryLimit = 3
	cfg.MaxQueries = 1
	cfg.ProfileLimit = 1
	cfg.BaselineAuthorLimit = 1
	cfg.BaselinePostLimit = 3
	cfg.MinBaselinePosts = 3
	cfg.TopPosts = 3
	cfg.TopAuthors = 1
	return cfg
}

func analysisFixtureCollector() *fakeCollector {
	return &fakeCollector{
		searches: map[string][]threads.SearchResult{
			"topic": {
				searchResult("p1", "creator", 100, "https://threads/p1"),
				searchResult("p2", "creator", 80, "https://threads/p2"),
				searchResult("p3", "creator", 60, "https://threads/p3"),
			},
		},
		profiles: map[string]*threads.Profile{
			"creator": {Username: "creator", FollowerCount: 1000, FollowerCountAvailable: true, URL: "https://threads/@creator"},
		},
		baselines: map[string][]threads.Post{
			"creator": {threadPost("b1", "creator", 10), threadPost("b2", "creator", 20), threadPost("b3", "creator", 30)},
		},
		profileErrors: map[string]error{}, baselineErrors: map[string]error{},
	}
}

package research

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Egor01KKK/threads-content-research-agent/threads"
)

func TestDeepAnchorBankIsShortAndTargeted(t *testing.T) {
	anchors := DeepAnchorBank()
	if len(anchors) < 160 || len(anchors) > 240 {
		t.Fatalf("anchor bank size = %d, want 160..240", len(anchors))
	}
	for _, anchor := range anchors {
		if n := deepAnchorTokenCount(anchor.Query); n < 1 || n > 3 {
			t.Errorf("anchor %q has %d tokens", anchor.Query, n)
		}
		if strings.Contains(anchor.Query, "актуальные проблемы") || strings.Contains(anchor.Query, "которые можно решить") {
			t.Errorf("long objective leaked into anchor bank: %q", anchor.Query)
		}
	}
}

func TestDeepRunUsesProfileFirstCorpusAndExportsEvidence(t *testing.T) {
	owner := searchResult("deep-owner", "owner-one", 25, "https://www.threads.com/@owner-one/post/deep-owner")
	owner.Text = "У меня свой бизнес. Заявки теряются, веду клиентов в Excel. Какую CRM выбрать?"
	collector := &fakeCollector{
		searches: map[string][]threads.SearchResult{
			"бизнес": {owner},
		},
		profiles: map[string]*threads.Profile{
			"owner-one": {Username: "owner-one", Name: "Owner", FollowerCount: 100, FollowerCountAvailable: true, URL: "https://www.threads.com/@owner-one"},
		},
		profileErrors: map[string]error{},
		baselines: map[string][]threads.Post{
			"owner-one": {
				threadPost("deep-owner", "owner-one", 25),
				threadPost("normal-1", "owner-one", 2),
				threadPost("normal-2", "owner-one", 3),
				threadPost("normal-3", "owner-one", 4),
			},
		},
		baselineErrors: map[string]error{},
	}
	dbPath := filepath.Join(t.TempDir(), "deep.db")
	store, err := OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	cfg := DefaultConfig()
	cfg.Topic = "проблемы малого бизнеса которые можно решить автоматизацией и IT"
	cfg.Mode = ModeDeep
	cfg.AnchorLimit = 3
	cfg.MaxSeedProfiles = 1
	cfg.ProfilePostLimit = 4
	cfg.MaxTotalProfilePosts = 4
	cfg.ProfileDelay = 0
	cfg.ReplyDelay = 0
	cfg.TopPosts = 3
	cfg.TopAuthors = 3
	report, err := Run(context.Background(), cfg, collector, nil, store)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Deep.AnonymousCollection || report.Deep.AnalysisProviderCalled {
		t.Fatalf("deep safety flags = anonymous=%t provider=%t", report.Deep.AnonymousCollection, report.Deep.AnalysisProviderCalled)
	}
	if len(collector.profiled) != 1 || collector.profiled[0] != "owner-one" {
		t.Fatalf("profile-first seeds = %v", collector.profiled)
	}
	for _, query := range collector.searched {
		if strings.Contains(query, cfg.Topic) {
			t.Fatalf("long objective was searched: %q", query)
		}
	}
	if len(report.Deep.Corpus) == 0 || len(report.Deep.PainSignals) == 0 || len(report.Deep.GoldSignals) == 0 {
		t.Fatalf("deep evidence corpus=%d pain=%d gold=%d", len(report.Deep.Corpus), len(report.Deep.PainSignals), len(report.Deep.GoldSignals))
	}
	if report.Deep.Funnel.ProfilePostsRaw != 4 || report.Deep.Funnel.ProfilePostsUnique != 4 {
		t.Errorf("profile funnel = %+v", report.Deep.Funnel)
	}
	if report.Deep.Funnel.OwnerLikelyPosts == 0 || report.Deep.Funnel.ITActionablePosts == 0 {
		t.Errorf("pain funnel = %+v", report.Deep.Funnel)
	}
	if report.Deep.Funnel.VerifiedOwnerProfiles != 1 || report.Deep.Funnel.ProfilesRejected != 0 {
		t.Errorf("owner verification funnel = %+v", report.Deep.Funnel)
	}
	exportDir := filepath.Join(t.TempDir(), "export")
	if err := ExportDeepArtifacts(report, exportDir, dbPath); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"complete-readable.json", "corpus.jsonl", "owners.json", "verified-owners.json", "rejected-owner-seeds.json", "owner-discovery-summary.json", "pain-signals.json", "gold-signals.json", "queries.json", "run-summary.json", "research.db"} {
		if _, err := os.Stat(filepath.Join(exportDir, name)); err != nil {
			t.Errorf("missing export %s: %v", name, err)
		}
	}
	if report.CompletedAt.Before(report.StartedAt) || report.Status == RunStatusRunning {
		t.Errorf("terminal report = status=%q started=%s completed=%s", report.Status, report.StartedAt, report.CompletedAt)
	}
}

func TestDeepOwnerVerificationRequiresCredibleOperatorContext(t *testing.T) {
	salon := verifyDeepOwnerProfile(Author{
		Username: "salon-owner",
		Name:     "Мой салон красоты",
		Bio:      "Владелица салона красоты. Запись клиентов и команда мастеров",
	}, []string{
		"У нас неявки клиентов, подтверждаем запись вручную.",
	})
	if !salon.Verified || salon.Confidence != "HIGH" {
		t.Fatalf("salon verification = %+v", salon)
	}
	if !containsString(salon.BusinessCategories, "beauty_wellness") {
		t.Fatalf("salon categories = %v", salon.BusinessCategories)
	}

	course := verifyDeepOwnerProfile(Author{
		Username: "course-seller",
		Bio:      "Английский онлайн. Уроки и курсы для взрослых",
	}, []string{
		"Набираю учеников на занятия.",
	})
	if course.Verified {
		t.Fatalf("course-only profile was accepted: %+v", course)
	}
	if !containsString(course.NegativeEvidence, "education_or_course_only_without_operating_evidence") {
		t.Fatalf("course rejection evidence = %v", course.NegativeEvidence)
	}
}

func TestDeepPaginationMaterializationKeepsCountsHonest(t *testing.T) {
	state := newDeepState()
	state.profilePosts["profile-post"] = true
	state.profiles["owner-one"] = DeepSeedProfile{
		Username:       "owner-one",
		Selected:       true,
		ProfileFetched: true,
		ProfilePosts:   2,
	}

	profilePagination := rebuildDeepProfilePagination(DeepPaginationMetrics{}, state, Config{ProfilePostLimit: 4})
	if profilePagination.Requests != 1 || profilePagination.PagesFetched != 1 ||
		profilePagination.RawYield != 2 || profilePagination.UniqueYield != 1 ||
		profilePagination.Duplicates != 1 || !profilePagination.Exhausted {
		t.Fatalf("profile pagination = %+v", profilePagination)
	}

	probe := DeepAnchorProbe{
		ProbeLimit:  3,
		Status:      "completed",
		UniquePosts: 2,
		Duplicates:  1,
		QueryReport: QueryReport{ResultCount: 3},
		Pagination:  DeepPaginationMetrics{UniqueYield: 4},
	}
	searchPagination := normalizeDeepProbePagination(probe)
	if searchPagination.RawYield != 3 || searchPagination.UniqueYield != 2 ||
		searchPagination.Duplicates != 1 || !searchPagination.StoppedAtCeiling || searchPagination.Exhausted {
		t.Fatalf("probe pagination = %+v", searchPagination)
	}

	merged := mergePagination(DeepPaginationMetrics{}, searchPagination)
	merged = mergePagination(merged, DeepPaginationMetrics{
		Requests:     1,
		PagesFetched: 1,
		Exhausted:    true,
		StopReason:   "ssr_window_exhausted_or_empty",
	})
	if merged.Exhausted || !merged.StoppedAtCeiling || merged.StopReason != "mixed_bounded_windows" {
		t.Fatalf("aggregate pagination = %+v", merged)
	}
}

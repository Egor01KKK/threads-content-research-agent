package research

import (
	"bytes"
	"strings"
	"testing"
)

func TestReportIncludesEvidenceLinksAndMissingData(t *testing.T) {
	report := Report{
		Topic:                   "automation",
		PostsAnalyzed:           1,
		AuthorsDiscovered:       1,
		Queries:                 []QueryReport{{Query: "automation", ResultCount: 1, UniqueNewPosts: 1}},
		PostsWithKnownFollowers: 0,
		TopPosts: []RankedPost{{
			Post:       Post{ID: "1", AuthorUsername: "ada", Text: "a useful post", URL: "https://www.threads.com/@ada/post/ABC123"},
			Engagement: 10, BaselinePostCount: 1, BaselineConfidence: baselineLimited,
		}},
		TopAuthors: []RankedAuthor{{
			Author:            Author{Username: "ada", ProfileURL: "https://www.threads.com/@ada"},
			RelevantPostCount: 1, BaselineConfidence: baselineLimited, BaselinePostCount: 1,
			EvidenceURLs: []string{"https://www.threads.com/@ada/post/ABC123"},
		}},
		SignalCounts: SignalCounts{
			RelevantPosts:         1,
			BuyerHiringSignals:    1,
			PotentialBuyerSignals: 1,
		},
		LeadBuyerSignals: []RankedPost{{
			Post:                     Post{AuthorUsername: "ada", Text: "Looking for a designer", URL: "https://www.threads.com/@ada/post/ABC123", RelevanceLabel: string(RelevanceRelevant)},
			SignalTypes:              []SignalType{SignalBuyerHiring},
			PrimarySignal:            SignalBuyerHiring,
			CommercialIntentPriority: 1,
			Need:                     "hire or book a designer",
			RequestedRole:            "designer",
			Engagement:               10,
			BaselineConfidence:       baselineLimited,
			MetricCoverage:           MetricCoverage{Known: 4, Total: 5, Percent: 80},
		}},
	}
	var buf bytes.Buffer
	if err := report.WriteText(&buf); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	for _, want := range []string{
		"TOP POSTS", "TOP CREATORS", "https://www.threads.com/@ada/post/ABC123",
		"https://www.threads.com/@ada", "rate=n/a", "baseline_posts=1 (limited)",
		"Not run; use --analyze", "LIMITATIONS", "SIGNAL COUNTS", "LEAD / BUYER SIGNALS",
		"BUYER_HIRING_SIGNAL", "priority=1", "original_text:",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("report missing %q:\n%s", want, output)
		}
	}
}

func TestReportRendersContentIntelligenceSectionsAndEvidenceStats(t *testing.T) {
	classified := []ClassifiedPost{
		classifiedFixture("p1", "ada", "https://threads/p1", "finding clients is hard", "How can I find clients?", 2),
		classifiedFixture("p2", "bob", "https://threads/p2", "can't find customers", "Where do I find customers?", 4),
		classifiedFixture("p3", "cy", "https://threads/p3", "client acquisition struggles", "How do I acquire clients?", 6),
	}
	report := Report{
		Topic: "finding clients",
		Analysis: AnalysisReport{
			Enabled:  true,
			Plan:     AnalysisPlan{Enabled: true, Provider: "fake", Model: "fixture", PostsEligible: 3, PostsToSend: 3, BatchCount: 1},
			Coverage: analysisCoverage(3, 3, classified),
		},
	}
	report.Analysis = BuildAnalysisReport(report.Topic, classified)
	report.Analysis.Plan = AnalysisPlan{Enabled: true, Provider: "fake", Model: "fixture", PostsEligible: 3, PostsToSend: 3, BatchCount: 1}
	report.Analysis.Coverage = analysisCoverage(3, 3, classified)
	var buf bytes.Buffer
	if err := report.WriteText(&buf); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	for _, want := range []string{
		"CONTENT INTELLIGENCE", "CLASSIFICATION BREAKDOWN: CONTENT TYPES", "HOOK TYPES", "STRUCTURES",
		"TONES", "INTENTS", "TOPICS", "COMBINATIONS", "PAIN CLUSTERS", "QUESTION CLUSTERS",
		"CONTENT OPPORTUNITIES", "PRODUCT/SERVICE PAIN RADAR", "WEAK SIGNALS", "N=3", "median=4.00x",
		"https://threads/p1",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("analysis report missing %q:\n%s", want, output)
		}
	}
}

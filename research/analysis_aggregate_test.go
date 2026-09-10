package research

import (
	"strings"
	"testing"
)

func TestBuildAnalysisReportAggregatesEvidenceAndKeepsWeakSignalsVisible(t *testing.T) {
	posts := []ClassifiedPost{
		classifiedFixture("p1", "ada", "https://threads/p1", "finding clients is hard", "How can I find clients?", 2),
		classifiedFixture("p2", "bob", "https://threads/p2", "can't find customers", "Where do I find customers?", 4),
		classifiedFixture("p3", "cy", "https://threads/p3", "client acquisition struggles", "How do I acquire clients?", 6),
		classifiedFixture("p4", "dee", "https://threads/p4", "n8n is confusing and expensive", "What is a simpler option?", 0),
		classifiedFixture("p5", "eli", "https://threads/p5", "", "", 0),
		classifiedFixture("p6", "fox", "https://threads/p6", "", "", 0),
	}
	posts[3].Analysis.ContentType = "opinion"
	posts[3].Analysis.HookType = "question"
	posts[3].Analysis.Structure = "question_discussion"
	posts[3].Analysis.Intent = "start_discussion"
	posts[3].Analysis.Pains = []string{"n8n is confusing and expensive"}
	posts[3].Analysis.Questions = []string{"What is a simpler option?"}
	posts[4].Analysis.ContentType = "opinion"
	posts[4].Analysis.HookType = "observation"
	posts[4].Analysis.Structure = "observation_opinion"
	posts[4].Analysis.Intent = "express_opinion"
	posts[5].Analysis.ContentType = "question"
	posts[5].Analysis.HookType = "question"
	posts[5].Analysis.Structure = "question_discussion"
	posts[5].Analysis.Intent = "start_discussion"

	report := BuildAnalysisReport("finding clients", posts)
	content := findPattern(report.ContentTypes, "educational")
	if content == nil || content.N != 3 || content.SignalStrength != "weak" {
		t.Fatalf("content type statistic = %+v", content)
	}
	if content.MedianRelativePerformance == nil || *content.MedianRelativePerformance != 4 {
		t.Errorf("median = %+v, want 4", content.MedianRelativePerformance)
	}
	if content.Q1RelativePerformance == nil || *content.Q1RelativePerformance != 2 || content.Q3RelativePerformance == nil || *content.Q3RelativePerformance != 6 {
		t.Errorf("quartiles = q1=%v q3=%v", content.Q1RelativePerformance, content.Q3RelativePerformance)
	}
	if content.ReliableBaselinePercent != 100 {
		t.Errorf("reliable baseline percent = %v", content.ReliableBaselinePercent)
	}
	if len(content.EvidenceURLs) != 3 {
		t.Errorf("evidence URLs = %v", content.EvidenceURLs)
	}

	if len(report.Pains) == 0 || report.Pains[0].Count != 3 {
		t.Fatalf("pain clusters = %+v", report.Pains)
	}
	if !strings.Contains(report.Pains[0].Description, "Normalized pain") || len(report.Pains[0].Authors) != 3 || len(report.Pains[0].EvidenceURLs) != 3 {
		t.Errorf("pain cluster evidence = %+v", report.Pains[0])
	}
	if len(report.Questions) == 0 || report.Questions[0].Count != 3 {
		t.Fatalf("question clusters = %+v", report.Questions)
	}
	if len(report.Opportunities) < 2 {
		t.Errorf("opportunities = %+v, want evidence-backed pain and question prompts", report.Opportunities)
	}
	if len(report.ProductPainRadar) == 0 {
		t.Fatal("product/service pain radar is empty for explicit n8n friction")
	}
	var foundClient, foundTool bool
	for _, signal := range report.ProductPainRadar {
		if signal.UniqueAuthors == 0 || signal.Count == 0 {
			t.Errorf("pain radar lacks frequency/author evidence: %+v", signal)
		}
		if signal.Category == "client_acquisition_problem" {
			foundClient = true
			if signal.Count != 3 || signal.ExplicitSolutionSeeking != 3 || signal.BuyerIntent != 3 {
				t.Errorf("client pain radar signal = %+v", signal)
			}
		}
		if signal.Category == "dissatisfaction_existing_tool" {
			foundTool = true
			if signal.ExistingToolComplaints == 0 || signal.MedianEngagement == nil {
				t.Errorf("tool friction radar signal = %+v", signal)
			}
		}
	}
	if !foundClient || !foundTool {
		t.Errorf("expected client and existing-tool radar categories: %+v", report.ProductPainRadar)
	}
	if len(report.WeakSignals) == 0 {
		t.Error("weak signals are empty")
	}
	for _, opportunity := range report.Opportunities {
		if !strings.Contains(strings.ToLower(opportunity.Rationale), "evidence prompt") && !strings.Contains(strings.ToLower(opportunity.Rationale), "historical evidence only") {
			t.Errorf("opportunity lacks an evidence-only disclaimer: %+v", opportunity)
		}
	}
}

func TestSignalStrengthThresholdsAreExplicitSampleLabels(t *testing.T) {
	for _, test := range []struct {
		n    int
		want string
	}{
		{2, "insufficient"}, {3, "weak"}, {5, "weak"}, {6, "moderate"}, {10, "moderate"}, {11, "stronger"},
	} {
		if got := signalStrength(test.n); got != test.want {
			t.Errorf("signalStrength(%d) = %q, want %q", test.n, got, test.want)
		}
	}
}

func classifiedFixture(id, author, url, pain, question string, relative float64) ClassifiedPost {
	analysis := testPostAnalysis(id)
	analysis.Pains = nil
	analysis.Questions = nil
	if pain != "" {
		analysis.Pains = []string{pain}
	}
	if question != "" {
		analysis.Questions = []string{question}
	}
	post := ClassifiedPost{PostID: id, AuthorUsername: author, URL: url, Analysis: analysis}
	post.Performance.BaselineConfidence = baselineUsable
	if relative > 0 {
		post.Performance.RelativePerformance = &relative
	}
	post.Performance.MetricCoverage = MetricCoverage{Known: 5, Total: 5, Percent: 100}
	return post
}

func findPattern(items []PatternStatistic, value string) *PatternStatistic {
	for i := range items {
		if items[i].Value == value {
			return &items[i]
		}
	}
	return nil
}

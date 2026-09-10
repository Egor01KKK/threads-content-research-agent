package research

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

// WriteText renders a concise evidence-linked terminal report. Every ranking
// item includes its source permalink when Threads exposed one, and coverage
// lines make partial evidence explicit.
func (r Report) WriteText(w io.Writer) error {
	status := string(r.Status)
	if status == "" {
		status = "unknown"
	}
	if _, err := fmt.Fprintf(w, "THREADS RESEARCH\nTopic: %s\nLanguage: %s\nStatus: %s\n\n", r.Topic, r.Language, status); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "OVERVIEW\n  Posts analyzed: %d\n  Creators discovered: %d\n  Profiles: %d discovered, %d attempted, %d fetched\n  Followers known: %s\n  Follower-relative ranking: %s\n  Posts with follower-relative rate: %d/%d\n  Baseline authors: %d attempted, %d reliable\n  Baseline posts: %d collected, %d with known engagement\n\n",
		r.PostsAnalyzed, r.AuthorsDiscovered, r.ProfilesDiscovered, r.ProfilesAttempted, r.ProfilesFetched,
		formatCoverage(r.Coverage.Followers), r.FollowerRelativeRanking, r.PostsWithKnownFollowers, r.PostsAnalyzed,
		r.Baseline.AuthorsAttempted, r.Baseline.AuthorsWithReliableData,
		r.Baseline.PostsCollected, r.Baseline.PostsWithKnownEngagement); err != nil {
		return err
	}

	if err := writeCoverage(w, r.Coverage); err != nil {
		return err
	}
	if err := writeBaselineCoverage(w, r.Baseline); err != nil {
		return err
	}
	if err := writeCollectionCoverage(w, r.Collection); err != nil {
		return err
	}
	if err := writeQueries(w, r.Queries); err != nil {
		return err
	}
	if err := writePosts(w, "TOP POSTS", r.TopPosts); err != nil {
		return err
	}
	if err := writePosts(w, "TOP OUTPERFORMING POSTS", r.TopOutperformingPosts); err != nil {
		return err
	}
	if err := writePosts(w, "RAW TOP POSTS (DIAGNOSTIC)", r.RawTopPosts); err != nil {
		return err
	}
	if err := writePosts(w, "RAW TOP OUTPERFORMING POSTS (DIAGNOSTIC)", r.RawTopOutperformingPosts); err != nil {
		return err
	}
	if err := writeAuthors(w, r.TopAuthors); err != nil {
		return err
	}
	if err := writeSignalSections(w, r); err != nil {
		return err
	}
	if err := writeBusinessPainSections(w, r); err != nil {
		return err
	}
	if r.Deep != nil {
		if err := writeDeepSummary(w, *r.Deep); err != nil {
			return err
		}
	}

	if err := writeAnalysis(w, r.Analysis); err != nil {
		return err
	}
	if err := writeLimitations(w, r.Limitations); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if len(r.Warnings) > 0 {
		if _, err := fmt.Fprintln(w, "COLLECTION WARNINGS"); err != nil {
			return err
		}
		for _, warning := range r.Warnings {
			if _, err := fmt.Fprintf(w, "  - %s\n", warning); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	return nil
}

func writeDeepSummary(w io.Writer, deep DeepReport) error {
	if _, err := fmt.Fprintf(w, "DEEP PROFILE-FIRST CORPUS\n  Mode: %s | anonymous=%t | provider_called=%t\n  Anchors: %d generated, %d probed, %d with results\n  Search: %d raw posts, %d unique posts, %d unique authors\n  Profiles: %d candidates, %d seeds selected, %d inspected, %d verified, %d rejected, %d profile fetches\n  Profile posts: %d raw, %d unique | replies: %d attempted, %d collected\n  Evidence: %d Russian, %d owner-likely, %d pain, %d IT-actionable, %d solution-seeking, %d buyer-intent, %d GOLD\n  Date range: %s — %s\n\n",
		deep.Mode, deep.AnonymousCollection, deep.AnalysisProviderCalled,
		deep.Funnel.AnchorsGenerated, deep.Funnel.AnchorsProbed, deep.Funnel.AnchorsWithResults,
		deep.Funnel.RawSearchPosts, deep.Funnel.UniqueSearchPosts, deep.Funnel.UniqueSearchAuthors,
		deep.Funnel.OwnerSeedCandidates, deep.Funnel.OwnerSeedProfiles, deep.Funnel.SeedProfilesInspected, deep.Funnel.VerifiedOwnerProfiles, deep.Funnel.ProfilesRejected, deep.Funnel.ProfilesFetched,
		deep.Funnel.ProfilePostsRaw, deep.Funnel.ProfilePostsUnique, deep.Funnel.RepliesAttempted, deep.Funnel.RepliesCollected,
		deep.Funnel.RussianPosts, deep.Funnel.OwnerLikelyPosts, deep.Funnel.PainPosts, deep.Funnel.ITActionablePosts,
		deep.Funnel.SolutionSeekingPosts, deep.Funnel.BuyerIntentPosts, deep.Funnel.GoldSignals,
		formatDeepDate(deep.Funnel.DateCoverage.Earliest), formatDeepDate(deep.Funnel.DateCoverage.Latest)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Search pagination: requests=%d pages=%d raw=%d unique=%d duplicates=%d stop=%s\n  Profile pagination: requests=%d pages=%d raw=%d unique=%d duplicates=%d stop=%s\n  Reply pagination: requests=%d pages=%d raw=%d unique=%d duplicates=%d stop=%s\n\n",
		deep.SearchPagination.Requests, deep.SearchPagination.PagesFetched, deep.SearchPagination.RawYield, deep.SearchPagination.UniqueYield,
		deep.SearchPagination.Duplicates, deep.SearchPagination.StopReason,
		deep.ProfilePagination.Requests, deep.ProfilePagination.PagesFetched, deep.ProfilePagination.RawYield, deep.ProfilePagination.UniqueYield,
		deep.ProfilePagination.Duplicates, deep.ProfilePagination.StopReason,
		deep.ReplyPagination.Requests, deep.ReplyPagination.PagesFetched, deep.ReplyPagination.RawYield, deep.ReplyPagination.UniqueYield,
		deep.ReplyPagination.Duplicates, deep.ReplyPagination.StopReason); err != nil {
		return err
	}
	return nil
}

func formatDeepDate(value *time.Time) string {
	if value == nil || value.IsZero() {
		return "n/a"
	}
	return value.UTC().Format("2006-01-02")
}

func writeSignalSections(w io.Writer, report Report) error {
	if _, err := fmt.Fprintf(w, "SIGNAL COUNTS\n  Relevant posts: %d\n  BUYER_HIRING_SIGNAL: %d\n  PAID_OPPORTUNITY: %d\n  CLIENT_SEEKING: %d\n  ACQUISITION_PAIN: %d\n  ACQUISITION_QUESTION: %d\n  ACQUISITION_METHOD: %d\n  ACQUISITION_SERVICE_OFFER: %d\n  ACQUISITION_OPINION: %d\n  OTHER_RELEVANT: %d\n  Potential buyer/hiring signals: %d\n\n",
		report.SignalCounts.RelevantPosts,
		report.SignalCounts.BuyerHiringSignals,
		report.SignalCounts.PaidOpportunities,
		report.SignalCounts.ClientSeeking,
		report.SignalCounts.AcquisitionPain,
		report.SignalCounts.AcquisitionQuestions,
		report.SignalCounts.AcquisitionMethods,
		report.SignalCounts.AcquisitionServiceOffers,
		report.SignalCounts.AcquisitionOpinions,
		report.SignalCounts.OtherRelevant,
		report.SignalCounts.PotentialBuyerSignals); err != nil {
		return err
	}
	if err := writeSignalPosts(w, "LEAD / BUYER SIGNALS", report.LeadBuyerSignals); err != nil {
		return err
	}
	return writeSignalPosts(w, "CONTENT / AUDIENCE SIGNALS", report.ContentAudienceSignals)
}

func writeSignalPosts(w io.Writer, title string, posts []RankedPost) error {
	if _, err := fmt.Fprintln(w, title); err != nil {
		return err
	}
	if len(posts) == 0 {
		if _, err := fmt.Fprintln(w, "  (none)"); err != nil {
			return err
		}
		_, err := fmt.Fprintln(w)
		return err
	}
	for index, post := range posts {
		username := post.Post.AuthorUsername
		if username == "" {
			username = "unknown-author"
		}
		if _, err := fmt.Fprintf(w, "  %d. @%s | signals=%s | primary=%s | priority=%d | relevance=%s | score=%s | engagement=%s | rate=%s | outperformance=%s | baseline_posts=%d (%s) | metrics=%s\n",
			index+1, username, joinSignalTypes(post.SignalTypes), post.PrimarySignal, post.CommercialIntentPriority,
			post.Post.RelevanceLabel, formatFloat(post.RankScore), formatFloat(post.Engagement), formatRate(post.EngagementRate),
			formatOptional(post.RelativePerformance, "x"), post.BaselinePostCount, post.BaselineConfidence, formatCoverage(post.MetricCoverage)); err != nil {
			return err
		}
		for _, detail := range []struct{ name, value string }{
			{"need", post.Need},
			{"requested_role", post.RequestedRole},
			{"explicit_paid_signal", post.PaidSignal},
			{"cta", post.CTA},
		} {
			if detail.value == "" {
				detail.value = "n/a"
			}
			if _, err := fmt.Fprintf(w, "     %s: %s\n", detail.name, detail.value); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w, "     signal_reasons:", strings.Join(post.SignalReasons, ", ")); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, "     original_text:"); err != nil {
			return err
		}
		textLines := strings.Split(post.Post.Text, "\n")
		if len(textLines) == 0 {
			textLines = []string{""}
		}
		for _, line := range textLines {
			if _, err := fmt.Fprintf(w, "       %s\n", line); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "     %s\n", sourceURL(post.Post.URL)); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w)
	return err
}

func joinSignalTypes(types []SignalType) string {
	values := make([]string, 0, len(types))
	for _, signal := range types {
		values = append(values, string(signal))
	}
	if len(values) == 0 {
		return "n/a"
	}
	return strings.Join(values, ",")
}

func writeLimitations(w io.Writer, limitations []string) error {
	if _, err := fmt.Fprintln(w, "LIMITATIONS"); err != nil {
		return err
	}
	if len(limitations) == 0 {
		limitations = []string{"Coverage and performance fields are source-dependent and bounded by the configured run limits."}
	}
	for _, limitation := range limitations {
		if _, err := fmt.Fprintf(w, "  - %s\n", limitation); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w)
	return err
}

func writeCollectionCoverage(w io.Writer, coverage CollectionCoverage) error {
	if _, err := fmt.Fprintf(w, "COLLECTION COVERAGE\n  Surface: %s\n  Queries: %d attempted, %d successful, %d with results\n  Posts: %d raw results, %d unique; relevant=%d adjacent=%d irrelevant=%d uncertain=%d (precision=%s)\n  Authors: %d unique, %d relevant; profiles %d attempted/%d fetched; followers known %d\n  Baselines: %d authors attempted, %d reliable, %d posts collected\n\n",
		coverage.SearchSurface, coverage.QueriesAttempted, coverage.QueriesSuccessful, coverage.QueriesWithResults,
		coverage.RawPostsCollected, coverage.UniquePosts, coverage.RelevantPosts, coverage.AdjacentPosts,
		coverage.IrrelevantPosts, coverage.UncertainPosts, formatPercent(coverage.Relevance.Precision*100),
		coverage.UniqueAuthors, coverage.RelevantAuthors,
		coverage.ProfilesAttempted, coverage.ProfilesFetched, coverage.AuthorsWithKnownFollowers,
		coverage.BaselineAuthorsAttempted, coverage.BaselineAuthorsReliable, coverage.BaselinePostsCollected); err != nil {
		return err
	}
	return nil
}

func writeAnalysis(w io.Writer, analysis AnalysisReport) error {
	if !analysis.Enabled {
		_, err := fmt.Fprintln(w, "CONTENT INTELLIGENCE\n  Not run; use --analyze with provider credentials to classify unique topic posts.")
		return err
	}
	if _, err := fmt.Fprintf(w, "CONTENT INTELLIGENCE\n  Provider: %s / %s\n  Plan: %d eligible, %d cached, %d to send, %d estimated input chars (~%d tokens), %d batches; actual tokens in/out/total=%d/%d/%d\n  Coverage: %s; reliable baselines %d/%d (%s); partial metrics %d\n\n",
		analysis.Plan.Provider, analysis.Plan.Model, analysis.Plan.PostsEligible, analysis.Plan.PostsCached,
		analysis.Plan.PostsToSend, analysis.Plan.InputCharacters, analysis.Plan.EstimatedInputTokens, analysis.Plan.BatchCount,
		analysis.Plan.ActualInputTokens, analysis.Plan.ActualOutputTokens, analysis.Plan.ActualTotalTokens,
		analysis.Coverage.String(), analysis.Coverage.PostsWithReliableBaseline, analysis.Coverage.PostsClassified,
		formatPercent(analysis.Coverage.ReliableBaselinePercent), analysis.Coverage.PostsWithPartialMetrics); err != nil {
		return err
	}
	sections := []struct {
		title string
		items []PatternStatistic
	}{
		{"CLASSIFICATION BREAKDOWN: CONTENT TYPES", analysis.ContentTypes},
		{"HOOK TYPES", analysis.Hooks},
		{"STRUCTURES", analysis.Structures},
		{"TONES", analysis.Tones},
		{"INTENTS", analysis.Intents},
		{"TOPICS", analysis.Topics},
		{"COMBINATIONS", analysis.Combinations},
	}
	for _, section := range sections {
		if err := writePatternSection(w, section.title, section.items); err != nil {
			return err
		}
	}
	if err := writeClusterSection(w, "PAIN CLUSTERS", analysis.Pains); err != nil {
		return err
	}
	if err := writeClusterSection(w, "QUESTION CLUSTERS", analysis.Questions); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "CONTENT OPPORTUNITIES"); err != nil {
		return err
	}
	if len(analysis.Opportunities) == 0 {
		if _, err := fmt.Fprintln(w, "  (none supported by the current evidence threshold)"); err != nil {
			return err
		}
	} else {
		for _, item := range analysis.Opportunities {
			if _, err := fmt.Fprintf(w, "  - %s | evidence=%d authors=%d questions=%d median_relative=%s | strength=%s\n     %s\n", item.Title, item.EvidenceCount, item.UniqueAuthors, item.ExplicitQuestionCount, formatOptional(item.MedianRelativePerformance, "x"), item.SignalStrength, item.Rationale); err != nil {
				return err
			}
			for _, evidence := range item.EvidenceURLs {
				if _, err := fmt.Fprintf(w, "     evidence: %s\n", evidence); err != nil {
					return err
				}
			}
		}
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "PRODUCT/SERVICE PAIN RADAR"); err != nil {
		return err
	}
	if len(analysis.ProductPainRadar) == 0 {
		if _, err := fmt.Fprintln(w, "  (none explicitly supported by the classified evidence)"); err != nil {
			return err
		}
	} else {
		for _, item := range analysis.ProductPainRadar {
			if _, err := fmt.Fprintf(w, "  - %s | %s | posts=%d authors=%d solution_seeking=%d buyer_intent=%d tool_complaints=%d | median_engagement=%s | median_relative=%s | strength=%s\n",
				item.Category, item.Signal, item.Count, item.UniqueAuthors, item.ExplicitSolutionSeeking,
				item.BuyerIntent, item.ExistingToolComplaints, formatOptional(item.MedianEngagement, ""),
				formatOptional(item.MedianRelativePerformance, "x"), item.SignalStrength); err != nil {
				return err
			}
			for _, evidence := range item.EvidenceURLs {
				if _, err := fmt.Fprintf(w, "     evidence: %s\n", evidence); err != nil {
					return err
				}
			}
		}
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if err := writePatternSection(w, "WEAK SIGNALS", analysis.WeakSignals); err != nil {
		return err
	}
	if len(analysis.Warnings) > 0 {
		if _, err := fmt.Fprintln(w, "ANALYSIS WARNINGS"); err != nil {
			return err
		}
		for _, warning := range analysis.Warnings {
			if _, err := fmt.Fprintf(w, "  - %s\n", warning); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	return nil
}

func writePatternSection(w io.Writer, title string, items []PatternStatistic) error {
	if _, err := fmt.Fprintln(w, title); err != nil {
		return err
	}
	if len(items) == 0 {
		if _, err := fmt.Fprintln(w, "  (none)"); err != nil {
			return err
		}
		_, err := fmt.Fprintln(w)
		return err
	}
	for _, item := range items {
		if _, err := fmt.Fprintf(w, "  - %s | N=%d | median=%s | q1=%s | q3=%s | reliable_baseline=%s | strength=%s\n",
			item.Value, item.N, formatOptional(item.MedianRelativePerformance, "x"), formatOptional(item.Q1RelativePerformance, "x"),
			formatOptional(item.Q3RelativePerformance, "x"), formatPercent(item.ReliableBaselinePercent), item.SignalStrength); err != nil {
			return err
		}
		for _, evidence := range item.EvidenceURLs {
			if _, err := fmt.Fprintf(w, "    evidence: %s\n", evidence); err != nil {
				return err
			}
		}
	}
	_, err := fmt.Fprintln(w)
	return err
}

func writeClusterSection(w io.Writer, title string, items []PainCluster) error {
	if _, err := fmt.Fprintln(w, title); err != nil {
		return err
	}
	if len(items) == 0 {
		if _, err := fmt.Fprintln(w, "  (none)"); err != nil {
			return err
		}
		_, err := fmt.Fprintln(w)
		return err
	}
	for _, item := range items {
		if _, err := fmt.Fprintf(w, "  - %s | count=%d | strength=%s\n     %s\n", item.Label, item.Count, item.SignalStrength, item.Description); err != nil {
			return err
		}
		for _, evidence := range item.EvidenceURLs {
			if _, err := fmt.Fprintf(w, "     evidence: %s\n", evidence); err != nil {
				return err
			}
		}
	}
	_, err := fmt.Fprintln(w)
	return err
}

func writeCoverage(w io.Writer, coverage DatasetCoverage) error {
	if _, err := fmt.Fprintln(w, "METRIC COVERAGE"); err != nil {
		return err
	}
	for _, item := range []struct {
		name     string
		coverage MetricCoverage
	}{
		{"likes", coverage.Likes},
		{"replies", coverage.Replies},
		{"reposts", coverage.Reposts},
		{"quotes", coverage.Quotes},
		{"views", coverage.Views},
	} {
		if _, err := fmt.Fprintf(w, "  %-8s %s\n", item.name+":", formatCoverage(item.coverage)); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w)
	return err
}

func writeBaselineCoverage(w io.Writer, coverage BaselineCoverage) error {
	if _, err := fmt.Fprintf(w, "BASELINE COVERAGE\n  Reliable author baselines: %s\n\n", formatPercent(coverage.Percent)); err != nil {
		return err
	}
	return nil
}

func writeQueries(w io.Writer, queries []QueryReport) error {
	if _, err := fmt.Fprintln(w, "QUERIES"); err != nil {
		return err
	}
	if len(queries) == 0 {
		if _, err := fmt.Fprintln(w, "  (none)"); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		return nil
	}
	for _, query := range queries {
		status := "ok"
		if query.Error != "" {
			status = "error: " + query.Error
		}
		if _, err := fmt.Fprintf(w, "  - %s | results=%d relevant=%d adjacent=%d irrelevant=%d uncertain=%d precision=%s contributes=%t | new=%d | %s\n",
			query.Query, query.ResultCount, query.Relevant, query.Adjacent, query.Irrelevant, query.Uncertain,
			formatPercent(query.Precision*100), query.ContributesToConclusions, query.UniqueNewPosts, status); err != nil {
			return err
		}
		if query.ProbeResultCount > 0 || query.SelectedForDeep {
			if _, err := fmt.Fprintf(w, "    probe: results=%d relevant=%d owner_likely=%d pain=%d it_actionable=%d precision=%s unique=%d duplicates=%d | deep_selected=%t deep_results=%d deep_unique=%d\n",
				query.ProbeResultCount, query.ProbeRelevantPosts, query.ProbeOwnerLikelyPosts, query.ProbePainPosts,
				query.ProbeITActionablePosts, formatPercent(query.ProbePrecision*100), query.ProbeUniqueContribution,
				query.ProbeDuplicates, query.SelectedForDeep, query.DeepResultCount, query.DeepUniqueContribution); err != nil {
				return err
			}
		}
	}
	_, err := fmt.Fprintln(w)
	return err
}

func writeBusinessPainSections(w io.Writer, report Report) error {
	counts := report.BusinessPainCounts
	if _, err := fmt.Fprintf(w, "RUSSIAN SMALL-BUSINESS PAIN DISCOVERY\n  Russian relevant posts: %d\n  Owner/operator likely: %d\n  Genuine pain: %d\n  IT-actionable pain: %d\n  Solution seeking: %d\n  Workaround present: %d\n  GOLD pain signals: %d\n",
		counts.RussianPosts, counts.OwnerLikelyPosts, counts.GenuinePainPosts, counts.ITActionablePainPosts,
		counts.SolutionSeekingPosts, counts.WorkaroundPosts, counts.GoldPainSignals); err != nil {
		return err
	}
	if len(counts.PainTypeCounts) > 0 {
		if _, err := fmt.Fprintf(w, "  Pain types: %s\n", formatCountMap(counts.PainTypeCounts)); err != nil {
			return err
		}
	}
	if len(counts.BusinessTypeCounts) > 0 {
		if _, err := fmt.Fprintf(w, "  Business types: %s\n", formatCountMap(counts.BusinessTypeCounts)); err != nil {
			return err
		}
	}
	if len(counts.BusinessProcessCounts) > 0 {
		if _, err := fmt.Fprintf(w, "  Business processes: %s\n", formatCountMap(counts.BusinessProcessCounts)); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "GOLD PAIN SIGNALS"); err != nil {
		return err
	}
	if len(report.GoldPainSignals) == 0 {
		if _, err := fmt.Fprintln(w, "  (none)"); err != nil {
			return err
		}
	} else {
		for index, post := range report.GoldPainSignals {
			if _, err := fmt.Fprintf(w, "  %d. @%s | pain=%s | types=%s | workaround=%s | solution_seeking=%s | actionability=%s | buyer_intent=%s | engagement=%s | outperformance=%s\n     %s\n     %s\n",
				index+1, post.Post.AuthorUsername, post.PainStrength, strings.Join(post.PainTypes, ","), post.WorkaroundPresent,
				post.SolutionSeeking, post.ITActionability, post.BuyerIntent, formatFloat(post.Engagement),
				formatOptional(post.RelativePerformance, "x"), excerpt(post.Post.Text, 240), sourceURL(post.Post.URL)); err != nil {
				return err
			}
		}
	}
	_, err := fmt.Fprintln(w)
	return err
}

func formatCountMap(values map[string]int) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+strconv.Itoa(values[key]))
	}
	return strings.Join(parts, ", ")
}

func writePosts(w io.Writer, title string, posts []RankedPost) error {
	if _, err := fmt.Fprintln(w, title); err != nil {
		return err
	}
	if len(posts) == 0 {
		if _, err := fmt.Fprintln(w, "  (none)"); err != nil {
			return err
		}
		_, err := fmt.Fprintln(w)
		return err
	}
	for i, post := range posts {
		username := post.Post.AuthorUsername
		if username == "" {
			username = "unknown-author"
		}
		if _, err := fmt.Fprintf(w, "  %d. @%s | relevance=%s (%s) | score=%s | coverage=%s | engagement=%s | rate=%s | outperformance=%s | baseline_posts=%d (%s) | metrics=%s\n",
			i+1, username, post.Post.RelevanceLabel, formatFloat(post.Post.RelevanceScore), formatFloat(post.RankScore), formatPercent(post.ScoringCoverage*100), formatFloat(post.Engagement),
			formatRate(post.EngagementRate), formatOptional(post.RelativePerformance, "x"), post.BaselinePostCount,
			post.BaselineConfidence, formatCoverage(post.MetricCoverage)); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "     %s\n     %s\n", excerpt(post.Post.Text, 220), sourceURL(post.Post.URL)); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w)
	return err
}

func writeAuthors(w io.Writer, authors []RankedAuthor) error {
	if _, err := fmt.Fprintln(w, "TOP CREATORS"); err != nil {
		return err
	}
	if len(authors) == 0 {
		if _, err := fmt.Fprintln(w, "  (none)"); err != nil {
			return err
		}
		_, err := fmt.Fprintln(w)
		return err
	}
	for i, author := range authors {
		profile := author.Author.ProfileURL
		if profile == "" {
			profile = "https://www.threads.com/@" + author.Author.Username
		}
		if _, err := fmt.Fprintf(w, "  %d. @%s | score=%s | coverage=%s | posts=%d | median_engagement=%s | median_rate=%s | outperformance=%s | baseline_posts=%d (%s)\n",
			i+1, author.Author.Username, formatFloat(author.RankScore), formatPercent(author.ScoringCoverage*100), author.RelevantPostCount,
			formatFloat(author.MedianEngagement), formatRate(author.MedianEngagementRate), formatOptional(author.MedianRelativePerformance, "x"),
			author.BaselinePostCount, author.BaselineConfidence); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "     %s | followers=%s | verified=%s | metrics=%s\n", profile, formatInt(author.Author.FollowerCount), formatBool(author.Author.Verified), formatCoverage(author.MetricCoverage)); err != nil {
			return err
		}
		for _, evidence := range author.EvidenceURLs {
			if _, err := fmt.Fprintf(w, "     evidence: %s\n", evidence); err != nil {
				return err
			}
		}
	}
	_, err := fmt.Fprintln(w)
	return err
}

func sourceURL(url string) string {
	if url == "" {
		return "source: unavailable"
	}
	return "source: " + url
}

func excerpt(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return "(text unavailable)"
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "..."
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func formatOptional(value *float64, suffix string) string {
	if value == nil {
		return "n/a"
	}
	return formatFloat(*value) + suffix
}

func formatRate(value *float64) string {
	if value == nil {
		return "n/a"
	}
	return formatFloat(*value)
}

func formatInt(value *int64) string {
	if value == nil {
		return "n/a"
	}
	return strconv.FormatInt(*value, 10)
}

func formatBool(value *bool) string {
	if value == nil {
		return "n/a"
	}
	return strconv.FormatBool(*value)
}

func formatCoverage(value MetricCoverage) string {
	return fmt.Sprintf("%d/%d (%s)", value.Known, value.Total, formatPercent(value.Percent))
}

func formatPercent(value float64) string {
	return formatFloat(value) + "%"
}

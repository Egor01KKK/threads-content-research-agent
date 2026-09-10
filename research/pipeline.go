package research

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sort"
	"strings"
	"time"

	"github.com/Egor01KKK/threads-content-research-agent/threads"
)

// Run executes a bounded topic-first research run without semantic analysis.
// It remains the compatibility entry point for callers that only need
// collection and numeric ranking.
func Run(ctx context.Context, cfg Config, collector Collector, expander QueryExpander, store *Store) (Report, error) {
	return RunWithAnalysis(ctx, cfg, collector, expander, store, AnalysisOptions{})
}

// RunWithAnalysis executes collection, ranking, and optional versioned
// content intelligence. Provider failures become report warnings so a failed
// LLM request cannot discard the evidence already collected in SQLite.
func RunWithAnalysis(ctx context.Context, cfg Config, collector Collector, expander QueryExpander, store *Store, analysisOptions AnalysisOptions) (report Report, runErr error) {
	cfg.Topic = normalizeSpace(cfg.Topic)
	if strings.TrimSpace(cfg.Mode) == "" {
		cfg.Mode = ModeBounded
	}
	if err := cfg.Validate(); err != nil {
		return Report{}, err
	}
	if collector == nil || store == nil || (cfg.Mode != ModeDeep && expander == nil) {
		return Report{}, errors.Join(ErrInvalidConfig, errors.New("collector, expander, and store are required"))
	}
	if cfg.Mode == ModeDeep {
		if analysisOptions.Enabled {
			return Report{}, errors.Join(ErrInvalidConfig, errors.New("deep mode never invokes an LLM analysis provider; omit --analyze"))
		}
		return runDeepResearch(ctx, cfg, collector, store)
	}

	queries, err := expander.Expand(ctx, cfg.Topic)
	if err != nil {
		return Report{}, fmt.Errorf("expand topic: %w", err)
	}
	if len(queries) == 0 {
		return Report{}, fmt.Errorf("expand topic: no search queries generated")
	}

	now := time.Now
	startedAt := now()
	runID, err := store.StartRun(cfg.Topic, startedAt, len(queries))
	if err != nil {
		return Report{}, err
	}
	report = Report{
		Topic:     cfg.Topic,
		Language:  researchLanguage(cfg.Topic),
		RunID:     runID,
		Status:    RunStatusRunning,
		StartedAt: startedAt,
		Scoring:   cfg,
		Collection: CollectionCoverage{
			SearchSurface: "public SSR search + bounded GraphQL cursor fallback",
		},
	}
	if analysisOptions.Enabled {
		report.Analysis.Enabled = true
		report.Analysis.Plan.Enabled = true
	}

	postsByID := map[string]Post{}
	queriesByPost := map[string][]string{}
	authors := map[string]Author{}
	baselinePostsByAuthor := map[string][]Post{}
	baselineAttempts := map[string]bool{}
	hadUsableQuery := false
	failedQueryCount := 0

	defer func() {
		report.CompletedAt = now()
		report.PostsAnalyzed = len(postsByID)
		report.AuthorsDiscovered = len(authors)
		report.ProfilesDiscovered = len(authors)
		report.Coverage = datasetCoverage(postsByID, authors)
		report.ProfilesWithKnownFollowers = report.Coverage.Followers.Known
		report.FollowerRelativeRanking = followerRelativeRankingStatus(report.Coverage.Followers)
		report.Baseline = baselineCoverage(
			baselineAttempts,
			baselinePostsByAuthor,
			report.Baseline.PostsCollected,
			report.Baseline.PostsWithKnownEngagement,
			cfg.MinBaselinePosts,
			cfg.Engagement,
		)
		report.Collection = collectionCoverage(report.Queries, postsByID, authors, report, baselinePostsByAuthor, report.Collection)
		report.Limitations = researchLimitations(report)
		report.Status = runStatus(runErr, len(report.Warnings))
		if finishErr := store.FinishRun(runID, report.CompletedAt, report.Status, errorText(runErr)); finishErr != nil {
			report.Status = RunStatusFailed
			runErr = errors.Join(runErr, finishErr)
		}
	}()

	// Search results are ingested through one closure so the Russian probe and
	// deep phases share the exact same dedupe, relevance, and persistence path.
	ingestSearchResults := func(query string, queryID int64, queryReport *QueryReport, phase string, seenIDs map[string]bool, results iter.Seq2[threads.SearchResult, error]) error {
		for result, streamErr := range results {
			if streamErr != nil {
				if abortErr := contextAbort(ctx, streamErr); abortErr != nil {
					return abortErr
				}
				return streamErr
			}
			queryReport.ResultCount++
			if phase == "probe" {
				queryReport.ProbeResultCount++
			} else if phase == "deep" {
				queryReport.DeepResultCount++
			}
			report.Collection.RawPostsCollected++
			post := postFromSearchResult(result, cfg.Topic, now())
			if phase == "probe" {
				probeAssessment := AssessBusinessPain(cfg.Topic, post.Text)
				if probeAssessment.OwnerLikelihood == OwnerLikelihoodHigh || probeAssessment.OwnerLikelihood == OwnerLikelihoodMedium {
					queryReport.ProbeOwnerLikelyPosts++
				}
				if probeAssessment.PainStrength != PainStrengthLow || probeAssessment.SolutionSeeking == Yes || probeAssessment.WorkaroundPresent == Yes || len(probeAssessment.BusinessConsequences) > 0 {
					queryReport.ProbePainPosts++
				}
				if probeAssessment.ITActionability == ITActionabilityHigh || probeAssessment.ITActionability == ITActionabilityMedium {
					queryReport.ProbeITActionablePosts++
				}
			}
			assessment := AssessRelevanceForQuery(cfg.Topic, query, post.Text)
			recordQueryRelevance(queryReport, assessment)
			if phase == "probe" && assessment.Label == RelevanceRelevant {
				queryReport.ProbeRelevantPosts++
			}
			applyRelevance(&post, assessment)
			if post.ID == "" {
				report.Warnings = appendWarningOnce(report.Warnings, fmt.Sprintf("query %q returned a record without a post ID", query))
				continue
			}
			if phase == "probe" {
				if seenIDs != nil {
					if seenIDs[post.ID] {
						queryReport.ProbeDuplicates++
					} else {
						seenIDs[post.ID] = true
					}
				}
				if _, exists := postsByID[post.ID]; !exists {
					queryReport.ProbeUniqueContribution++
				}
			} else if phase == "deep" {
				if _, exists := postsByID[post.ID]; !exists {
					queryReport.DeepUniqueContribution++
				}
			}
			if existing, ok := postsByID[post.ID]; ok {
				post = mergePosts(existing, post)
			} else {
				queryReport.UniqueNewPosts++
			}
			postsByID[post.ID] = post
			if post.AuthorUsername != "" {
				username := normalizeAuthor(post.AuthorUsername)
				authors[username] = authorFor(authors, username)
			}
			queriesByPost[post.ID] = appendUnique(queriesByPost[post.ID], query)
			if err := store.UpsertPost(post); err != nil {
				return err
			}
			if err := store.SavePostRelevance(runID, post.ID, assessmentForPost(post), now()); err != nil {
				return err
			}
			if err := store.LinkPostToQuery(runID, queryID, post.ID, now()); err != nil {
				return err
			}
		}
		return nil
	}

	finishQuery := func(queryID int64, query string, queryReport QueryReport, queryErr error) error {
		if queryErr != nil {
			failedQueryCount++
			queryReport.Error = queryErr.Error()
			report.Warnings = append(report.Warnings, fmt.Sprintf("query %q failed after %d result(s): %v", query, queryReport.ResultCount, queryErr))
		} else {
			hadUsableQuery = true
			queryReport.Successful = true
			report.Collection.QueriesSuccessful++
		}
		if queryReport.ResultCount > 0 {
			hadUsableQuery = true
			report.Collection.QueriesWithResults++
		}
		if err := store.FinishQuery(queryID, queryReport.ResultCount, queryReport.UniqueNewPosts, queryReport.Error); err != nil {
			return err
		}
		if err := store.SaveQueryQuality(queryID, queryReport); err != nil {
			return err
		}
		report.Queries = append(report.Queries, queryReport)
		if queryReport.ResultCount > 0 && queryReport.Relevant == 0 {
			report.Warnings = appendWarningOnce(report.Warnings, fmt.Sprintf(
				"query %q returned %d result(s) but no relevant posts; its hits are excluded from topic conclusions",
				query, queryReport.ResultCount))
		}
		return nil
	}

	if isRussianSmallBusinessPainTopic(cfg.Topic) {
		probes := make([]russianQueryProbe, 0, len(queries))
		probeSeenIDs := map[string]bool{}
		probeLimit := minInt(cfg.PerQueryLimit, 3)
		for position, query := range queries {
			if err := ctx.Err(); err != nil {
				return report, err
			}
			report.Collection.QueriesAttempted++
			queryID, addErr := store.AddQuery(runID, query, position, now())
			if addErr != nil {
				return report, addErr
			}
			queryReport := QueryReport{Query: query, Language: "ru"}
			probeErr := ingestSearchResults(query, queryID, &queryReport, "probe", probeSeenIDs, collector.Search(ctx, query, probeLimit))
			if abortErr := contextAbort(ctx, probeErr); abortErr != nil {
				return report, abortErr
			}
			queryReport.ProbePrecision = precision(queryReport.ProbeRelevantPosts, queryReport.ProbeResultCount)
			probes = append(probes, russianQueryProbe{
				QueryID:  queryID,
				Query:    query,
				Report:   queryReport,
				ProbeErr: probeErr,
				Position: position,
			})
		}
		for _, index := range selectRussianDeepProbes(probes, cfg.MaxQueries) {
			probes[index].Report.SelectedForDeep = true
		}
		for index := range probes {
			probe := &probes[index]
			queryErr := probe.ProbeErr
			if probe.Report.SelectedForDeep {
				deepErr := ingestSearchResults(probe.Query, probe.QueryID, &probe.Report, "deep", nil, collector.Search(ctx, probe.Query, cfg.PerQueryLimit))
				if abortErr := contextAbort(ctx, deepErr); abortErr != nil {
					return report, abortErr
				}
				if deepErr != nil {
					queryErr = errors.Join(queryErr, deepErr)
				}
			}
			if err := finishQuery(probe.QueryID, probe.Query, probe.Report, queryErr); err != nil {
				return report, err
			}
		}
	} else {
		for position, query := range queries {
			if err := ctx.Err(); err != nil {
				return report, err
			}
			report.Collection.QueriesAttempted++
			queryID, err := store.AddQuery(runID, query, position, now())
			if err != nil {
				return report, err
			}
			queryReport := QueryReport{Query: query}
			queryErr := ingestSearchResults(query, queryID, &queryReport, "deep", nil, collector.Search(ctx, query, cfg.PerQueryLimit))
			if abortErr := contextAbort(ctx, queryErr); abortErr != nil {
				return report, abortErr
			}
			if err := finishQuery(queryID, query, queryReport, queryErr); err != nil {
				return report, err
			}
		}
	}
	if !hadUsableQuery && failedQueryCount == len(queries) {
		return report, errors.New("all research queries failed")
	}

	posts := sortedPosts(postsByID)
	report.ProfilesDiscovered = len(authors)
	relevantPosts := filterPostsByRelevance(posts, RelevanceRelevant)
	adjacentPosts := filterPostsByRelevance(posts, RelevanceAdjacent)
	profileCandidates := selectProfileCandidatesByRelevance(relevantPosts, adjacentPosts, cfg.ProfileLimit)
	for _, username := range profileCandidates {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		report.ProfilesAttempted++
		profile, profileErr := collector.Profile(ctx, username)
		if profileErr != nil {
			if abortErr := contextAbort(ctx, profileErr); abortErr != nil {
				return report, abortErr
			}
			report.Warnings = append(report.Warnings, fmt.Sprintf("profile @%s unavailable: %v", username, profileErr))
			continue
		}
		author := authorFromProfile(profile, username)
		authors[username] = author
		if err := store.UpsertAuthor(runID, author, now()); err != nil {
			return report, err
		}
		report.ProfilesFetched++
	}
	for username := range authors {
		if err := store.EnsureAuthor(authorFor(authors, username), now()); err != nil {
			return report, err
		}
	}

	// Candidate ranking now sees follower counts for every profile that fit the
	// profile cap, so the baseline fetch is not selected from raw likes alone.
	_, relevantCandidateAuthors := Rank(relevantPosts, authors, queriesByPost, BaselineData{MinPosts: cfg.MinBaselinePosts}, cfg)
	_, adjacentCandidateAuthors := Rank(adjacentPosts, authors, queriesByPost, BaselineData{MinPosts: cfg.MinBaselinePosts}, cfg)
	candidateAuthors := appendRankedAuthors(relevantCandidateAuthors, adjacentCandidateAuthors)
	baselineCandidates := selectBaselineCandidates(candidateAuthors, cfg.BaselineAuthorLimit)
	for _, username := range baselineCandidates {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		baselineAttempts[username] = true
		seenBaseline := map[string]bool{}
		for baselinePost, streamErr := range collector.ProfilePosts(ctx, username, cfg.BaselinePostLimit) {
			if streamErr != nil {
				if abortErr := contextAbort(ctx, streamErr); abortErr != nil {
					return report, abortErr
				}
				report.Warnings = append(report.Warnings, fmt.Sprintf("baseline posts for @%s unavailable: %v", username, streamErr))
				break
			}
			post := postFromThreadPost(baselinePost, username, now())
			if post.ID == "" {
				report.Warnings = appendWarningOnce(report.Warnings, fmt.Sprintf("baseline feed for @%s returned a record without a post ID", username))
				continue
			}
			if seenBaseline[post.ID] {
				continue
			}
			seenBaseline[post.ID] = true
			report.Baseline.PostsCollected++
			if engagementMetricCount(post) > 0 {
				report.Baseline.PostsWithKnownEngagement++
			}
			if err := store.UpsertPost(post); err != nil {
				return report, err
			}
			if err := store.LinkBaselinePost(runID, username, post.ID, now()); err != nil {
				return report, err
			}
			if err := store.RecordPostSnapshot(runID, post, now()); err != nil {
				return report, err
			}
			// A discovered post can also appear in the recent feed. Store the
			// baseline observation, but exclude it from the independent baseline
			// denominator to avoid measuring a post against itself.
			if _, isEvidence := postsByID[post.ID]; isEvidence {
				continue
			}
			baselinePostsByAuthor[username] = append(baselinePostsByAuthor[username], post)
		}
	}

	posts = sortedPosts(postsByID)
	relevantPosts = filterPostsByRelevance(posts, RelevanceRelevant)
	rankedPosts, rankedAuthors := Rank(relevantPosts, authors, queriesByPost, BaselineData{
		PostsByAuthor: baselinePostsByAuthor,
		MinPosts:      cfg.MinBaselinePosts,
	}, cfg)
	rankedPosts = annotateRankedSignals(report.Topic, rankedPosts)
	rankedPosts = annotateBusinessPain(report.Topic, rankedPosts)
	report.BusinessPainCounts, report.GoldPainSignals, report.BusinessPainPosts = buildBusinessPainSections(rankedPosts)
	rawRankedPosts, rawRankedAuthors := Rank(posts, authors, queriesByPost, BaselineData{
		PostsByAuthor: baselinePostsByAuthor,
		MinPosts:      cfg.MinBaselinePosts,
	}, cfg)
	report.PostsAnalyzed = len(posts)
	report.AuthorsDiscovered = len(authors)
	report.ProfilesDiscovered = len(authors)
	report.PostsWithKnownFollowers = 0
	for _, post := range rawRankedPosts {
		if post.EngagementRate != nil {
			report.PostsWithKnownFollowers++
		}
		if err := store.UpsertPost(post.Post); err != nil {
			return report, err
		}
		if err := store.RecordPostSnapshot(runID, post.Post, now()); err != nil {
			return report, err
		}
	}
	report.Coverage = datasetCoverage(postsByID, authors)
	report.ProfilesWithKnownFollowers = report.Coverage.Followers.Known
	report.FollowerRelativeRanking = followerRelativeRankingStatus(report.Coverage.Followers)
	report.Baseline = baselineCoverage(
		baselineAttempts,
		baselinePostsByAuthor,
		report.Baseline.PostsCollected,
		report.Baseline.PostsWithKnownEngagement,
		cfg.MinBaselinePosts,
		cfg.Engagement,
	)
	report.Collection = collectionCoverage(report.Queries, postsByID, authors, report, baselinePostsByAuthor, report.Collection)
	if report.ProfilesAttempted < report.ProfilesDiscovered {
		report.Warnings = appendWarningOnce(report.Warnings, fmt.Sprintf(
			"profile enrichment selected %d/%d discovered authors after relevance gating and the profile cap",
			report.ProfilesAttempted, report.ProfilesDiscovered))
	}
	if report.Coverage.Followers.Total > 0 && report.Coverage.Followers.Known < report.Coverage.Followers.Total {
		report.Warnings = appendWarningOnce(report.Warnings, fmt.Sprintf(
			"follower-relative ranking is partial: %d/%d discovered authors have known follower counts",
			report.Coverage.Followers.Known, report.Coverage.Followers.Total))
	}
	if report.Baseline.AuthorsAttempted > 0 && report.Baseline.AuthorsWithReliableData < report.Baseline.AuthorsAttempted {
		report.Warnings = appendWarningOnce(report.Warnings, fmt.Sprintf(
			"author baseline coverage is limited: %d/%d selected authors have a usable independent recent-post baseline",
			report.Baseline.AuthorsWithReliableData, report.Baseline.AuthorsAttempted))
	}
	report.TopPosts = takeRankedPosts(rankedPosts, cfg.TopPosts)
	report.TopOutperformingPosts = topOutperforming(rankedPosts, cfg.TopPosts)
	report.TopAuthors = takeRankedAuthors(rankedAuthors, cfg.TopAuthors)
	report.SignalCounts, report.LeadBuyerSignals, report.ContentAudienceSignals = buildSignalSections(rankedPosts)
	report.RawTopPosts = takeRankedPosts(rawRankedPosts, cfg.TopPosts)
	report.RawTopOutperformingPosts = topOutperforming(rawRankedPosts, cfg.TopPosts)
	report.RawTopAuthors = takeRankedAuthors(rawRankedAuthors, cfg.TopAuthors)

	if analysisOptions.Enabled {
		if err := analyzeRankedPosts(ctx, report.Topic, report.RunID, rankedPosts, store, analysisOptions, &report); err != nil {
			if abortErr := contextAbort(ctx, err); abortErr != nil {
				return report, abortErr
			}
			// Analysis errors are deliberately non-fatal. Collection and ranking
			// have already been persisted and remain useful to the caller.
			report.Warnings = appendWarningOnce(report.Warnings, "content analysis failed: "+err.Error())
			report.Analysis.Warnings = appendWarningOnce(report.Analysis.Warnings, err.Error())
		}
	}

	return report, nil
}

func postFromSearchResult(result threads.SearchResult, topic string, observedAt time.Time) Post {
	return Post{
		ID:             result.ID,
		Shortcode:      result.Shortcode,
		URL:            result.Permalink,
		Text:           result.Text,
		AuthorUsername: normalizeAuthor(result.Username),
		PublishedAt:    result.Timestamp,
		Likes:          cloneInt(result.LikeCount),
		Replies:        cloneInt(result.ReplyCount),
		Reposts:        cloneInt(result.RepostCount),
		Quotes:         cloneInt(result.QuoteCount),
		Views:          cloneInt(result.ViewCount),
		DetectedTopic:  topic,
		FirstSeenAt:    observedAt,
		LastSeenAt:     observedAt,
	}
}

func postFromThreadPost(result threads.Post, fallbackUsername string, observedAt time.Time) Post {
	username := normalizeAuthor(result.Username)
	if username == "" {
		username = normalizeAuthor(fallbackUsername)
	}
	return Post{
		ID:             result.ID,
		Shortcode:      result.Shortcode,
		URL:            result.Permalink,
		Text:           result.Text,
		AuthorUsername: username,
		PublishedAt:    result.Timestamp,
		Likes:          sourceMetric(result.LikeCount, result.LikeCountAvailable),
		Replies:        sourceMetric(result.ReplyCount, result.ReplyCountAvailable),
		Reposts:        sourceMetric(result.RepostCount, result.RepostCountAvailable),
		Quotes:         sourceMetric(result.QuoteCount, result.QuoteCountAvailable),
		Views:          sourceMetric(result.ViewCount, result.ViewCountAvailable),
		FirstSeenAt:    observedAt,
		LastSeenAt:     observedAt,
	}
}

func sourceMetric(value int64, available bool) *int64 {
	if !available && value == 0 {
		return nil
	}
	return &value
}

func authorFromProfile(profile *threads.Profile, fallbackUsername string) Author {
	if profile == nil {
		return authorFor(nil, fallbackUsername)
	}
	username := normalizeAuthor(profile.Username)
	if username == "" {
		username = normalizeAuthor(fallbackUsername)
	}
	author := Author{
		Username:   username,
		Name:       profile.Name,
		Bio:        profile.Biography,
		ProfileURL: profile.URL,
	}
	if author.ProfileURL == "" {
		author.ProfileURL = "https://www.threads.com/@" + username
	}
	if profile.FollowerCountAvailable || profile.FollowerCount != 0 {
		value := profile.FollowerCount
		author.FollowerCount = &value
	}
	if profile.FollowingCountAvailable || profile.FollowingCount != 0 {
		value := profile.FollowingCount
		author.FollowingCount = &value
	}
	if profile.VerifiedAvailable {
		value := profile.IsVerified
		author.Verified = &value
	}
	return author
}

func mergePosts(existing, incoming Post) Post {
	merged := existing
	if merged.Shortcode == "" {
		merged.Shortcode = incoming.Shortcode
	}
	if merged.URL == "" {
		merged.URL = incoming.URL
	}
	if merged.Text == "" {
		merged.Text = incoming.Text
	}
	if merged.AuthorUsername == "" {
		merged.AuthorUsername = incoming.AuthorUsername
	}
	if merged.PublishedAt.IsZero() {
		merged.PublishedAt = incoming.PublishedAt
	}
	if incoming.Likes != nil {
		merged.Likes = cloneInt(incoming.Likes)
	}
	if incoming.Replies != nil {
		merged.Replies = cloneInt(incoming.Replies)
	}
	if incoming.Reposts != nil {
		merged.Reposts = cloneInt(incoming.Reposts)
	}
	if incoming.Quotes != nil {
		merged.Quotes = cloneInt(incoming.Quotes)
	}
	if incoming.Views != nil {
		merged.Views = cloneInt(incoming.Views)
	}
	if merged.DetectedTopic == "" {
		merged.DetectedTopic = incoming.DetectedTopic
	}
	if merged.FirstSeenAt.IsZero() || (!incoming.FirstSeenAt.IsZero() && incoming.FirstSeenAt.Before(merged.FirstSeenAt)) {
		merged.FirstSeenAt = incoming.FirstSeenAt
	}
	if incoming.LastSeenAt.After(merged.LastSeenAt) {
		merged.LastSeenAt = incoming.LastSeenAt
	}
	if relevanceAssessmentForPost(incoming).Score > relevanceAssessmentForPost(merged).Score || merged.RelevanceLabel == "" {
		merged.RelevanceScore = incoming.RelevanceScore
		merged.RelevanceLabel = incoming.RelevanceLabel
		merged.RelevanceReasons = append([]string(nil), incoming.RelevanceReasons...)
	}
	return merged
}

func applyRelevance(post *Post, assessment RelevanceAssessment) {
	if post == nil {
		return
	}
	post.RelevanceScore = assessment.Score
	post.RelevanceLabel = string(assessment.Label)
	post.RelevanceReasons = append([]string(nil), assessment.Reasons...)
}

func assessmentForPost(post Post) RelevanceAssessment {
	return RelevanceAssessment{
		Score:   post.RelevanceScore,
		Label:   RelevanceLabel(post.RelevanceLabel),
		Reasons: append([]string(nil), post.RelevanceReasons...),
	}
}

func relevanceAssessmentForPost(post Post) RelevanceAssessment {
	assessment := assessmentForPost(post)
	if !validRelevanceLabel(assessment.Label) {
		return RelevanceAssessment{Score: relevanceScoreIrrelevant, Label: RelevanceIrrelevant}
	}
	return assessment
}

func recordQueryRelevance(report *QueryReport, assessment RelevanceAssessment) {
	if report == nil {
		return
	}
	switch assessment.Label {
	case RelevanceRelevant:
		report.Relevant++
	case RelevanceAdjacent:
		report.Adjacent++
	case RelevanceIrrelevant:
		report.Irrelevant++
	case RelevanceUncertain:
		report.Uncertain++
	default:
		report.Uncertain++
	}
	report.Precision = 0
	if report.ResultCount > 0 {
		report.Precision = float64(report.Relevant) / float64(report.ResultCount)
	}
	report.ContributesToConclusions = report.Relevant > 0
}

func filterPostsByRelevance(posts []Post, label RelevanceLabel) []Post {
	out := make([]Post, 0, len(posts))
	for _, post := range posts {
		if RelevanceLabel(post.RelevanceLabel) == label {
			out = append(out, post)
		}
	}
	return out
}

func selectProfileCandidatesByRelevance(relevant, adjacent []Post, limit int) []string {
	if limit <= 0 {
		return nil
	}
	out := selectProfileCandidates(relevant, limit)
	if len(out) >= limit {
		return out
	}
	seen := map[string]bool{}
	for _, username := range out {
		seen[username] = true
	}
	for _, username := range selectProfileCandidates(adjacent, limit) {
		if seen[username] {
			continue
		}
		out = append(out, username)
		seen[username] = true
		if len(out) >= limit {
			break
		}
	}
	return out
}

func appendRankedAuthors(primary, secondary []RankedAuthor) []RankedAuthor {
	out := make([]RankedAuthor, 0, len(primary)+len(secondary))
	seen := map[string]bool{}
	for _, authors := range [][]RankedAuthor{primary, secondary} {
		for _, author := range authors {
			username := normalizeAuthor(author.Author.Username)
			if username == "" || seen[username] {
				continue
			}
			seen[username] = true
			out = append(out, author)
		}
	}
	return out
}

func selectProfileCandidates(posts []Post, limit int) []string {
	if limit <= 0 {
		return nil
	}
	type candidate struct {
		username string
		latest   time.Time
	}
	byAuthor := map[string]*candidate{}
	for _, post := range posts {
		username := normalizeAuthor(post.AuthorUsername)
		if username == "" {
			continue
		}
		item := byAuthor[username]
		if item == nil {
			item = &candidate{username: username}
			byAuthor[username] = item
		}
		if post.PublishedAt.After(item.latest) {
			item.latest = post.PublishedAt
		}
	}
	items := make([]candidate, 0, len(byAuthor))
	for _, item := range byAuthor {
		items = append(items, *item)
	}
	sort.Slice(items, func(i, j int) bool {
		if !items[i].latest.Equal(items[j].latest) {
			return items[i].latest.After(items[j].latest)
		}
		return items[i].username < items[j].username
	})
	if len(items) > limit {
		items = items[:limit]
	}
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.username
	}
	return out
}

func selectBaselineCandidates(authors []RankedAuthor, limit int) []string {
	if limit <= 0 {
		return nil
	}
	out := make([]string, 0, minInt(limit, len(authors)))
	seen := map[string]bool{}
	for _, author := range authors {
		username := normalizeAuthor(author.Author.Username)
		if username == "" || seen[username] {
			continue
		}
		seen[username] = true
		out = append(out, username)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func sortedPosts(postsByID map[string]Post) []Post {
	out := make([]Post, 0, len(postsByID))
	for _, post := range postsByID {
		out = append(out, post)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if strings.EqualFold(existing, value) {
			return values
		}
	}
	return append(values, value)
}

func cloneInt(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func takeRankedPosts(posts []RankedPost, limit int) []RankedPost {
	if len(posts) <= limit {
		return append([]RankedPost(nil), posts...)
	}
	return append([]RankedPost(nil), posts[:limit]...)
}

func takeRankedAuthors(authors []RankedAuthor, limit int) []RankedAuthor {
	if len(authors) <= limit {
		return append([]RankedAuthor(nil), authors...)
	}
	return append([]RankedAuthor(nil), authors[:limit]...)
}

func topOutperforming(posts []RankedPost, limit int) []RankedPost {
	out := make([]RankedPost, 0, len(posts))
	for _, post := range posts {
		if post.RelativePerformance != nil {
			out = append(out, post)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if compareOptional(out[i].RelativePerformance, out[j].RelativePerformance) != 0 {
			return compareOptional(out[i].RelativePerformance, out[j].RelativePerformance) > 0
		}
		if out[i].RankScore != out[j].RankScore {
			return out[i].RankScore > out[j].RankScore
		}
		return out[i].Post.ID < out[j].Post.ID
	})
	return takeRankedPosts(out, limit)
}

func datasetCoverage(posts map[string]Post, authors map[string]Author) DatasetCoverage {
	total := len(posts)
	known := [5]int{}
	for _, post := range posts {
		for i, value := range []*int64{post.Likes, post.Replies, post.Reposts, post.Quotes, post.Views} {
			if value != nil {
				known[i]++
			}
		}
	}
	followersKnown := 0
	for _, author := range authors {
		if author.FollowerCount != nil {
			followersKnown++
		}
	}
	return DatasetCoverage{
		Posts:     total,
		Likes:     metricCoverage(known[0], total),
		Replies:   metricCoverage(known[1], total),
		Reposts:   metricCoverage(known[2], total),
		Quotes:    metricCoverage(known[3], total),
		Views:     metricCoverage(known[4], total),
		Followers: metricCoverage(followersKnown, len(authors)),
	}
}

func collectionCoverage(queries []QueryReport, posts map[string]Post, authors map[string]Author, report Report, baselinePosts map[string][]Post, partial CollectionCoverage) CollectionCoverage {
	relevance := relevanceCoverage(posts)
	coverage := CollectionCoverage{
		SearchSurface:             "public SSR search + bounded GraphQL cursor fallback",
		QueriesAttempted:          len(queries),
		UniquePosts:               len(posts),
		RelevantPosts:             relevance.Relevant,
		AdjacentPosts:             relevance.Adjacent,
		IrrelevantPosts:           relevance.Irrelevant,
		UncertainPosts:            relevance.Uncertain,
		UniqueAuthors:             len(authors),
		RelevantAuthors:           relevantAuthorCount(posts),
		ProfilesAttempted:         report.ProfilesAttempted,
		ProfilesFetched:           report.ProfilesFetched,
		AuthorsWithKnownFollowers: report.Coverage.Followers.Known,
		BaselineAuthorsAttempted:  report.Baseline.AuthorsAttempted,
		BaselineAuthorsReliable:   report.Baseline.AuthorsWithReliableData,
		BaselinePostsCollected:    report.Baseline.PostsCollected,
		Relevance:                 relevance,
	}
	for _, query := range queries {
		coverage.RawPostsCollected += query.ResultCount
		if query.Successful {
			coverage.QueriesSuccessful++
		}
		if query.ResultCount > 0 {
			coverage.QueriesWithResults++
		}
	}
	if coverage.QueriesAttempted < partial.QueriesAttempted {
		coverage.QueriesAttempted = partial.QueriesAttempted
	}
	if coverage.QueriesSuccessful < partial.QueriesSuccessful {
		coverage.QueriesSuccessful = partial.QueriesSuccessful
	}
	if coverage.QueriesWithResults < partial.QueriesWithResults {
		coverage.QueriesWithResults = partial.QueriesWithResults
	}
	if coverage.RawPostsCollected < partial.RawPostsCollected {
		coverage.RawPostsCollected = partial.RawPostsCollected
	}
	if coverage.RelevantPosts < partial.RelevantPosts {
		coverage.RelevantPosts = partial.RelevantPosts
	}
	if coverage.AdjacentPosts < partial.AdjacentPosts {
		coverage.AdjacentPosts = partial.AdjacentPosts
	}
	if coverage.IrrelevantPosts < partial.IrrelevantPosts {
		coverage.IrrelevantPosts = partial.IrrelevantPosts
	}
	if coverage.UncertainPosts < partial.UncertainPosts {
		coverage.UncertainPosts = partial.UncertainPosts
	}
	if coverage.RelevantAuthors < partial.RelevantAuthors {
		coverage.RelevantAuthors = partial.RelevantAuthors
	}
	if coverage.Relevance.Assessed < partial.Relevance.Assessed {
		coverage.Relevance = partial.Relevance
	}
	// baselinePosts is intentionally accepted so callers can see that the
	// coverage field remains meaningful even when all baseline rows overlapped
	// with topic evidence and were excluded from the independent denominator.
	if coverage.BaselinePostsCollected == 0 {
		for _, authorPosts := range baselinePosts {
			coverage.BaselinePostsCollected += len(authorPosts)
		}
	}
	return coverage
}

func relevanceCoverage(posts map[string]Post) RelevanceCoverage {
	coverage := RelevanceCoverage{Assessed: len(posts)}
	for _, post := range posts {
		switch RelevanceLabel(post.RelevanceLabel) {
		case RelevanceRelevant:
			coverage.Relevant++
		case RelevanceAdjacent:
			coverage.Adjacent++
		case RelevanceIrrelevant:
			coverage.Irrelevant++
		default:
			coverage.Uncertain++
		}
	}
	if coverage.Assessed > 0 {
		coverage.Precision = float64(coverage.Relevant) / float64(coverage.Assessed)
	}
	return coverage
}

func relevantAuthorCount(posts map[string]Post) int {
	seen := map[string]bool{}
	for _, post := range posts {
		if RelevanceLabel(post.RelevanceLabel) != RelevanceRelevant {
			continue
		}
		if username := normalizeAuthor(post.AuthorUsername); username != "" {
			seen[username] = true
		}
	}
	return len(seen)
}

func researchLimitations(report Report) []string {
	limitations := []string{
		"Collection uses public SSR search with a bounded persisted-query cursor fallback; authentication walls, CAPTCHAs, and access controls are not bypassed.",
		"Search, profile, and baseline limits are bounded; counts describe this run and are not the total Threads population.",
		"Relevance labels are a conservative deterministic lexical/context gate; adjacent and uncertain posts require human review before being treated as topic evidence.",
		"Raw diagnostic rankings retain every collected post, while topic rankings and downstream conclusions use relevant posts only.",
		"Follower-relative metrics and outperformance are shown only when the source fields and an independent author baseline support them.",
		"Sample-strength labels describe observed evidence size, not universal Threads truth or a forecast.",
	}
	if report.Analysis.Enabled {
		limitations = append(limitations, "LLM classification can be partial or unavailable; deterministic mechanics and aggregation remain usable and are reported separately.")
	}
	return limitations
}

func followerRelativeRankingStatus(coverage MetricCoverage) string {
	if coverage.Known == 0 {
		return "unavailable"
	}
	if coverage.Known < coverage.Total {
		return "partial"
	}
	return "available"
}

func baselineCoverage(attempts map[string]bool, posts map[string][]Post, collected, known, minPosts int, weights EngagementWeights) BaselineCoverage {
	withPosts := 0
	reliable := 0
	for _, authorPosts := range posts {
		values := make([]float64, 0, len(authorPosts))
		for _, post := range authorPosts {
			if engagementMetricCount(post) > 0 {
				values = append(values, weightedEngagement(post, weights))
			}
		}
		if len(values) > 0 {
			withPosts++
		}
		if len(values) >= minPosts && median(values) > 0 {
			reliable++
		}
	}
	coverage := BaselineCoverage{
		AuthorsAttempted:         len(attempts),
		AuthorsWithPosts:         withPosts,
		AuthorsWithReliableData:  reliable,
		PostsCollected:           collected,
		PostsWithKnownEngagement: known,
	}
	if coverage.AuthorsAttempted > 0 {
		coverage.Percent = float64(reliable) * 100 / float64(coverage.AuthorsAttempted)
	}
	return coverage
}

func runStatus(runErr error, warningCount int) RunStatus {
	if runErr != nil {
		return RunStatusFailed
	}
	if warningCount > 0 {
		return RunStatusCompletedWithWarns
	}
	return RunStatusCompleted
}

func errorText(err error) any {
	if err == nil {
		return nil
	}
	return err.Error()
}

func contextAbort(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return nil
}

func appendWarningOnce(warnings []string, warning string) []string {
	for _, existing := range warnings {
		if existing == warning {
			return warnings
		}
	}
	return append(warnings, warning)
}

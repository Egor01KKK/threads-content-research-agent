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

const (
	deepWavePrimary   = "primary"
	deepWaveSecondary = "secondary"

	deepStageAnchors   = "anchors"
	deepStageProfiles  = "profiles"
	deepStageSecondary = "secondary_anchors"
	deepStageReplies   = "replies"
	deepStageFinalize  = "finalize"

	// Ten hits per short identity anchor keeps discovery broad enough to find
	// operators beyond the first visible post while remaining a bounded search
	// window. Profile crawling is separately capped after verification.
	deepProbeLimit        = 10
	deepMaxSecondary      = 30
	deepMaxReplyRoots     = 100
	deepMaxRepliesPerRoot = 20
)

type deepSeedCandidate struct {
	Username         string
	Score            float64
	Likelihood       string
	PositiveEvidence map[string]bool
	NegativeEvidence map[string]bool
	AnchorQueries    map[string]bool
	SeedPostIDs      map[string]bool
	SourceStages     map[string]bool
}

type deepState struct {
	posts         map[string]DeepPostRecord
	probes        map[string]DeepAnchorProbe
	profiles      map[string]DeepSeedProfile
	secondary     map[string]DeepSecondaryAnchor
	replies       map[string]DeepReplySignal
	provenance    map[string]DeepProvenance
	authors       map[string]Author
	candidates    map[string]*deepSeedCandidate
	searchPosts   map[string]bool
	profilePosts  map[string]bool
	baselinePosts map[string][]Post
	queriesByPost map[string][]string
}

func newDeepState() *deepState {
	return &deepState{
		posts:         map[string]DeepPostRecord{},
		probes:        map[string]DeepAnchorProbe{},
		profiles:      map[string]DeepSeedProfile{},
		secondary:     map[string]DeepSecondaryAnchor{},
		replies:       map[string]DeepReplySignal{},
		provenance:    map[string]DeepProvenance{},
		authors:       map[string]Author{},
		candidates:    map[string]*deepSeedCandidate{},
		searchPosts:   map[string]bool{},
		profilePosts:  map[string]bool{},
		baselinePosts: map[string][]Post{},
		queriesByPost: map[string][]string{},
	}
}

func (s *deepState) load(store *Store, runID int64) error {
	var err error
	if s.probes, err = store.loadDeepAnchorProbes(runID); err != nil {
		return err
	}
	if s.profiles, err = store.loadDeepProfiles(runID); err != nil {
		return err
	}
	if s.posts, err = store.loadDeepPosts(runID); err != nil {
		return err
	}
	if s.secondary, err = store.loadDeepSecondaryAnchors(runID); err != nil {
		return err
	}
	if s.replies, err = loadDeepRepliesMap(store, runID); err != nil {
		return err
	}
	provenance, err := store.loadDeepProvenance(runID)
	if err != nil {
		return err
	}
	for _, edge := range provenance {
		s.provenance[deepProvenanceKey(edge)] = edge
		record := s.posts[edge.PostID]
		record.Provenance = appendDeepProvenance(record.Provenance, edge)
		record.Stages = appendUnique(record.Stages, edge.SourceStage)
		record.SearchQueries = appendUnique(record.SearchQueries, edge.AnchorQuery)
		s.posts[edge.PostID] = record
		if edge.SourceStage == "ANCHOR_QUERY" || edge.SourceStage == "SECONDARY_ANCHOR" {
			s.searchPosts[edge.PostID] = true
			if edge.AnchorQuery != "" {
				s.queriesByPost[edge.PostID] = appendUnique(s.queriesByPost[edge.PostID], edge.AnchorQuery)
			}
		}
		if edge.ProfileUsername != "" {
			s.profilePosts[edge.PostID] = true
		}
	}
	for id, record := range s.posts {
		username := normalizeAuthor(record.Post.AuthorUsername)
		if username != "" {
			s.authors[username] = authorFor(s.authors, username)
		}
		if record.Assessment.RussianLanguage == Yes && username != "" {
			s.observeCandidate(username, record.Post.Text, record.Assessment, "", id, "")
		}
	}
	for username, profile := range s.profiles {
		if profile.Profile.Username != "" {
			s.authors[username] = profile.Profile
		}
		candidate := s.candidates[username]
		if candidate == nil {
			candidate = newDeepCandidate(username)
			s.candidates[username] = candidate
		}
		candidate.Score = maxFloat(candidate.Score, profile.SeedScore)
		candidate.Likelihood = strongerOwnerLikelihood(candidate.Likelihood, profile.OwnerLikelihood)
		for _, value := range profile.PositiveEvidence {
			candidate.PositiveEvidence[value] = true
		}
		for _, value := range profile.NegativeEvidence {
			candidate.NegativeEvidence[value] = true
		}
		for _, value := range profile.AnchorQueries {
			candidate.AnchorQueries[value] = true
		}
		for _, value := range profile.SeedPostIDs {
			candidate.SeedPostIDs[value] = true
		}
		for _, value := range profile.SourceStages {
			candidate.SourceStages[value] = true
		}
	}
	return nil
}

func loadDeepRepliesMap(store *Store, runID int64) (map[string]DeepReplySignal, error) {
	values, err := store.loadDeepReplies(runID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]DeepReplySignal, len(values))
	for _, value := range values {
		out[value.ReplyID] = value
	}
	return out, nil
}

// runDeepResearch is intentionally a separate pipeline. It does not call the
// QueryExpander and therefore cannot accidentally send the long objective as a
// search query.
func runDeepResearch(ctx context.Context, cfg Config, collector Collector, store *Store) (report Report, runErr error) {
	anchors := DeepAnchorBank()
	if cfg.AnchorLimit > 0 && cfg.AnchorLimit < len(anchors) {
		anchors = anchors[:cfg.AnchorLimit]
	}
	runID, startedAt, resumed, err := store.startOrResumeDeepRun(cfg.Topic, cfg.ResumeRunID, int64(len(anchors)))
	if err != nil {
		return Report{}, err
	}
	now := time.Now
	deep := &DeepReport{
		Mode:                   ModeDeep,
		Objective:              cfg.Topic,
		AnonymousCollection:    true,
		AnalysisProviderCalled: false,
		AnchorBank:             append([]DeepAnchor(nil), anchors...),
		StageStatus:            map[string]string{},
	}
	report = Report{
		Topic:     cfg.Topic,
		Language:  "ru",
		RunID:     runID,
		Status:    RunStatusRunning,
		StartedAt: startedAt,
		Scoring:   cfg,
		Deep:      deep,
		Collection: CollectionCoverage{
			SearchSurface: "public anonymous SSR search + profile-first bounded corpus collection",
		},
	}
	state := newDeepState()
	if resumed {
		if err := state.load(store, runID); err != nil {
			return report, err
		}
		hydrateDeepFunnelFromState(&deep.Funnel, state)
	}
	stageStarted := map[string]time.Time{deepStageAnchors: now()}
	markStage := func(stage, status string) {
		deep.StageStatus[stage] = status
		if started, ok := stageStarted[stage]; ok {
			if deep.Funnel.StageElapsed == nil {
				deep.Funnel.StageElapsed = map[string]string{}
			}
			deep.Funnel.StageElapsed[stage] = now().Sub(started).Round(time.Millisecond).String()
		}
	}
	startStage := func(stage string) {
		stageStarted[stage] = now()
		deep.StageStatus[stage] = "running"
	}

	defer func() {
		if finalizeErr := finalizeDeepResearch(ctx, cfg, store, state, &report, deep, startedAt); finalizeErr != nil {
			runErr = errors.Join(runErr, finalizeErr)
		}
		report.CompletedAt = now()
		report.Status = runStatus(runErr, len(report.Warnings))
		deep.Warnings = append([]string(nil), report.Warnings...)
		deep.Limitations = deepLimitations(report, state)
		report.Limitations = append([]string(nil), deep.Limitations...)
		deep.StageStatus[deepStageFinalize] = "completed"
		_ = store.updateDeepRunStatus(runID, deepStageFinalize, report.Status, map[string]any{
			"completed_at": report.CompletedAt,
			"status":       report.Status,
			"corpus_posts": len(state.posts),
		})
		if finishErr := store.FinishRun(runID, report.CompletedAt, report.Status, errorText(runErr)); finishErr != nil {
			runErr = errors.Join(runErr, finishErr)
			report.Status = RunStatusFailed
		}
	}()

	startStage(deepStageAnchors)
	for _, anchor := range anchors {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		key := deepRecordKey(deepWavePrimary, anchor.Query)
		if existing, ok := state.probes[key]; ok && existing.Status == "completed" {
			continue
		}
		probe, probeErr := collectDeepAnchor(ctx, cfg, collector, store, state, runID, anchor, deepWavePrimary, deepProbeLimit, &deep.Funnel, &deep.SearchPagination)
		state.probes[key] = probe
		if probeErr != nil {
			if abortErr := contextAbort(ctx, probeErr); abortErr != nil {
				return report, abortErr
			}
			report.Warnings = appendWarningOnce(report.Warnings, fmt.Sprintf("anchor %q failed: %v", anchor.Query, probeErr))
		}
		if err := store.saveDeepAnchorProbe(runID, probe); err != nil {
			return report, err
		}
		if err := store.saveDeepCheckpoint(runID, deepStageAnchors, anchor.Query, probe); err != nil {
			return report, err
		}
	}
	markStage(deepStageAnchors, "completed")
	if err := store.updateDeepRunStatus(runID, deepStageAnchors, RunStatusRunning, map[string]any{"anchors_completed": len(state.probes)}); err != nil {
		return report, err
	}

	selectDeepSeedProfiles(state, cfg.MaxSeedProfiles)
	markDeepSelectedAnchorProbes(state)
	if err := persistDeepProfiles(store, runID, state); err != nil {
		return report, err
	}
	if err := persistDeepAnchorProbes(store, runID, state); err != nil {
		return report, err
	}
	deep.Funnel.OwnerSeedCandidates = len(state.candidates)
	deep.Funnel.OwnerSeedProfiles = countSeedSelectedProfiles(state.profiles)
	startStage(deepStageProfiles)
	if err := collectSelectedProfiles(ctx, cfg, collector, store, state, runID, deep, false, &report); err != nil {
		return report, err
	}
	markStage(deepStageProfiles, "completed")
	if err := store.updateDeepRunStatus(runID, deepStageProfiles, RunStatusRunning, map[string]any{"profiles_completed": deep.Funnel.ProfilesFetched}); err != nil {
		return report, err
	}

	startStage(deepStageSecondary)
	prepareSecondaryAnchors(state, cfg, deepMaxSecondary)
	for _, phrase := range sortedSecondaryPhrases(state.secondary) {
		anchor := state.secondary[phrase]
		if !anchor.Eligible || anchor.Collected {
			continue
		}
		position := len(anchors) + len(state.probes)
		anchorSpec := DeepAnchor{Query: anchor.Phrase, Category: "observed_pain_language", Position: position}
		probe, probeErr := collectDeepAnchor(ctx, cfg, collector, store, state, runID, anchorSpec, deepWaveSecondary, deepProbeLimit, &deep.Funnel, &deep.SearchPagination)
		probe.SelectedForProfileSeeds = false
		state.probes[deepRecordKey(deepWaveSecondary, anchor.Phrase)] = probe
		anchor.Collected = true
		anchor.ResultCount = probe.QueryReport.ResultCount
		anchor.UniqueContribution = probe.UniqueAuthorContribution
		if probeErr != nil {
			anchor.Error = probeErr.Error()
			report.Warnings = appendWarningOnce(report.Warnings, fmt.Sprintf("secondary anchor %q failed: %v", anchor.Phrase, probeErr))
		}
		state.secondary[phrase] = anchor
		if err := store.saveDeepSecondaryAnchor(runID, anchor); err != nil {
			return report, err
		}
		if err := store.saveDeepAnchorProbe(runID, probe); err != nil {
			return report, err
		}
		if err := store.saveDeepCheckpoint(runID, deepStageSecondary, phrase, anchor); err != nil {
			return report, err
		}
		if err := contextAbort(ctx, probeErr); err != nil {
			return report, err
		}
	}
	selectDeepSeedProfiles(state, cfg.MaxSeedProfiles)
	markDeepSelectedAnchorProbes(state)
	if err := persistDeepProfiles(store, runID, state); err != nil {
		return report, err
	}
	if err := persistDeepAnchorProbes(store, runID, state); err != nil {
		return report, err
	}
	deep.Funnel.OwnerSeedProfiles = countSeedSelectedProfiles(state.profiles)
	if err := collectSelectedProfiles(ctx, cfg, collector, store, state, runID, deep, true, &report); err != nil {
		return report, err
	}
	markStage(deepStageSecondary, "completed")
	if err := store.updateDeepRunStatus(runID, deepStageSecondary, RunStatusRunning, map[string]any{"secondary_anchors": len(state.secondary)}); err != nil {
		return report, err
	}

	startStage(deepStageReplies)
	if err := collectDeepReplies(ctx, cfg, collector, store, state, runID, deep, &report); err != nil {
		return report, err
	}
	// One bounded reply-driven snowball pass. It is deliberately after root
	// evidence collection and can never exceed the same global profile ceiling.
	selectReplySnowballProfiles(state, cfg.MaxSeedProfiles)
	if err := persistDeepProfiles(store, runID, state); err != nil {
		return report, err
	}
	if err := collectSelectedProfiles(ctx, cfg, collector, store, state, runID, deep, false, &report); err != nil {
		return report, err
	}
	markStage(deepStageReplies, "completed")
	if err := store.updateDeepRunStatus(runID, deepStageReplies, RunStatusRunning, map[string]any{"replies": len(state.replies)}); err != nil {
		return report, err
	}

	return report, nil
}

func collectDeepAnchor(ctx context.Context, cfg Config, collector Collector, store *Store, state *deepState, runID int64, anchor DeepAnchor, wave string, limit int, funnel *DeepFunnel, pagination *DeepPaginationMetrics) (DeepAnchorProbe, error) {
	probe := DeepAnchorProbe{
		Anchor:       anchor,
		Wave:         wave,
		ProbeLimit:   limit,
		Status:       "running",
		DateCoverage: DeepDateCoverage{ByWeek: map[string]int{}, ByMonth: map[string]int{}},
		Pagination:   DeepPaginationMetrics{Requests: 1, PagesFetched: 1, StopReason: "ssr_window_or_per_query_ceiling"},
	}
	queryReport := QueryReport{Query: anchor.Query, Language: "ru"}
	queryID, err := store.addOrGetQuery(runID, anchor.Query, anchor.Position, time.Now().UTC())
	if err != nil {
		probe.Status = "failed"
		probe.Error = err.Error()
		return probe, err
	}
	funnel.Requests++
	funnel.SearchRequests++
	seenLocal := map[string]bool{}
	seenLocalAuthors := map[string]bool{}
	newGlobalPosts := 0
	resultErr := error(nil)
	for result, streamErr := range collector.Search(ctx, anchor.Query, limit) {
		if streamErr != nil {
			resultErr = streamErr
			break
		}
		if limit > 0 && queryReport.ResultCount >= limit {
			break
		}
		queryReport.ResultCount++
		probe.Pagination.RawYield++
		funnel.RawSearchPosts++
		post := postFromSearchResult(result, cfg.Topic, time.Now().UTC())
		if post.ID == "" {
			continue
		}
		assessment := AssessBusinessPain(cfg.Topic, post.Text)
		if assessment.RussianLanguage == Yes {
			probe.RussianPosts++
		}
		if ownerLikely(assessment.OwnerLikelihood) {
			probe.OwnerLikelyPosts++
		}
		if isPainSignal(assessment) {
			probe.PainPosts++
		}
		if isITActionable(assessment) {
			probe.ITActionablePosts++
		}
		if assessment.SolutionSeeking == Yes {
			probe.SolutionSeekingPosts++
		}
		if !post.PublishedAt.IsZero() {
			addDate(&probe.DateCoverage, post.PublishedAt)
		}
		if seenLocal[post.ID] {
			probe.Duplicates++
			probe.Pagination.Duplicates++
		} else {
			seenLocal[post.ID] = true
			probe.UniquePosts++
			probe.Pagination.UniqueYield++
			username := normalizeAuthor(post.AuthorUsername)
			if username != "" && !seenLocalAuthors[username] {
				seenLocalAuthors[username] = true
				probe.UniqueAuthors++
				if !state.hasSearchAuthor(username) {
					probe.UniqueAuthorContribution++
				}
			}
			if !state.searchPosts[post.ID] {
				newGlobalPosts++
			}
		}
		if post.AuthorUsername != "" {
			probe.Anchor = anchor
			state.observeCandidate(post.AuthorUsername, post.Text, assessment, anchor.Query, post.ID, anchorStage(wave))
		}
		record := state.upsertPost(post, assessment, anchorStage(wave))
		state.searchPosts[post.ID] = true
		edge := DeepProvenance{
			PostID: post.ID, SourceStage: anchorStage(wave), AnchorQuery: anchor.Query,
			AnchorCategory: anchor.Category, Depth: 0, ObservedAt: time.Now().UTC(),
		}
		if wave == deepWaveSecondary {
			edge.ExtractedPhrase = anchor.Query
			edge.SecondaryAnchor = anchor.Query
		}
		record.Provenance = appendDeepProvenance(record.Provenance, edge)
		state.provenance[deepProvenanceKey(edge)] = edge
		record.SearchQueries = appendUnique(record.SearchQueries, anchor.Query)
		state.queriesByPost[post.ID] = appendUnique(state.queriesByPost[post.ID], anchor.Query)
		state.posts[post.ID] = record
		if post.AuthorUsername != "" {
			username := normalizeAuthor(post.AuthorUsername)
			state.authors[username] = authorFor(state.authors, username)
		}
		if assessmentLabel := deepRelevanceAssessment(assessment); assessmentLabel.Label != "" {
			recordQueryRelevance(&queryReport, assessmentLabel)
		}
		if err := store.UpsertPost(post); err != nil {
			return probe, err
		}
		if err := store.saveDeepPost(runID, record); err != nil {
			return probe, err
		}
		if err := store.saveDeepProvenance(runID, edge); err != nil {
			return probe, err
		}
		if err := store.LinkPostToQuery(runID, queryID, post.ID, time.Now().UTC()); err != nil {
			return probe, err
		}
	}
	probe.Pagination.PostsPerPage = []int{probe.UniquePosts}
	if probe.QueryReport.ResultCount < limit && resultErr == nil {
		probe.Pagination.Exhausted = true
	} else if probe.QueryReport.ResultCount >= limit {
		probe.Pagination.StoppedAtCeiling = true
		probe.Pagination.StopReason = "per_query_probe_ceiling"
	}
	if resultErr != nil {
		probe.Status = "completed_with_warning"
		probe.Error = resultErr.Error()
		probe.Pagination.StopReason = "access_or_stream_error"
	} else {
		probe.Status = "completed"
	}
	probe.QueryReport = queryReport
	probe.QueryReport.UniqueNewPosts = newGlobalPosts
	probe.QueryReport.Successful = resultErr == nil
	if resultErr != nil {
		probe.QueryReport.Error = resultErr.Error()
	}
	probe.QueryReport.Precision = probe.PainPrecision
	probe.QueryReport.ContributesToConclusions = probe.OwnerLikelyPosts > 0 || probe.PainPosts > 0
	probe.RussianRate = precision(probe.RussianPosts, probe.UniquePosts)
	probe.OwnerLikelihoodRate = precision(probe.OwnerLikelyPosts, probe.UniquePosts)
	probe.PainRate = precision(probe.PainPosts, probe.UniquePosts)
	probe.ITActionabilityRate = precision(probe.ITActionablePosts, probe.UniquePosts)
	probe.PainPrecision = probe.PainRate
	probe.QueryReport.ProbeResultCount = probe.QueryReport.ResultCount
	probe.QueryReport.ProbeRelevantPosts = probe.QueryReport.Relevant
	probe.QueryReport.ProbeOwnerLikelyPosts = probe.OwnerLikelyPosts
	probe.QueryReport.ProbePainPosts = probe.PainPosts
	probe.QueryReport.ProbeITActionablePosts = probe.ITActionablePosts
	probe.QueryReport.ProbeUniqueContribution = probe.UniqueAuthorContribution
	probe.QueryReport.ProbeDuplicates = probe.Duplicates
	probe.QueryReport.ProbePrecision = probe.PainPrecision
	probe.DiscoveryValue = deepDiscoveryValue(probe)
	probe.Pagination.RateLimited = looksRateLimited(resultErr)
	probe.Pagination.AccessDegraded = resultErr != nil
	if probe.Pagination.RateLimited {
		probe.Pagination.StopReason = "rate_limit_or_access_degradation"
	}
	*pagination = mergePagination(*pagination, probe.Pagination)
	probe.QueryReport.DeepResultCount = probe.QueryReport.ResultCount
	queryError := ""
	if resultErr != nil {
		queryError = resultErr.Error()
	}
	if err := store.FinishQuery(queryID, probe.QueryReport.ResultCount, probe.UniquePosts, queryError); err != nil {
		return probe, err
	}
	if err := store.SaveQueryQuality(queryID, probe.QueryReport); err != nil {
		return probe, err
	}
	return probe, resultErr
}

func (s *deepState) upsertPost(post Post, assessment BusinessPainAssessment, stage string) DeepPostRecord {
	record, ok := s.posts[post.ID]
	if ok {
		record.Post = mergePosts(record.Post, post)
	} else {
		record.Post = post
	}
	record.Stages = appendUnique(record.Stages, stage)
	relevance := deepRelevanceAssessment(assessment)
	record.Post.RelevanceScore = relevance.Score
	record.Post.RelevanceLabel = string(relevance.Label)
	record.Post.RelevanceReasons = append([]string(nil), relevance.Reasons...)
	if record.Assessment.RussianLanguage != Yes || assessment.RussianLanguage == Yes {
		record.Assessment = assessment
	}
	return record
}

func (s *deepState) observeCandidate(username, text string, assessment BusinessPainAssessment, anchor, postID, stage string) {
	username = normalizeAuthor(username)
	if username == "" || !isRussianText(text) {
		return
	}
	candidate := s.candidates[username]
	if candidate == nil {
		candidate = newDeepCandidate(username)
		s.candidates[username] = candidate
	}
	score, likelihood, positives, negatives := ownerSeedEvidence(text, assessment)
	if likelihoodRank(likelihood) > likelihoodRank(candidate.Likelihood) {
		candidate.Likelihood = likelihood
	}
	if !candidate.SeedPostIDs[postID] {
		candidate.Score += score
		candidate.SeedPostIDs[postID] = true
	}
	if anchor != "" && !candidate.AnchorQueries[anchor] {
		candidate.AnchorQueries[anchor] = true
		candidate.Score += 0.5
	}
	if stage != "" {
		candidate.SourceStages[stage] = true
	}
	for _, value := range positives {
		candidate.PositiveEvidence[value] = true
	}
	for _, value := range negatives {
		candidate.NegativeEvidence[value] = true
	}
}

func newDeepCandidate(username string) *deepSeedCandidate {
	return &deepSeedCandidate{
		Username:         normalizeAuthor(username),
		PositiveEvidence: map[string]bool{},
		NegativeEvidence: map[string]bool{},
		AnchorQueries:    map[string]bool{},
		SeedPostIDs:      map[string]bool{},
		SourceStages:     map[string]bool{},
	}
}

func selectDeepSeedProfiles(state *deepState, limit int) {
	// Selected used to mean both "seed selected" and "profile-post crawl
	// selected". Keep old completed runs resumable, while new runs use the
	// explicit SeedSelected/VerifiedOwnerContext split.
	for username, profile := range state.profiles {
		profile.Username = normalizeAuthor(username)
		if profile.Selected || profile.ProfileFetched {
			profile.SeedSelected = true
		}
		if profile.ProfileFetched && (profile.Status == "completed" || profile.Status == "completed_with_warning") {
			profile.Selected = true
			if profile.VerificationStatus == "" {
				profile.VerificationStatus = "legacy_collected_without_profile_gate"
				profile.VerificationConfidence = "UNKNOWN"
				profile.VerificationReasons = appendUniqueStrings(profile.VerificationReasons, "completed_by_previous_deep_run")
			}
		}
		if profile.VerificationStatus == "" {
			profile.VerificationStatus = "not_inspected"
		}
		if profile.Status == "" {
			profile.Status = "candidate"
		}
		state.profiles[username] = profile
	}

	// Keep LOW/UNKNOWN candidates in the export, but only candidates with at
	// least two concrete identity/workflow clues are sent to the cheap profile
	// metadata verification stage.
	for username, candidate := range state.candidates {
		profile := deepProfileFromCandidate(candidate)
		if existing, ok := state.profiles[username]; ok {
			profile = mergeDeepSeedProfile(existing, profile)
		}
		if profile.Status == "" {
			profile.Status = "candidate"
		}
		if profile.VerificationStatus == "" {
			profile.VerificationStatus = "not_inspected"
		}
		state.profiles[username] = profile
	}
	candidates := make([]*deepSeedCandidate, 0, len(state.candidates))
	for _, candidate := range state.candidates {
		if !ownerSeedCandidateEligible(candidate) {
			continue
		}
		candidates = append(candidates, candidate)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		if len(candidates[i].AnchorQueries) != len(candidates[j].AnchorQueries) {
			return len(candidates[i].AnchorQueries) > len(candidates[j].AnchorQueries)
		}
		if likelihoodRank(candidates[i].Likelihood) != likelihoodRank(candidates[j].Likelihood) {
			return likelihoodRank(candidates[i].Likelihood) > likelihoodRank(candidates[j].Likelihood)
		}
		return candidates[i].Username < candidates[j].Username
	})
	selected := countSeedSelectedProfiles(state.profiles)
	for _, candidate := range candidates {
		profile := deepProfileFromCandidate(candidate)
		if existing, ok := state.profiles[candidate.Username]; ok {
			profile = mergeDeepSeedProfile(existing, profile)
		}
		if profile.ProfileFetched && (profile.Status == "completed" || profile.Status == "completed_with_warning") {
			profile.Selected = true
			profile.SeedSelected = true
		}
		if !profile.SeedSelected && selected < limit && !profile.ProfileFetched {
			profile.SeedSelected = true
			profile.VerificationStatus = "pending_profile_check"
			profile.Status = "seed_selected"
			selected++
		}
		state.profiles[candidate.Username] = profile
	}
	for username, profile := range state.profiles {
		if !profile.SeedSelected && profile.VerificationStatus == "not_inspected" {
			profile.VerificationReasons = appendUniqueStrings(profile.VerificationReasons, "not_selected_for_profile_check_within_seed_limit")
			state.profiles[username] = profile
		}
	}
}

func persistDeepProfiles(store *Store, runID int64, state *deepState) error {
	for _, profile := range state.profiles {
		if err := store.saveDeepProfile(runID, profile); err != nil {
			return err
		}
	}
	return nil
}

func deepProfileFromCandidate(candidate *deepSeedCandidate) DeepSeedProfile {
	return DeepSeedProfile{
		Username:           candidate.Username,
		SeedScore:          candidate.Score,
		OwnerLikelihood:    candidate.Likelihood,
		PositiveEvidence:   sortedBoolKeys(candidate.PositiveEvidence),
		NegativeEvidence:   sortedBoolKeys(candidate.NegativeEvidence),
		AnchorQueries:      sortedBoolKeys(candidate.AnchorQueries),
		SeedPostIDs:        sortedBoolKeys(candidate.SeedPostIDs),
		SourceStages:       sortedBoolKeys(candidate.SourceStages),
		VerificationStatus: "not_inspected",
		Status:             "candidate",
	}
}

func mergeDeepSeedProfile(existing, current DeepSeedProfile) DeepSeedProfile {
	if current.SeedScore < existing.SeedScore {
		current.SeedScore = existing.SeedScore
	}
	current.OwnerLikelihood = strongerOwnerLikelihood(existing.OwnerLikelihood, current.OwnerLikelihood)
	current.PositiveEvidence = appendUniqueStrings(existing.PositiveEvidence, current.PositiveEvidence...)
	current.NegativeEvidence = appendUniqueStrings(existing.NegativeEvidence, current.NegativeEvidence...)
	current.AnchorQueries = appendUniqueStrings(existing.AnchorQueries, current.AnchorQueries...)
	current.SeedPostIDs = appendUniqueStrings(existing.SeedPostIDs, current.SeedPostIDs...)
	current.SourceStages = appendUniqueStrings(existing.SourceStages, current.SourceStages...)
	current.BusinessCategories = appendUniqueStrings(existing.BusinessCategories, current.BusinessCategories...)
	current.VerificationReasons = appendUniqueStrings(existing.VerificationReasons, current.VerificationReasons...)
	current.VerificationNegativeEvidence = appendUniqueStrings(existing.VerificationNegativeEvidence, current.VerificationNegativeEvidence...)
	current.Profile = mergeAuthor(existing.Profile, current.Profile)
	current.SeedSelected = existing.SeedSelected || current.SeedSelected
	current.Selected = existing.Selected || current.Selected
	current.VerifiedOwnerContext = existing.VerifiedOwnerContext || current.VerifiedOwnerContext
	if existing.VerificationStatus != "" && existing.VerificationStatus != "not_inspected" && existing.VerificationStatus != "pending_profile_check" {
		current.VerificationStatus = existing.VerificationStatus
		current.VerificationConfidence = existing.VerificationConfidence
	}
	current.ProfileFetched = existing.ProfileFetched || current.ProfileFetched
	current.ProfilePosts = maxInt(existing.ProfilePosts, current.ProfilePosts)
	if existing.Status == "completed" || existing.Status == "completed_with_warning" || existing.Status == "rejected" {
		current.Status = existing.Status
	}
	if existing.VerificationStatus == "rejected" {
		current.Status = "rejected"
		current.Selected = false
	}
	return current
}

func countSeedSelectedProfiles(profiles map[string]DeepSeedProfile) int {
	n := 0
	for _, profile := range profiles {
		if profile.SeedSelected || profile.Selected || profile.ProfileFetched {
			n++
		}
	}
	return n
}

func countVerifiedOwnerProfiles(profiles map[string]DeepSeedProfile) int {
	n := 0
	for _, profile := range profiles {
		if profile.VerifiedOwnerContext || profile.VerificationStatus == "accepted" || profile.VerificationStatus == "legacy_collected_without_profile_gate" {
			n++
		}
	}
	return n
}

func countRejectedProfiles(profiles map[string]DeepSeedProfile) int {
	n := 0
	for _, profile := range profiles {
		if profile.VerificationStatus == "rejected" {
			n++
		}
	}
	return n
}

func collectSelectedProfiles(ctx context.Context, cfg Config, collector Collector, store *Store, state *deepState, runID int64, deep *DeepReport, snowballOnly bool, report *Report) error {
	profiles := make([]DeepSeedProfile, 0)
	for _, profile := range state.profiles {
		if !profile.SeedSelected && (!profile.Selected || !profile.ProfileFetched) {
			continue
		}
		if snowballOnly && !containsString(profile.SourceStages, "REPLY_SNOWBALL") {
			continue
		}
		if !snowballOnly && containsString(profile.SourceStages, "REPLY_SNOWBALL") && profile.ProfileFetched {
			continue
		}
		profiles = append(profiles, profile)
	}
	sort.SliceStable(profiles, func(i, j int) bool {
		if profiles[i].SeedScore != profiles[j].SeedScore {
			return profiles[i].SeedScore > profiles[j].SeedScore
		}
		return profiles[i].Username < profiles[j].Username
	})
	for index := range profiles {
		profile := profiles[index]
		if profile.VerificationStatus == "rejected" {
			continue
		}
		if deep.Funnel.ProfilePostsRaw >= cfg.MaxTotalProfilePosts {
			deep.ProfilePagination.StoppedAtCeiling = true
			deep.ProfilePagination.StopReason = "global_profile_post_ceiling"
			break
		}
		if index > 0 {
			if err := waitDeep(ctx, cfg.ProfileDelay); err != nil {
				return err
			}
		}
		if profile.ProfileFetched && (profile.Status == "completed" || profile.Status == "completed_with_warning") {
			continue
		}
		if !profile.ProfileFetched && countVerifiedOwnerProfiles(state.profiles) >= cfg.MaxVerifiedProfiles {
			break
		}
		profile.Status = "fetching"
		state.profiles[profile.Username] = profile
		if err := store.saveDeepProfile(runID, profile); err != nil {
			return err
		}
		deep.Funnel.ProfilesAttempted++
		deep.Funnel.Requests++
		deep.Funnel.ProfileRequests++
		profileData, profileErr := collector.Profile(ctx, profile.Username)
		if profileErr != nil {
			profile.Status = "failed"
			profile.Error = profileErr.Error()
			deep.Funnel.Failures++
			if looksRateLimited(profileErr) {
				deep.Funnel.RateLimitEvents++
			}
			report.Warnings = appendWarningOnce(report.Warnings, fmt.Sprintf("profile @%s unavailable: %v", profile.Username, profileErr))
			state.profiles[profile.Username] = profile
			if err := store.saveDeepProfile(runID, profile); err != nil {
				return err
			}
			if abortErr := contextAbort(ctx, profileErr); abortErr != nil {
				return abortErr
			}
			continue
		}
		author := authorFromProfile(profileData, profile.Username)
		profile.Profile = author
		profile.ProfileFetched = true
		state.authors[profile.Username] = author
		if err := store.UpsertAuthor(runID, author, time.Now().UTC()); err != nil {
			return err
		}
		deep.Funnel.ProfilesFetched++
		deep.Funnel.SeedProfilesInspected++

		verification := verifyDeepOwnerProfile(author, deepSeedPostTexts(state, profile))
		profile.VerifiedOwnerContext = verification.Verified
		profile.VerificationConfidence = verification.Confidence
		profile.VerificationReasons = appendUniqueStrings(profile.VerificationReasons, verification.Reasons...)
		profile.VerificationNegativeEvidence = appendUniqueStrings(profile.VerificationNegativeEvidence, verification.NegativeEvidence...)
		profile.BusinessCategories = appendUniqueStrings(profile.BusinessCategories, verification.BusinessCategories...)
		if verification.Verified {
			profile.VerificationStatus = "accepted"
			profile.Selected = true
			profile.Status = "verified"
		} else {
			profile.VerificationStatus = "rejected"
			profile.Selected = false
			profile.Status = "rejected"
			if err := store.saveDeepProfile(runID, profile); err != nil {
				return err
			}
			if err := store.saveDeepCheckpoint(runID, deepStageProfiles, profile.Username, profile); err != nil {
				return err
			}
			continue
		}

		deep.Funnel.Requests++
		deep.Funnel.ProfilePostRequests++
		deep.ProfilePagination.Requests++
		seen := map[string]bool{}
		streamErr := error(nil)
		for postValue, postErr := range collector.ProfilePosts(ctx, profile.Username, cfg.ProfilePostLimit) {
			if postErr != nil {
				streamErr = postErr
				break
			}
			if deep.Funnel.ProfilePostsRaw >= cfg.MaxTotalProfilePosts {
				deep.ProfilePagination.StoppedAtCeiling = true
				deep.ProfilePagination.StopReason = "global_profile_post_ceiling"
				break
			}
			deep.Funnel.ProfilePostsRaw++
			deep.ProfilePagination.RawYield++
			post := postFromThreadPost(postValue, profile.Username, time.Now().UTC())
			if post.ID == "" {
				continue
			}
			assessment := AssessBusinessPain(cfg.Topic, post.Text)
			if seen[post.ID] {
				deep.ProfilePagination.Duplicates++
				continue
			}
			seen[post.ID] = true
			deep.ProfilePagination.UniqueYield++
			if !post.PublishedAt.IsZero() {
				addDate(&deep.Funnel.DateCoverage, post.PublishedAt)
			}
			if !state.profilePosts[post.ID] {
				deep.Funnel.ProfilePostsUnique++
			}
			state.profilePosts[post.ID] = true
			record := state.upsertPost(post, assessment, "PROFILE_POST")
			seedPostID := firstValue(profile.SeedPostIDs)
			anchorQuery := firstValue(profile.AnchorQueries)
			edge := DeepProvenance{
				PostID: post.ID, SourceStage: "PROFILE_POST", AnchorQuery: anchorQuery,
				SeedPostID: seedPostID, ProfileUsername: profile.Username, Depth: 0,
				ObservedAt: time.Now().UTC(),
			}
			if containsString(profile.SourceStages, "REPLY_SNOWBALL") {
				edge.SourceStage = "REPLY_SNOWBALL"
				edge.Depth = 1
			}
			record.Provenance = appendDeepProvenance(record.Provenance, edge)
			record.Stages = appendUnique(record.Stages, edge.SourceStage)
			state.provenance[deepProvenanceKey(edge)] = edge
			state.posts[post.ID] = record
			if post.AuthorUsername != "" {
				state.authors[normalizeAuthor(post.AuthorUsername)] = authorFor(state.authors, post.AuthorUsername)
			}
			if err := store.UpsertPost(post); err != nil {
				return err
			}
			if err := store.saveDeepPost(runID, record); err != nil {
				return err
			}
			if err := store.saveDeepProvenance(runID, edge); err != nil {
				return err
			}
			if err := store.RecordPostSnapshot(runID, post, time.Now().UTC()); err != nil {
				return err
			}
		}
		deep.ProfilePagination.PagesFetched++
		deep.ProfilePagination.PostsPerPage = append(deep.ProfilePagination.PostsPerPage, len(seen))
		if streamErr != nil {
			profile.Status = "completed_with_warning"
			profile.Error = streamErr.Error()
			deep.Funnel.Failures++
			report.Warnings = appendWarningOnce(report.Warnings, fmt.Sprintf("profile posts for @%s unavailable after %d post(s): %v", profile.Username, len(seen), streamErr))
			if abortErr := contextAbort(ctx, streamErr); abortErr != nil {
				return abortErr
			}
		} else {
			profile.Status = "completed"
			if len(seen) >= cfg.ProfilePostLimit {
				deep.ProfilePagination.StoppedAtCeiling = true
				deep.ProfilePagination.StopReason = "per_profile_post_ceiling"
			} else {
				deep.ProfilePagination.Exhausted = true
				deep.ProfilePagination.StopReason = "profile_cursor_exhausted_or_ssr_window_complete"
			}
		}
		profile.ProfilePosts = maxInt(profile.ProfilePosts, len(seen))
		state.profiles[profile.Username] = profile
		if err := store.saveDeepProfile(runID, profile); err != nil {
			return err
		}
		if err := store.saveDeepCheckpoint(runID, deepStageProfiles, profile.Username, profile); err != nil {
			return err
		}
	}
	return nil
}

func deepSeedPostTexts(state *deepState, profile DeepSeedProfile) []string {
	seen := map[string]bool{}
	texts := make([]string, 0, len(profile.SeedPostIDs))
	for _, postID := range profile.SeedPostIDs {
		record, ok := state.posts[postID]
		if !ok || strings.TrimSpace(record.Post.Text) == "" || seen[postID] {
			continue
		}
		seen[postID] = true
		texts = append(texts, record.Post.Text)
	}
	return texts
}

func collectDeepReplies(ctx context.Context, cfg Config, collector Collector, store *Store, state *deepState, runID int64, deep *DeepReport, report *Report) error {
	replyCollector, ok := collector.(deepReplyCollector)
	if !ok {
		deep.ReplyPagination.StopReason = "collector_does_not_expose_replies"
		deep.Limitations = appendUniqueStrings(deep.Limitations, "Reply collection was unavailable through the configured collector interface.")
		return nil
	}
	roots := selectDeepReplyRoots(state)
	for index, root := range roots {
		if index > 0 {
			if err := waitDeep(ctx, cfg.ReplyDelay); err != nil {
				return err
			}
		}
		deep.Funnel.RepliesAttempted++
		deep.Funnel.Requests++
		deep.Funnel.ReplyRequests++
		deep.ReplyPagination.Requests++
		deep.ReplyPagination.PagesFetched++
		seen := map[string]bool{}
		streamErr := error(nil)
		for reply, replyErr := range replyCollector.PostReplies(ctx, root.Post.URL, deepMaxRepliesPerRoot) {
			if replyErr != nil {
				streamErr = replyErr
				break
			}
			if len(seen) >= deepMaxRepliesPerRoot {
				deep.ReplyPagination.StoppedAtCeiling = true
				break
			}
			if reply.ID == "" || seen[reply.ID] {
				deep.ReplyPagination.Duplicates++
				continue
			}
			seen[reply.ID] = true
			signal := DeepReplySignal{
				RootPostID: root.Post.ID, ReplyID: reply.ID, ReplyAuthor: normalizeAuthor(reply.Username),
				ReplyText: reply.Text, ReplyURL: reply.Permalink, ReplySignalType: classifyReplySignal(reply.Text), ObservedAt: time.Now().UTC(),
			}
			state.replies[signal.ReplyID] = signal
			deep.Funnel.RepliesCollected++
			deep.ReplyPagination.RawYield++
			deep.ReplyPagination.UniqueYield++
			if err := store.saveDeepReply(runID, signal); err != nil {
				return err
			}
			observeReplySeed(state, signal)
		}
		deep.ReplyPagination.PostsPerPage = append(deep.ReplyPagination.PostsPerPage, len(seen))
		if streamErr != nil {
			deep.Funnel.Failures++
			report.Warnings = appendWarningOnce(report.Warnings, fmt.Sprintf("replies for %s unavailable after %d reply(s): %v", root.Post.URL, len(seen), streamErr))
			if looksRateLimited(streamErr) {
				deep.Funnel.RateLimitEvents++
			}
			if abortErr := contextAbort(ctx, streamErr); abortErr != nil {
				return abortErr
			}
		}
	}
	if len(roots) == 0 {
		deep.ReplyPagination.StopReason = "no_high_or_gold_roots"
	} else if deep.Funnel.RepliesCollected == 0 && deep.ReplyPagination.StopReason == "" {
		deep.ReplyPagination.StopReason = "ssr_reply_window_empty_or_unavailable"
	}
	return nil
}

// deepReplyCollector is optional so the original bounded Collector contract
// and all existing fakes remain source-compatible.
type deepReplyCollector interface {
	PostReplies(context.Context, string, int) iter.Seq2[threads.Reply, error]
}

func selectDeepReplyRoots(state *deepState) []DeepPostRecord {
	var roots []DeepPostRecord
	for _, record := range state.posts {
		assessment := record.Assessment
		if !ownerLikely(assessment.OwnerLikelihood) {
			continue
		}
		if assessment.GoldPainSignal || (assessment.PainStrength == PainStrengthHigh && assessment.ITActionability == ITActionabilityHigh) {
			roots = append(roots, record)
		}
	}
	sort.SliceStable(roots, func(i, j int) bool {
		if roots[i].Assessment.GoldPainSignal != roots[j].Assessment.GoldPainSignal {
			return roots[i].Assessment.GoldPainSignal
		}
		if roots[i].Assessment.PainStrength != roots[j].Assessment.PainStrength {
			return roots[i].Assessment.PainStrength > roots[j].Assessment.PainStrength
		}
		return roots[i].Post.ID < roots[j].Post.ID
	})
	if len(roots) > deepMaxReplyRoots {
		roots = roots[:deepMaxReplyRoots]
	}
	return roots
}

func observeReplySeed(state *deepState, signal DeepReplySignal) {
	if signal.ReplyAuthor == "" || !isRussianText(signal.ReplyText) {
		return
	}
	assessment := AssessBusinessPain("проблемы малого бизнеса автоматизация", signal.ReplyText)
	candidate := state.candidates[signal.ReplyAuthor]
	if candidate == nil {
		candidate = newDeepCandidate(signal.ReplyAuthor)
		state.candidates[signal.ReplyAuthor] = candidate
	}
	candidate.SourceStages["REPLY_SNOWBALL"] = true
	candidate.Score += 1
	if assessment.OwnerLikelihood == OwnerLikelihoodHigh || assessment.OwnerLikelihood == OwnerLikelihoodMedium {
		candidate.Score += 3
	}
	if signal.ReplySignalType == "OWNER_PAIN" || signal.ReplySignalType == "SOLUTION_SEEKING" {
		candidate.Score += 2
	}
	candidate.Likelihood = strongerOwnerLikelihood(candidate.Likelihood, assessment.OwnerLikelihood)
	candidate.PositiveEvidence["public_reply_interaction"] = true
	if signal.ReplySignalType != "OWNER_PAIN" && signal.ReplySignalType != "SOLUTION_SEEKING" {
		candidate.NegativeEvidence["reply_not_owner_pain"] = true
	}
}

func selectReplySnowballProfiles(state *deepState, limit int) {
	for username, candidate := range state.candidates {
		if !candidate.SourceStages["REPLY_SNOWBALL"] || (candidate.Likelihood != OwnerLikelihoodHigh && candidate.Likelihood != OwnerLikelihoodMedium) {
			continue
		}
		profile := deepProfileFromCandidate(candidate)
		profile.SourceStages = appendUnique(profile.SourceStages, "REPLY_SNOWBALL")
		profile.Status = "candidate"
		if existing, ok := state.profiles[username]; ok {
			profile = mergeDeepSeedProfile(existing, profile)
		}
		state.profiles[username] = profile
	}
	selectDeepSeedProfiles(state, limit)
}

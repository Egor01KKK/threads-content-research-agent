package research

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

func ownerLikely(value string) bool {
	return value == OwnerLikelihoodHigh || value == OwnerLikelihoodMedium
}

func isPainSignal(assessment BusinessPainAssessment) bool {
	return assessment.PainStrength == PainStrengthHigh || assessment.PainStrength == PainStrengthMedium ||
		assessment.SolutionSeeking == Yes || assessment.WorkaroundPresent == Yes || len(assessment.BusinessConsequences) > 0
}

func isITActionable(assessment BusinessPainAssessment) bool {
	return assessment.ITActionability == ITActionabilityHigh || assessment.ITActionability == ITActionabilityMedium
}

func likelihoodRank(value string) int {
	switch value {
	case OwnerLikelihoodHigh:
		return 4
	case OwnerLikelihoodMedium:
		return 3
	case OwnerLikelihoodLow:
		return 2
	case OwnerLikelihoodUnknown:
		return 1
	default:
		return 0
	}
}

func strongerOwnerLikelihood(current, candidate string) string {
	if likelihoodRank(candidate) > likelihoodRank(current) {
		return candidate
	}
	if current == "" {
		return candidate
	}
	return current
}

func ownerSeedEvidence(text string, assessment BusinessPainAssessment) (float64, string, []string, []string) {
	normalized := normalizeRussian(text)
	tokens := relevanceTokens(normalized)
	likelihood := assessment.OwnerLikelihood
	score := float64(likelihoodRank(likelihood) * 2)
	positive := []string{}
	negative := []string{}
	if containsRussianPhrase(normalized,
		"у меня свой бизнес", "у меня свое дело", "у меня своё дело", "мой бизнес",
		"мой магазин", "у нас бизнес", "у нас салон", "у нас студия", "у нас агентство",
		"я предприниматель", "я ип", "работаю на себя", "самозанятый", "самозанятая",
	) {
		score += 5
		positive = append(positive, "explicit_ownership_statement")
	}
	if hasRussianFirstPerson(tokens) {
		score += 2
		positive = append(positive, "first_person_context")
	}
	if containsRussianPhrase(normalized,
		"владелец", "владелица", "основатель", "сооснователь", "директор", "управляющий", "управляющая",
		"руководитель", "предприниматель", "самозанятый", "самозанятая", "фрилансер",
	) {
		score += 1
		positive = append(positive, "owner_role_anchor")
	}
	if hasRussianBusinessAnchor(normalized, tokens) {
		score += 2
		positive = append(positive, "business_identity_anchor")
	}
	if hasRussianOperationalContext(normalized, tokens) {
		score += 2
		positive = append(positive, "operational_context")
	}
	if isPainSignal(assessment) {
		score += 1
		positive = append(positive, "concrete_pain_or_solution_signal")
	}
	if assessment.ITActionability == ITActionabilityHigh {
		score += 1
		positive = append(positive, "high_it_actionability")
	}
	if containsRussianPhrase(normalized,
		"ищу исполнителя", "нужен специалист", "нужна услуга", "ищу подрядчика",
		"для клиентов", "клиентам нужен", "посоветуйте специалиста", "кто может сделать",
	) {
		score -= 4
		negative = append(negative, "third_party_or_buyer_request")
	}
	if containsRussianPhrase(normalized, "вопрос от клиента", "клиент спрашивает", "для моего клиента") {
		score -= 3
		negative = append(negative, "speaking_for_client")
	}
	if len(positive) == 0 {
		negative = append(negative, "no_explicit_owner_evidence")
	}
	return score, likelihood, uniqueStrings(positive), uniqueStrings(negative)
}

// ownerSeedCandidateEligible is intentionally broader than the old
// HIGH/MEDIUM owner-likelihood filter. The extra candidates only buy a cheap
// profile-metadata check; their recent feeds are not fetched until the
// collection-time verification gate accepts them.
func ownerSeedCandidateEligible(candidate *deepSeedCandidate) bool {
	if candidate == nil {
		return false
	}
	if candidate.Likelihood == OwnerLikelihoodHigh || candidate.Likelihood == OwnerLikelihoodMedium {
		return true
	}
	if candidate.PositiveEvidence["explicit_ownership_statement"] {
		return true
	}
	if candidate.PositiveEvidence["owner_role_anchor"] {
		return true
	}
	if candidate.PositiveEvidence["business_identity_anchor"] || candidate.PositiveEvidence["operational_context"] {
		return true
	}
	// A candidate that has a concrete anchor hit is still worth the cheap
	// metadata check. This keeps resume runs from losing anchor evidence when
	// the candidate is reconstructed from persisted posts; the expensive
	// profile-feed crawl remains behind verifyDeepOwnerProfile.
	if len(candidate.AnchorQueries) > 0 || len(candidate.SeedPostIDs) > 0 {
		return true
	}
	return candidate.PositiveEvidence["first_person_context"] && candidate.PositiveEvidence["concrete_pain_or_solution_signal"]
}

func sortedBoolKeys(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value, ok := range values {
		if ok && strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func mergeAuthor(existing, incoming Author) Author {
	out := existing
	if out.Username == "" {
		out.Username = incoming.Username
	}
	if out.Name == "" {
		out.Name = incoming.Name
	}
	if out.Bio == "" {
		out.Bio = incoming.Bio
	}
	if out.ProfileURL == "" {
		out.ProfileURL = incoming.ProfileURL
	}
	if out.FollowerCount == nil {
		out.FollowerCount = incoming.FollowerCount
	}
	if out.FollowingCount == nil {
		out.FollowingCount = incoming.FollowingCount
	}
	if out.Verified == nil {
		out.Verified = incoming.Verified
	}
	return out
}

func appendUniqueStrings(values []string, additions ...string) []string {
	seen := map[string]bool{}
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			seen[value] = true
		}
	}
	for _, value := range additions {
		if strings.TrimSpace(value) != "" && !seen[value] {
			values = append(values, value)
			seen[value] = true
		}
	}
	sort.Strings(values)
	return values
}

func appendDeepProvenance(values []DeepProvenance, edge DeepProvenance) []DeepProvenance {
	key := deepProvenanceKey(edge)
	for _, existing := range values {
		if deepProvenanceKey(existing) == key {
			return values
		}
	}
	return append(values, edge)
}

func anchorStage(wave string) string {
	if wave == deepWaveSecondary {
		return "SECONDARY_ANCHOR"
	}
	return "ANCHOR_QUERY"
}

func waitDeep(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func looksRateLimited(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	for _, term := range []string{"429", "rate limit", "rate-limited", "too many", "captcha", "blocked", "forbidden", "access denied"} {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}

func mergePagination(total, current DeepPaginationMetrics) DeepPaginationMetrics {
	first := total.Requests == 0 && total.PagesFetched == 0
	total.Requests += current.Requests
	total.PagesFetched += current.PagesFetched
	total.RawYield += current.RawYield
	total.UniqueYield += current.UniqueYield
	total.Duplicates += current.Duplicates
	total.GraphQLAttempts += current.GraphQLAttempts
	total.GraphQLErrors += current.GraphQLErrors
	total.PostsPerPage = append(total.PostsPerPage, current.PostsPerPage...)
	if first {
		total.Exhausted = current.Exhausted
	} else {
		// For an aggregate of independent bounded windows, exhaustion is true
		// only when every window was exhausted. A single per-window ceiling is
		// retained separately in StoppedAtCeiling.
		total.Exhausted = total.Exhausted && current.Exhausted
	}
	total.StoppedAtCeiling = total.StoppedAtCeiling || current.StoppedAtCeiling
	total.RateLimited = total.RateLimited || current.RateLimited
	total.AccessDegraded = total.AccessDegraded || current.AccessDegraded
	if current.StopReason != "" {
		if total.StopReason == "" {
			total.StopReason = current.StopReason
		} else if total.StopReason != current.StopReason {
			total.StopReason = "mixed_bounded_windows"
		}
	}
	return total
}

// normalizeDeepProbePagination rebuilds the per-anchor SSR envelope from the
// persisted probe counters. Older interrupted/materialized runs could carry a
// duplicated UniqueYield value, so the probe's own raw and distinct counts are
// the authoritative values for this one bounded request.
func normalizeDeepProbePagination(probe DeepAnchorProbe) DeepPaginationMetrics {
	pagination := probe.Pagination
	pagination.Requests = 1
	pagination.PagesFetched = 1
	pagination.RawYield = probe.QueryReport.ResultCount
	pagination.UniqueYield = probe.UniquePosts
	pagination.Duplicates = probe.Duplicates
	pagination.PostsPerPage = []int{probe.UniquePosts}

	if probe.Error != "" || probe.Status == "completed_with_warning" || probe.Status == "failed" {
		pagination.Exhausted = false
		pagination.StoppedAtCeiling = false
		pagination.AccessDegraded = true
		pagination.StopReason = "access_or_stream_error"
	} else if probe.ProbeLimit > 0 && probe.QueryReport.ResultCount >= probe.ProbeLimit {
		pagination.Exhausted = false
		pagination.StoppedAtCeiling = true
		pagination.StopReason = "per_query_probe_ceiling"
	} else {
		pagination.Exhausted = true
		pagination.StoppedAtCeiling = false
		pagination.StopReason = "ssr_window_exhausted_or_empty"
	}
	return pagination
}

// rebuildDeepProfilePagination reconstructs the observable profile-post
// envelope when a run is resumed. Profile requests are idempotently skipped on
// resume, so their in-memory pagination counters are not available; the
// persisted per-profile yields still give an honest one-window summary.
func rebuildDeepProfilePagination(previous DeepPaginationMetrics, state *deepState, cfg Config) DeepPaginationMetrics {
	pagination := DeepPaginationMetrics{
		RateLimited:     previous.RateLimited,
		AccessDegraded:  previous.AccessDegraded,
		GraphQLAttempts: previous.GraphQLAttempts,
		GraphQLErrors:   previous.GraphQLErrors,
	}
	for _, profile := range state.profiles {
		if !profile.Selected || !profile.ProfileFetched {
			continue
		}
		pagination.Requests++
		pagination.PagesFetched++
		pagination.RawYield += profile.ProfilePosts
		pagination.PostsPerPage = append(pagination.PostsPerPage, profile.ProfilePosts)
		if cfg.ProfilePostLimit > 0 && profile.ProfilePosts >= cfg.ProfilePostLimit {
			pagination.StoppedAtCeiling = true
		}
	}
	pagination.UniqueYield = len(state.profilePosts)
	if pagination.RawYield > pagination.UniqueYield {
		pagination.Duplicates = pagination.RawYield - pagination.UniqueYield
	}
	if pagination.StoppedAtCeiling {
		pagination.StopReason = "per_profile_post_ceiling"
	} else if pagination.PagesFetched > 0 {
		pagination.Exhausted = true
		pagination.StopReason = "profile_cursor_exhausted_or_ssr_window_complete"
	} else {
		pagination.StopReason = "no_profile_post_windows_fetched"
	}
	return pagination
}

func markDeepSelectedAnchorProbes(state *deepState) {
	selectedQueries := map[string]bool{}
	for _, profile := range state.profiles {
		if !profile.Selected {
			continue
		}
		for _, query := range profile.AnchorQueries {
			selectedQueries[strings.ToLower(normalizeSpace(query))] = true
		}
	}
	for key, probe := range state.probes {
		selected := selectedQueries[strings.ToLower(normalizeSpace(probe.Anchor.Query))]
		probe.SelectedForProfileSeeds = selected && probe.Wave != deepWaveSecondary
		probe.QueryReport.SelectedForDeep = selected
		state.probes[key] = probe
	}
}

func persistDeepAnchorProbes(store *Store, runID int64, state *deepState) error {
	for _, probe := range state.probes {
		if err := store.saveDeepAnchorProbe(runID, probe); err != nil {
			return err
		}
	}
	return nil
}

func hydrateDeepFunnelFromState(funnel *DeepFunnel, state *deepState) {
	funnel.OwnerSeedProfiles = countSeedSelectedProfiles(state.profiles)
	for _, profile := range state.profiles {
		if profile.SeedSelected && (profile.Status == "fetching" || profile.Status == "seed_selected" || profile.Status == "verified" || profile.Status == "selected" || profile.Status == "completed" || profile.Status == "completed_with_warning" || profile.Status == "failed" || profile.Status == "rejected") {
			funnel.ProfilesAttempted++
		}
		if profile.VerifiedOwnerContext {
			funnel.VerifiedOwnerProfiles++
		}
		if profile.VerificationStatus == "rejected" {
			funnel.ProfilesRejected++
		}
		if profile.ProfileFetched {
			funnel.ProfilesFetched++
		}
		if profile.Selected && profile.ProfileFetched {
			funnel.ProfilePostsRaw += profile.ProfilePosts
		}
	}
	funnel.ProfilePostsUnique = len(state.profilePosts)
	funnel.ProfileRequests = funnel.ProfilesAttempted
	funnel.ProfilePostRequests = funnel.ProfilesFetched
	funnel.TotalUniquePosts = len(state.posts)
}

func deepDiscoveryValue(probe DeepAnchorProbe) float64 {
	if probe.UniquePosts == 0 {
		return 0
	}
	postBreadth := float64(probe.UniquePosts)
	if postBreadth > 3 {
		postBreadth = 3
	}
	authorBreadth := float64(probe.UniqueAuthors)
	if authorBreadth > 3 {
		authorBreadth = 3
	}
	contribution := float64(probe.UniqueAuthorContribution) / float64(probe.UniquePosts)
	if contribution > 1 {
		contribution = 1
	}
	return (postBreadth/3)*0.45 + (authorBreadth/3)*0.35 + contribution*0.20
}

func addDate(coverage *DeepDateCoverage, value time.Time) {
	if value.IsZero() {
		return
	}
	value = value.UTC()
	if coverage.Earliest == nil || value.Before(*coverage.Earliest) {
		copyValue := value
		coverage.Earliest = &copyValue
	}
	if coverage.Latest == nil || value.After(*coverage.Latest) {
		copyValue := value
		coverage.Latest = &copyValue
	}
	if coverage.ByWeek == nil {
		coverage.ByWeek = map[string]int{}
	}
	if coverage.ByMonth == nil {
		coverage.ByMonth = map[string]int{}
	}
	year, week := value.ISOWeek()
	coverage.ByWeek[fmt.Sprintf("%04d-W%02d", year, week)]++
	coverage.ByMonth[value.Format("2006-01")]++
}

func deepRelevanceAssessment(assessment BusinessPainAssessment) RelevanceAssessment {
	reasons := append([]string(nil), assessment.Reasons...)
	if assessment.RussianLanguage != Yes {
		return RelevanceAssessment{Score: relevanceScoreIrrelevant, Label: RelevanceIrrelevant, Reasons: append(reasons, "not_russian")}
	}
	if ownerLikely(assessment.OwnerLikelihood) && isPainSignal(assessment) {
		return RelevanceAssessment{Score: relevanceScoreRelevant, Label: RelevanceRelevant, Reasons: append(reasons, "owner_pain_evidence")}
	}
	if isPainSignal(assessment) || isITActionable(assessment) {
		return RelevanceAssessment{Score: relevanceScoreRelevant, Label: RelevanceRelevant, Reasons: append(reasons, "pain_or_it_evidence")}
	}
	if assessment.OwnerLikelihood != OwnerLikelihoodUnknown || len(assessment.BusinessTypes) > 0 {
		return RelevanceAssessment{Score: relevanceScoreAdjacent, Label: RelevanceAdjacent, Reasons: append(reasons, "owner_or_business_context")}
	}
	return RelevanceAssessment{Score: relevanceScoreUncertain, Label: RelevanceUncertain, Reasons: append(reasons, "russian_without_pain_signal")}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (s *deepState) hasSearchAuthor(username string) bool {
	username = normalizeAuthor(username)
	if username == "" {
		return false
	}
	for id := range s.searchPosts {
		if normalizeAuthor(s.posts[id].Post.AuthorUsername) == username {
			return true
		}
	}
	return false
}

func firstValue(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func prepareSecondaryAnchors(state *deepState, cfg Config, limit int) {
	byPhrase := map[string]*DeepSecondaryAnchor{}
	for phrase, existing := range state.secondary {
		copyValue := existing
		byPhrase[phrase] = &copyValue
	}
	for _, record := range state.posts {
		assessment := record.Assessment
		if !ownerLikely(assessment.OwnerLikelihood) || !isRussianText(record.Post.Text) {
			continue
		}
		text := normalizeRussian(record.Post.Text)
		for _, phrase := range deepObservedAnchorLexicon {
			phrase = normalizeSpace(phrase)
			if deepAnchorTokenCount(phrase) > 3 || !containsRussianPhrase(text, phrase) {
				continue
			}
			item := byPhrase[phrase]
			if item == nil {
				item = &DeepSecondaryAnchor{Phrase: phrase}
				byPhrase[phrase] = item
			}
			item.SourcePostIDs = appendUniqueStrings(item.SourcePostIDs, record.Post.ID)
			item.SourceAuthors = appendUniqueStrings(item.SourceAuthors, record.Post.AuthorUsername)
			for _, edge := range record.Provenance {
				item.SourceProfiles = appendUniqueStrings(item.SourceProfiles, edge.ProfileUsername)
				item.OriginalAnchors = appendUniqueStrings(item.OriginalAnchors, edge.AnchorQuery)
			}
			if assessment.PainStrength == PainStrengthHigh {
				item.HighPainEvidence++
			}
			if isITActionable(assessment) {
				item.ITActionableEvidence++
			}
		}
	}
	items := make([]DeepSecondaryAnchor, 0, len(byPhrase))
	for _, item := range byPhrase {
		item.Eligible = len(item.SourceAuthors) >= 2 || item.HighPainEvidence > 0 || item.ITActionableEvidence > 0
		if len(item.SourceAuthors) >= 2 {
			item.Reason = "observed_across_multiple_authors"
		} else if item.HighPainEvidence > 0 {
			item.Reason = "high_strength_pain"
		} else if item.ITActionableEvidence > 0 {
			item.Reason = "high_or_medium_it_actionability"
		}
		items = append(items, *item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Eligible != items[j].Eligible {
			return items[i].Eligible
		}
		if items[i].HighPainEvidence != items[j].HighPainEvidence {
			return items[i].HighPainEvidence > items[j].HighPainEvidence
		}
		if items[i].ITActionableEvidence != items[j].ITActionableEvidence {
			return items[i].ITActionableEvidence > items[j].ITActionableEvidence
		}
		if len(items[i].SourceAuthors) != len(items[j].SourceAuthors) {
			return len(items[i].SourceAuthors) > len(items[j].SourceAuthors)
		}
		return items[i].Phrase < items[j].Phrase
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	state.secondary = map[string]DeepSecondaryAnchor{}
	for _, item := range items {
		state.secondary[item.Phrase] = item
	}
}

func sortedSecondaryPhrases(values map[string]DeepSecondaryAnchor) []string {
	items := make([]DeepSecondaryAnchor, 0, len(values))
	for _, value := range values {
		if value.Eligible {
			items = append(items, value)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].HighPainEvidence != items[j].HighPainEvidence {
			return items[i].HighPainEvidence > items[j].HighPainEvidence
		}
		if items[i].ITActionableEvidence != items[j].ITActionableEvidence {
			return items[i].ITActionableEvidence > items[j].ITActionableEvidence
		}
		return items[i].Phrase < items[j].Phrase
	})
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Phrase)
	}
	return out
}

func classifyReplySignal(text string) string {
	normalized := normalizeRussian(text)
	if containsRussianPhrase(normalized, "кто может", "посоветуйте", "как решить", "что делать", "как настроить", "какую crm") {
		return "SOLUTION_SEEKING"
	}
	if strings.Contains(normalized, "?") {
		return "QUESTION"
	}
	if containsRussianPhrase(normalized, "могу помочь", "пишите", "обращайтесь", "настрою", "внедрю", "делаю сайты") {
		return "SERVICE_OFFER"
	}
	if containsRussianPhrase(normalized, "у меня", "у нас", "теряются", "не успеваю", "бесит", "вручную", "не работает") {
		return "OWNER_PAIN"
	}
	if len(russianTools(normalized)) > 0 {
		return "TOOL_MENTION"
	}
	if containsRussianPhrase(normalized, "рекомендую", "советую", "попробуйте", "знакомый") {
		return "REFERRAL"
	}
	return "OTHER"
}

func applyBusinessAssessment(post *RankedPost, assessment BusinessPainAssessment) {
	post.RussianLanguage = assessment.RussianLanguage
	post.OwnerLikelihood = assessment.OwnerLikelihood
	post.PainTypes = append([]string(nil), assessment.PainTypes...)
	post.PainStrength = assessment.PainStrength
	post.SolutionSeeking = assessment.SolutionSeeking
	post.WorkaroundPresent = assessment.WorkaroundPresent
	post.ToolsMentioned = append([]string(nil), assessment.ToolsMentioned...)
	post.ITActionability = assessment.ITActionability
	post.BuyerIntent = assessment.BuyerIntent
	post.BusinessType = assessment.BusinessType
	post.BusinessTypes = append([]string(nil), assessment.BusinessTypes...)
	post.BusinessProcess = assessment.BusinessProcess
	post.BusinessProcesses = append([]string(nil), assessment.BusinessProcesses...)
	post.BusinessConsequences = append([]string(nil), assessment.BusinessConsequences...)
	post.CurrentWorkaround = assessment.CurrentWorkaround
	post.GoldPainSignal = assessment.GoldPainSignal
	post.BusinessPainReasons = append([]string(nil), assessment.Reasons...)
}

func deepPostRecordLess(left, right DeepPostRecord) bool {
	if !left.Post.PublishedAt.Equal(right.Post.PublishedAt) {
		return left.Post.PublishedAt.After(right.Post.PublishedAt)
	}
	return left.Post.ID < right.Post.ID
}

func deepLimitations(report Report, state *deepState) []string {
	limitations := []string{
		"Deep mode uses the fixed short Russian anchor bank and never searches the long objective; discovery and pain precision are run-specific measurements, not universal Threads trends.",
		"The collector uses public anonymous SSR windows and bounded profile requests. The current persisted-query GraphQL continuation is optional and can fail without discarding SSR evidence; date coverage is therefore not controllable history.",
		"Owner likelihood, pain, business type/process, workaround, IT actionability, and buyer intent are deterministic heuristics. They are audit signals, not verified user attributes or purchase commitments.",
		"Root posts and replies are separate evidence units. Multiple replies under one root are not counted as independent owners.",
		"Performance is calculated only where the collected follower fields and an independent baseline support it; missing metrics remain missing.",
		"Corpus ceilings are safety bounds, not quotas. Actual exhaustion, empty anchors, failures, and date span are reported in the funnel and pagination sections.",
	}
	if len(state.posts) == 0 {
		limitations = append(limitations, "No public post evidence was collected in this run; profile and pain conclusions are unavailable.")
	}
	if len(state.profiles) == 0 {
		limitations = append(limitations, "No owner-likely profile seeds survived the deterministic seed gate.")
	}
	return limitations
}

func maxIntValue(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func finalizeDeepResearch(ctx context.Context, cfg Config, store *Store, state *deepState, report *Report, deep *DeepReport, startedAt time.Time) error {
	if err := ctx.Err(); err != nil {
		// A cancelled run still gets a complete partial export from the records
		// already checkpointed in SQLite. Do not turn cancellation into a silent
		// success, but allow the caller's named error to preserve it.
		_ = err
	}
	// Collection stages checkpoint profiles as they are verified. Reload the
	// persisted rows before building the human-readable export so a resumed run
	// cannot expose an older in-memory status for a profile that was already
	// accepted or rejected in SQLite.
	persistedProfiles, err := store.loadDeepProfiles(report.RunID)
	if err != nil {
		return err
	}
	state.profiles = persistedProfiles
	allPosts := make(map[string]Post, len(state.posts))
	for id, record := range state.posts {
		allPosts[id] = record.Post
		if username := normalizeAuthor(record.Post.AuthorUsername); username != "" {
			state.authors[username] = mergeAuthor(authorFor(state.authors, username), state.authors[username])
		}
	}
	for username := range state.authors {
		if err := store.EnsureAuthor(authorFor(state.authors, username), time.Now().UTC()); err != nil {
			return err
		}
	}

	// Profile posts that were not discovered through search and do not carry a
	// pain signal are the closest available independent recent baseline. They
	// remain in the database as baseline evidence and never become pain claims.
	state.baselinePosts = map[string][]Post{}
	baselineIDs := map[string]bool{}
	for id, record := range state.posts {
		if !state.profilePosts[id] || state.searchPosts[id] || isPainSignal(record.Assessment) {
			continue
		}
		username := normalizeAuthor(record.Post.AuthorUsername)
		if username == "" {
			continue
		}
		state.baselinePosts[username] = append(state.baselinePosts[username], record.Post)
		baselineIDs[id] = true
	}
	for username, posts := range state.baselinePosts {
		sort.SliceStable(posts, func(i, j int) bool {
			if !posts[i].PublishedAt.Equal(posts[j].PublishedAt) {
				return posts[i].PublishedAt.After(posts[j].PublishedAt)
			}
			return posts[i].ID < posts[j].ID
		})
		for _, post := range posts {
			if err := store.LinkBaselinePost(report.RunID, username, post.ID, time.Now().UTC()); err != nil {
				return err
			}
			if err := store.RecordPostSnapshot(report.RunID, post, time.Now().UTC()); err != nil {
				return err
			}
		}
	}

	rankedAll, rankedAuthors := Rank(sortedPosts(allPosts), state.authors, state.queriesByPost, BaselineData{
		PostsByAuthor: state.baselinePosts,
		MinPosts:      cfg.MinBaselinePosts,
	}, cfg)
	rankedByID := make(map[string]RankedPost, len(rankedAll))
	for _, ranked := range rankedAll {
		if record, ok := state.posts[ranked.Post.ID]; ok {
			applyBusinessAssessment(&ranked, record.Assessment)
		}
		rankedByID[ranked.Post.ID] = ranked
	}

	// Preserve Rank's deterministic order while restricting topic conclusions
	// to owner-likely pain/solution evidence from the profile-first corpus.
	rankedEvidence := make([]RankedPost, 0, len(rankedAll))
	for _, original := range rankedAll {
		ranked := rankedByID[original.Post.ID]
		if isDeepEvidence(state.posts[original.Post.ID].Assessment) {
			rankedEvidence = append(rankedEvidence, ranked)
		}
	}
	if len(rankedEvidence) == 0 {
		// Keep a small deterministic diagnostic ranking even when the owner gate
		// found no pain, while leaving the evidence sections empty.
		for _, original := range rankedAll {
			rankedEvidence = append(rankedEvidence, rankedByID[original.Post.ID])
			if len(rankedEvidence) >= cfg.TopPosts {
				break
			}
		}
	}

	for id, record := range state.posts {
		if ranked, ok := rankedByID[id]; ok {
			performance := performanceContext(ranked)
			record.Performance = &performance
			record.Post = ranked.Post
			state.posts[id] = record
			if err := store.saveDeepPost(report.RunID, record); err != nil {
				return err
			}
		}
	}

	report.TopPosts = takeRankedPosts(rankedEvidence, cfg.TopPosts)
	report.TopOutperformingPosts = topOutperforming(rankedEvidence, cfg.TopPosts)
	report.TopAuthors = takeRankedAuthors(RankAuthors(rankedEvidence, state.authors, cfg), cfg.TopAuthors)
	report.RawTopPosts = takeRankedPosts(rankedAll, cfg.TopPosts)
	report.RawTopOutperformingPosts = topOutperforming(rankedAll, cfg.TopPosts)
	report.RawTopAuthors = takeRankedAuthors(rankedAuthors, cfg.TopAuthors)
	report.SignalCounts, report.LeadBuyerSignals, report.ContentAudienceSignals = buildSignalSections(rankedEvidence)
	report.BusinessPainCounts, report.GoldPainSignals, report.BusinessPainPosts = buildBusinessPainSections(rankedEvidence)
	report.PostsAnalyzed = len(allPosts)
	report.AuthorsDiscovered = len(state.authors)
	report.ProfilesDiscovered = len(state.candidates)
	report.Coverage = datasetCoverage(allPosts, state.authors)
	report.ProfilesWithKnownFollowers = report.Coverage.Followers.Known
	report.FollowerRelativeRanking = followerRelativeRankingStatus(report.Coverage.Followers)
	baselineAttempts := map[string]bool{}
	for username := range state.baselinePosts {
		baselineAttempts[username] = true
	}
	baselineCollected := 0
	baselineKnown := 0
	for _, posts := range state.baselinePosts {
		baselineCollected += len(posts)
		for _, post := range posts {
			if engagementMetricCount(post) > 0 {
				baselineKnown++
			}
		}
	}
	report.Baseline = baselineCoverage(baselineAttempts, state.baselinePosts, baselineCollected, baselineKnown, cfg.MinBaselinePosts, cfg.Engagement)

	deep.Funnel.AnchorsGenerated = len(deep.AnchorBank)
	deep.Funnel.AnchorsProbed = 0
	deep.Funnel.AnchorsWithResults = 0
	deep.Funnel.RawSearchPosts = 0
	deep.Funnel.UniqueSearchPosts = len(state.searchPosts)
	searchAuthors := map[string]bool{}
	for key, probe := range state.probes {
		if strings.HasPrefix(key, deepWavePrimary+"\x00") || strings.HasPrefix(key, deepWaveSecondary+"\x00") {
			deep.Funnel.AnchorsProbed++
		}
		if probe.QueryReport.ResultCount > 0 {
			deep.Funnel.AnchorsWithResults++
		}
		deep.Funnel.RawSearchPosts += probe.QueryReport.ResultCount
	}
	for id := range state.searchPosts {
		if username := normalizeAuthor(state.posts[id].Post.AuthorUsername); username != "" {
			searchAuthors[username] = true
		}
	}
	deep.Funnel.UniqueSearchAuthors = len(searchAuthors)
	deep.Funnel.ProfilePostsUnique = len(state.profilePosts)
	deep.Funnel.RepliesCollected = len(state.replies)
	profileRaw := 0
	for _, profile := range state.profiles {
		if profile.ProfileFetched {
			profileRaw += profile.ProfilePosts
		}
	}
	if deep.Funnel.ProfilePostsRaw < profileRaw {
		deep.Funnel.ProfilePostsRaw = profileRaw
	}
	deep.Funnel.TotalUniquePosts = len(state.posts)
	deep.Funnel.OwnerSeedCandidates = len(state.candidates)
	deep.Funnel.OwnerSeedProfiles = countSeedSelectedProfiles(state.profiles)
	deep.Funnel.VerifiedOwnerProfiles = countVerifiedOwnerProfiles(state.profiles)
	deep.Funnel.ProfilesRejected = countRejectedProfiles(state.profiles)
	deep.Funnel.SeedProfilesInspected = 0
	profileAttempted := 0
	profileFetched := 0
	profilePostFetched := 0
	for _, profile := range state.profiles {
		if !profile.SeedSelected && !profile.ProfileFetched {
			continue
		}
		if profile.SeedSelected && (profile.Status == "fetching" || profile.Status == "seed_selected" || profile.Status == "verified" || profile.Status == "selected" || profile.Status == "completed" || profile.Status == "completed_with_warning" || profile.Status == "failed" || profile.Status == "rejected") {
			profileAttempted++
		}
		if profile.ProfileFetched {
			profileFetched++
			deep.Funnel.SeedProfilesInspected++
			if profile.Selected {
				profilePostFetched++
			}
		}
	}
	if deep.Funnel.ProfilesAttempted < profileAttempted {
		deep.Funnel.ProfilesAttempted = profileAttempted
	}
	if deep.Funnel.ProfilesFetched < profileFetched {
		deep.Funnel.ProfilesFetched = profileFetched
	}
	if deep.Funnel.ProfileRequests < profileAttempted {
		deep.Funnel.ProfileRequests = profileAttempted
	}
	if deep.Funnel.ProfilePostRequests < profilePostFetched {
		deep.Funnel.ProfilePostRequests = profilePostFetched
	}
	report.ProfilesAttempted = deep.Funnel.ProfilesAttempted
	report.ProfilesFetched = deep.Funnel.ProfilesFetched
	deep.Funnel.RussianPosts = 0
	deep.Funnel.OwnerLikelyPosts = 0
	deep.Funnel.PainPosts = 0
	deep.Funnel.ITActionablePosts = 0
	deep.Funnel.SolutionSeekingPosts = 0
	deep.Funnel.BuyerIntentPosts = 0
	deep.Funnel.GoldSignals = 0
	deep.Funnel.DateCoverage = DeepDateCoverage{ByWeek: map[string]int{}, ByMonth: map[string]int{}}
	for _, record := range state.posts {
		assessment := record.Assessment
		if assessment.RussianLanguage == Yes {
			deep.Funnel.RussianPosts++
		}
		if ownerLikely(assessment.OwnerLikelihood) {
			deep.Funnel.OwnerLikelyPosts++
		}
		if isPainSignal(assessment) {
			deep.Funnel.PainPosts++
		}
		if isITActionable(assessment) {
			deep.Funnel.ITActionablePosts++
		}
		if assessment.SolutionSeeking == Yes {
			deep.Funnel.SolutionSeekingPosts++
		}
		if assessment.BuyerIntent != BuyerIntentNone {
			deep.Funnel.BuyerIntentPosts++
		}
		if assessment.GoldPainSignal {
			deep.Funnel.GoldSignals++
		}
		addDate(&deep.Funnel.DateCoverage, record.Post.PublishedAt)
	}
	deep.SearchPagination = DeepPaginationMetrics{}
	for key, probe := range state.probes {
		probe.Pagination = normalizeDeepProbePagination(probe)
		probe.QueryReport.Successful = probe.Status == "completed"
		probe.QueryReport.Error = probe.Error
		probe.QueryReport.ProbeResultCount = probe.QueryReport.ResultCount
		probe.QueryReport.ProbeRelevantPosts = probe.QueryReport.Relevant
		probe.QueryReport.ProbeOwnerLikelyPosts = probe.OwnerLikelyPosts
		probe.QueryReport.ProbePainPosts = probe.PainPosts
		probe.QueryReport.ProbeITActionablePosts = probe.ITActionablePosts
		probe.QueryReport.ProbeUniqueContribution = probe.UniqueAuthorContribution
		probe.QueryReport.ProbeDuplicates = probe.Duplicates
		probe.QueryReport.ProbePrecision = probe.PainPrecision
		probe.QueryReport.Precision = probe.PainPrecision
		probe.QueryReport.DeepResultCount = probe.QueryReport.ResultCount
		state.probes[key] = probe
		if err := store.saveDeepAnchorProbe(report.RunID, probe); err != nil {
			return err
		}
		deep.SearchPagination = mergePagination(deep.SearchPagination, probe.Pagination)
	}
	deep.ProfilePagination = rebuildDeepProfilePagination(deep.ProfilePagination, state, cfg)
	deep.Funnel.SearchRequests = deep.SearchPagination.Requests
	deep.Funnel.Requests = deep.SearchPagination.Requests + deep.Funnel.ProfileRequests + deep.Funnel.ReplyRequests
	if deep.SearchPagination.StopReason == "" {
		deep.SearchPagination.StopReason = "ssr_windows_completed_or_empty"
	}
	if deep.ProfilePagination.StopReason == "" {
		deep.ProfilePagination.StopReason = "ssr_profile_windows_completed_or_empty"
	}
	if deep.ReplyPagination.StopReason == "" {
		deep.ReplyPagination.StopReason = "no_additional_reply_pages_exposed"
	}

	deep.AnchorProbes = sortedDeepProbes(state.probes)
	deep.SeedProfiles = sortedDeepProfiles(state.profiles)
	deep.SecondaryAnchors = sortedDeepSecondary(state.secondary)
	deep.Replies = sortedDeepReplies(state.replies)
	deep.Provenance = sortedDeepProvenance(state.provenance)
	deep.Corpus = sortedDeepPosts(state.posts)
	deep.PainSignals = deepRecordsForIDs(state, rankedEvidence, true)
	deep.GoldSignals = deepRecordsForIDs(state, report.GoldPainSignals, false)
	report.Queries = deepQueryReports(deep.AnchorProbes)
	report.Collection = deepCollectionCoverage(report, deep, state)
	deep.SearchPagination.GraphQLErrors = maxIntValue(deep.SearchPagination.GraphQLErrors, 0)
	deep.ProfilePagination.GraphQLErrors = maxIntValue(deep.ProfilePagination.GraphQLErrors, 0)
	deep.StageStatus[deepStageFinalize] = "ready"
	if err := store.saveDeepCheckpoint(report.RunID, deepStageFinalize, "summary", map[string]any{
		"posts": len(state.posts), "pain_posts": len(deep.PainSignals), "gold_signals": len(deep.GoldSignals),
		"baseline_posts": baselineCollected, "baseline_ids": len(baselineIDs), "started_at": startedAt,
	}); err != nil {
		return err
	}
	return nil
}

func isDeepEvidence(assessment BusinessPainAssessment) bool {
	return ownerLikely(assessment.OwnerLikelihood) && isPainSignal(assessment)
}

func deepCollectionCoverage(report *Report, deep *DeepReport, state *deepState) CollectionCoverage {
	coverage := CollectionCoverage{
		SearchSurface:             "public anonymous SSR search + profile-first bounded corpus collection",
		QueriesAttempted:          len(deep.AnchorProbes),
		UniquePosts:               len(state.posts),
		RelevantPosts:             len(deep.PainSignals),
		AdjacentPosts:             0,
		UniqueAuthors:             len(state.authors),
		RelevantAuthors:           len(uniqueAuthorsFromRecords(deep.PainSignals)),
		ProfilesAttempted:         deep.Funnel.ProfilesAttempted,
		ProfilesFetched:           deep.Funnel.ProfilesFetched,
		AuthorsWithKnownFollowers: report.Coverage.Followers.Known,
		BaselineAuthorsAttempted:  report.Baseline.AuthorsAttempted,
		BaselineAuthorsReliable:   report.Baseline.AuthorsWithReliableData,
		BaselinePostsCollected:    report.Baseline.PostsCollected,
	}
	for _, probe := range deep.AnchorProbes {
		coverage.RawPostsCollected += probe.QueryReport.ResultCount
		if probe.Status == "completed" || probe.Status == "completed_with_warning" {
			coverage.QueriesSuccessful++
		}
		if probe.QueryReport.ResultCount > 0 {
			coverage.QueriesWithResults++
		}
	}
	coverage.RawPostsCollected += deep.Funnel.ProfilePostsRaw
	coverage.Relevance = relevanceCoverage(postsToMap(state.posts))
	return coverage
}

func postsToMap(records map[string]DeepPostRecord) map[string]Post {
	out := make(map[string]Post, len(records))
	for id, record := range records {
		out[id] = record.Post
	}
	return out
}

func uniqueAuthorsFromRecords(records []DeepPostRecord) map[string]bool {
	out := map[string]bool{}
	for _, record := range records {
		if username := normalizeAuthor(record.Post.AuthorUsername); username != "" {
			out[username] = true
		}
	}
	return out
}

func sortedDeepProbes(values map[string]DeepAnchorProbe) []DeepAnchorProbe {
	out := make([]DeepAnchorProbe, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Anchor.Position != out[j].Anchor.Position {
			return out[i].Anchor.Position < out[j].Anchor.Position
		}
		if out[i].Wave != out[j].Wave {
			return out[i].Wave < out[j].Wave
		}
		return out[i].Anchor.Query < out[j].Anchor.Query
	})
	return out
}

func sortedDeepProfiles(values map[string]DeepSeedProfile) []DeepSeedProfile {
	out := make([]DeepSeedProfile, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Selected != out[j].Selected {
			return out[i].Selected
		}
		if out[i].SeedScore != out[j].SeedScore {
			return out[i].SeedScore > out[j].SeedScore
		}
		return out[i].Username < out[j].Username
	})
	return out
}

func sortedDeepSecondary(values map[string]DeepSecondaryAnchor) []DeepSecondaryAnchor {
	out := make([]DeepSecondaryAnchor, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Phrase < out[j].Phrase })
	return out
}

func sortedDeepReplies(values map[string]DeepReplySignal) []DeepReplySignal {
	out := make([]DeepReplySignal, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].RootPostID != out[j].RootPostID {
			return out[i].RootPostID < out[j].RootPostID
		}
		return out[i].ReplyID < out[j].ReplyID
	})
	return out
}

func sortedDeepProvenance(values map[string]DeepProvenance) []DeepProvenance {
	out := make([]DeepProvenance, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].PostID != out[j].PostID {
			return out[i].PostID < out[j].PostID
		}
		return deepProvenanceKey(out[i]) < deepProvenanceKey(out[j])
	})
	return out
}

func sortedDeepPosts(values map[string]DeepPostRecord) []DeepPostRecord {
	out := make([]DeepPostRecord, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	sort.SliceStable(out, func(i, j int) bool { return deepPostRecordLess(out[i], out[j]) })
	return out
}

func deepRecordsForIDs(state *deepState, ranked []RankedPost, filterGold bool) []DeepPostRecord {
	seen := map[string]bool{}
	out := make([]DeepPostRecord, 0)
	for _, item := range ranked {
		record, ok := state.posts[item.Post.ID]
		if !ok || seen[item.Post.ID] {
			continue
		}
		if filterGold && !isDeepEvidence(record.Assessment) {
			continue
		}
		if !filterGold && !record.Assessment.GoldPainSignal {
			continue
		}
		seen[item.Post.ID] = true
		out = append(out, record)
	}
	return out
}

func deepQueryReports(probes []DeepAnchorProbe) []QueryReport {
	out := make([]QueryReport, 0, len(probes))
	for _, probe := range probes {
		out = append(out, probe.QueryReport)
	}
	return out
}

package research

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// BuildAnalysisReport turns validated classifications and numeric rank output
// into an evidence-linked report. All aggregation in this file is
// deterministic; no model is asked to compute a median, threshold, or claim.
func BuildAnalysisReport(topic string, posts []ClassifiedPost) AnalysisReport {
	report := AnalysisReport{Enabled: true}
	report.Classifications = append([]ClassifiedPost(nil), posts...)
	sort.Slice(report.Classifications, func(i, j int) bool { return report.Classifications[i].PostID < report.Classifications[j].PostID })

	report.ContentTypes = buildPatternStatistics("content_type", posts, func(post ClassifiedPost) []string {
		values := []string{post.Analysis.ContentType}
		return append(values, post.Analysis.SecondaryContentTypes...)
	})
	report.Hooks = buildPatternStatistics("hook_type", posts, func(post ClassifiedPost) []string {
		return []string{post.Analysis.HookType}
	})
	report.Structures = buildPatternStatistics("structure", posts, func(post ClassifiedPost) []string {
		return []string{post.Analysis.Structure}
	})
	report.Tones = buildPatternStatistics("tone", posts, func(post ClassifiedPost) []string {
		return post.Analysis.Tone
	})
	report.Intents = buildPatternStatistics("intent", posts, func(post ClassifiedPost) []string {
		return []string{post.Analysis.Intent}
	})
	report.Topics = buildPatternStatistics("topic", posts, func(post ClassifiedPost) []string {
		if strings.EqualFold(post.Analysis.MainTopic, UnknownLabel) {
			return nil
		}
		return []string{post.Analysis.MainTopic}
	})
	report.Combinations = buildCombinations(posts)
	report.Pains = clusterAnalysisMentions(posts, "pain", func(post ClassifiedPost) []string {
		return post.Analysis.Pains
	})
	report.Questions = clusterAnalysisMentions(posts, "question", func(post ClassifiedPost) []string {
		return post.Analysis.Questions
	})
	report.Opportunities = buildOpportunities(report.Pains, report.Questions, report.ContentTypes)
	report.ProductPainRadar = buildProductPainRadar(posts)
	report.WeakSignals = weakPatternSignals(report.ContentTypes, report.Hooks, report.Structures, report.Tones, report.Intents, report.Topics, report.Combinations)
	if len(posts) > 0 {
		report.Warnings = append(report.Warnings, "pain/question clusters use normalized lexical similarity; review source examples before treating a cluster as one semantic theme")
	}
	if len(report.Opportunities) > 0 {
		report.Warnings = append(report.Warnings, "content opportunities are evidence prompts, not unsupported demand or business claims")
	}
	if len(report.ProductPainRadar) > 0 {
		report.Warnings = append(report.Warnings, "product/service pain radar reports explicit signals only; it is not a business opportunity claim")
	}
	_ = topic // kept in the signature for future topic-scoped narrative providers
	return report
}

func buildPatternStatistics(dimension string, posts []ClassifiedPost, selector func(ClassifiedPost) []string) []PatternStatistic {
	groups := map[string][]ClassifiedPost{}
	for _, post := range posts {
		seen := map[string]bool{}
		for _, value := range selector(post) {
			value = normalizePatternValue(value)
			if value == "" || seen[value] {
				continue
			}
			seen[value] = true
			groups[value] = append(groups[value], post)
		}
	}
	return patternStatisticsFromGroups(dimension, groups)
}

func patternStatisticsFromGroups(dimension string, groups map[string][]ClassifiedPost) []PatternStatistic {
	items := make([]PatternStatistic, 0, len(groups))
	for value, posts := range groups {
		stat := PatternStatistic{Dimension: dimension, Value: value, N: len(posts), SignalStrength: signalStrength(len(posts))}
		var reliable []float64
		seenURLs := map[string]bool{}
		seenIDs := map[string]bool{}
		for _, post := range posts {
			if post.Performance.BaselineConfidence == baselineUsable && post.Performance.RelativePerformance != nil {
				reliable = append(reliable, *post.Performance.RelativePerformance)
			}
			if post.URL != "" && !seenURLs[post.URL] && len(stat.EvidenceURLs) < 10 {
				seenURLs[post.URL] = true
				stat.EvidenceURLs = append(stat.EvidenceURLs, post.URL)
			}
			if post.PostID != "" && !seenIDs[post.PostID] && len(stat.ExamplePostIDs) < 10 {
				seenIDs[post.PostID] = true
				stat.ExamplePostIDs = append(stat.ExamplePostIDs, post.PostID)
			}
		}
		stat.ReliableBaselinePercent = percent(len(reliable), len(posts))
		stat.MedianRelativePerformance, stat.Q1RelativePerformance, stat.Q3RelativePerformance = quartiles(reliable)
		items = append(items, stat)
	}
	sortPatternStatistics(items)
	return items
}

func buildCombinations(posts []ClassifiedPost) []PatternStatistic {
	groups := map[string][]ClassifiedPost{}
	for _, post := range posts {
		pairs := [][3]string{
			{"content_type+hook_type", post.Analysis.ContentType, post.Analysis.HookType},
			{"content_type+structure", post.Analysis.ContentType, post.Analysis.Structure},
			{"hook_type+structure", post.Analysis.HookType, post.Analysis.Structure},
			{"content_type+intent", post.Analysis.ContentType, post.Analysis.Intent},
		}
		seen := map[string]bool{}
		for _, pair := range pairs {
			left := normalizePatternValue(pair[1])
			right := normalizePatternValue(pair[2])
			if left == "" || right == "" || left == UnknownLabel || right == UnknownLabel {
				continue
			}
			key := pair[0] + ":" + left + " + " + right
			if seen[key] {
				continue
			}
			seen[key] = true
			groups[key] = append(groups[key], post)
		}
	}
	byDimension := map[string]map[string][]ClassifiedPost{}
	for key, items := range groups {
		parts := strings.SplitN(key, ":", 2)
		if len(parts) != 2 {
			continue
		}
		if byDimension[parts[0]] == nil {
			byDimension[parts[0]] = map[string][]ClassifiedPost{}
		}
		byDimension[parts[0]][parts[1]] = items
	}
	var out []PatternStatistic
	for dimension, values := range byDimension {
		out = append(out, patternStatisticsFromGroups(dimension, values)...)
	}
	sortPatternStatistics(out)
	return out
}

func sortPatternStatistics(items []PatternStatistic) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].N != items[j].N {
			return items[i].N > items[j].N
		}
		if items[i].Dimension != items[j].Dimension {
			return items[i].Dimension < items[j].Dimension
		}
		return items[i].Value < items[j].Value
	})
}

func weakPatternSignals(groups ...[]PatternStatistic) []PatternStatistic {
	var out []PatternStatistic
	for _, group := range groups {
		for _, item := range group {
			if item.N > 0 && item.N <= 5 {
				out = append(out, item)
			}
		}
	}
	sortPatternStatistics(out)
	return out
}

func normalizePatternValue(value string) string {
	value = strings.ToLower(strings.Join(strings.Fields(value), " "))
	return value
}

func quartiles(values []float64) (medianValue, q1, q3 *float64) {
	if len(values) == 0 {
		return nil, nil, nil
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	medianValue = floatPointer(median(sorted))
	middle := len(sorted) / 2
	q1Values := sorted[:middle]
	q3Values := sorted[(len(sorted)+1)/2:]
	if len(q1Values) > 0 {
		q1 = floatPointer(median(q1Values))
	}
	if len(q3Values) > 0 {
		q3 = floatPointer(median(q3Values))
	}
	return medianValue, q1, q3
}

func floatPointer(value float64) *float64 { return &value }

func percent(n, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(n) * 100 / float64(total)
}

type analysisMention struct {
	text string
	post ClassifiedPost
}

type mentionCluster struct {
	members []analysisMention
}

func clusterAnalysisMentions(posts []ClassifiedPost, kind string, selector func(ClassifiedPost) []string) []PainCluster {
	var mentions []analysisMention
	for _, post := range posts {
		seen := map[string]bool{}
		for _, value := range selector(post) {
			value = strings.Join(strings.Fields(value), " ")
			key := strings.ToLower(value)
			if value == "" || key == UnknownLabel || seen[key] {
				continue
			}
			seen[key] = true
			mentions = append(mentions, analysisMention{text: value, post: post})
		}
	}
	if len(mentions) == 0 {
		return nil
	}
	parent := make([]int, len(mentions))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(index int) int {
		if parent[index] != index {
			parent[index] = find(parent[index])
		}
		return parent[index]
	}
	union := func(left, right int) {
		leftRoot, rightRoot := find(left), find(right)
		if leftRoot != rightRoot {
			if leftRoot < rightRoot {
				parent[rightRoot] = leftRoot
			} else {
				parent[leftRoot] = rightRoot
			}
		}
	}
	for i := range mentions {
		left := clusterTokens(mentions[i].text)
		for j := 0; j < i; j++ {
			if similarTokenSets(left, clusterTokens(mentions[j].text)) {
				union(i, j)
			}
		}
	}
	groups := map[int][]analysisMention{}
	for i, mention := range mentions {
		root := find(i)
		groups[root] = append(groups[root], mention)
	}
	out := make([]PainCluster, 0, len(groups))
	for _, members := range groups {
		out = append(out, painClusterFromMentions(kind, members))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Label < out[j].Label
	})
	return out
}

func painClusterFromMentions(kind string, members []analysisMention) PainCluster {
	cluster := PainCluster{SignalStrength: signalStrength(len(uniqueMentionPostIDs(members)))}
	shortest := ""
	postSeen := map[string]bool{}
	authorSeen := map[string]bool{}
	urlSeen := map[string]bool{}
	exampleSeen := map[string]bool{}
	var reliable []float64
	for _, member := range members {
		value := strings.Join(strings.Fields(member.text), " ")
		if shortest == "" || len([]rune(value)) < len([]rune(shortest)) {
			shortest = value
		}
		if member.post.PostID != "" && !postSeen[member.post.PostID] {
			postSeen[member.post.PostID] = true
			cluster.SourcePostIDs = append(cluster.SourcePostIDs, member.post.PostID)
			if member.post.Performance.BaselineConfidence == baselineUsable && member.post.Performance.RelativePerformance != nil {
				reliable = append(reliable, *member.post.Performance.RelativePerformance)
			}
		}
		if member.post.AuthorUsername != "" && !authorSeen[member.post.AuthorUsername] {
			authorSeen[member.post.AuthorUsername] = true
			cluster.Authors = append(cluster.Authors, member.post.AuthorUsername)
		}
		if member.post.URL != "" && !urlSeen[member.post.URL] {
			urlSeen[member.post.URL] = true
			cluster.EvidenceURLs = append(cluster.EvidenceURLs, member.post.URL)
		}
		if value != "" && !exampleSeen[value] && len(cluster.RepresentativeExamples) < 5 {
			exampleSeen[value] = true
			cluster.RepresentativeExamples = append(cluster.RepresentativeExamples, value)
		}
	}
	sort.Strings(cluster.SourcePostIDs)
	sort.Strings(cluster.Authors)
	sort.Strings(cluster.EvidenceURLs)
	cluster.Count = len(cluster.SourcePostIDs)
	cluster.Label = normalizeClusterLabel(shortest)
	cluster.Description = fmt.Sprintf("Normalized %s wording around %q; source examples are retained for manual review.", kind, cluster.Label)
	cluster.MedianRelativePerformance, _, _ = quartiles(reliable)
	return cluster
}

func uniqueMentionPostIDs(mentions []analysisMention) []string {
	seen := map[string]bool{}
	var out []string
	for _, mention := range mentions {
		if mention.post.PostID != "" && !seen[mention.post.PostID] {
			seen[mention.post.PostID] = true
			out = append(out, mention.post.PostID)
		}
	}
	return out
}

func normalizeClusterLabel(value string) string {
	tokens := clusterTokens(value)
	if len(tokens) == 0 {
		return strings.ToLower(strings.Join(strings.Fields(value), " "))
	}
	return strings.Join(tokens, " ")
}

func clusterTokens(value string) []string {
	var tokens []string
	var current []rune
	flush := func() {
		if len(current) == 0 {
			return
		}
		value := strings.ToLower(string(current))
		current = current[:0]
		if clusterStopWords[value] {
			return
		}
		if alias, ok := clusterAliases[value]; ok {
			value = alias
		}
		if len(value) > 4 && strings.HasSuffix(value, "ies") {
			value = strings.TrimSuffix(value, "ies") + "y"
		} else if len(value) > 4 && strings.HasSuffix(value, "s") {
			value = strings.TrimSuffix(value, "s")
		}
		if !clusterStopWords[value] {
			tokens = append(tokens, value)
		}
	}
	for _, r := range []rune(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current = append(current, r)
			continue
		}
		flush()
	}
	flush()
	seen := map[string]bool{}
	unique := tokens[:0]
	for _, token := range tokens {
		if !seen[token] {
			seen[token] = true
			unique = append(unique, token)
		}
	}
	sort.Strings(unique)
	return unique
}

func similarTokenSets(left, right []string) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	leftSet := map[string]bool{}
	for _, value := range left {
		leftSet[value] = true
	}
	intersection := 0
	for _, value := range right {
		if leftSet[value] {
			intersection++
		}
	}
	union := len(leftSet)
	for _, value := range right {
		if !leftSet[value] {
			union++
		}
	}
	if union == 0 {
		return false
	}
	// A high overlap handles short extracted phrases without merging every
	// mention that merely shares a generic word.
	return float64(intersection)/float64(union) >= 0.5 ||
		(intersection == 1 && intersection == minInt(len(leftSet), len(right)))
}

var clusterStopWords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "be": true, "can": true, "do": true,
	"for": true, "from": true, "how": true, "i": true, "in": true, "is": true,
	"it": true, "me": true, "my": true, "of": true, "on": true, "or": true, "our": true,
	"people": true, "the": true, "this": true, "to": true, "we": true, "what": true, "where": true,
	"who": true, "why": true, "with": true, "you": true, "your": true, "t": true, "s": true,
}

var clusterAliases = map[string]string{
	"client": "client", "clients": "client", "customer": "client", "customers": "client",
	"find": "acquire", "finding": "acquire", "finds": "acquire", "found": "acquire", "get": "acquire", "getting": "acquire",
	"acquire": "acquire", "acquiring": "acquire", "acquisition": "acquire",
	"freelance": "freelance", "freelancer": "freelance", "freelancers": "freelance",
	"problem": "problem", "problems": "problem", "issue": "problem", "issues": "problem",
}

type radarEvidence struct {
	category                string
	signal                  string
	posts                   map[string]bool
	authors                 map[string]bool
	urls                    map[string]bool
	explicitSolutionSeeking int
	buyerIntent             int
	existingToolComplaints  int
	engagement              []float64
	relativePerformance     []float64
}

func buildProductPainRadar(posts []ClassifiedPost) []ProductPainSignal {
	evidence := map[string]*radarEvidence{}
	add := func(category, signal string, post ClassifiedPost, explicitSolutionSeeking, buyerIntent, existingToolComplaint bool) {
		key := category + "\x00" + signal
		item := evidence[key]
		if item == nil {
			item = &radarEvidence{
				category: category,
				signal:   signal,
				posts:    map[string]bool{},
				authors:  map[string]bool{},
				urls:     map[string]bool{},
			}
			evidence[key] = item
		}
		postKey := post.PostID
		if postKey == "" {
			postKey = post.URL
		}
		if postKey == "" {
			postKey = post.AuthorUsername + "\x00" + post.Analysis.MainTopic
		}
		if item.posts[postKey] {
			return
		}
		item.posts[postKey] = true
		if post.AuthorUsername != "" {
			item.authors[post.AuthorUsername] = true
		}
		if post.URL != "" {
			item.urls[post.URL] = true
		}
		if explicitSolutionSeeking {
			item.explicitSolutionSeeking++
		}
		if buyerIntent {
			item.buyerIntent++
		}
		if existingToolComplaint {
			item.existingToolComplaints++
		}
		if post.Performance.MetricCoverage.Known > 0 {
			item.engagement = append(item.engagement, post.Performance.Engagement)
		}
		if post.Performance.BaselineConfidence == baselineUsable && post.Performance.RelativePerformance != nil {
			item.relativePerformance = append(item.relativePerformance, *post.Performance.RelativePerformance)
		}
	}
	for _, post := range posts {
		mentions := append(append([]string(nil), post.Analysis.Pains...), post.Analysis.Questions...)
		if len(mentions) == 0 {
			continue
		}
		var cleanMentions []string
		for _, mention := range mentions {
			mention = strings.Join(strings.Fields(mention), " ")
			if mention != "" && !strings.EqualFold(mention, UnknownLabel) {
				cleanMentions = append(cleanMentions, mention)
			}
		}
		if len(cleanMentions) == 0 {
			continue
		}
		lower := strings.ToLower(strings.Join(cleanMentions, " "))
		tokens := clusterTokens(lower)
		tools := nonEmptyAnalysisValues(post.Analysis.ToolsProductsServices)
		complaint := containsAnySubstring(lower, "problem", "issue", "expensive", "broken", "confusing", "difficult", "can't", "cannot", "struggle", "hard", "frustrat", "hate", "alternative")
		solutionIntent := post.Analysis.Intent == "seek_solution" || post.Analysis.Intent == "recommend_solution"
		explicitSolutionSeeking := len(post.Analysis.Questions) > 0 || solutionIntent || containsAny(tokens, "need", "want", "looking", "alternative", "solution")
		buyerIntent := containsAny(tokens, "client", "acquire", "lead", "hire", "buy", "pay", "budget", "service") || containsAnySubstring(lower, "willing to pay", "looking for a provider", "need help")
		existingToolComplaint := len(tools) > 0 && complaint
		categories := painRadarCategories(tokens, lower, len(tools) > 0, explicitSolutionSeeking)
		if len(categories) == 0 {
			categories["other"] = "Explicit pain/question signal that does not match a narrower radar category"
		}
		for category, signal := range categories {
			add(category, signal, post, explicitSolutionSeeking, buyerIntent, existingToolComplaint)
		}
	}
	result := make([]ProductPainSignal, 0, len(evidence))
	for _, item := range evidence {
		signal := ProductPainSignal{
			Category:                item.category,
			Signal:                  item.signal,
			Count:                   len(item.posts),
			SignalStrength:          signalStrength(len(item.posts)),
			UniqueAuthors:           len(item.authors),
			ExplicitSolutionSeeking: item.explicitSolutionSeeking,
			BuyerIntent:             item.buyerIntent,
			ExistingToolComplaints:  item.existingToolComplaints,
		}
		if len(item.engagement) > 0 {
			signal.MedianEngagement = floatPointer(median(item.engagement))
		}
		if len(item.relativePerformance) > 0 {
			signal.MedianRelativePerformance = floatPointer(median(item.relativePerformance))
		}
		for id := range item.posts {
			if id != "" && !strings.Contains(id, "\x00") {
				signal.SourcePostIDs = append(signal.SourcePostIDs, id)
			}
		}
		for author := range item.authors {
			signal.Authors = append(signal.Authors, author)
		}
		for url := range item.urls {
			signal.EvidenceURLs = append(signal.EvidenceURLs, url)
		}
		sort.Strings(signal.SourcePostIDs)
		sort.Strings(signal.Authors)
		sort.Strings(signal.EvidenceURLs)
		result = append(result, signal)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count != result[j].Count {
			return result[i].Count > result[j].Count
		}
		if result[i].Category != result[j].Category {
			return result[i].Category < result[j].Category
		}
		return result[i].Signal < result[j].Signal
	})
	return result
}

func nonEmptyAnalysisValues(values []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.Join(strings.Fields(value), " ")
		if value == "" || strings.EqualFold(value, UnknownLabel) {
			continue
		}
		key := strings.ToLower(value)
		if !seen[key] {
			seen[key] = true
			out = append(out, value)
		}
	}
	return out
}

func painRadarCategories(tokens []string, lower string, hasTool, solutionSeeking bool) map[string]string {
	categories := map[string]string{}
	add := func(category, signal string) { categories[category] = signal }
	if containsAny(tokens, "manual", "repetitive", "workflow", "process", "time") || containsAnySubstring(lower, "time-consuming", "too much time") {
		add("manual_repetitive_workflow", "Explicit pain/question signal around manual or repetitive workflow work")
	}
	if containsAny(tokens, "search", "discover", "discovery", "acquire", "lookup", "research") {
		add("discovery_search_problem", "Explicit pain/question signal around discovering, searching, or finding something")
	}
	if containsAny(tokens, "analytic", "metric", "measure", "measurement", "dashboard", "report", "track", "data", "attribution") {
		add("analytics_problem", "Explicit pain/question signal around measuring, analyzing, or attributing results")
	}
	if containsAny(tokens, "organize", "organization", "planning", "plan", "calendar", "task", "manage", "management", "note") {
		add("organization_problem", "Explicit pain/question signal around organizing, planning, or tracking work")
	}
	if containsAny(tokens, "communicate", "communication", "email", "meeting", "message", "messaging", "team", "feedback") {
		add("communication_problem", "Explicit pain/question signal around communication or coordination")
	}
	if containsAny(tokens, "client", "acquire", "lead", "hire", "freelance") {
		add("client_acquisition_problem", "Explicit pain/question signal around acquiring clients, customers, leads, or work")
	}
	if containsAny(tokens, "content", "post", "write", "writing", "idea", "copy", "video", "creator", "social") {
		add("content_creation_problem", "Explicit pain/question signal around creating, planning, or distributing content")
	}
	if containsAny(tokens, "implement", "integration", "integrate", "api", "code", "bug", "setup", "technical", "automation", "agent", "developer", "configure") {
		add("technical_implementation_problem", "Explicit pain/question signal around technical implementation or integration")
	}
	if hasTool && containsAnySubstring(lower, "problem", "issue", "expensive", "broken", "confusing", "difficult", "can't", "cannot", "struggle", "hard", "frustrat", "hate", "alternative") {
		add("dissatisfaction_existing_tool", "Explicit complaint or alternative-seeking language involving a named tool or service")
	}
	if !hasTool && solutionSeeking && containsAny(tokens, "tool", "software", "app", "platform", "solution", "alternative", "missing") {
		add("missing_tool", "Explicit solution-seeking language for a tool, app, platform, or missing capability")
	}
	return categories
}

func containsAny(values []string, wanted ...string) bool {
	set := map[string]bool{}
	for _, value := range values {
		set[value] = true
	}
	for _, value := range wanted {
		if set[value] {
			return true
		}
	}
	return false
}

func containsAnySubstring(value string, wanted ...string) bool {
	for _, item := range wanted {
		if strings.Contains(value, item) {
			return true
		}
	}
	return false
}

func buildOpportunities(pains, questions []PainCluster, contentTypes []PatternStatistic) []ContentOpportunity {
	var out []ContentOpportunity
	seen := map[string]bool{}
	appendCluster := func(cluster PainCluster, kind string) {
		if cluster.Count < 3 || cluster.Label == "" {
			return
		}
		key := kind + ":" + cluster.Label
		if seen[key] {
			return
		}
		seen[key] = true
		questionCount := 0
		if kind == "question" {
			questionCount = cluster.Count
		} else {
			for _, question := range questions {
				if similarTokenSets(clusterTokens(cluster.Label), clusterTokens(question.Label)) {
					questionCount += question.Count
				}
			}
		}
		rationale := fmt.Sprintf("Normalized %s wording appears in %d source posts across %d authors; use the linked examples to test an angle as an evidence prompt, not a demand forecast.", kind, cluster.Count, len(cluster.Authors))
		if questionCount > 0 && kind == "pain" {
			rationale = fmt.Sprintf("Recurring pain appears in %d source posts across %d authors and matches %d explicit question mentions; use the linked examples to test an angle as an evidence prompt, not a demand forecast.", cluster.Count, len(cluster.Authors), questionCount)
		}
		out = append(out, ContentOpportunity{
			Title:                     fmt.Sprintf("Explore content on %s", cluster.Label),
			Rationale:                 rationale,
			EvidenceType:              kind + "_cluster",
			EvidenceCount:             cluster.Count,
			UniqueAuthors:             len(cluster.Authors),
			ExplicitQuestionCount:     questionCount,
			MedianRelativePerformance: cloneFloat(cluster.MedianRelativePerformance),
			SignalStrength:            cluster.SignalStrength,
			SourcePostIDs:             append([]string(nil), cluster.SourcePostIDs...),
			EvidenceURLs:              append([]string(nil), cluster.EvidenceURLs...),
		})
	}
	for _, cluster := range pains {
		appendCluster(cluster, "pain")
	}
	for _, cluster := range questions {
		appendCluster(cluster, "question")
	}
	// A content angle is only surfaced when its own pattern has at least three
	// posts and a reliable median exists; it remains a testable prompt, not a
	// claim that the angle will perform in the future.
	for _, pattern := range contentTypes {
		if pattern.N < 3 || pattern.MedianRelativePerformance == nil || *pattern.MedianRelativePerformance <= 1 || seen["pattern:"+pattern.Value] {
			continue
		}
		seen["pattern:"+pattern.Value] = true
		out = append(out, ContentOpportunity{
			Title:                     fmt.Sprintf("Test the %s content angle", pattern.Value),
			Rationale:                 fmt.Sprintf("This label appears in %d classified posts and has a reliable-baseline median of %.2fx; this is historical evidence only.", pattern.N, *pattern.MedianRelativePerformance),
			EvidenceType:              "pattern",
			EvidenceCount:             pattern.N,
			MedianRelativePerformance: cloneFloat(pattern.MedianRelativePerformance),
			SignalStrength:            pattern.SignalStrength,
			SourcePostIDs:             append([]string(nil), pattern.ExamplePostIDs...),
			EvidenceURLs:              append([]string(nil), pattern.EvidenceURLs...),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].EvidenceCount != out[j].EvidenceCount {
			return out[i].EvidenceCount > out[j].EvidenceCount
		}
		return out[i].Title < out[j].Title
	})
	if len(out) > 10 {
		out = out[:10]
	}
	return out
}

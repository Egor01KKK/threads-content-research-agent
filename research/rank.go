package research

import (
	"math"
	"sort"
	"strings"
)

const (
	baselineUnavailable = "unavailable"
	baselineLimited     = "limited"
	baselineUsable      = "usable"
)

// Rank calculates post and author rankings from topic evidence and independent
// recent-author baselines. It is deterministic for a fixed input and does not
// call Threads or an LLM.
func Rank(posts []Post, authors map[string]Author, queries map[string][]string, baseline BaselineData, cfg Config) ([]RankedPost, []RankedAuthor) {
	rankedPosts := RankPosts(posts, authors, queries, baseline, cfg)
	rankedAuthors := RankAuthors(rankedPosts, authors, cfg)
	return rankedPosts, rankedAuthors
}

// RankPosts ranks topic posts with logarithmically damped engagement,
// follower-relative engagement when available, and performance against an
// independent recent-post median for the same author.
func RankPosts(posts []Post, authors map[string]Author, queries map[string][]string, baseline BaselineData, cfg Config) []RankedPost {
	if baseline.MinPosts <= 0 {
		baseline.MinPosts = DefaultConfig().MinBaselinePosts
	}
	baselineStats := buildBaselineStats(baseline, cfg.Engagement)
	out := make([]RankedPost, 0, len(posts))
	for _, post := range posts {
		authorKey := normalizeAuthor(post.AuthorUsername)
		engagement := weightedEngagement(post, cfg.Engagement)
		stats := baselineStats[authorKey]
		baselineEngagement := optionalValue(stats.median, stats.count > 0)
		relative, confidence := relativePerformance(engagement, stats, baseline.MinPosts)
		if engagementMetricCount(post) == 0 {
			relative = nil
		}
		var rate *float64
		if engagementMetricCount(post) > 0 {
			rate = postEngagementRate(engagement, authorFor(authors, authorKey))
		}
		rawScore := postRankScore(engagement, rate, relative, cfg.PostScore)
		coverage := scoringCoverage(post, rate, relative, cfg.PostScore)
		queriesForPost := append([]string(nil), queries[post.ID]...)
		sort.Strings(queriesForPost)
		out = append(out, RankedPost{
			Post:                post,
			SearchQueries:       queriesForPost,
			MetricCoverage:      postMetricCoverage(post),
			ScoringCoverage:     coverage,
			KnownMetricCount:    knownMetricCount(post),
			Engagement:          engagement,
			EngagementRate:      rate,
			BaselineEngagement:  baselineEngagement,
			RelativePerformance: relative,
			BaselinePostCount:   stats.count,
			BaselineConfidence:  confidence,
			RawRankScore:        rawScore,
			RankScore:           rawScore * coverage,
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].RankScore != out[j].RankScore {
			return out[i].RankScore > out[j].RankScore
		}
		if out[i].ScoringCoverage != out[j].ScoringCoverage {
			return out[i].ScoringCoverage > out[j].ScoringCoverage
		}
		if compareOptional(out[i].RelativePerformance, out[j].RelativePerformance) != 0 {
			return compareOptional(out[i].RelativePerformance, out[j].RelativePerformance) > 0
		}
		if out[i].Engagement != out[j].Engagement {
			return out[i].Engagement > out[j].Engagement
		}
		return out[i].Post.ID < out[j].Post.ID
	})
	return out
}

// RankAuthors aggregates the already-ranked topic posts. Baseline confidence
// and metric coverage remain visible, so a creator with no independent sample
// cannot be mistaken for one with a reliable outperformance history.
func RankAuthors(posts []RankedPost, authors map[string]Author, cfg Config) []RankedAuthor {
	byAuthor := map[string][]RankedPost{}
	for _, post := range posts {
		key := normalizeAuthor(post.Post.AuthorUsername)
		if key == "" {
			continue
		}
		byAuthor[key] = append(byAuthor[key], post)
	}

	out := make([]RankedAuthor, 0, len(byAuthor))
	for username, authorPosts := range byAuthor {
		var engagements []float64
		var rates []float64
		var relatives []float64
		var coverages []float64
		urls := make([]string, 0, 3)
		knownByMetric := [5]int{}
		baselinePostCount := 0
		baselineConfidence := baselineUnavailable
		for _, post := range authorPosts {
			engagements = append(engagements, post.Engagement)
			if post.EngagementRate != nil {
				rates = append(rates, *post.EngagementRate)
			}
			if post.RelativePerformance != nil {
				relatives = append(relatives, *post.RelativePerformance)
			}
			coverages = append(coverages, post.ScoringCoverage)
			if post.BaselinePostCount > baselinePostCount {
				baselinePostCount = post.BaselinePostCount
			}
			baselineConfidence = strongerBaselineConfidence(baselineConfidence, post.BaselineConfidence)
			if post.Post.URL != "" && len(urls) < 3 {
				urls = append(urls, post.Post.URL)
			}
			for i, value := range []*int64{post.Post.Likes, post.Post.Replies, post.Post.Reposts, post.Post.Quotes, post.Post.Views} {
				if value != nil {
					knownByMetric[i]++
				}
			}
		}
		medianRate := optionalMedian(rates)
		medianRelative := optionalMedian(relatives)
		rawScore := authorRankScore(median(engagements), medianRate, medianRelative, len(authorPosts), cfg.AuthorScore)
		coverage := median(coverages)
		out = append(out, RankedAuthor{
			Author:                    authorFor(authors, username),
			RelevantPostCount:         len(authorPosts),
			MetricCoverage:            postMetricCoverageFromCounts(knownByMetric, len(authorPosts)),
			ScoringCoverage:           coverage,
			MedianEngagement:          median(engagements),
			MedianEngagementRate:      medianRate,
			MedianRelativePerformance: medianRelative,
			BaselinePostCount:         baselinePostCount,
			BaselineConfidence:        baselineConfidence,
			RawRankScore:              rawScore,
			RankScore:                 rawScore * coverage,
			EvidenceURLs:              urls,
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].RankScore != out[j].RankScore {
			return out[i].RankScore > out[j].RankScore
		}
		if out[i].ScoringCoverage != out[j].ScoringCoverage {
			return out[i].ScoringCoverage > out[j].ScoringCoverage
		}
		if compareOptional(out[i].MedianRelativePerformance, out[j].MedianRelativePerformance) != 0 {
			return compareOptional(out[i].MedianRelativePerformance, out[j].MedianRelativePerformance) > 0
		}
		if out[i].RelevantPostCount != out[j].RelevantPostCount {
			return out[i].RelevantPostCount > out[j].RelevantPostCount
		}
		return out[i].Author.Username < out[j].Author.Username
	})
	return out
}

type baselineStat struct {
	median float64
	count  int
}

func buildBaselineStats(data BaselineData, weights EngagementWeights) map[string]baselineStat {
	byAuthor := map[string][]float64{}
	for username, posts := range data.PostsByAuthor {
		key := normalizeAuthor(username)
		for _, post := range posts {
			if engagementMetricCount(post) == 0 {
				continue
			}
			byAuthor[key] = append(byAuthor[key], weightedEngagement(post, weights))
		}
	}
	stats := map[string]baselineStat{}
	for author, values := range byAuthor {
		stats[author] = baselineStat{median: median(values), count: len(values)}
	}
	return stats
}

func relativePerformance(engagement float64, baseline baselineStat, minPosts int) (*float64, string) {
	if baseline.count == 0 {
		return nil, baselineUnavailable
	}
	confidence := baselineUsable
	if baseline.count < minPosts {
		confidence = baselineLimited
	}
	if baseline.median <= 0 {
		return nil, baselineUnavailable
	}
	relative := engagement / baseline.median
	return &relative, confidence
}

func weightedEngagement(post Post, weights EngagementWeights) float64 {
	var total float64
	if post.Likes != nil {
		total += weights.Likes * float64(*post.Likes)
	}
	if post.Replies != nil {
		total += weights.Replies * float64(*post.Replies)
	}
	if post.Reposts != nil {
		total += weights.Reposts * float64(*post.Reposts)
	}
	if post.Quotes != nil {
		total += weights.Quotes * float64(*post.Quotes)
	}
	return total
}

func engagementMetricCount(post Post) int {
	count := 0
	for _, value := range []*int64{post.Likes, post.Replies, post.Reposts, post.Quotes} {
		if value != nil {
			count++
		}
	}
	return count
}

func knownMetricCount(post Post) int {
	count := engagementMetricCount(post)
	if post.Views != nil {
		count++
	}
	return count
}

func postMetricCoverage(post Post) MetricCoverage {
	return metricCoverage(knownMetricCount(post), 5)
}

func postMetricCoverageFromCounts(known [5]int, total int) MetricCoverage {
	count := 0
	for _, value := range known {
		count += value
	}
	return metricCoverage(count, total*len(known))
}

func metricCoverage(known, total int) MetricCoverage {
	coverage := MetricCoverage{Known: known, Total: total}
	if total > 0 {
		coverage.Percent = float64(known) * 100 / float64(total)
	}
	return coverage
}

func scoringCoverage(post Post, rate, relative *float64, weights PostScoreWeights) float64 {
	engagementWeight := weights.Engagement
	if weights.Outperformance > 0 {
		engagementWeight += weights.Outperformance
	}
	rateWeight := weights.EngagementRate
	total := engagementWeight + rateWeight
	if total <= 0 {
		return 1
	}
	engagementCoverage := float64(engagementMetricCount(post)) / 4
	coverage := engagementWeight * engagementCoverage
	if rateWeight > 0 && rate != nil {
		coverage += rateWeight
	}
	if weights.Outperformance > 0 && relative == nil {
		coverage -= weights.Outperformance * engagementCoverage
	}
	return maxFloat(coverage/total, 0)
}

func postEngagementRate(engagement float64, author Author) *float64 {
	if author.FollowerCount == nil || *author.FollowerCount <= 0 {
		return nil
	}
	rate := engagement / float64(*author.FollowerCount)
	return &rate
}

func postRankScore(engagement float64, rate, relative *float64, weights PostScoreWeights) float64 {
	rateTerm := 0.0
	if rate != nil {
		rateTerm = math.Log1p(1000 * maxFloat(*rate, 0))
	}
	relativeTerm := 0.0
	if relative != nil {
		relativeTerm = math.Log1p(maxFloat(*relative, 0))
	}
	return weights.Engagement*math.Log1p(maxFloat(engagement, 0)) +
		weights.EngagementRate*rateTerm +
		weights.Outperformance*relativeTerm
}

func authorRankScore(engagement float64, rate, relative *float64, volume int, weights AuthorScoreWeights) float64 {
	rateTerm := 0.0
	if rate != nil {
		rateTerm = math.Log1p(1000 * maxFloat(*rate, 0))
	}
	relativeTerm := 0.0
	if relative != nil {
		relativeTerm = math.Log1p(maxFloat(*relative, 0))
	}
	return weights.Engagement*math.Log1p(maxFloat(engagement, 0)) +
		weights.EngagementRate*rateTerm +
		weights.Outperformance*relativeTerm +
		weights.Volume*math.Log1p(float64(volume))
}

func authorFor(authors map[string]Author, username string) Author {
	key := normalizeAuthor(username)
	if author, ok := authors[key]; ok {
		if author.Username == "" {
			author.Username = key
		}
		return author
	}
	return Author{
		Username:   key,
		ProfileURL: "https://www.threads.com/@" + key,
	}
}

func optionalMedian(values []float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	value := median(values)
	return &value
}

func optionalValue(value float64, present bool) *float64 {
	if !present {
		return nil
	}
	return &value
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[middle]
	}
	return (sorted[middle-1] + sorted[middle]) / 2
}

func normalizeAuthor(username string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(username), "@"))
}

func strongerBaselineConfidence(current, candidate string) string {
	rank := map[string]int{baselineUnavailable: 0, baselineLimited: 1, baselineUsable: 2}
	if rank[candidate] > rank[current] {
		return candidate
	}
	return current
}

func compareOptional(left, right *float64) int {
	if left == nil && right == nil {
		return 0
	}
	if left == nil {
		return -1
	}
	if right == nil {
		return 1
	}
	if *left < *right {
		return -1
	}
	if *left > *right {
		return 1
	}
	return 0
}

func maxFloat(value, floor float64) float64 {
	if value < floor {
		return floor
	}
	return value
}

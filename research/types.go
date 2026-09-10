// Package research contains the topic-first research pipeline built on top of
// the reusable Threads collection package.
package research

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"
	"time"

	"github.com/Egor01KKK/threads-content-research-agent/threads"
)

// QueryExpander turns a topic into search queries. The MVP uses
// DeterministicExpander; an LLM-backed implementation can satisfy this
// interface later without changing the pipeline.
type QueryExpander interface {
	Expand(context.Context, string) ([]string, error)
}

// Collector is the smallest part of threads.Client needed by the pipeline.
// ProfilePosts is deliberately separate from search results: those posts are
// used to establish a personal baseline and are not automatically topic
// evidence.
type Collector interface {
	Search(context.Context, string, int) iter.Seq2[threads.SearchResult, error]
	Profile(context.Context, string) (*threads.Profile, error)
	ProfilePosts(context.Context, string, int) iter.Seq2[threads.Post, error]
}

// ErrInvalidConfig indicates that a research run cannot be started safely.
var ErrInvalidConfig = errors.New("invalid research configuration")

// RunStatus is persisted so an interrupted run is distinguishable from a
// completed run that merely had partial collection warnings.
type RunStatus string

const (
	RunStatusRunning            RunStatus = "running"
	RunStatusCompleted          RunStatus = "completed"
	RunStatusCompletedWithWarns RunStatus = "completed_with_warnings"
	RunStatusFailed             RunStatus = "failed"
)

func (s RunStatus) valid() bool {
	switch s {
	case RunStatusRunning, RunStatusCompleted, RunStatusCompletedWithWarns, RunStatusFailed:
		return true
	default:
		return false
	}
}

// EngagementWeights controls the transparent engagement heuristic.
type EngagementWeights struct {
	Likes   float64 `json:"likes"`
	Replies float64 `json:"replies"`
	Reposts float64 `json:"reposts"`
	Quotes  float64 `json:"quotes"`
}

// PostScoreWeights controls the post rank formula. Each component is
// logarithmically damped; weights are deliberately configurable rather than
// hidden constants.
type PostScoreWeights struct {
	Engagement     float64 `json:"engagement"`
	EngagementRate float64 `json:"engagement_rate"`
	Outperformance float64 `json:"outperformance"`
}

// AuthorScoreWeights controls author ranking.
type AuthorScoreWeights struct {
	Engagement     float64 `json:"engagement"`
	EngagementRate float64 `json:"engagement_rate"`
	Outperformance float64 `json:"outperformance"`
	Volume         float64 `json:"volume"`
}

const (
	// ModeBounded is the original topic-first bounded run. It is kept as the
	// default so existing callers and datasets retain their previous behavior.
	ModeBounded = "bounded"
	// ModeDeep is the profile-first Russian small-business corpus run.
	ModeDeep = "deep"
)

// Config contains the research run settings. The fields prefixed with deep
// are ignored by the bounded mode and exist only to make the deep safety
// envelope explicit and resumable.
type Config struct {
	Topic                string             `json:"topic"`
	Mode                 string             `json:"mode,omitempty"`
	PerQueryLimit        int                `json:"per_query_limit"`
	MaxQueries           int                `json:"max_queries"`
	ProfileLimit         int                `json:"profile_limit"`
	BaselineAuthorLimit  int                `json:"baseline_author_limit"`
	BaselinePostLimit    int                `json:"baseline_post_limit"`
	MinBaselinePosts     int                `json:"min_baseline_posts"`
	TopPosts             int                `json:"top_posts"`
	TopAuthors           int                `json:"top_authors"`
	Engagement           EngagementWeights  `json:"engagement_weights"`
	PostScore            PostScoreWeights   `json:"post_score_weights"`
	AuthorScore          AuthorScoreWeights `json:"author_score_weights"`
	MaxSeedProfiles      int                `json:"max_seed_profiles,omitempty"`
	MaxVerifiedProfiles  int                `json:"max_verified_profiles,omitempty"`
	ProfilePostLimit     int                `json:"profile_posts,omitempty"`
	MaxTotalProfilePosts int                `json:"max_total_profile_posts,omitempty"`
	ProfileDelay         time.Duration      `json:"profile_delay,omitempty"`
	ReplyDelay           time.Duration      `json:"reply_delay,omitempty"`
	ResumeRunID          int64              `json:"resume_run_id,omitempty"`

	// AnchorLimit is intentionally not exposed as a CLI flag. It is a test and
	// embedding hook; zero means use the complete production anchor bank.
	AnchorLimit int `json:"anchor_limit,omitempty"`
}

// DefaultConfig returns the smallest useful bounded research configuration.
func DefaultConfig() Config {
	return Config{
		Mode:                ModeBounded,
		PerQueryLimit:       10,
		MaxQueries:          10,
		ProfileLimit:        50,
		BaselineAuthorLimit: 20,
		BaselinePostLimit:   30,
		MinBaselinePosts:    3,
		TopPosts:            10,
		TopAuthors:          10,
		Engagement: EngagementWeights{
			Likes:   1,
			Replies: 2,
			Reposts: 3,
			Quotes:  3,
		},
		PostScore: PostScoreWeights{
			Engagement:     0.25,
			EngagementRate: 0.40,
			Outperformance: 0.35,
		},
		AuthorScore: AuthorScoreWeights{
			Engagement:     0.15,
			EngagementRate: 0.40,
			Outperformance: 0.30,
			Volume:         0.15,
		},
		MaxSeedProfiles:      1000,
		MaxVerifiedProfiles:  300,
		ProfilePostLimit:     50,
		MaxTotalProfilePosts: 15000,
		ProfileDelay:         2 * time.Second,
		ReplyDelay:           2 * time.Second,
	}
}

// Validate checks bounds before any network or database work begins.
func (c Config) Validate() error {
	if normalizeSpace(c.Topic) == "" {
		return errors.Join(ErrInvalidConfig, errors.New("topic is required"))
	}
	mode := strings.ToLower(strings.TrimSpace(c.Mode))
	if mode == "" {
		mode = ModeBounded
	}
	if mode != ModeBounded && mode != ModeDeep {
		return errors.Join(ErrInvalidConfig, fmt.Errorf("unknown research mode %q", c.Mode))
	}
	if c.PerQueryLimit < 1 || c.MaxQueries < 1 || c.ProfileLimit < 0 ||
		c.BaselineAuthorLimit < 0 || c.BaselinePostLimit < 1 || c.MinBaselinePosts < 1 ||
		c.TopPosts < 1 || c.TopAuthors < 1 {
		return errors.Join(ErrInvalidConfig, errors.New("limits must be positive; profile and baseline-author limits may be zero"))
	}
	for _, value := range []float64{
		c.Engagement.Likes, c.Engagement.Replies, c.Engagement.Reposts, c.Engagement.Quotes,
		c.PostScore.Engagement, c.PostScore.EngagementRate, c.PostScore.Outperformance,
		c.AuthorScore.Engagement, c.AuthorScore.EngagementRate,
		c.AuthorScore.Outperformance, c.AuthorScore.Volume,
	} {
		if value < 0 {
			return errors.Join(ErrInvalidConfig, errors.New("weights cannot be negative"))
		}
	}
	if mode == ModeDeep {
		if !isRussianSmallBusinessPainTopic(c.Topic) {
			return errors.Join(ErrInvalidConfig, errors.New("deep mode is restricted to the Russian small-business research domain"))
		}
		if c.MaxSeedProfiles < 1 || c.MaxSeedProfiles > 1000 ||
			c.MaxVerifiedProfiles < 1 || c.MaxVerifiedProfiles > 300 ||
			c.ProfilePostLimit < 1 || c.ProfilePostLimit > 50 ||
			c.MaxTotalProfilePosts < 1 || c.MaxTotalProfilePosts > 15000 {
			return errors.Join(ErrInvalidConfig, errors.New("deep limits must be within max_seed_profiles 1..1000, max_verified_profiles 1..300, profile_posts 1..50, max_total_profile_posts 1..15000"))
		}
		if c.ProfileDelay < 0 || c.ReplyDelay < 0 || c.AnchorLimit < 0 {
			return errors.Join(ErrInvalidConfig, errors.New("deep delays and anchor limit cannot be negative"))
		}
	}
	return nil
}

// Post is a canonical topic-evidence post. Pointer metrics are intentional:
// nil means Threads did not expose the field, not that the value was zero.
type Post struct {
	ID               string    `json:"id"`
	Shortcode        string    `json:"shortcode,omitempty"`
	URL              string    `json:"url,omitempty"`
	Text             string    `json:"text,omitempty"`
	AuthorUsername   string    `json:"author_username,omitempty"`
	PublishedAt      time.Time `json:"published_at,omitempty"`
	Likes            *int64    `json:"likes,omitempty"`
	Replies          *int64    `json:"replies,omitempty"`
	Reposts          *int64    `json:"reposts,omitempty"`
	Quotes           *int64    `json:"quotes,omitempty"`
	Views            *int64    `json:"views,omitempty"`
	DetectedTopic    string    `json:"detected_topic,omitempty"`
	RelevanceScore   float64   `json:"relevance_score,omitempty"`
	RelevanceLabel   string    `json:"relevance_label,omitempty"`
	RelevanceReasons []string  `json:"relevance_reasons,omitempty"`
	FirstSeenAt      time.Time `json:"first_seen_at,omitempty"`
	LastSeenAt       time.Time `json:"last_seen_at,omitempty"`
}

// Author is a canonical research author record.
type Author struct {
	Username       string `json:"username"`
	Name           string `json:"name,omitempty"`
	Bio            string `json:"bio,omitempty"`
	FollowerCount  *int64 `json:"follower_count,omitempty"`
	FollowingCount *int64 `json:"following_count,omitempty"`
	Verified       *bool  `json:"verified,omitempty"`
	ProfileURL     string `json:"profile_url,omitempty"`
}

// MetricCoverage reports known values without imputing any missing values.
type MetricCoverage struct {
	Known   int     `json:"known"`
	Total   int     `json:"total"`
	Percent float64 `json:"percent"`
}

// DatasetCoverage makes the evidence denominator visible to the user.
type DatasetCoverage struct {
	Posts     int            `json:"posts"`
	Likes     MetricCoverage `json:"likes"`
	Replies   MetricCoverage `json:"replies"`
	Reposts   MetricCoverage `json:"reposts"`
	Quotes    MetricCoverage `json:"quotes"`
	Views     MetricCoverage `json:"views"`
	Followers MetricCoverage `json:"followers"`
}

// BaselineCoverage reports how much independent recent-author evidence was
// collected for the selected baseline authors.
type BaselineCoverage struct {
	AuthorsAttempted         int     `json:"authors_attempted"`
	AuthorsWithPosts         int     `json:"authors_with_posts"`
	AuthorsWithReliableData  int     `json:"authors_with_reliable_data"`
	PostsCollected           int     `json:"posts_collected"`
	PostsWithKnownEngagement int     `json:"posts_with_known_engagement"`
	Percent                  float64 `json:"percent"`
}

// BaselineData keeps recent author posts separate from topic evidence.
type BaselineData struct {
	PostsByAuthor map[string][]Post
	MinPosts      int
}

// RankedPost contains computed evidence metrics for a discovered topic post.
type RankedPost struct {
	Post                     Post           `json:"post"`
	SearchQueries            []string       `json:"search_queries,omitempty"`
	MetricCoverage           MetricCoverage `json:"metric_coverage"`
	ScoringCoverage          float64        `json:"scoring_coverage"`
	KnownMetricCount         int            `json:"known_metric_count"`
	Engagement               float64        `json:"engagement"`
	EngagementRate           *float64       `json:"engagement_rate,omitempty"`
	BaselineEngagement       *float64       `json:"baseline_engagement,omitempty"`
	RelativePerformance      *float64       `json:"relative_performance,omitempty"`
	BaselinePostCount        int            `json:"baseline_post_count"`
	BaselineConfidence       string         `json:"baseline_confidence"`
	RawRankScore             float64        `json:"raw_rank_score"`
	RankScore                float64        `json:"rank_score"`
	SignalTypes              []SignalType   `json:"signal_types,omitempty"`
	PrimarySignal            SignalType     `json:"primary_signal,omitempty"`
	CommercialIntentPriority int            `json:"commercial_intent_priority,omitempty"`
	RequestedRole            string         `json:"requested_role,omitempty"`
	Need                     string         `json:"need,omitempty"`
	PaidSignal               string         `json:"paid_signal,omitempty"`
	CTA                      string         `json:"cta,omitempty"`
	SignalReasons            []string       `json:"signal_reasons,omitempty"`
	RussianLanguage          string         `json:"russian_language,omitempty"`
	OwnerLikelihood          string         `json:"owner_likelihood,omitempty"`
	PainTypes                []string       `json:"pain_types,omitempty"`
	PainStrength             string         `json:"pain_strength,omitempty"`
	SolutionSeeking          string         `json:"solution_seeking,omitempty"`
	WorkaroundPresent        string         `json:"workaround_present,omitempty"`
	ToolsMentioned           []string       `json:"tools_mentioned,omitempty"`
	ITActionability          string         `json:"it_actionability,omitempty"`
	BuyerIntent              string         `json:"buyer_intent,omitempty"`
	BusinessType             string         `json:"business_type,omitempty"`
	BusinessTypes            []string       `json:"business_types,omitempty"`
	BusinessProcess          string         `json:"business_process,omitempty"`
	BusinessProcesses        []string       `json:"business_processes,omitempty"`
	BusinessConsequences     []string       `json:"business_consequences,omitempty"`
	CurrentWorkaround        string         `json:"current_workaround,omitempty"`
	GoldPainSignal           bool           `json:"gold_pain_signal,omitempty"`
	BusinessPainReasons      []string       `json:"business_pain_reasons,omitempty"`
}

// RankedAuthor contains robust aggregate evidence for an author.
type RankedAuthor struct {
	Author                    Author         `json:"author"`
	RelevantPostCount         int            `json:"relevant_post_count"`
	MetricCoverage            MetricCoverage `json:"metric_coverage"`
	ScoringCoverage           float64        `json:"scoring_coverage"`
	MedianEngagement          float64        `json:"median_engagement"`
	MedianEngagementRate      *float64       `json:"median_engagement_rate,omitempty"`
	MedianRelativePerformance *float64       `json:"median_relative_performance,omitempty"`
	BaselinePostCount         int            `json:"baseline_post_count"`
	BaselineConfidence        string         `json:"baseline_confidence"`
	RawRankScore              float64        `json:"raw_rank_score"`
	RankScore                 float64        `json:"rank_score"`
	EvidenceURLs              []string       `json:"evidence_urls,omitempty"`
}

// RelevanceLabel is the deterministic gate between public search collection
// and topic conclusions. It is deliberately separate from semantic content
// analysis: a post can be relevant without needing an LLM classification.
type RelevanceLabel string

const (
	RelevanceRelevant   RelevanceLabel = "relevant"
	RelevanceAdjacent   RelevanceLabel = "adjacent"
	RelevanceIrrelevant RelevanceLabel = "irrelevant"
	RelevanceUncertain  RelevanceLabel = "uncertain"
)

// RelevanceAssessment keeps the score and the rules that produced it visible
// for audit and regression testing.
type RelevanceAssessment struct {
	Score   float64        `json:"relevance_score"`
	Label   RelevanceLabel `json:"relevance_label"`
	Reasons []string       `json:"relevance_reasons,omitempty"`
}

// SignalCounts is calculated over relevant posts only. Counts are not
// mutually exclusive because one post may express several signals.
type SignalCounts struct {
	RelevantPosts            int `json:"relevant_posts"`
	BuyerHiringSignals       int `json:"buyer_hiring_signals"`
	PaidOpportunities        int `json:"paid_opportunities"`
	ClientSeeking            int `json:"client_seeking"`
	AcquisitionPain          int `json:"acquisition_pain"`
	AcquisitionQuestions     int `json:"acquisition_questions"`
	AcquisitionMethods       int `json:"acquisition_methods"`
	AcquisitionServiceOffers int `json:"acquisition_service_offers"`
	AcquisitionOpinions      int `json:"acquisition_opinions"`
	OtherRelevant            int `json:"other_relevant"`
	PotentialBuyerSignals    int `json:"potential_buyer_signals"`
}

// BusinessPainCounts summarizes the deterministic Russian small-business
// pain layer. Counts are calculated over relevant posts only and are not
// mutually exclusive where a post carries multiple pain types.
type BusinessPainCounts struct {
	Language              string         `json:"language,omitempty"`
	RussianPosts          int            `json:"russian_posts"`
	OwnerLikelyPosts      int            `json:"owner_likely_posts"`
	GenuinePainPosts      int            `json:"genuine_pain_posts"`
	ITActionablePainPosts int            `json:"it_actionable_pain_posts"`
	SolutionSeekingPosts  int            `json:"solution_seeking_posts"`
	WorkaroundPosts       int            `json:"workaround_posts"`
	GoldPainSignals       int            `json:"gold_pain_signals"`
	OwnerLikelihoodCounts map[string]int `json:"owner_likelihood_counts,omitempty"`
	PainTypeCounts        map[string]int `json:"pain_type_counts,omitempty"`
	BusinessTypeCounts    map[string]int `json:"business_type_counts,omitempty"`
	BusinessProcessCounts map[string]int `json:"business_process_counts,omitempty"`
}

// QueryReport records what happened for one expanded search query.
type QueryReport struct {
	Query                    string  `json:"query"`
	Language                 string  `json:"language,omitempty"`
	Successful               bool    `json:"successful"`
	ResultCount              int     `json:"result_count"`
	UniqueNewPosts           int     `json:"unique_new_posts"`
	Relevant                 int     `json:"relevant"`
	Adjacent                 int     `json:"adjacent"`
	Irrelevant               int     `json:"irrelevant"`
	Uncertain                int     `json:"uncertain"`
	Precision                float64 `json:"precision"`
	ContributesToConclusions bool    `json:"contributes_to_conclusions"`
	ProbeResultCount         int     `json:"probe_result_count,omitempty"`
	ProbeRelevantPosts       int     `json:"probe_relevant_posts,omitempty"`
	ProbeOwnerLikelyPosts    int     `json:"probe_owner_likely_posts,omitempty"`
	ProbePainPosts           int     `json:"probe_pain_posts,omitempty"`
	ProbeITActionablePosts   int     `json:"probe_it_actionable_posts,omitempty"`
	ProbeUniqueContribution  int     `json:"probe_unique_contribution,omitempty"`
	ProbeDuplicates          int     `json:"probe_duplicates,omitempty"`
	ProbePrecision           float64 `json:"probe_precision,omitempty"`
	SelectedForDeep          bool    `json:"selected_for_deep,omitempty"`
	DeepResultCount          int     `json:"deep_result_count,omitempty"`
	DeepUniqueContribution   int     `json:"deep_unique_contribution,omitempty"`
	Error                    string  `json:"error,omitempty"`
}

// Report is the machine-readable and terminal-renderable result of a run.
type Report struct {
	Topic                      string             `json:"topic"`
	Language                   string             `json:"language,omitempty"`
	RunID                      int64              `json:"run_id"`
	Status                     RunStatus          `json:"status"`
	StartedAt                  time.Time          `json:"started_at"`
	CompletedAt                time.Time          `json:"completed_at"`
	Queries                    []QueryReport      `json:"queries"`
	PostsAnalyzed              int                `json:"posts_analyzed"`
	AuthorsDiscovered          int                `json:"authors_discovered"`
	ProfilesDiscovered         int                `json:"profiles_discovered"`
	ProfilesAttempted          int                `json:"profiles_attempted"`
	ProfilesFetched            int                `json:"profiles_fetched"`
	ProfilesWithKnownFollowers int                `json:"profiles_with_known_followers"`
	FollowerRelativeRanking    string             `json:"follower_relative_ranking"`
	PostsWithKnownFollowers    int                `json:"posts_with_known_followers"`
	Baseline                   BaselineCoverage   `json:"baseline"`
	Coverage                   DatasetCoverage    `json:"coverage"`
	Collection                 CollectionCoverage `json:"collection_coverage"`
	Analysis                   AnalysisReport     `json:"analysis"`
	TopPosts                   []RankedPost       `json:"top_posts"`
	TopOutperformingPosts      []RankedPost       `json:"top_outperforming_posts"`
	TopAuthors                 []RankedAuthor     `json:"top_authors"`
	RawTopPosts                []RankedPost       `json:"raw_top_posts,omitempty"`
	RawTopOutperformingPosts   []RankedPost       `json:"raw_top_outperforming_posts,omitempty"`
	RawTopAuthors              []RankedAuthor     `json:"raw_top_authors,omitempty"`
	SignalCounts               SignalCounts       `json:"signal_counts"`
	LeadBuyerSignals           []RankedPost       `json:"lead_buyer_signals,omitempty"`
	ContentAudienceSignals     []RankedPost       `json:"content_audience_signals,omitempty"`
	BusinessPainCounts         BusinessPainCounts `json:"business_pain_counts,omitempty"`
	GoldPainSignals            []RankedPost       `json:"gold_pain_signals,omitempty"`
	BusinessPainPosts          []RankedPost       `json:"business_pain_posts,omitempty"`
	Warnings                   []string           `json:"warnings,omitempty"`
	Limitations                []string           `json:"limitations,omitempty"`
	Scoring                    Config             `json:"scoring"`
	Deep                       *DeepReport        `json:"deep,omitempty"`
}

func normalizeSpace(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

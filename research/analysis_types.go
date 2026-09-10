package research

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	// AnalysisSchemaVersion changes whenever the stored semantic payload or its
	// validation contract changes. It is part of the cache key on purpose.
	AnalysisSchemaVersion = "content-intelligence.v2"
	// AnalysisPromptVersion changes whenever the instructions sent to a model
	// change in a way that can alter classification results.
	AnalysisPromptVersion = "content-intelligence-prompt.v2"
	UnknownLabel          = "unknown"

	AnalysisProviderOpenAI    = "openai"
	AnalysisProviderAnthropic = "anthropic"
)

// AnalysisProvider is the model boundary for content intelligence. The
// collection pipeline remains usable without one, while concrete providers
// can be swapped or faked without changing storage or aggregation code.
type AnalysisProvider interface {
	Name() string
	Model() string
	AnalyzePosts(context.Context, []AnalysisInput) (AnalysisBatch, error)
	AnalyzeAggregatePatterns(context.Context, AggregatePatternInput) (AggregatePatternResult, error)
}

// AnalysisInput deliberately excludes performance metrics. Metrics are
// computed from collected fields in Go and attached after classification so a
// model cannot turn missing data into a performance judgment.
type AnalysisInput struct {
	PostID         string           `json:"post_id"`
	AuthorUsername string           `json:"author_username,omitempty"`
	URL            string           `json:"url,omitempty"`
	Text           string           `json:"text"`
	Mechanics      WritingMechanics `json:"mechanics"`
}

// AnalysisBatch preserves the exact provider response for every successfully
// parsed item. The store writes it alongside each cacheable classification.
type AnalysisBatch struct {
	Analyses    []PostAnalysis `json:"analyses"`
	RawResponse string         `json:"-"`
	Usage       AnalysisUsage  `json:"usage,omitempty"`
}

// AnalysisUsage is provider-reported token usage. It is observational only:
// the MVP deliberately does not hardcode vendor prices or calculate cost.
type AnalysisUsage struct {
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`
	TotalTokens  int `json:"total_tokens,omitempty"`
}

// AggregatePatternInput is available to providers that want to produce a
// narrative summary. The MVP report intentionally uses deterministic
// aggregation for counts, medians, quartiles, thresholds, and evidence.
type AggregatePatternInput struct {
	Topic           string           `json:"topic"`
	ClassifiedPosts []ClassifiedPost `json:"classified_posts"`
}

type AggregatePatternResult struct {
	Summary      string   `json:"summary,omitempty"`
	Observations []string `json:"observations,omitempty"`
	RawResponse  string   `json:"-"`
}

// PostAnalysis is the strict, versioned semantic payload returned by a model.
// Mechanics are attached deterministically after parsing and are not model
// output, even though they are included in the persisted payload.
type PostAnalysis struct {
	PostID                   string           `json:"post_id"`
	ContentType              string           `json:"content_type"`
	ContentTypeConfidence    float64          `json:"content_type_confidence"`
	SecondaryContentTypes    []string         `json:"secondary_content_types"`
	HookType                 string           `json:"hook_type"`
	HookConfidence           float64          `json:"hook_confidence"`
	Structure                string           `json:"structure"`
	StructureConfidence      float64          `json:"structure_confidence"`
	Tone                     []string         `json:"tone"`
	ToneConfidence           float64          `json:"tone_confidence"`
	Intent                   string           `json:"intent"`
	IntentConfidence         float64          `json:"intent_confidence"`
	MainTopic                string           `json:"main_topic"`
	MainTopicConfidence      float64          `json:"main_topic_confidence"`
	Subtopics                []string         `json:"subtopics"`
	Pains                    []string         `json:"pains"`
	Desires                  []string         `json:"desires"`
	Questions                []string         `json:"questions"`
	ToolsProductsServices    []string         `json:"tools_products_services"`
	TargetAudience           string           `json:"target_audience"`
	TargetAudienceConfidence float64          `json:"target_audience_confidence"`
	Claims                   []string         `json:"claims"`
	CTA                      string           `json:"cta"`
	Mechanics                WritingMechanics `json:"mechanics"`
}

// ClassifiedPost joins semantic output with source and deterministic
// performance context for evidence-linked reporting.
type ClassifiedPost struct {
	PostID         string             `json:"post_id"`
	AuthorUsername string             `json:"author_username,omitempty"`
	URL            string             `json:"url,omitempty"`
	Text           string             `json:"text,omitempty"`
	Analysis       PostAnalysis       `json:"analysis"`
	Performance    PerformanceContext `json:"performance"`
}

type ClassificationRecord struct {
	ID            int64              `json:"id,omitempty"`
	PostID        string             `json:"post_id"`
	Provider      string             `json:"provider"`
	Model         string             `json:"model"`
	SchemaVersion string             `json:"schema_version"`
	PromptVersion string             `json:"prompt_version"`
	ClassifiedAt  time.Time          `json:"classified_at"`
	Analysis      PostAnalysis       `json:"analysis"`
	Performance   PerformanceContext `json:"performance"`
	RawResponse   string             `json:"-"`
}

type PerformanceContext struct {
	Engagement          float64        `json:"engagement"`
	EngagementRate      *float64       `json:"engagement_rate,omitempty"`
	BaselineEngagement  *float64       `json:"baseline_engagement,omitempty"`
	RelativePerformance *float64       `json:"relative_performance,omitempty"`
	BaselinePostCount   int            `json:"baseline_post_count"`
	BaselineConfidence  string         `json:"baseline_confidence"`
	MetricCoverage      MetricCoverage `json:"metric_coverage"`
	ScoringCoverage     float64        `json:"scoring_coverage"`
}

// WritingMechanics is computed solely from the collected text. The rules are
// intentionally transparent and stable enough for regression tests.
type WritingMechanics struct {
	CharacterCount        int     `json:"character_count"`
	WordCount             int     `json:"word_count"`
	SentenceCount         int     `json:"sentence_count"`
	AverageSentenceLength float64 `json:"average_sentence_length"`
	LineCount             int     `json:"line_count"`
	LineBreakDensity      float64 `json:"line_break_density"`
	QuestionCount         int     `json:"question_count"`
	EmojiCount            int     `json:"emoji_count"`
	FirstPersonPresent    bool    `json:"first_person_present"`
	NumbersPresent        bool    `json:"numbers_present"`
	URLPresent            bool    `json:"url_present"`
}

type CollectionCoverage struct {
	SearchSurface             string            `json:"search_surface"`
	QueriesAttempted          int               `json:"queries_attempted"`
	QueriesSuccessful         int               `json:"queries_successful"`
	QueriesWithResults        int               `json:"queries_with_results"`
	RawPostsCollected         int               `json:"raw_posts_collected"`
	UniquePosts               int               `json:"unique_posts"`
	RelevantPosts             int               `json:"relevant_posts"`
	AdjacentPosts             int               `json:"adjacent_posts"`
	IrrelevantPosts           int               `json:"irrelevant_posts"`
	UncertainPosts            int               `json:"uncertain_posts"`
	UniqueAuthors             int               `json:"unique_authors"`
	RelevantAuthors           int               `json:"relevant_authors"`
	ProfilesAttempted         int               `json:"profiles_attempted"`
	ProfilesFetched           int               `json:"profiles_fetched"`
	AuthorsWithKnownFollowers int               `json:"authors_with_known_followers"`
	BaselineAuthorsAttempted  int               `json:"baseline_authors_attempted"`
	BaselineAuthorsReliable   int               `json:"baseline_authors_reliable"`
	BaselinePostsCollected    int               `json:"baseline_posts_collected"`
	Relevance                 RelevanceCoverage `json:"relevance"`
}

// RelevanceCoverage is calculated over unique search-discovered posts. Query
// precision is kept separately on each QueryReport because one post can be
// returned by several queries.
type RelevanceCoverage struct {
	Assessed   int     `json:"assessed"`
	Relevant   int     `json:"relevant"`
	Adjacent   int     `json:"adjacent"`
	Irrelevant int     `json:"irrelevant"`
	Uncertain  int     `json:"uncertain"`
	Precision  float64 `json:"precision"`
}

type AnalysisCoverage struct {
	PostsEligible             int     `json:"posts_eligible"`
	PostsClassified           int     `json:"posts_classified"`
	PostsWithReliableBaseline int     `json:"posts_with_reliable_baseline"`
	PostsWithPartialMetrics   int     `json:"posts_with_partial_metrics"`
	ClassificationPercent     float64 `json:"classification_percent"`
	ReliableBaselinePercent   float64 `json:"reliable_baseline_percent"`
}

type AnalysisPlan struct {
	Enabled              bool   `json:"enabled"`
	Provider             string `json:"provider,omitempty"`
	Model                string `json:"model,omitempty"`
	PostsRequested       int    `json:"posts_requested"`
	PostsEligible        int    `json:"posts_eligible"`
	PostsCached          int    `json:"posts_cached"`
	PostsToSend          int    `json:"posts_to_send"`
	InputCharacters      int    `json:"estimated_input_characters"`
	EstimatedInputTokens int    `json:"estimated_input_tokens"`
	ActualInputTokens    int    `json:"actual_input_tokens,omitempty"`
	ActualOutputTokens   int    `json:"actual_output_tokens,omitempty"`
	ActualTotalTokens    int    `json:"actual_total_tokens,omitempty"`
	BatchSize            int    `json:"batch_size"`
	BatchCount           int    `json:"batch_count"`
}

type AnalysisOptions struct {
	Enabled       bool
	Provider      AnalysisProvider
	ProviderError error
	PostsLimit    int
	BatchSize     int
	BeforeAnalyze func(AnalysisPlan)
}

type PatternStatistic struct {
	Dimension                 string   `json:"dimension"`
	Value                     string   `json:"value"`
	N                         int      `json:"n"`
	MedianRelativePerformance *float64 `json:"median_relative_performance,omitempty"`
	Q1RelativePerformance     *float64 `json:"q1_relative_performance,omitempty"`
	Q3RelativePerformance     *float64 `json:"q3_relative_performance,omitempty"`
	ReliableBaselinePercent   float64  `json:"reliable_baseline_percent"`
	SignalStrength            string   `json:"signal_strength"`
	EvidenceURLs              []string `json:"evidence_urls,omitempty"`
	ExamplePostIDs            []string `json:"example_post_ids,omitempty"`
}

type PainCluster struct {
	Label                     string   `json:"label"`
	Description               string   `json:"description"`
	Count                     int      `json:"count"`
	SignalStrength            string   `json:"signal_strength"`
	SourcePostIDs             []string `json:"source_post_ids,omitempty"`
	Authors                   []string `json:"authors,omitempty"`
	RepresentativeExamples    []string `json:"representative_examples,omitempty"`
	EvidenceURLs              []string `json:"evidence_urls,omitempty"`
	MedianRelativePerformance *float64 `json:"median_relative_performance,omitempty"`
}

type ContentOpportunity struct {
	Title                     string   `json:"title"`
	Rationale                 string   `json:"rationale"`
	EvidenceType              string   `json:"evidence_type"`
	EvidenceCount             int      `json:"evidence_count"`
	UniqueAuthors             int      `json:"unique_authors"`
	ExplicitQuestionCount     int      `json:"explicit_question_count"`
	MedianRelativePerformance *float64 `json:"median_relative_performance,omitempty"`
	SignalStrength            string   `json:"signal_strength"`
	SourcePostIDs             []string `json:"source_post_ids,omitempty"`
	EvidenceURLs              []string `json:"evidence_urls,omitempty"`
}

type ProductPainSignal struct {
	Category                  string   `json:"category"`
	Signal                    string   `json:"signal"`
	Count                     int      `json:"count"`
	SignalStrength            string   `json:"signal_strength"`
	UniqueAuthors             int      `json:"unique_authors"`
	ExplicitSolutionSeeking   int      `json:"explicit_solution_seeking_posts"`
	BuyerIntent               int      `json:"buyer_intent_posts"`
	ExistingToolComplaints    int      `json:"existing_tool_complaint_posts"`
	MedianEngagement          *float64 `json:"median_engagement,omitempty"`
	MedianRelativePerformance *float64 `json:"median_relative_performance,omitempty"`
	SourcePostIDs             []string `json:"source_post_ids,omitempty"`
	Authors                   []string `json:"authors,omitempty"`
	EvidenceURLs              []string `json:"evidence_urls,omitempty"`
}

type AnalysisReport struct {
	Enabled          bool                 `json:"enabled"`
	Plan             AnalysisPlan         `json:"plan"`
	Coverage         AnalysisCoverage     `json:"coverage"`
	Classifications  []ClassifiedPost     `json:"classifications,omitempty"`
	ContentTypes     []PatternStatistic   `json:"content_types,omitempty"`
	Hooks            []PatternStatistic   `json:"hooks,omitempty"`
	Structures       []PatternStatistic   `json:"structures,omitempty"`
	Tones            []PatternStatistic   `json:"tones,omitempty"`
	Intents          []PatternStatistic   `json:"intents,omitempty"`
	Topics           []PatternStatistic   `json:"topics,omitempty"`
	Combinations     []PatternStatistic   `json:"combinations,omitempty"`
	Pains            []PainCluster        `json:"pains,omitempty"`
	Questions        []PainCluster        `json:"questions,omitempty"`
	Opportunities    []ContentOpportunity `json:"opportunities,omitempty"`
	ProductPainRadar []ProductPainSignal  `json:"product_service_pain_radar,omitempty"`
	WeakSignals      []PatternStatistic   `json:"weak_signals,omitempty"`
	Warnings         []string             `json:"warnings,omitempty"`
}

// ValidatePostAnalysis enforces the schema after provider JSON decoding. It
// accepts UNKNOWN case-insensitively but normalizes the persisted value.
func ValidatePostAnalysis(value *PostAnalysis) error {
	if value == nil {
		return errors.New("analysis item is nil")
	}
	value.PostID = strings.TrimSpace(value.PostID)
	if value.PostID == "" {
		return errors.New("analysis post_id is required")
	}
	if err := validateEnum("content_type", &value.ContentType, contentTypeLabels); err != nil {
		return err
	}
	if err := validateConfidence("content_type_confidence", value.ContentTypeConfidence); err != nil {
		return err
	}
	if err := validateEnumList("secondary_content_types", &value.SecondaryContentTypes, contentTypeLabels, 4); err != nil {
		return err
	}
	if err := validateEnum("hook_type", &value.HookType, hookLabels); err != nil {
		return err
	}
	if err := validateConfidence("hook_confidence", value.HookConfidence); err != nil {
		return err
	}
	if err := validateEnum("structure", &value.Structure, structureLabels); err != nil {
		return err
	}
	if err := validateConfidence("structure_confidence", value.StructureConfidence); err != nil {
		return err
	}
	if len(value.Tone) == 0 {
		return errors.New("tone must contain at least one label")
	}
	if len(value.Tone) > 4 {
		return errors.New("tone contains too many labels")
	}
	for i := range value.Tone {
		if err := validateEnum(fmt.Sprintf("tone[%d]", i), &value.Tone[i], toneLabels); err != nil {
			return err
		}
	}
	if err := validateConfidence("tone_confidence", value.ToneConfidence); err != nil {
		return err
	}
	if err := validateEnum("intent", &value.Intent, intentLabels); err != nil {
		return err
	}
	if err := validateConfidence("intent_confidence", value.IntentConfidence); err != nil {
		return err
	}
	if err := validateTextField("main_topic", &value.MainTopic); err != nil {
		return err
	}
	if err := validateConfidence("main_topic_confidence", value.MainTopicConfidence); err != nil {
		return err
	}
	if err := validateList("subtopics", &value.Subtopics); err != nil {
		return err
	}
	if err := validateList("pains", &value.Pains); err != nil {
		return err
	}
	if err := validateList("desires", &value.Desires); err != nil {
		return err
	}
	if err := validateList("questions", &value.Questions); err != nil {
		return err
	}
	if err := validateList("tools_products_services", &value.ToolsProductsServices); err != nil {
		return err
	}
	if err := validateTextField("target_audience", &value.TargetAudience); err != nil {
		return err
	}
	if err := validateConfidence("target_audience_confidence", value.TargetAudienceConfidence); err != nil {
		return err
	}
	if err := validateList("claims", &value.Claims); err != nil {
		return err
	}
	if err := validateTextField("cta", &value.CTA); err != nil {
		return err
	}
	return nil
}

func validateEnum(field string, value *string, labels map[string]bool) error {
	*value = strings.ToLower(strings.TrimSpace(*value))
	if *value == "" {
		*value = UnknownLabel
	}
	if !labels[*value] {
		return fmt.Errorf("%s has unsupported value %q", field, *value)
	}
	return nil
}

func validateTextField(field string, value *string) error {
	*value = strings.TrimSpace(*value)
	if *value == "" {
		*value = UnknownLabel
	}
	if strings.EqualFold(*value, UnknownLabel) {
		*value = UnknownLabel
	}
	if len([]rune(*value)) > 300 {
		return fmt.Errorf("%s is too long", field)
	}
	return nil
}

func validateConfidence(field string, value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
		return fmt.Errorf("%s must be between 0 and 1", field)
	}
	return nil
}

func validateList(field string, values *[]string) error {
	if len(*values) > 20 {
		return fmt.Errorf("%s contains too many items", field)
	}
	out := (*values)[:0]
	seen := map[string]bool{}
	for _, value := range *values {
		value = strings.Join(strings.Fields(value), " ")
		if value == "" || strings.EqualFold(value, UnknownLabel) {
			continue
		}
		if len([]rune(value)) > 300 {
			return fmt.Errorf("%s item is too long", field)
		}
		key := strings.ToLower(value)
		if !seen[key] {
			seen[key] = true
			out = append(out, value)
		}
	}
	*values = out
	return nil
}

func validateEnumList(field string, values *[]string, labels map[string]bool, maxItems int) error {
	if len(*values) > maxItems {
		return fmt.Errorf("%s contains too many items", field)
	}
	out := (*values)[:0]
	seen := map[string]bool{}
	for _, value := range *values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || value == UnknownLabel {
			continue
		}
		if !labels[value] {
			return fmt.Errorf("%s has unsupported value %q", field, value)
		}
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	*values = out
	return nil
}

func validateAnalysisIDs(analyses []PostAnalysis, expected []AnalysisInput) error {
	expectedIDs := make(map[string]bool, len(expected))
	for _, item := range expected {
		if strings.TrimSpace(item.PostID) == "" {
			return errors.New("analysis input post_id is required")
		}
		expectedIDs[item.PostID] = true
	}
	seen := make(map[string]bool, len(analyses))
	for i := range analyses {
		if err := ValidatePostAnalysis(&analyses[i]); err != nil {
			return fmt.Errorf("analysis item %d: %w", i, err)
		}
		if !expectedIDs[analyses[i].PostID] {
			return fmt.Errorf("analysis returned unexpected post_id %q", analyses[i].PostID)
		}
		if seen[analyses[i].PostID] {
			return fmt.Errorf("analysis returned duplicate post_id %q", analyses[i].PostID)
		}
		seen[analyses[i].PostID] = true
	}
	return nil
}

var contentTypeLabels = labelSet(
	"educational", "personal_story", "opinion", "contrarian", "question", "case_study",
	"build_in_public", "pain_problem", "recommendation", "promotional", "humor_shitpost",
	"news_commentary", "other", UnknownLabel,
)

var hookLabels = labelSet(
	"strong_claim", "question", "personal_result", "number_statistic", "pain", "observation",
	"story_opening", "contrarian_statement", "curiosity", "direct_advice", "no_explicit_hook", UnknownLabel,
)

var structureLabels = labelSet(
	"one_liner", "hook_explanation", "hook_story_insight", "problem_solution", "list",
	"observation_opinion", "question_discussion", "result_explanation", "story_lesson", "claim_arguments", "other", UnknownLabel,
)

var toneLabels = labelSet(
	"conversational", "expert", "casual", "provocative", "vulnerable", "educational",
	"storytelling", "concise", "humorous", "sales_oriented", UnknownLabel,
)

var intentLabels = labelSet(
	"share_knowledge", "share_experience", "express_opinion", "start_discussion", "report_result",
	"describe_problem", "seek_solution", "recommend_solution", "sell", "build_in_public", "entertain", "other", UnknownLabel,
)

func labelSet(values ...string) map[string]bool {
	labels := make(map[string]bool, len(values))
	for _, value := range values {
		labels[value] = true
	}
	return labels
}

// signalStrength is shared by patterns, clusters, and opportunities. It is a
// sample-size label, never a claim about statistical significance.
func signalStrength(n int) string {
	switch {
	case n < 3:
		return "insufficient"
	case n <= 5:
		return "weak"
	case n <= 10:
		return "moderate"
	default:
		return "stronger"
	}
}

func analysisCoverage(eligible, classified int, posts []ClassifiedPost) AnalysisCoverage {
	coverage := AnalysisCoverage{PostsEligible: eligible, PostsClassified: classified}
	for _, post := range posts {
		if post.Performance.BaselineConfidence == baselineUsable && post.Performance.RelativePerformance != nil {
			coverage.PostsWithReliableBaseline++
		}
		if post.Performance.MetricCoverage.Known < post.Performance.MetricCoverage.Total {
			coverage.PostsWithPartialMetrics++
		}
	}
	if eligible > 0 {
		coverage.ClassificationPercent = float64(classified) * 100 / float64(eligible)
	}
	if classified > 0 {
		coverage.ReliableBaselinePercent = float64(coverage.PostsWithReliableBaseline) * 100 / float64(classified)
	}
	return coverage
}

func performanceContext(post RankedPost) PerformanceContext {
	return PerformanceContext{
		Engagement:          post.Engagement,
		EngagementRate:      cloneFloat(post.EngagementRate),
		BaselineEngagement:  cloneFloat(post.BaselineEngagement),
		RelativePerformance: cloneFloat(post.RelativePerformance),
		BaselinePostCount:   post.BaselinePostCount,
		BaselineConfidence:  post.BaselineConfidence,
		MetricCoverage:      post.MetricCoverage,
		ScoringCoverage:     post.ScoringCoverage,
	}
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func (c AnalysisCoverage) String() string {
	return fmt.Sprintf("%d/%d classified (%s)", c.PostsClassified, c.PostsEligible, formatPercent(c.ClassificationPercent))
}

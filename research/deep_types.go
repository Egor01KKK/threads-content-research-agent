package research

import "time"

// DeepAnchor is one short, deterministic Russian discovery term. Deep mode
// never sends the long research objective to Threads search.
type DeepAnchor struct {
	Query    string `json:"query"`
	Category string `json:"category"`
	Position int    `json:"position"`
}

// DeepDateCoverage keeps date coverage explicit. A missing date is not
// replaced with a guessed value.
type DeepDateCoverage struct {
	Earliest *time.Time     `json:"earliest,omitempty"`
	Latest   *time.Time     `json:"latest,omitempty"`
	ByWeek   map[string]int `json:"by_week,omitempty"`
	ByMonth  map[string]int `json:"by_month,omitempty"`
}

// DeepPaginationMetrics describe the observable page/yield envelope for one
// collection surface. The current public client exposes one SSR window and
// may attempt a hidden persisted-query continuation; those limitations are
// recorded in StopReason instead of being presented as complete history.
type DeepPaginationMetrics struct {
	Requests         int    `json:"requests"`
	PagesFetched     int    `json:"pages_fetched"`
	PostsPerPage     []int  `json:"posts_per_page,omitempty"`
	RawYield         int    `json:"raw_yield"`
	UniqueYield      int    `json:"unique_yield"`
	Duplicates       int    `json:"duplicates"`
	Exhausted        bool   `json:"exhausted"`
	StoppedAtCeiling bool   `json:"stopped_at_ceiling"`
	RateLimited      bool   `json:"rate_limited"`
	AccessDegraded   bool   `json:"access_degraded"`
	GraphQLAttempts  int    `json:"graphql_attempts"`
	GraphQLErrors    int    `json:"graphql_errors"`
	StopReason       string `json:"stop_reason,omitempty"`
}

// DeepAnchorProbe is the cheap first-pass measurement used to decide which
// authors are worth fetching. DiscoveryValue and PainPrecision are separate
// scores on purpose: a broad term can discover many authors without being a
// precise pain signal.
type DeepAnchorProbe struct {
	Anchor                   DeepAnchor            `json:"anchor"`
	Wave                     string                `json:"wave"`
	QueryReport              QueryReport           `json:"query_report"`
	ProbeLimit               int                   `json:"probe_limit"`
	UniquePosts              int                   `json:"unique_posts"`
	UniqueAuthors            int                   `json:"unique_authors"`
	RussianPosts             int                   `json:"russian_posts"`
	RussianRate              float64               `json:"russian_rate"`
	OwnerLikelyPosts         int                   `json:"owner_likely_posts"`
	OwnerLikelihoodRate      float64               `json:"owner_likelihood_rate"`
	PainPosts                int                   `json:"pain_posts"`
	PainRate                 float64               `json:"pain_rate"`
	ITActionablePosts        int                   `json:"it_actionable_posts"`
	ITActionabilityRate      float64               `json:"it_actionability_rate"`
	SolutionSeekingPosts     int                   `json:"solution_seeking_posts"`
	UniqueAuthorContribution int                   `json:"unique_author_contribution"`
	Duplicates               int                   `json:"duplicates"`
	DiscoveryValue           float64               `json:"discovery_value"`
	PainPrecision            float64               `json:"pain_precision"`
	DateCoverage             DeepDateCoverage      `json:"date_coverage"`
	Pagination               DeepPaginationMetrics `json:"pagination"`
	SelectedForProfileSeeds  bool                  `json:"selected_for_profile_seeds"`
	Status                   string                `json:"status"`
	Error                    string                `json:"error,omitempty"`
}

// DeepSeedProfile records both positive and negative deterministic owner
// evidence. It is retained for candidates as well as selected profiles so a
// later audit can see why a seed was or was not fetched.
type DeepSeedProfile struct {
	Username                     string   `json:"username"`
	SeedScore                    float64  `json:"seed_score"`
	OwnerLikelihood              string   `json:"owner_likelihood"`
	PositiveEvidence             []string `json:"positive_evidence,omitempty"`
	NegativeEvidence             []string `json:"negative_evidence,omitempty"`
	AnchorQueries                []string `json:"anchor_queries,omitempty"`
	SeedPostIDs                  []string `json:"seed_post_ids,omitempty"`
	SourceStages                 []string `json:"source_stages,omitempty"`
	SeedSelected                 bool     `json:"seed_selected"`
	Selected                     bool     `json:"selected"`
	VerificationStatus           string   `json:"verification_status,omitempty"`
	VerificationConfidence       string   `json:"verification_confidence,omitempty"`
	VerificationReasons          []string `json:"verification_reasons,omitempty"`
	VerificationNegativeEvidence []string `json:"verification_negative_evidence,omitempty"`
	VerifiedOwnerContext         bool     `json:"verified_owner_context"`
	BusinessCategories           []string `json:"business_categories,omitempty"`
	Status                       string   `json:"status"`
	Profile                      Author   `json:"profile"`
	ProfileFetched               bool     `json:"profile_fetched"`
	ProfilePosts                 int      `json:"profile_posts"`
	Error                        string   `json:"error,omitempty"`
}

// DeepProvenance is a lineage edge. A post can have multiple edges when
// several anchors or a profile expose the same public post.
type DeepProvenance struct {
	PostID          string    `json:"post_id"`
	SourceStage     string    `json:"source_stage"`
	AnchorQuery     string    `json:"anchor_query,omitempty"`
	AnchorCategory  string    `json:"anchor_category,omitempty"`
	SeedPostID      string    `json:"seed_post_id,omitempty"`
	ProfileUsername string    `json:"profile_username,omitempty"`
	RootPostID      string    `json:"root_post_id,omitempty"`
	ExtractedPhrase string    `json:"extracted_phrase,omitempty"`
	SecondaryAnchor string    `json:"secondary_anchor,omitempty"`
	Depth           int       `json:"depth"`
	ObservedAt      time.Time `json:"observed_at"`
}

// DeepPostRecord is the complete readable unit exported in corpus.jsonl. The
// original post text remains inside Post.Text; deterministic business-pain
// assessment and performance are adjacent, not replacements for the source.
type DeepPostRecord struct {
	Post          Post                   `json:"post"`
	Stages        []string               `json:"stages,omitempty"`
	SearchQueries []string               `json:"search_queries,omitempty"`
	Assessment    BusinessPainAssessment `json:"assessment"`
	Performance   *PerformanceContext    `json:"performance,omitempty"`
	Provenance    []DeepProvenance       `json:"provenance,omitempty"`
}

// DeepSecondaryAnchor captures the exact observed phrase and its eligibility
// evidence before a secondary search wave is attempted.
type DeepSecondaryAnchor struct {
	Phrase               string   `json:"phrase"`
	SourcePostIDs        []string `json:"source_post_ids,omitempty"`
	SourceAuthors        []string `json:"source_authors,omitempty"`
	SourceProfiles       []string `json:"source_profiles,omitempty"`
	OriginalAnchors      []string `json:"original_anchors,omitempty"`
	HighPainEvidence     int      `json:"high_pain_evidence"`
	ITActionableEvidence int      `json:"it_actionable_evidence"`
	Eligible             bool     `json:"eligible"`
	Reason               string   `json:"reason,omitempty"`
	Collected            bool     `json:"collected"`
	ResultCount          int      `json:"result_count"`
	UniqueContribution   int      `json:"unique_contribution"`
	Error                string   `json:"error,omitempty"`
}

// DeepReplySignal is intentionally separate from root-post evidence. Several
// replies under one root are not independent observations unless their
// authors are distinct.
type DeepReplySignal struct {
	RootPostID      string    `json:"root_post_id"`
	ReplyID         string    `json:"reply_id"`
	ReplyAuthor     string    `json:"reply_author,omitempty"`
	ReplyText       string    `json:"reply_text,omitempty"`
	ReplyURL        string    `json:"reply_url,omitempty"`
	ReplySignalType string    `json:"reply_signal_type"`
	ObservedAt      time.Time `json:"observed_at"`
}

// DeepFunnel is the run-level denominator ledger. It distinguishes raw
// collection volume from deduplicated owner/pain evidence.
type DeepFunnel struct {
	AnchorsGenerated        int               `json:"anchors_generated"`
	AnchorsProbed           int               `json:"anchors_probed"`
	AnchorsWithResults      int               `json:"anchors_with_results"`
	RawSearchPosts          int               `json:"raw_search_posts"`
	UniqueSearchPosts       int               `json:"unique_search_posts"`
	UniqueSearchAuthors     int               `json:"unique_search_authors"`
	OwnerSeedCandidates     int               `json:"owner_seed_candidates"`
	OwnerSeedProfiles       int               `json:"owner_seed_profiles"`
	SeedProfilesInspected   int               `json:"seed_profiles_inspected"`
	VerifiedOwnerProfiles   int               `json:"verified_owner_profiles"`
	ProfilesRejected        int               `json:"profiles_rejected"`
	ProfilesAttempted       int               `json:"profiles_attempted"`
	ProfilesFetched         int               `json:"profiles_fetched"`
	ProfilePostsRaw         int               `json:"profile_posts_raw"`
	ProfilePostsUnique      int               `json:"profile_posts_unique"`
	RepliesAttempted        int               `json:"replies_attempted"`
	RepliesCollected        int               `json:"replies_collected"`
	TotalUniquePosts        int               `json:"total_unique_posts"`
	RussianPosts            int               `json:"russian_posts"`
	OwnerLikelyPosts        int               `json:"owner_likely_posts"`
	PainPosts               int               `json:"pain_posts"`
	ITActionablePosts       int               `json:"it_actionable_posts"`
	SolutionSeekingPosts    int               `json:"solution_seeking_posts"`
	BuyerIntentPosts        int               `json:"buyer_intent_posts"`
	GoldSignals             int               `json:"gold_signals"`
	Requests                int               `json:"requests"`
	SearchRequests          int               `json:"search_requests"`
	ProfileRequests         int               `json:"profile_requests"`
	ProfilePostRequests     int               `json:"profile_post_requests"`
	ReplyRequests           int               `json:"reply_requests"`
	Failures                int               `json:"failures"`
	RateLimitEvents         int               `json:"rate_limit_events"`
	AccessDegradationEvents int               `json:"access_degradation_events"`
	StageElapsed            map[string]string `json:"stage_elapsed,omitempty"`
	DateCoverage            DeepDateCoverage  `json:"date_coverage"`
}

// DeepReport is the complete collection artifact embedded in the normal
// report and fanned out into the individual export files.
type DeepReport struct {
	Mode                   string                `json:"mode"`
	Objective              string                `json:"objective"`
	AnonymousCollection    bool                  `json:"anonymous_collection"`
	AnalysisProviderCalled bool                  `json:"analysis_provider_called"`
	AnchorBank             []DeepAnchor          `json:"anchor_bank"`
	AnchorProbes           []DeepAnchorProbe     `json:"anchor_probes"`
	SeedProfiles           []DeepSeedProfile     `json:"seed_profiles"`
	SecondaryAnchors       []DeepSecondaryAnchor `json:"secondary_anchors,omitempty"`
	Corpus                 []DeepPostRecord      `json:"corpus"`
	PainSignals            []DeepPostRecord      `json:"pain_signals,omitempty"`
	GoldSignals            []DeepPostRecord      `json:"gold_signals,omitempty"`
	Replies                []DeepReplySignal     `json:"replies,omitempty"`
	Provenance             []DeepProvenance      `json:"provenance"`
	Funnel                 DeepFunnel            `json:"funnel"`
	SearchPagination       DeepPaginationMetrics `json:"search_pagination"`
	ProfilePagination      DeepPaginationMetrics `json:"profile_pagination"`
	ReplyPagination        DeepPaginationMetrics `json:"reply_pagination"`
	StageStatus            map[string]string     `json:"stage_status"`
	Limitations            []string              `json:"limitations,omitempty"`
	Warnings               []string              `json:"warnings,omitempty"`
}

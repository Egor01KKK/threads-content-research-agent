package research

import "time"

// SemanticV2Version changes whenever the audited business-evidence contract
// changes. The v2 reclassifier is deliberately independent from the original
// deterministic pain fields so old exports remain comparable.
const SemanticV2Version = "business-evidence.v2"

// BusinessEvidenceClass is an evidence class, not a claim about market size.
// A post may carry more than one class; PrimaryEvidenceClass is only a
// deterministic display priority.
type BusinessEvidenceClass string

const (
	EvidenceExplicitBusinessPain  BusinessEvidenceClass = "EXPLICIT_BUSINESS_PAIN"
	EvidenceOperationalDemand     BusinessEvidenceClass = "OPERATIONAL_DEMAND_SIGNAL"
	EvidenceActiveSolutionSeeking BusinessEvidenceClass = "ACTIVE_SOLUTION_SEEKING"
	EvidenceWorkaround            BusinessEvidenceClass = "WORKAROUND_SIGNAL"
	EvidenceBusinessOpinion       BusinessEvidenceClass = "BUSINESS_OPINION"
	EvidenceServiceOffer          BusinessEvidenceClass = "SERVICE_OFFER"
	EvidenceNonTarget             BusinessEvidenceClass = "NON_TARGET"
	EvidenceUncertain             BusinessEvidenceClass = "UNCERTAIN"
)

type EvidenceStrength string

const (
	EvidenceSingleSource EvidenceStrength = "SINGLE_SOURCE"
	EvidenceWeakRepeat   EvidenceStrength = "WEAK_REPEAT"
	EvidenceRepeated     EvidenceStrength = "REPEATED"
	EvidenceStrongRepeat EvidenceStrength = "STRONG_REPEAT"
)

type BusinessConsequence string

const (
	ConsequenceLostLeads          BusinessConsequence = "LOST_LEADS"
	ConsequenceLostSales          BusinessConsequence = "LOST_SALES"
	ConsequenceLostClients        BusinessConsequence = "LOST_CLIENTS"
	ConsequenceTimeCost           BusinessConsequence = "TIME_COST"
	ConsequenceLaborCost          BusinessConsequence = "LABOR_COST"
	ConsequenceErrorRisk          BusinessConsequence = "ERROR_RISK"
	ConsequenceMissedFollowup     BusinessConsequence = "MISSED_FOLLOWUP"
	ConsequenceNoShows            BusinessConsequence = "NO_SHOWS"
	ConsequenceSlowResponse       BusinessConsequence = "SLOW_RESPONSE"
	ConsequencePoorConversion     BusinessConsequence = "POOR_CONVERSION"
	ConsequenceReportingBlindness BusinessConsequence = "REPORTING_BLINDNESS"
	ConsequenceProcessChaos       BusinessConsequence = "PROCESS_CHAOS"
	ConsequenceCustomerFriction   BusinessConsequence = "CUSTOMER_FRICTION"
	ConsequenceOther              BusinessConsequence = "OTHER"
	ConsequenceNone               BusinessConsequence = "NONE"
)

type HumanWorkflowLoad string

const (
	WorkflowLoadLow    HumanWorkflowLoad = "LOW"
	WorkflowLoadMedium HumanWorkflowLoad = "MEDIUM"
	WorkflowLoadHigh   HumanWorkflowLoad = "HIGH"
)

type ITOpportunityTier string

const (
	ITOpportunityLow    ITOpportunityTier = "LOW"
	ITOpportunityMedium ITOpportunityTier = "MEDIUM"
	ITOpportunityHigh   ITOpportunityTier = "HIGH"
)

type SemanticSignalTier string

const (
	SemanticTierInformational SemanticSignalTier = "INFORMATIONAL"
	SemanticTierWeak          SemanticSignalTier = "WEAK"
	SemanticTierStrong        SemanticSignalTier = "STRONG"
	SemanticTierGold          SemanticSignalTier = "GOLD"
)

// AuthorBusinessContext is an author-level summary. Profile evidence and post
// evidence are kept separate so a discussion of business is not silently
// treated as proof that the author operates one.
type AuthorBusinessContext struct {
	Username                   string   `json:"username"`
	Profile                    Author   `json:"profile"`
	OwnerLikelihood            string   `json:"owner_likelihood"`
	OperatorLikelihood         string   `json:"operator_likelihood"`
	BusinessType               string   `json:"business_type,omitempty"`
	BusinessNameIfExplicit     string   `json:"business_name_if_explicit,omitempty"`
	CommercialActivityEvidence []string `json:"commercial_activity_evidence,omitempty"`
	TeamEvidence               []string `json:"team_evidence,omitempty"`
	ClientEvidence             []string `json:"client_evidence,omitempty"`
	TransactionEvidence        []string `json:"transaction_evidence,omitempty"`
	ProfileEvidenceReasons     []string `json:"profile_evidence_reasons,omitempty"`
	PostEvidenceReasons        []string `json:"post_evidence_reasons,omitempty"`
	Confidence                 string   `json:"confidence"`
	VerifiedOwnerContext       bool     `json:"verified_owner_context"`
	ProfileFetched             bool     `json:"profile_fetched"`
}

// WorkflowEvidence describes only what the text supports. Empty fields mean
// that the source post did not expose that part of the workflow.
type WorkflowEvidence struct {
	WorkflowName        string   `json:"workflow_name,omitempty"`
	WorkflowTrigger     string   `json:"workflow_trigger,omitempty"`
	CurrentProcess      string   `json:"current_process,omitempty"`
	HumanRoles          []string `json:"human_roles,omitempty"`
	ToolsUsed           []string `json:"tools_used,omitempty"`
	ManualSteps         []string `json:"manual_steps,omitempty"`
	FailurePoint        string   `json:"failure_point,omitempty"`
	BusinessConsequence string   `json:"business_consequence,omitempty"`
	DesiredOutcome      string   `json:"desired_outcome,omitempty"`
}

// SemanticAssessmentV2 is the high-precision, auditable business evidence
// layer. It does not replace the original BusinessPainAssessment; both are
// exported for side-by-side validation.
type SemanticAssessmentV2 struct {
	EvidenceClasses                []BusinessEvidenceClass `json:"evidence_classes,omitempty"`
	PrimaryEvidenceClass           BusinessEvidenceClass   `json:"primary_evidence_class"`
	BusinessContextPresent         bool                    `json:"business_context_present"`
	BusinessContextPostEvidence    []string                `json:"business_context_post_evidence,omitempty"`
	BusinessContextProfileEvidence []string                `json:"business_context_profile_evidence,omitempty"`
	OwnerLikelihood                string                  `json:"owner_likelihood"`
	OperatorLikelihood             string                  `json:"operator_likelihood"`
	BusinessType                   string                  `json:"business_type,omitempty"`
	BusinessNameIfExplicit         string                  `json:"business_name_if_explicit,omitempty"`
	ExplicitBusinessPain           bool                    `json:"explicit_business_pain"`
	OperationalDemandSignal        bool                    `json:"operational_demand_signal"`
	ActiveSolutionSeeking          bool                    `json:"active_solution_seeking"`
	WorkaroundSignal               bool                    `json:"workaround_signal"`
	BusinessOpinion                bool                    `json:"business_opinion"`
	ServiceOffer                   bool                    `json:"service_offer"`
	NonTarget                      bool                    `json:"non_target"`
	Uncertain                      bool                    `json:"uncertain"`
	BusinessConsequence            BusinessConsequence     `json:"business_consequence"`
	ConsequenceExplicit            bool                    `json:"consequence_explicit"`
	HumanWorkflowLoad              HumanWorkflowLoad       `json:"human_workflow_load,omitempty"`
	AutomationHypothesis           string                  `json:"automation_hypothesis,omitempty"`
	Workflow                       WorkflowEvidence        `json:"workflow,omitempty"`
	ITOpportunityScore             int                     `json:"it_opportunity_score"`
	ITOpportunityTier              ITOpportunityTier       `json:"it_opportunity_tier"`
	GoldScore                      int                     `json:"gold_score"`
	SignalTier                     SemanticSignalTier      `json:"signal_tier"`
	Confidence                     string                  `json:"confidence"`
	Reasons                        []string                `json:"reasons,omitempty"`
}

// SemanticV2PostRecord keeps the source post and old deterministic fields
// intact while adding the reclassified evidence.
type SemanticV2PostRecord struct {
	Post               Post                   `json:"post"`
	Stages             []string               `json:"stages,omitempty"`
	SearchQueries      []string               `json:"search_queries,omitempty"`
	Provenance         []DeepProvenance       `json:"provenance,omitempty"`
	PreviousAssessment BusinessPainAssessment `json:"previous_assessment"`
	Performance        *PerformanceContext    `json:"performance,omitempty"`
	Assessment         SemanticAssessmentV2   `json:"assessment"`
}

type SemanticPerformanceSummary struct {
	PostsWithKnownBaseline int      `json:"posts_with_known_baseline"`
	PostsAboveBaseline     int      `json:"posts_above_baseline"`
	MedianRelative         *float64 `json:"median_relative_performance,omitempty"`
}

// WorkflowCluster aggregates by a specific workflow name. Post count and
// author count are intentionally separate, and verified-owner count is stricter
// than either one.
type WorkflowCluster struct {
	WorkflowName                string                     `json:"workflow_name"`
	EvidenceStrength            EvidenceStrength           `json:"evidence_strength"`
	Posts                       int                        `json:"posts"`
	UniqueAuthors               int                        `json:"unique_authors"`
	UniqueVerifiedOwnerContexts int                        `json:"unique_verified_owner_contexts"`
	BusinessTypes               []string                   `json:"business_types,omitempty"`
	DateCoverage                DeepDateCoverage           `json:"date_coverage"`
	ToolsMentioned              []string                   `json:"tools_mentioned,omitempty"`
	ManualSteps                 []string                   `json:"manual_steps,omitempty"`
	BusinessConsequences        []BusinessConsequence      `json:"business_consequences,omitempty"`
	PainPosts                   int                        `json:"pain_posts"`
	OperationalDemandPosts      int                        `json:"operational_demand_posts"`
	SolutionSeekingPosts        int                        `json:"solution_seeking_posts"`
	ITActionablePosts           int                        `json:"it_actionable_posts"`
	Performance                 SemanticPerformanceSummary `json:"performance"`
	EvidenceURLs                []string                   `json:"evidence_urls,omitempty"`
	ExamplePostIDs              []string                   `json:"example_post_ids,omitempty"`
}

type SemanticV2Summary struct {
	CorpusPosts                int            `json:"corpus_posts"`
	Authors                    int            `json:"authors"`
	AuthorsWithVerifiedContext int            `json:"authors_with_verified_context"`
	ExplicitBusinessPainPosts  int            `json:"explicit_business_pain_posts"`
	OperationalDemandPosts     int            `json:"operational_demand_posts"`
	ActiveSolutionSeekingPosts int            `json:"active_solution_seeking_posts"`
	WorkaroundPosts            int            `json:"workaround_posts"`
	StrongSignals              int            `json:"strong_signals"`
	GoldSignals                int            `json:"gold_signals"`
	PrimaryClassCounts         map[string]int `json:"primary_class_counts"`
	ClassCounts                map[string]int `json:"class_counts"`
	ITOpportunityCounts        map[string]int `json:"it_opportunity_counts"`
}

// SemanticV2Result is the complete reclassification pack. It is generated
// solely from an existing deep export and never performs network collection.
type SemanticV2Result struct {
	ExportVersion         string                  `json:"export_version"`
	GeneratedAt           time.Time               `json:"generated_at"`
	SourcePath            string                  `json:"source_path"`
	Topic                 string                  `json:"topic"`
	Corpus                []SemanticV2PostRecord  `json:"corpus"`
	AuthorBusinessContext []AuthorBusinessContext `json:"author_business_context"`
	VerifiedPains         []SemanticV2PostRecord  `json:"verified_pains"`
	OperationalWorkloads  []SemanticV2PostRecord  `json:"operational_workloads"`
	SolutionSeeking       []SemanticV2PostRecord  `json:"solution_seeking"`
	StrongSignals         []SemanticV2PostRecord  `json:"strong_signals"`
	GoldSignalsV2         []SemanticV2PostRecord  `json:"gold_signals_v2"`
	WorkflowClusters      []WorkflowCluster       `json:"workflow_clusters"`
	Summary               SemanticV2Summary       `json:"summary"`
	Limitations           []string                `json:"limitations,omitempty"`
}

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Egor01KKK/threads-content-research-agent/research"
	"github.com/Egor01KKK/threads-content-research-agent/threads"
	"github.com/spf13/cobra"
)

func newResearchCmd(a *App) *cobra.Command {
	defaults := research.DefaultConfig()
	var (
		dbPath               string
		mode                 string
		perQuery             int
		maxQueries           int
		profileLimit         int
		baselineAuthors      int
		baselinePosts        int
		minBaseline          int
		topPosts             int
		topAuthors           int
		likeWeight           float64
		replyWeight          float64
		repostWeight         float64
		quoteWeight          float64
		postEngage           float64
		postRate             float64
		postRelative         float64
		authorEngage         float64
		authorRate           float64
		authorRelative       float64
		authorVolume         float64
		analyze              bool
		analyzePosts         int
		analysisBatch        int
		analysisProvider     string
		analysisModel        string
		maxSeedProfiles      int
		maxVerifiedProfiles  int
		profilePosts         int
		maxTotalProfilePosts int
		profileDelay         time.Duration
		replyDelay           time.Duration
		resumeRunID          int64
		exportDir            string
	)
	cmd := &cobra.Command{
		Use:   "research <topic>",
		Short: "Research a Threads topic and rank posts and creators",
		Long: `Run a bounded topic-first research pipeline, or use --mode deep for
anonymous Russian anchor → profile → post discovery with resumable corpus export:

bounded mode expands a topic into deterministic intent-diverse queries, searches
public Threads posts, deduplicates them into SQLite, enriches a bounded author
set, fetches independent recent-post baselines, and renders an evidence-linked
ranking report. Add --analyze to classify a bounded set of unique posts in bounded
mode. Deep mode never invokes an LLM analysis provider.`,
		Args: minArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := research.Config{
				Topic:               strings.Join(args, " "),
				Mode:                mode,
				PerQueryLimit:       perQuery,
				MaxQueries:          maxQueries,
				ProfileLimit:        profileLimit,
				BaselineAuthorLimit: baselineAuthors,
				BaselinePostLimit:   baselinePosts,
				MinBaselinePosts:    minBaseline,
				TopPosts:            topPosts,
				TopAuthors:          topAuthors,
				Engagement: research.EngagementWeights{
					Likes: likeWeight, Replies: replyWeight, Reposts: repostWeight, Quotes: quoteWeight,
				},
				PostScore: research.PostScoreWeights{
					Engagement: postEngage, EngagementRate: postRate, Outperformance: postRelative,
				},
				AuthorScore: research.AuthorScoreWeights{
					Engagement: authorEngage, EngagementRate: authorRate,
					Outperformance: authorRelative, Volume: authorVolume,
				},
				MaxSeedProfiles:      maxSeedProfiles,
				MaxVerifiedProfiles:  maxVerifiedProfiles,
				ProfilePostLimit:     profilePosts,
				MaxTotalProfilePosts: maxTotalProfilePosts,
				ProfileDelay:         profileDelay,
				ReplyDelay:           replyDelay,
				ResumeRunID:          resumeRunID,
			}
			if cfg.Mode == research.ModeDeep && analyze {
				return threads.Usagef("--mode deep never invokes an LLM analysis provider; omit --analyze")
			}
			var analyzer research.AnalysisProvider
			var analysisSetupErr error
			if analyze {
				providerConfig, configErr := research.AnalysisProviderConfigFromEnv(analysisProvider, analysisModel)
				if configErr != nil {
					analysisSetupErr = configErr
				} else {
					analyzer, analysisSetupErr = research.NewAnalysisProvider(providerConfig, nil)
				}
			}
			store, err := research.OpenStore(dbPath)
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()

			a.progress("researching topic %q", cfg.Topic)
			researchClient := a.Client
			if cfg.Mode == research.ModeDeep {
				// Deep mode is deliberately anonymous even when credentials exist in
				// the shell or on global flags. It never touches the user's account.
				anonymousCfg := a.Cfg
				anonymousCfg.Session = ""
				anonymousCfg.CSRF = ""
				var clientErr error
				researchClient, clientErr = threads.NewClient(anonymousCfg)
				if clientErr != nil {
					return clientErr
				}
			}
			report, runErr := research.RunWithAnalysis(cmd.Context(), cfg, researchClient,
				research.DeterministicExpander{MaxQueries: cfg.MaxQueries}, store,
				research.AnalysisOptions{
					Enabled:       analyze,
					Provider:      analyzer,
					ProviderError: analysisSetupErr,
					PostsLimit:    analyzePosts,
					BatchSize:     analysisBatch,
					BeforeAnalyze: func(plan research.AnalysisPlan) {
						a.progress("analysis plan: eligible=%d cached=%d to_send=%d chars=%d est_tokens=%d batches=%d provider=%s model=%s",
							plan.PostsEligible, plan.PostsCached, plan.PostsToSend, plan.InputCharacters,
							plan.EstimatedInputTokens, plan.BatchCount, plan.Provider, plan.Model)
					},
				})
			if cfg.Mode == research.ModeDeep && report.Topic != "" {
				if exportDir == "" {
					exportDir = filepath.Join("research-output", fmt.Sprintf("deep-research-%d", report.RunID))
				}
				if err := research.ExportDeepArtifacts(report, exportDir, dbPath); err != nil {
					return err
				}
				a.progress("deep export written to %s", exportDir)
			}
			if report.Topic != "" {
				if err := renderResearchReport(a, report); err != nil {
					return err
				}
			}
			return runErr
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&dbPath, "db", "research.db", "research SQLite database path")
	flags.StringVar(&mode, "mode", research.ModeBounded, "research mode: bounded or deep")
	flags.IntVar(&perQuery, "per-query", defaults.PerQueryLimit, "maximum search results per expanded query")
	flags.IntVar(&maxQueries, "queries", defaults.MaxQueries, "maximum deterministic search queries")
	flags.IntVar(&profileLimit, "profile-limit", defaults.ProfileLimit, "maximum candidate author profiles to fetch")
	flags.IntVar(&baselineAuthors, "baseline-authors", defaults.BaselineAuthorLimit, "maximum authors to sample for independent baselines")
	flags.IntVar(&baselinePosts, "baseline-posts", defaults.BaselinePostLimit, "recent posts to fetch per baseline author")
	flags.IntVar(&minBaseline, "min-baseline-posts", defaults.MinBaselinePosts, "minimum usable baseline posts for reliable outperformance")
	flags.IntVar(&topPosts, "top-posts", defaults.TopPosts, "number of posts in each report ranking")
	flags.IntVar(&topAuthors, "top-authors", defaults.TopAuthors, "number of authors in the report ranking")
	flags.Float64Var(&likeWeight, "weight-likes", defaults.Engagement.Likes, "engagement weight for likes")
	flags.Float64Var(&replyWeight, "weight-replies", defaults.Engagement.Replies, "engagement weight for replies")
	flags.Float64Var(&repostWeight, "weight-reposts", defaults.Engagement.Reposts, "engagement weight for reposts")
	flags.Float64Var(&quoteWeight, "weight-quotes", defaults.Engagement.Quotes, "engagement weight for quotes")
	flags.Float64Var(&postEngage, "post-weight-engagement", defaults.PostScore.Engagement, "post score weight for log engagement")
	flags.Float64Var(&postRate, "post-weight-rate", defaults.PostScore.EngagementRate, "post score weight for log follower-relative rate")
	flags.Float64Var(&postRelative, "post-weight-outperformance", defaults.PostScore.Outperformance, "post score weight for log relative performance")
	flags.Float64Var(&authorEngage, "author-weight-engagement", defaults.AuthorScore.Engagement, "author score weight for log median engagement")
	flags.Float64Var(&authorRate, "author-weight-rate", defaults.AuthorScore.EngagementRate, "author score weight for log median follower-relative rate")
	flags.Float64Var(&authorRelative, "author-weight-outperformance", defaults.AuthorScore.Outperformance, "author score weight for log median relative performance")
	flags.Float64Var(&authorVolume, "author-weight-volume", defaults.AuthorScore.Volume, "author score weight for relevant post volume")
	flags.BoolVar(&analyze, "analyze", false, "classify unique topic posts with the configured LLM provider")
	flags.IntVar(&analyzePosts, "analyze-posts", 50, "maximum unique topic posts to classify")
	flags.IntVar(&analysisBatch, "analysis-batch-size", 10, "posts per LLM analysis request")
	flags.StringVar(&analysisProvider, "analysis-provider", "", "LLM provider: openai or anthropic (or THREADS_ANALYSIS_PROVIDER)")
	flags.StringVar(&analysisModel, "analysis-model", "", "LLM model (or provider-specific environment variable)")
	flags.IntVar(&maxSeedProfiles, "max-seed-profiles", defaults.MaxSeedProfiles, "deep mode: maximum candidate profiles to inspect before verification")
	flags.IntVar(&maxVerifiedProfiles, "max-verified-profiles", defaults.MaxVerifiedProfiles, "deep mode: maximum verified owner/operator profiles to crawl")
	flags.IntVar(&profilePosts, "profile-posts", defaults.ProfilePostLimit, "deep mode: recent posts per selected profile")
	flags.IntVar(&maxTotalProfilePosts, "max-total-profile-posts", defaults.MaxTotalProfilePosts, "deep mode: global profile-post ceiling")
	flags.DurationVar(&profileDelay, "profile-delay", defaults.ProfileDelay, "deep mode: delay between profile fetches")
	flags.DurationVar(&replyDelay, "reply-delay", defaults.ReplyDelay, "deep mode: delay between reply fetches")
	flags.Int64Var(&resumeRunID, "resume", 0, "deep mode: resume an incomplete SQLite run by id")
	flags.StringVar(&exportDir, "export-dir", "", "deep mode: directory for the complete corpus export")
	return cmd
}

func renderResearchReport(a *App, report research.Report) error {
	switch a.Out.Format() {
	case FormatJSON, FormatJSONL:
		if err := a.Out.Emit(Row{Value: report}); err != nil {
			return err
		}
		return a.Out.Flush()
	default:
		return report.WriteText(os.Stdout)
	}
}

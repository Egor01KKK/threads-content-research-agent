package cli

import (
	"fmt"
	"path/filepath"

	"github.com/Egor01KKK/threads-content-research-agent/research"
	"github.com/spf13/cobra"
)

func newSemanticAuditCmd() *cobra.Command {
	var outputDir string
	cmd := &cobra.Command{
		Use:   "semantic-audit <complete-readable.json>",
		Short: "Reclassify an existing deep export with the local business-evidence model",
		Long: `Read an existing complete-readable.json and write a separate semantic-v2
evidence pack. This command is local-only: it does not recollect Threads posts,
use credentials, or call an LLM analysis provider.`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := research.ReclassifyDeepCorpus(args[0])
			if err != nil {
				return err
			}
			if outputDir == "" {
				outputDir = filepath.Join(filepath.Dir(args[0]), "semantic-v2")
			}
			if err := research.ExportSemanticV2Artifacts(result, outputDir); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "semantic v2 audit written to %s (posts=%d authors=%d pains=%d workloads=%d solutions=%d strong=%d gold=%d)\n",
				outputDir, result.Summary.CorpusPosts, result.Summary.Authors,
				result.Summary.ExplicitBusinessPainPosts, result.Summary.OperationalDemandPosts,
				result.Summary.ActiveSolutionSeekingPosts, result.Summary.StrongSignals,
				result.Summary.GoldSignals)
			return err
		},
	}
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "directory for the separate semantic v2 evidence pack")
	return cmd
}

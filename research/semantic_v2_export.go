package research

import (
	"fmt"
	"os"
	"path/filepath"
)

// ExportSemanticV2Artifacts writes a separate v2 evidence pack. It never
// changes the original deep export, so the old deterministic classification
// remains available for before/after comparison.
func ExportSemanticV2Artifacts(result SemanticV2Result, outputDir string) error {
	if outputDir == "" {
		return fmt.Errorf("semantic v2 export directory is required")
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create semantic v2 export directory: %w", err)
	}
	artifacts := []struct {
		name  string
		value any
	}{
		{"semantic-v2-complete.json", result},
		{"verified-pains.json", result.VerifiedPains},
		{"operational-workloads.json", result.OperationalWorkloads},
		{"solution-seeking.json", result.SolutionSeeking},
		{"strong-signals.json", result.StrongSignals},
		{"gold-signals-v2.json", result.GoldSignalsV2},
		{"workflow-clusters.json", result.WorkflowClusters},
		{"author-business-context.json", result.AuthorBusinessContext},
		{"semantic-summary.json", result.Summary},
	}
	for _, artifact := range artifacts {
		if err := writeDeepJSON(filepath.Join(outputDir, artifact.name), artifact.value); err != nil {
			return err
		}
	}
	if err := writeSemanticAuditMarkdown(filepath.Join(outputDir, "semantic-audit.md"), result); err != nil {
		return err
	}
	return nil
}

func writeSemanticAuditMarkdown(path string, result SemanticV2Result) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	_, err = fmt.Fprintf(file, "# Semantic business-evidence audit v2\n\n"+
		"This file is a deterministic local reclassification of `%s`. It does not recollect posts and does not call an LLM analysis provider. The original post text and deterministic performance context remain in `semantic-v2-complete.json`.\n\n"+
		"- Topic: `%s`\n"+"- Corpus posts: %d\n"+"- Authors: %d\n"+"- Authors with verified owner/operator context: %d\n"+"- Explicit business-pain posts: %d\n"+"- Operational-demand posts: %d\n"+"- Active solution-seeking posts: %d\n"+"- Workaround posts: %d\n"+"- Strong signals: %d\n"+"- Gold signals: %d\n\n"+"The narrative interpretation, manual audit labels, evidence counts, source links, and final product verdict should be completed from the exported records before treating any pattern as a product or market conclusion.\n",
		result.SourcePath, result.Topic, result.Summary.CorpusPosts, result.Summary.Authors,
		result.Summary.AuthorsWithVerifiedContext, result.Summary.ExplicitBusinessPainPosts,
		result.Summary.OperationalDemandPosts, result.Summary.ActiveSolutionSeekingPosts,
		result.Summary.WorkaroundPosts, result.Summary.StrongSignals, result.Summary.GoldSignals)
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

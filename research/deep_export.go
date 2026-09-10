package research

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const deepExportVersion = "deep-corpus.v1"

// ExportDeepArtifacts fans one in-memory deep report out into files that can be
// inspected without the CLI or a provider. The source SQLite file is copied
// alongside them so the export remains a complete, replayable evidence pack.
func ExportDeepArtifacts(report Report, outputDir, dbPath string) error {
	if report.Deep == nil {
		return fmt.Errorf("deep artifacts require a deep report")
	}
	if outputDir == "" {
		return fmt.Errorf("deep export directory is required")
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create deep export directory: %w", err)
	}
	deep := report.Deep
	if err := writeDeepJSON(filepath.Join(outputDir, "complete-readable.json"), map[string]any{
		"export_version":           deepExportVersion,
		"generated_at":             time.Now().UTC(),
		"analysis_provider_called": false,
		"report":                   report,
		"anchor_bank":              deep.AnchorBank,
		"anchor_probes":            deep.AnchorProbes,
		"seed_profiles":            deep.SeedProfiles,
		"secondary_anchors":        deep.SecondaryAnchors,
		"corpus":                   deep.Corpus,
		"pain_signals":             deep.PainSignals,
		"gold_signals":             deep.GoldSignals,
		"replies":                  deep.Replies,
		"provenance":               deep.Provenance,
	}); err != nil {
		return err
	}
	if err := writeDeepJSONLines(filepath.Join(outputDir, "corpus.jsonl"), deep.Corpus); err != nil {
		return err
	}
	if err := writeDeepJSON(filepath.Join(outputDir, "owners.json"), deep.SeedProfiles); err != nil {
		return err
	}
	verifiedOwners := make([]DeepSeedProfile, 0)
	rejectedOwnerSeeds := make([]DeepSeedProfile, 0)
	categoryCounts := map[string]int{}
	for _, profile := range deep.SeedProfiles {
		if profile.VerifiedOwnerContext || profile.VerificationStatus == "accepted" || profile.VerificationStatus == "legacy_collected_without_profile_gate" {
			verifiedOwners = append(verifiedOwners, profile)
			for _, category := range profile.BusinessCategories {
				categoryCounts[category]++
			}
		}
		if profile.VerificationStatus == "rejected" {
			rejectedOwnerSeeds = append(rejectedOwnerSeeds, profile)
		}
	}
	if err := writeDeepJSON(filepath.Join(outputDir, "verified-owners.json"), verifiedOwners); err != nil {
		return err
	}
	if err := writeDeepJSON(filepath.Join(outputDir, "rejected-owner-seeds.json"), rejectedOwnerSeeds); err != nil {
		return err
	}
	if err := writeDeepJSON(filepath.Join(outputDir, "owner-discovery-summary.json"), map[string]any{
		"seed_candidates":          len(deep.SeedProfiles),
		"seed_profiles_selected":   deep.Funnel.OwnerSeedProfiles,
		"seed_profiles_inspected":  deep.Funnel.SeedProfilesInspected,
		"verified_owner_profiles":  deep.Funnel.VerifiedOwnerProfiles,
		"rejected_owner_profiles":  deep.Funnel.ProfilesRejected,
		"business_category_counts": categoryCounts,
	}); err != nil {
		return err
	}
	if err := writeDeepJSON(filepath.Join(outputDir, "pain-signals.json"), deep.PainSignals); err != nil {
		return err
	}
	if err := writeDeepJSON(filepath.Join(outputDir, "gold-signals.json"), deep.GoldSignals); err != nil {
		return err
	}
	if err := writeDeepJSON(filepath.Join(outputDir, "queries.json"), map[string]any{
		"anchor_bank":   deep.AnchorBank,
		"anchor_probes": deep.AnchorProbes,
		"query_reports": report.Queries,
	}); err != nil {
		return err
	}
	if err := writeDeepJSON(filepath.Join(outputDir, "run-summary.json"), map[string]any{
		"export_version":           deepExportVersion,
		"run_id":                   report.RunID,
		"topic":                    report.Topic,
		"objective":                deep.Objective,
		"mode":                     deep.Mode,
		"status":                   report.Status,
		"started_at":               report.StartedAt,
		"completed_at":             report.CompletedAt,
		"anonymous_collection":     deep.AnonymousCollection,
		"analysis_provider_called": false,
		"funnel":                   deep.Funnel,
		"collection_coverage":      report.Collection,
		"dataset_coverage":         report.Coverage,
		"baseline":                 report.Baseline,
		"search_pagination":        deep.SearchPagination,
		"profile_pagination":       deep.ProfilePagination,
		"reply_pagination":         deep.ReplyPagination,
		"stage_status":             deep.StageStatus,
		"warnings":                 report.Warnings,
		"limitations":              report.Limitations,
		"sqlite_source":            dbPath,
	}); err != nil {
		return err
	}
	if dbPath != "" {
		if err := copyDeepSQLite(dbPath, filepath.Join(outputDir, "research.db")); err != nil {
			return err
		}
	}
	return nil
}

func writeDeepJSON(path string, value any) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func writeDeepJSONLines(path string, values []DeepPostRecord) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	encoder := json.NewEncoder(file)
	encoder.SetEscapeHTML(false)
	for _, value := range values {
		if err := encoder.Encode(value); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}

func copyDeepSQLite(source, target string) error {
	sourceAbs, err := filepath.Abs(source)
	if err != nil {
		return fmt.Errorf("resolve SQLite source: %w", err)
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolve SQLite target: %w", err)
	}
	if sourceAbs == targetAbs {
		return nil
	}
	in, err := os.Open(sourceAbs)
	if err != nil {
		return fmt.Errorf("open SQLite source: %w", err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(targetAbs)
	if err != nil {
		return fmt.Errorf("create SQLite export: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("copy SQLite export: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close SQLite export: %w", err)
	}
	return nil
}

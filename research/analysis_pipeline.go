package research

import (
	"context"
	"errors"
	"fmt"
)

func analyzeRankedPosts(ctx context.Context, topic string, runID int64, rankedPosts []RankedPost, store *Store, options AnalysisOptions, report *Report) error {
	if report == nil {
		return errors.New("analysis report is required")
	}
	limit := options.PostsLimit
	if limit <= 0 || limit > len(rankedPosts) {
		limit = len(rankedPosts)
	}
	selected := append([]RankedPost(nil), rankedPosts[:limit]...)
	plan := AnalysisPlan{
		Enabled:        true,
		PostsRequested: options.PostsLimit,
		PostsEligible:  len(selected),
		BatchSize:      options.BatchSize,
	}
	if plan.PostsRequested <= 0 {
		plan.PostsRequested = len(selected)
	}
	if plan.BatchSize <= 0 {
		plan.BatchSize = 10
	}
	if options.Provider != nil {
		plan.Provider = options.Provider.Name()
		plan.Model = options.Provider.Model()
	}

	inputs := make([]AnalysisInput, 0, len(selected))
	postsByID := make(map[string]RankedPost, len(selected))
	for _, ranked := range selected {
		input := AnalysisInput{
			PostID:         ranked.Post.ID,
			AuthorUsername: ranked.Post.AuthorUsername,
			URL:            ranked.Post.URL,
			Text:           ranked.Post.Text,
			Mechanics:      CalculateMechanics(ranked.Post.Text),
		}
		inputs = append(inputs, input)
		postsByID[input.PostID] = ranked
	}

	cached := map[string]*ClassificationRecord{}
	pending := append([]AnalysisInput(nil), inputs...)
	var localWarnings []string
	if options.Provider != nil {
		pending = pending[:0]
		for _, input := range inputs {
			record, err := store.FindClassification(input.PostID, options.Provider.Name(), options.Provider.Model(), AnalysisSchemaVersion, AnalysisPromptVersion)
			if err != nil {
				localWarnings = append(localWarnings, fmt.Sprintf("cached classification for %s unavailable: %v", input.PostID, err))
				pending = append(pending, input)
				continue
			}
			if record == nil {
				pending = append(pending, input)
				continue
			}
			if err := ValidatePostAnalysis(&record.Analysis); err != nil {
				localWarnings = append(localWarnings, fmt.Sprintf("cached classification for %s is invalid: %v", input.PostID, err))
				pending = append(pending, input)
				continue
			}
			record.Analysis.Mechanics = input.Mechanics
			cached[input.PostID] = record
		}
	}
	plan.PostsCached = len(cached)
	plan.PostsToSend = len(pending)
	plan.BatchCount = ceilDiv(len(pending), plan.BatchSize)
	plan.InputCharacters = estimatedAnalysisInputCharacters(pending)
	plan.EstimatedInputTokens = (plan.InputCharacters + 3) / 4
	report.Analysis.Plan = plan
	if options.BeforeAnalyze != nil {
		options.BeforeAnalyze(plan)
	}

	classified := make([]ClassifiedPost, 0, len(selected))
	classifiedIDs := make(map[string]bool, len(selected))
	var actualUsage AnalysisUsage
	finalize := func() {
		if len(classified) < len(selected) {
			localWarnings = appendWarningOnce(localWarnings, fmt.Sprintf("analysis coverage is partial: %d/%d eligible posts classified", len(classified), len(selected)))
		}
		analysisReport := BuildAnalysisReport(topic, classified)
		plan.ActualInputTokens = actualUsage.InputTokens
		plan.ActualOutputTokens = actualUsage.OutputTokens
		plan.ActualTotalTokens = actualUsage.TotalTokens
		analysisReport.Plan = plan
		analysisReport.Coverage = analysisCoverage(len(selected), len(classified), classified)
		for _, warning := range localWarnings {
			analysisReport.Warnings = appendWarningOnce(analysisReport.Warnings, warning)
			report.Warnings = appendWarningOnce(report.Warnings, "content analysis: "+warning)
		}
		report.Analysis = analysisReport
	}
	for _, input := range inputs {
		if record := cached[input.PostID]; record != nil {
			classifiedIDs[input.PostID] = true
			classified = append(classified, ClassifiedPost{
				PostID:         input.PostID,
				AuthorUsername: input.AuthorUsername,
				URL:            input.URL,
				Text:           input.Text,
				Analysis:       record.Analysis,
				Performance:    performanceContext(postsByID[input.PostID]),
			})
		}
	}

	if options.ProviderError != nil {
		finalize()
		return options.ProviderError
	}
	if options.Provider == nil {
		finalize()
		return errors.New("analysis provider is not configured")
	}

	for start := 0; start < len(pending); start += plan.BatchSize {
		end := start + plan.BatchSize
		if end > len(pending) {
			end = len(pending)
		}
		batchInputs := pending[start:end]
		inputByID := make(map[string]AnalysisInput, len(batchInputs))
		for _, input := range batchInputs {
			inputByID[input.PostID] = input
		}
		batchNumber := start/plan.BatchSize + 1
		batch, err := options.Provider.AnalyzePosts(ctx, batchInputs)
		if err != nil {
			if abortErr := contextAbort(ctx, err); abortErr != nil {
				localWarnings = append(localWarnings, fmt.Sprintf("analysis batch %d/%d interrupted: %v", batchNumber, plan.BatchCount, err))
				finalize()
				return abortErr
			}
			localWarnings = append(localWarnings, fmt.Sprintf("analysis batch %d/%d failed: %v", batchNumber, plan.BatchCount, err))
			// A provider failure is usually shared by all remaining batches; stop
			// here to avoid repeated requests while keeping earlier results.
			break
		}
		actualUsage = addAnalysisUsage(actualUsage, batch.Usage)
		if len(batch.Analyses) == 0 {
			localWarnings = append(localWarnings, fmt.Sprintf("analysis batch %d/%d returned no classifications", batchNumber, plan.BatchCount))
			continue
		}
		returned := map[string]bool{}
		for _, analysis := range batch.Analyses {
			input, ok := inputByID[analysis.PostID]
			if !ok {
				localWarnings = append(localWarnings, fmt.Sprintf("analysis batch %d/%d returned unexpected post %s", batchNumber, plan.BatchCount, analysis.PostID))
				continue
			}
			analysis.Mechanics = input.Mechanics
			if err := ValidatePostAnalysis(&analysis); err != nil {
				localWarnings = append(localWarnings, fmt.Sprintf("analysis batch %d/%d returned invalid post %s: %v", batchNumber, plan.BatchCount, analysis.PostID, err))
				continue
			}
			if returned[analysis.PostID] {
				continue
			}
			if classifiedIDs[analysis.PostID] {
				localWarnings = append(localWarnings, fmt.Sprintf("analysis batch %d/%d repeated post %s", batchNumber, plan.BatchCount, analysis.PostID))
				continue
			}
			returned[analysis.PostID] = true
			if err := store.SaveClassification(runID, ClassificationRecord{
				PostID:        analysis.PostID,
				Provider:      options.Provider.Name(),
				Model:         options.Provider.Model(),
				SchemaVersion: AnalysisSchemaVersion,
				PromptVersion: AnalysisPromptVersion,
				Analysis:      analysis,
				Performance:   performanceContext(postsByID[analysis.PostID]),
				RawResponse:   batch.RawResponse,
			}); err != nil {
				return fmt.Errorf("store classification %s: %w", analysis.PostID, err)
			}
			classified = append(classified, ClassifiedPost{
				PostID:         analysis.PostID,
				AuthorUsername: input.AuthorUsername,
				URL:            input.URL,
				Text:           input.Text,
				Analysis:       analysis,
				Performance:    performanceContext(postsByID[analysis.PostID]),
			})
			classifiedIDs[analysis.PostID] = true
		}
		if len(returned) < end-start {
			localWarnings = append(localWarnings, fmt.Sprintf("analysis batch %d/%d was partial: %d/%d posts classified", batchNumber, plan.BatchCount, len(returned), end-start))
		}
	}

	finalize()
	return nil
}

func addAnalysisUsage(total, batch AnalysisUsage) AnalysisUsage {
	total.InputTokens += batch.InputTokens
	total.OutputTokens += batch.OutputTokens
	total.TotalTokens += batch.TotalTokens
	if total.TotalTokens == 0 {
		total.TotalTokens = total.InputTokens + total.OutputTokens
	}
	return total
}

func estimatedAnalysisInputCharacters(inputs []AnalysisInput) int {
	characters := 0
	for _, input := range inputs {
		characters += len([]rune(input.PostID)) + len([]rune(input.AuthorUsername)) + len([]rune(input.URL)) + len([]rune(input.Text)) + 120
	}
	return characters + 240 // system/prompt framing overhead
}

func ceilDiv(value, divisor int) int {
	if value <= 0 || divisor <= 0 {
		return 0
	}
	return (value + divisor - 1) / divisor
}

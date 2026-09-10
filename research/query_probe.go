package research

import "sort"

// russianQueryProbe is kept in-memory until the probe and optional deep pass
// have both completed. The underlying query row remains one row per candidate;
// the report fields make the two bounded collection phases auditable.
type russianQueryProbe struct {
	QueryID  int64
	Query    string
	Report   QueryReport
	ProbeErr error
	Position int
}

func precision(numerator, denominator int) float64 {
	if denominator <= 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

// selectRussianDeepProbes selects at most half of the candidate query budget
// for a second, deeper pass. Probe quality is intentionally transparent: IT
// actionability is weighted first, then concrete pain, owner/operator context,
// and finally probe precision. A query with no useful probe evidence is not
// expanded merely because it was generated.
func selectRussianDeepProbes(probes []russianQueryProbe, maxQueries int) []int {
	if len(probes) == 0 || maxQueries <= 0 {
		return nil
	}
	budget := maxQueries / 2
	if budget < 1 {
		budget = 1
	}
	if budget > len(probes) {
		budget = len(probes)
	}
	indices := make([]int, 0, len(probes))
	for index, probe := range probes {
		if probe.Report.ProbeResultCount == 0 {
			continue
		}
		if probe.Report.ProbeITActionablePosts == 0 && probe.Report.ProbePainPosts == 0 {
			continue
		}
		indices = append(indices, index)
	}
	sort.SliceStable(indices, func(left, right int) bool {
		leftProbe := probes[indices[left]].Report
		rightProbe := probes[indices[right]].Report
		leftScore := russianProbeScore(leftProbe)
		rightScore := russianProbeScore(rightProbe)
		if leftScore != rightScore {
			return leftScore > rightScore
		}
		return probes[indices[left]].Position < probes[indices[right]].Position
	})
	if len(indices) > budget {
		indices = indices[:budget]
	}
	return indices
}

func russianProbeScore(report QueryReport) float64 {
	return float64(report.ProbeITActionablePosts)*4 +
		float64(report.ProbePainPosts)*2 +
		float64(report.ProbeOwnerLikelyPosts) + report.ProbePrecision
}

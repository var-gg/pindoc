package preflight

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	OutcomeSectionMissing    = "OUTCOME_SECTION_MISSING"
	OutcomeFindingMissing    = "OUTCOME_FINDING_MISSING"
	OutcomeCommitMissing     = "OUTCOME_COMMIT_MISSING"
	OutcomeRegressionMissing = "OUTCOME_REGRESSION_MISSING"
)

var (
	outcomeCommitRe = regexp.MustCompile(`(?i)\b[0-9a-f]{7,64}\b`)
	outcomePRURLRe  = regexp.MustCompile(`https?://\S+/(?:pull|pulls|merge_requests)/\d+`)
)

type OutcomeCheckOptions struct {
	CommitRequired bool
}

type OutcomeCheckResult struct {
	Section   string
	Codes     []string
	Checklist []string
}

func (r OutcomeCheckResult) OK() bool {
	return len(r.Codes) == 0
}

func CheckOutcomeSection(body string, opts OutcomeCheckOptions) OutcomeCheckResult {
	section, ok := outcomeSection(body)
	out := OutcomeCheckResult{Section: section}
	if !ok {
		out.add(OutcomeSectionMissing, "Task body must contain an H2 `## Outcome` section before claim_done.")
		return out
	}
	if !outcomeHasFinding(section) {
		out.add(OutcomeFindingMissing, "Outcome must include at least one key finding/result line.")
	}
	if opts.CommitRequired && !outcomeHasCommitOrPR(section) {
		out.add(OutcomeCommitMissing, "Outcome for code/doc/config work must include a commit hash or PR URL.")
	}
	if !outcomeHasRegressionStatement(section) {
		out.add(OutcomeRegressionMissing, "Outcome must include a regression statement.")
	}
	return out
}

func (r *OutcomeCheckResult) add(code, line string) {
	r.Codes = append(r.Codes, code)
	r.Checklist = append(r.Checklist, "✗ "+line)
}

func OutcomeTemplateSuggestedActions() []string {
	return []string{
		"Append an H2 section before claim_done:",
		"## Outcome\n\n- 핵심 결과: <what changed or what was learned>.\n- 코드 변경: commit `<sha>` or PR URL `<url>`.\n- 회귀 진술: <tests/smoke/compatibility statement>.",
		"For policy/research/non-code tasks, set task_meta.outcome_commit_exempt=true or artifact_meta.outcome_commit_exempt=true and still include the finding/regression lines.",
	}
}

func ReverseEngineerOutcomePrompt(projectSlug, taskSlug string, codes []string) string {
	return fmt.Sprintf(
		"Read `%s/%s`, inspect its latest claim_done revision, pins, verification notes, and nearby commits; append an Outcome section with key result, commit/PR evidence when applicable, and regression statement. Missing checks: %s.",
		projectSlug,
		taskSlug,
		strings.Join(codes, ", "),
	)
}

func outcomeSection(body string) (string, bool) {
	var b strings.Builder
	inSection := false
	found := false
	for _, line := range strings.Split(body, "\n") {
		if after, ok := strings.CutPrefix(line, "## "); ok {
			if found && inSection {
				return strings.TrimSpace(b.String()), true
			}
			inSection = outcomeHeadingMatches(after)
			found = inSection
			b.Reset()
			continue
		}
		if inSection {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	if found {
		return strings.TrimSpace(b.String()), true
	}
	return "", false
}

func outcomeHeadingMatches(heading string) bool {
	normalized := strings.ToLower(strings.TrimSpace(heading))
	for _, token := range []string{"outcome", "결과", "완료 결과", "산출"} {
		if normalized == strings.ToLower(token) || strings.Contains(normalized, strings.ToLower(token)) {
			return true
		}
	}
	return false
}

func outcomeHasFinding(section string) bool {
	for _, line := range nonEmptyOutcomeLines(section) {
		lower := strings.ToLower(line)
		for _, marker := range []string{"핵심", "결과", "발견", "구현", "완료", "요약", "result", "finding", "summary", "implemented"} {
			if strings.Contains(lower, strings.ToLower(marker)) {
				return true
			}
		}
	}
	return false
}

func outcomeHasCommitOrPR(section string) bool {
	return outcomeCommitRe.MatchString(section) || outcomePRURLRe.MatchString(section)
}

func outcomeHasRegressionStatement(section string) bool {
	for _, line := range nonEmptyOutcomeLines(section) {
		lower := strings.ToLower(line)
		for _, marker := range []string{"회귀", "regression", "regress", "호환", "compat", "기존", "no regression"} {
			if strings.Contains(lower, strings.ToLower(marker)) {
				return true
			}
		}
	}
	return false
}

func nonEmptyOutcomeLines(section string) []string {
	var out []string
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

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
	// Section is the body of the section the gate verdict is based on:
	// the first satisfying section when OK(), otherwise the
	// fewest-missing section whose Codes are reported.
	Section   string
	Codes     []string
	Checklist []string

	// InspectedHeadings lists, in document order, the raw heading text of
	// every Outcome-like H2 section that was evaluated. Empty when no
	// Outcome section exists. Diagnostic-only: lets a closeout worker see
	// which heading(s) the gate actually read instead of guessing.
	InspectedHeadings []string
	// DuplicateOutcomeSections is true when more than one Outcome-like H2
	// section was found. Multiple sections are evaluated independently and
	// the gate passes if ANY single one self-satisfies, but duplicates are
	// worth flagging so authors consolidate into one `## Outcome`.
	DuplicateOutcomeSections bool
}

func (r OutcomeCheckResult) OK() bool {
	return len(r.Codes) == 0
}

// CheckOutcomeSection evaluates EVERY Outcome-like H2 section in the body
// independently and passes when ANY single section self-contains all
// required checks (finding, commit/PR when CommitRequired, regression).
// Evidence is never merged across sections — each section must satisfy the
// bar on its own — so the gate is identical to the canonical single-section
// contract; the only behaviour removed is the previous dependence on
// heading order, which could spuriously fail a Task whose compliant
// `## Outcome` was preceded by a stale `## Outcome (결과)`.
//
// When no section satisfies the bar, the fewest-missing section's codes are
// reported (ties broken by document order) so the checklist points at the
// closest-to-complete section deterministically.
func CheckOutcomeSection(body string, opts OutcomeCheckOptions) OutcomeCheckResult {
	hits := outcomeSections(body)
	out := OutcomeCheckResult{DuplicateOutcomeSections: len(hits) > 1}
	for _, h := range hits {
		out.InspectedHeadings = append(out.InspectedHeadings, h.heading)
	}
	if len(hits) == 0 {
		out.add(OutcomeSectionMissing, "Task body must contain an H2 `## Outcome` section before claim_done.")
		return out
	}

	var best *OutcomeCheckResult
	for i := range hits {
		codes, checklist := outcomeSectionChecks(hits[i].body, opts)
		if len(codes) == 0 {
			// A single section satisfies every required check → pass.
			out.Section = hits[i].body
			return out
		}
		// Strict `<` keeps the FIRST minimum (document-order tie-break).
		if best == nil || len(codes) < len(best.Codes) {
			candidate := OutcomeCheckResult{Section: hits[i].body, Codes: codes, Checklist: checklist}
			best = &candidate
		}
	}
	out.Section = best.Section
	out.Codes = best.Codes
	out.Checklist = best.Checklist
	return out
}

// outcomeSectionChecks runs the three evidence detectors against a single
// section body and returns the missing-check codes in the stable order
// finding → commit → regression, so OutcomeCheckResult.Codes[0] (and the
// derived top-level error_code) is deterministic.
func outcomeSectionChecks(section string, opts OutcomeCheckOptions) ([]string, []string) {
	var codes, checklist []string
	add := func(code, line string) {
		codes = append(codes, code)
		checklist = append(checklist, "✗ "+line)
	}
	if !outcomeHasFinding(section) {
		add(OutcomeFindingMissing, "Outcome must include at least one key finding/result line.")
	}
	if opts.CommitRequired && !outcomeHasCommitOrPR(section) {
		add(OutcomeCommitMissing, "Outcome for code/doc/config work must include a commit hash or PR URL.")
	}
	if !outcomeHasRegressionStatement(section) {
		add(OutcomeRegressionMissing, "Outcome must include a regression statement.")
	}
	return codes, checklist
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

type outcomeSectionHit struct {
	heading string
	body    string
}

// outcomeSections collects EVERY Outcome-like H2 section in document order.
// A section runs from its `## <outcome-like>` heading up to the next `## `
// heading (or end of body). Non-Outcome `## ` headings bound a section but
// are not collected. This replaces the previous first-match-only scan that
// returned only the leading Outcome section and ignored the rest.
func outcomeSections(body string) []outcomeSectionHit {
	var hits []outcomeSectionHit
	var b strings.Builder
	inSection := false
	heading := ""
	flush := func() {
		if inSection {
			hits = append(hits, outcomeSectionHit{heading: heading, body: strings.TrimSpace(b.String())})
		}
		b.Reset()
		inSection = false
		heading = ""
	}
	for _, line := range strings.Split(body, "\n") {
		if after, ok := strings.CutPrefix(line, "## "); ok {
			flush()
			if outcomeHeadingMatches(after) {
				inSection = true
				heading = strings.TrimSpace(after)
			}
			continue
		}
		if inSection {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	flush()
	return hits
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

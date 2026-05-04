package tools

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// TestParseValidatorHints covers the four shapes the preflight layer
// expects from `<!-- validator: ... -->` comments on _template_*
// bodies (Task preflight-template-drift-통합).
func TestParseValidatorHints(t *testing.T) {
	cases := []struct {
		name string
		body string
		want *validatorHints
	}{
		{
			name: "both axes populated",
			body: "<!-- validator: required_h2=Purpose,Scope,Acceptance criteria; required_keywords=acceptance -->\n" +
				"## Purpose\nbody\n",
			want: &validatorHints{
				RequiredH2:       []string{"Purpose", "Scope", "Acceptance criteria"},
				RequiredKeywords: []string{"acceptance"},
			},
		},
		{
			name: "only h2 axis",
			body: "<!-- validator: required_h2=TL;DR -->\n...",
			// TL;DR splits at semicolon in our grammar (semicolons separate
			// axes). Callers aware of this use `TL` as the canonical slot
			// — so we assert the parser's literal behaviour here to lock
			// the contract.
			want: &validatorHints{
				RequiredH2: []string{"TL"},
			},
		},
		{
			name: "only keywords axis",
			body: "<!-- validator: required_keywords=repro,증상,resolution,해결 -->",
			want: &validatorHints{
				RequiredKeywords: []string{"repro", "증상", "resolution", "해결"},
			},
		},
		{
			name: "no comment returns nil",
			body: "## just body\nno hints here.\n",
			want: nil,
		},
		{
			name: "empty axis values skipped",
			body: "<!-- validator: required_h2=; required_keywords=acceptance -->",
			want: &validatorHints{
				RequiredKeywords: []string{"acceptance"},
			},
		},
		{
			name: "whitespace around tokens trimmed",
			body: "<!-- validator: required_h2=  Purpose ,  Scope  -->",
			want: &validatorHints{
				RequiredH2: []string{"Purpose", "Scope"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseValidatorHints(tc.body)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseValidatorHints mismatch\n  got:  %+v\n  want: %+v", got, tc.want)
			}
		})
	}
}

// TestRequiredH2WarningsSlashMixed locks the fuzzy-heading upgrade:
// "## 목적 / Purpose" should satisfy a Purpose slot even though the raw
// heading text is neither "Purpose" nor "목적" alone.
func TestRequiredH2WarningsSlashMixed(t *testing.T) {
	body := "## 목적 / Purpose\n\n## 범위 / Scope\n\n- [ ] acceptance criteria stub\n"
	// Task type expects Purpose / Scope / Acceptance criteria slots.
	warns := requiredH2Warnings(body, "Task")
	for _, w := range warns {
		if w == "MISSING_H2:Purpose" || w == "MISSING_H2:Scope" {
			t.Fatalf("slash-mixed heading should satisfy slot, got %v", warns)
		}
	}
}

func TestRequiredH2WarningsParentheticalBilingual(t *testing.T) {
	body := "## 증상 (Symptom)\n\n## 재현 (Reproduction)\n\n## 가설 (Hypotheses tried)\n\n## 원인 (Root cause)\n\n## 해결 (Resolution)\n\n## 검증 (Verification)\n"
	if warns := requiredH2Warnings(body, "Debug"); len(warns) != 0 {
		t.Fatalf("parenthetical bilingual headings should satisfy Debug slots, got %v", warns)
	}
}

func TestRequiredH2WarningsStandaloneKoreanEnglishAndMixed(t *testing.T) {
	cases := []struct {
		name string
		typ  string
		body string
	}{
		{
			name: "decision korean only",
			typ:  "Decision",
			body: "## 요약\n\n결론.\n\n## 맥락\n\n배경.\n\n## 결정\n\n결정.\n\n## 근거\n\n근거.\n\n## 대안\n\n대안.\n\n## 결과\n\n결과.\n",
		},
		{
			name: "decision english only",
			typ:  "Decision",
			body: "## TL;DR\n\nDecision.\n\n## Context\n\nContext.\n\n## Decision\n\nDecision.\n\n## Rationale\n\nRationale.\n\n## Alternatives considered\n\nAlternatives.\n\n## Consequences\n\nConsequences.\n",
		},
		{
			name: "debug korean only",
			typ:  "Debug",
			body: "## 증상\n\n증상.\n\n## 재현\n\n재현.\n\n## 가설\n\n가설.\n\n## 원인\n\n원인.\n\n## 해결\n\n해결.\n\n## 검증\n\n검증.\n",
		},
		{
			name: "task mixed",
			typ:  "Task",
			body: "## 목적\n\n목적.\n\n## Scope\n\nScope.\n\n## 코드 좌표\n\n`internal/pindoc/mcp/tools/artifact_propose.go`\n\n## Acceptance criteria\n\n- [ ] acceptance exists.\n\n## 검증\n\nTests pass.\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if warns := requiredH2Warnings(tc.body, tc.typ); len(warns) != 0 {
				t.Fatalf("%s standalone headings should satisfy %s slots, got %v", tc.name, tc.typ, warns)
			}
		})
	}
}

func TestRequiredH2SlotsMergeStaleTemplateHintsWithDefaults(t *testing.T) {
	slots := requiredH2SlotsFromHints("Task", &validatorHints{
		RequiredH2: []string{"목적", "범위", "TODO"},
	})
	for _, want := range []string{"Purpose", "Scope", "코드 좌표", "TODO", "TC / DoD"} {
		if !requiredH2SlotLabelsContain(slots, want) {
			t.Fatalf("merged Task slots missing %q: %+v", want, slots)
		}
	}
}

func TestDecisionPreflightRequiresTLDRAndCapsLines(t *testing.T) {
	missingTLDR := artifactProposeInput{
		Type: "Decision", Title: "decision without summary", AreaSlug: "mcp", AuthorID: "test-agent",
		BodyMarkdown: "## Context\nContext.\n\n## Decision\nDecision.\n\n## Rationale\nBecause.\n\n## Alternatives considered\nA.\n\n## Consequences\nImpact.",
	}
	_, failed, _ := preflight(context.Background(), Deps{}, "", &missingTLDR, "en")
	if !containsString(failed, "MISSING_H2:TL;DR") {
		t.Fatalf("Decision without TL;DR should fail H2 validation: %v", failed)
	}

	tooLong := artifactProposeInput{
		Type: "Decision", Title: "decision long summary", AreaSlug: "mcp", AuthorID: "test-agent",
		BodyMarkdown: "## TL;DR\nLine one.\nLine two.\nLine three.\n\n## Context\nContext.\n\n## Decision\nDecision.\n\n## Rationale\nBecause.\n\n## Alternatives considered\nA.\n\n## Consequences\nImpact.",
	}
	_, failed, _ = preflight(context.Background(), Deps{}, "", &tooLong, "en")
	if !containsString(failed, "TLDR_LINE_CAP") {
		t.Fatalf("Decision TL;DR with 3 non-empty lines should fail: %v", failed)
	}
}

func TestTaskPreflightRequiresCodeCoordinatesAndTCDOD(t *testing.T) {
	valid := artifactProposeInput{
		Type: "Task", Title: "task with coordinates", AreaSlug: "mcp", AuthorID: "test-agent",
		BodyMarkdown: taskBodyWithCodeCoordinate("`internal/pindoc/mcp/tools/artifact_propose.go` and package internal/pindoc/mcp/tools."),
	}
	_, failed, _ := preflight(context.Background(), Deps{}, "", &valid, "en")
	if hasCodePrefix(failed, "MISSING_H2:") || containsString(failed, "TASK_CODE_COORDINATE_MISSING") {
		t.Fatalf("Task with canonical sections and code coordinate should pass structure gates: %v", failed)
	}

	emptyCoordinate := valid
	emptyCoordinate.BodyMarkdown = taskBodyWithCodeCoordinate("   ")
	_, failed, _ = preflight(context.Background(), Deps{}, "", &emptyCoordinate, "en")
	if !containsString(failed, "TASK_CODE_COORDINATE_MISSING") {
		t.Fatalf("Task with empty code coordinate section should fail: %v", failed)
	}

	missingTCDOD := valid
	missingTCDOD.BodyMarkdown = strings.Replace(valid.BodyMarkdown, "\n## TC / DoD\n\nAutomatic tests and DoD.", "", 1)
	_, failed, _ = preflight(context.Background(), Deps{}, "", &missingTCDOD, "en")
	if !containsString(failed, "MISSING_H2:TC / DoD") {
		t.Fatalf("Task without TC / DoD should fail H2 validation: %v", failed)
	}
}

func TestTaskCodeCoordinateExemptionsAreExplicitMeta(t *testing.T) {
	body := taskBodyWithCodeCoordinate("")
	cases := []struct {
		name string
		in   artifactProposeInput
	}{
		{
			name: "task meta",
			in: artifactProposeInput{
				Type: "Task", Title: "policy task", AreaSlug: "mcp", AuthorID: "test-agent",
				BodyMarkdown: body,
				TaskMeta:     &TaskMetaInput{CodeCoordinateExempt: true},
			},
		},
		{
			name: "artifact meta",
			in: artifactProposeInput{
				Type: "Task", Title: "vision task", AreaSlug: "mcp", AuthorID: "test-agent",
				BodyMarkdown: body,
				ArtifactMeta: &ArtifactMetaInput{CodeCoordinateExempt: true},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, failed, _ := preflight(context.Background(), Deps{}, "", &tc.in, "en")
			if containsString(failed, "TASK_CODE_COORDINATE_MISSING") {
				t.Fatalf("code coordinate exemption should suppress coordinate gate: %v", failed)
			}
		})
	}
}

func TestTaskAcceptanceVerbLintBlocksForbiddenVerbs(t *testing.T) {
	body := taskBodyWithCodeCoordinate("`internal/pindoc/mcp/tools/artifact_propose.go`")
	body = strings.Replace(body, "- [ ] acceptance criterion is explicit.", "- [ ] 기존 Task를 확인한다.", 1)
	in := artifactProposeInput{
		Type: "Task", Title: "verb lint task", AreaSlug: "mcp", AuthorID: "test-agent",
		BodyMarkdown: body,
	}
	_, failed, _ := preflight(context.Background(), Deps{}, "", &in, "ko")
	if !containsString(failed, "TASK_ACCEPTANCE_FORBIDDEN_VERB") {
		t.Fatalf("forbidden acceptance verb should fail preflight: %v", failed)
	}
	actions := suggestedActionsForNotReady("ko", "Task", failed, nil)
	if !containsSubstring(actions, "Example:") {
		t.Fatalf("forbidden verb lint should include rewrite examples: %v", actions)
	}
}

func TestTaskAcceptanceVerbLintExemptionsAreExplicitMeta(t *testing.T) {
	body := taskBodyWithCodeCoordinate("`internal/pindoc/mcp/tools/artifact_propose.go`")
	body = strings.Replace(body, "- [ ] acceptance criterion is explicit.", "- [ ] 기존 Task를 확인합니다.", 1)
	cases := []struct {
		name string
		in   artifactProposeInput
	}{
		{
			name: "task meta",
			in: artifactProposeInput{
				Type: "Task", Title: "legacy task", AreaSlug: "mcp", AuthorID: "test-agent",
				BodyMarkdown: body,
				TaskMeta:     &TaskMetaInput{AcceptanceVerbLintExempt: true},
			},
		},
		{
			name: "artifact meta",
			in: artifactProposeInput{
				Type: "Task", Title: "policy task", AreaSlug: "mcp", AuthorID: "test-agent",
				BodyMarkdown: body,
				ArtifactMeta: &ArtifactMetaInput{AcceptanceVerbLintExempt: true},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, failed, _ := preflight(context.Background(), Deps{}, "", &tc.in, "ko")
			if containsString(failed, "TASK_ACCEPTANCE_FORBIDDEN_VERB") {
				t.Fatalf("explicit verb lint exemption should suppress gate: %v", failed)
			}
		})
	}
}

func TestBodyPatchUpdateSkipsStructurePreflightUntilMaterialized(t *testing.T) {
	version := 1
	in := artifactProposeInput{
		Type:            "Task",
		Title:           "patch update",
		AreaSlug:        "mcp",
		AuthorID:        "test-agent",
		UpdateOf:        "task-propose-section-preflight",
		ExpectedVersion: &version,
		BodyPatch: &BodyPatchInput{
			Mode:       "append",
			AppendText: "## Outcome\n\nPatch-only update.",
		},
	}
	_, failed, _ := preflight(context.Background(), Deps{}, "", &in, "en")
	for _, unexpected := range []string{"TASK_NO_ACCEPTANCE", "TASK_CODE_COORDINATE_MISSING"} {
		if containsString(failed, unexpected) {
			t.Fatalf("body_patch preflight should not run full-body structure gate %s: %v", unexpected, failed)
		}
	}
	if hasCodePrefix(failed, "MISSING_H2:") {
		t.Fatalf("body_patch preflight should not emit missing H2 before materialization: %v", failed)
	}
}

func TestDebugPreflightRequiresHypothesesAndVerification(t *testing.T) {
	in := artifactProposeInput{
		Type: "Debug", Title: "debug missing new slots", AreaSlug: "mcp", AuthorID: "test-agent",
		BodyMarkdown: "## 증상 (Symptom)\nSymptom.\n\n## 재현 (Reproduction)\nRepro.\n\n## 원인 (Root cause)\nCause.\n\n## 해결 (Resolution)\nResolution.",
	}
	_, failed, _ := preflight(context.Background(), Deps{}, "", &in, "en")
	for _, want := range []string{"MISSING_H2:Hypotheses tried", "MISSING_H2:Verification"} {
		if !containsString(failed, want) {
			t.Fatalf("Debug missing new H2 slot %s; got %v", want, failed)
		}
	}
}

func TestTemplateSelfHealHintsForStructureFailures(t *testing.T) {
	cases := []struct {
		typ      string
		body     string
		wantSlug string
	}{
		{
			typ:      "Decision",
			body:     "decision context keywords only",
			wantSlug: "_template_decision",
		},
		{
			typ:      "Task",
			body:     "- [ ] acceptance criterion",
			wantSlug: "_template_task",
		},
		{
			typ:      "Analysis",
			body:     "plain analysis without the summary heading",
			wantSlug: "_template_analysis",
		},
		{
			typ:      "Debug",
			body:     "reproduction symptom resolution root cause keywords only",
			wantSlug: "_template_debug",
		},
	}

	for _, tc := range cases {
		t.Run(tc.typ, func(t *testing.T) {
			in := artifactProposeInput{
				Type:         tc.typ,
				Title:        "structure test",
				BodyMarkdown: tc.body,
				AreaSlug:     "mcp",
				AuthorID:     "test-agent",
			}
			_, failed, code := preflight(context.Background(), Deps{}, "", &in, "en")
			if !hasCodePrefix(failed, "MISSING_H2:") {
				t.Fatalf("expected MISSING_H2 code, got %v", failed)
			}
			tools := nextToolsForNotReady(code, in.Type, failed)
			if len(tools) == 0 {
				t.Fatalf("expected next_tools hint")
			}
			if tools[0].Tool != "pindoc.artifact.read" {
				t.Fatalf("first next tool = %q, want pindoc.artifact.read", tools[0].Tool)
			}
			if got := tools[0].Args["id_or_slug"]; got != tc.wantSlug {
				t.Fatalf("template slug arg = %v, want %s", got, tc.wantSlug)
			}
			expected := expectedForNotReady(context.Background(), Deps{}, "", in.Type, failed)
			if expected == nil || expected.TemplateSlug != tc.wantSlug || len(expected.RequiredH2) == 0 {
				t.Fatalf("expected schema missing template/required_h2: %+v", expected)
			}
			actions := suggestedActionsForNotReady("en", in.Type, failed, nil)
			if !containsSubstring(actions, "self-heal") {
				t.Fatalf("expected self-heal suggested action, got %v", actions)
			}
		})
	}
}

func TestTemplateHintsForTypesExposeRequiredH2Aliases(t *testing.T) {
	hints := templateHintsForTypes(context.Background(), Deps{}, "", []string{"Analysis"})
	analysis, ok := hints["Analysis"]
	if !ok {
		t.Fatalf("Analysis template hint missing: %+v", hints)
	}
	if analysis.TemplateSlug != "_template_analysis" {
		t.Fatalf("template slug = %q, want _template_analysis", analysis.TemplateSlug)
	}
	if len(analysis.RequiredH2) != 1 || analysis.RequiredH2[0].Label != "TL;DR" {
		t.Fatalf("required_h2 = %+v, want TL;DR slot", analysis.RequiredH2)
	}
	for _, wantAlias := range []string{"TL", "요약"} {
		if !containsExactString(analysis.RequiredH2[0].Aliases, wantAlias) {
			t.Fatalf("aliases %v missing %q", analysis.RequiredH2[0].Aliases, wantAlias)
		}
	}
}

func TestTemplateHintsForAllArtifactTypesIncludesTierABTypes(t *testing.T) {
	hints := templateHintsForAllArtifactTypes(context.Background(), Deps{}, "")
	for artifactType := range validArtifactTypes {
		hint, ok := hints[artifactType]
		if !ok {
			t.Fatalf("template_hints missing %s in %+v", artifactType, hints)
		}
		if hint.ArtifactType != artifactType {
			t.Fatalf("%s hint artifact_type = %q", artifactType, hint.ArtifactType)
		}
	}
}

func TestAnalysisWithTLDRPassesStructureOnFirstTry(t *testing.T) {
	in := artifactProposeInput{
		Type:         "Analysis",
		Title:        "analysis first draft",
		BodyMarkdown: "## TL;DR\n\nThe important result is stated first.\n\n## Details\n\nSupporting notes.",
		AreaSlug:     "mcp",
		AuthorID:     "test-agent",
	}
	_, failed, _ := preflight(context.Background(), Deps{}, "", &in, "en")
	if hasCodePrefix(failed, "MISSING_H2:") {
		t.Fatalf("Analysis draft with TL;DR should not fail H2 validation: %v", failed)
	}
}

func hasCodePrefix(codes []string, prefix string) bool {
	for _, code := range codes {
		if strings.HasPrefix(code, prefix) {
			return true
		}
	}
	return false
}

func containsSubstring(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func containsExactString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func requiredH2SlotLabelsContain(slots []requiredH2Slot, label string) bool {
	for _, slot := range slots {
		if slot.Label == label {
			return true
		}
	}
	return false
}

func taskBodyWithCodeCoordinate(coordinate string) string {
	return "## 목적 / Purpose\n\nPurpose.\n\n" +
		"## 범위 / Scope\n\nScope.\n\n" +
		"## 코드 좌표 (Code coordinates)\n\n" + coordinate + "\n\n" +
		"## TODO — Acceptance criteria\n\n- [ ] acceptance criterion is explicit.\n\n" +
		"## TC / DoD\n\nAutomatic tests and DoD."
}

// TestBodyContainsAnyKeywordCaseInsensitive guards the template-driven
// keyword gate used by preflight for Task/Debug types.
func TestBodyContainsAnyKeywordCaseInsensitive(t *testing.T) {
	if !bodyContainsAnyKeyword("The Acceptance Criteria", []string{"acceptance"}) {
		t.Fatal("expected case-insensitive match")
	}
	if bodyContainsAnyKeyword("no match here", []string{"foo", "bar"}) {
		t.Fatal("unexpected match")
	}
	if !bodyContainsAnyKeyword("anything", nil) {
		t.Fatal("nil keywords should pass (no gate)")
	}
}

// TestBodyContainsAllKeywords guards the AND-semantics Decision path.
func TestBodyContainsAllKeywords(t *testing.T) {
	if !bodyContainsAllKeywords("Decision section plus Context header", []string{"decision", "context"}) {
		t.Fatal("expected all keywords matched")
	}
	if bodyContainsAllKeywords("Decision only — second word intentionally omitted.", []string{"decision", "context"}) {
		t.Fatal("missing keyword should fail")
	}
}

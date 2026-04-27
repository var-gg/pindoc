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
	body := "## 증상 (Symptom)\n\n## 재현 (Reproduction)\n\n## 원인 (Root cause)\n\n## 해결 (Resolution)\n"
	if warns := requiredH2Warnings(body, "Debug"); len(warns) != 0 {
		t.Fatalf("parenthetical bilingual headings should satisfy Debug slots, got %v", warns)
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

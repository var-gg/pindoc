package tools

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// TestResolveArtifactMeta covers the six resolver branches Task
// `artifact-meta-jsonb-스키마-추가-6축-epistemic-metadata-도입` enumerates:
// code-only / user_chat / mixed / update without delta / excluded path /
// private audience. Each case asserts exactly the axes the resolver is
// expected to set — unrelated axes stay empty so the JSONB column stays
// minimal for rows that don't need every classification.
func TestResolveArtifactMeta(t *testing.T) {
	cases := []struct {
		name     string
		input    *ArtifactMetaInput
		pins     []ArtifactPinInput
		body     string
		isUpdate bool
		want     ResolvedArtifactMeta
	}{
		{
			name:  "code only from pins infers source and verification",
			input: nil,
			pins: []ArtifactPinInput{
				{Kind: "code", Path: "internal/foo.go"},
			},
			body: "bodies are mostly prose",
			want: ResolvedArtifactMeta{
				SourceType:        "code",
				VerificationState: "partially_verified",
			},
		},
		{
			name: "user chat declared forces opt_in context",
			input: &ArtifactMetaInput{
				SourceType:   "user_chat",
				ConsentState: "granted",
				Confidence:   "low",
			},
			pins: nil,
			body: "사용자는 이 경로가 맞다고 말했다",
			want: ResolvedArtifactMeta{
				SourceType:        "user_chat",
				ConsentState:      "granted",
				Confidence:        "low",
				NextContextPolicy: "opt_in",
			},
		},
		{
			name: "mixed declared keeps caller axes and pins still confirm code substrate",
			input: &ArtifactMetaInput{
				SourceType:        "mixed",
				Confidence:        "medium",
				VerificationState: "partially_verified",
			},
			pins: []ArtifactPinInput{{Kind: "code", Path: "docs/foo.md"}},
			body: "analysis body",
			want: ResolvedArtifactMeta{
				SourceType:        "mixed",
				Confidence:        "medium",
				VerificationState: "partially_verified",
			},
		},
		{
			name: "update without delta leaves axes as caller sent them",
			input: &ArtifactMetaInput{
				SourceType:        "artifact",
				VerificationState: "verified",
			},
			pins:     nil,
			body:     "revision body",
			isUpdate: true,
			want: ResolvedArtifactMeta{
				SourceType:        "artifact",
				VerificationState: "verified",
			},
		},
		{
			name: "excluded policy is preserved (not overridden by user_chat default)",
			input: &ArtifactMetaInput{
				SourceType:        "user_chat",
				NextContextPolicy: "excluded",
			},
			body: "draft memo",
			want: ResolvedArtifactMeta{
				SourceType:        "user_chat",
				NextContextPolicy: "excluded",
			},
		},
		{
			name: "user_chat with email-like body downgrades audience to owner_only",
			input: &ArtifactMetaInput{
				SourceType: "user_chat",
			},
			body: "user paste: alice@example.com token=xyz",
			want: ResolvedArtifactMeta{
				SourceType:        "user_chat",
				NextContextPolicy: "opt_in",
				Audience:          "owner_only",
			},
		},
		{
			name: "applicable rule metadata is preserved",
			input: &ArtifactMetaInput{
				SourceType:        "artifact",
				AppliesToAreas:    []string{" ui ", "ui", "experience/*"},
				AppliesToTypes:    []string{"Task", "Decision", "Task"},
				RuleSeverity:      "binding",
				RuleExcerpt:       "  Follow the design contract.  ",
				VerificationState: "verified",
			},
			body: "policy body",
			want: ResolvedArtifactMeta{
				SourceType:        "artifact",
				VerificationState: "verified",
				AppliesToAreas:    []string{"ui", "experience/*"},
				AppliesToTypes:    []string{"Task", "Decision"},
				RuleSeverity:      "binding",
				RuleExcerpt:       "Follow the design contract.",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveArtifactMeta(tc.input, tc.pins, tc.body, tc.isUpdate)
			got.Warnings = nil // warnings are advisory; assert on axes only
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("resolveArtifactMeta mismatch\n  got:  %+v\n  want: %+v", got, tc.want)
			}
		})
	}
}

// TestArtifactMetaToJSONOmitsEmpty asserts the JSONB payload stays minimal
// — unset axes never serialize. Downstream filters rely on
// `artifact_meta <> '{}'` (see migration 0012) to keep the partial GIN
// index small, so spurious empty keys would break index eligibility.
func TestArtifactMetaToJSONOmitsEmpty(t *testing.T) {
	js := artifactMetaToJSON(ResolvedArtifactMeta{
		SourceType:        "code",
		NextContextPolicy: "default",
	})
	var parsed map[string]any
	if err := json.Unmarshal([]byte(js), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := parsed["confidence"]; ok {
		t.Fatalf("expected confidence to be omitted, got %v", parsed)
	}
	if parsed["source_type"] != "code" {
		t.Fatalf("expected source_type=code, got %v", parsed["source_type"])
	}
	if parsed["next_context_policy"] != "default" {
		t.Fatalf("expected next_context_policy=default, got %v", parsed["next_context_policy"])
	}
}

func TestArtifactMetaToJSONIncludesApplicableRuleFields(t *testing.T) {
	js := artifactMetaToJSON(ResolvedArtifactMeta{
		AppliesToAreas: []string{"ui", "experience/*"},
		AppliesToTypes: []string{"Task"},
		RuleSeverity:   "guidance",
		RuleExcerpt:    "Use the shared empty-state component.",
	})
	var parsed map[string]any
	if err := json.Unmarshal([]byte(js), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["rule_severity"] != "guidance" {
		t.Fatalf("expected rule_severity=guidance, got %v", parsed["rule_severity"])
	}
	if parsed["rule_excerpt"] != "Use the shared empty-state component." {
		t.Fatalf("expected rule_excerpt to round-trip, got %v", parsed["rule_excerpt"])
	}
	areas, ok := parsed["applies_to_areas"].([]any)
	if !ok || len(areas) != 2 || areas[0] != "ui" || areas[1] != "experience/*" {
		t.Fatalf("unexpected applies_to_areas: %#v", parsed["applies_to_areas"])
	}
	types, ok := parsed["applies_to_types"].([]any)
	if !ok || len(types) != 1 || types[0] != "Task" {
		t.Fatalf("unexpected applies_to_types: %#v", parsed["applies_to_types"])
	}
}

func TestApplicableRuleMetaPreflightValidation(t *testing.T) {
	in := artifactProposeInput{
		Type:         "Decision",
		AreaSlug:     "policies",
		Title:        "Policy",
		BodyMarkdown: "## Context\nx\n\n## Decision\nx",
		AuthorID:     "codex",
		ArtifactMeta: &ArtifactMetaInput{
			AppliesToAreas: []string{"ui/**"},
			AppliesToTypes: []string{"Bogus"},
			RuleSeverity:   "must",
		},
	}
	_, failed, _ := preflight(context.Background(), Deps{}, "", &in, "en")
	for _, want := range []string{
		"META_APPLIES_AREA_INVALID",
		"META_APPLIES_TYPE_INVALID",
		"META_RULE_SEVERITY_INVALID",
	} {
		if !containsString(failed, want) {
			t.Fatalf("failed[] missing %s: %v", want, failed)
		}
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// TestDetectUnclassifiedUserChat exercises the SOURCE_TYPE_UNCLASSIFIED
// warning trigger: no pins, no declared source_type, body that quotes a
// user. False-positive avoidance cases (pins present, source_type set,
// body without a chat marker) are equally important because the warning
// is advisory only — a loud advisory on code-grounded writes would train
// agents to ignore it.
func TestDetectUnclassifiedUserChat(t *testing.T) {
	t.Run("quote plus user marker triggers", func(t *testing.T) {
		got := detectUnclassifiedUserChat(
			ResolvedArtifactMeta{},
			nil,
			`user said "restart the worker"`,
		)
		if !got {
			t.Fatal("expected unclassified user_chat detection to fire")
		}
	})
	t.Run("korean marker triggers", func(t *testing.T) {
		got := detectUnclassifiedUserChat(
			ResolvedArtifactMeta{},
			nil,
			"사용자는 \"다음 주 다시 보자\"라고 했다",
		)
		if !got {
			t.Fatal("expected korean user marker to fire")
		}
	})
	t.Run("pins present suppresses", func(t *testing.T) {
		got := detectUnclassifiedUserChat(
			ResolvedArtifactMeta{},
			[]ArtifactPinInput{{Kind: "code", Path: "main.go"}},
			`user said "restart the worker"`,
		)
		if got {
			t.Fatal("expected detection to skip when pins exist")
		}
	})
	t.Run("declared source_type suppresses", func(t *testing.T) {
		got := detectUnclassifiedUserChat(
			ResolvedArtifactMeta{SourceType: "mixed"},
			nil,
			`user said "restart the worker"`,
		)
		if got {
			t.Fatal("expected detection to skip when source_type already classified")
		}
	})
	t.Run("body without quotes does not fire", func(t *testing.T) {
		got := detectUnclassifiedUserChat(
			ResolvedArtifactMeta{},
			nil,
			"agent observations about the system, user said this but no quote",
		)
		if got {
			t.Fatal("expected detection to require quote marker")
		}
	})
}

// TestHasPIISignal covers the coarse PII heuristic that can downgrade
// audience. A negative case (purely technical prose) is included to catch
// over-eager matching that would cascade into wrong audience routing.
func TestHasPIISignal(t *testing.T) {
	positives := []string{
		"contact alice@example.com for details",
		"Authorization: Bearer abc123",
		"auth header api_key=xxx",
	}
	for _, s := range positives {
		if !hasPIISignal(s) {
			t.Errorf("expected PII signal to fire on %q", s)
		}
	}
	negatives := []string{
		"pure technical narrative without identifiers",
		"references pindoc://some-slug and docs/foo.md",
	}
	for _, s := range negatives {
		if hasPIISignal(s) {
			t.Errorf("expected PII signal to stay quiet on %q", s)
		}
	}
}

// TestClassifyConversationDerived asserts the declared-substrate classifier
// (Task `conversation-derived-write-기본-draft-라우팅-...`): only user_chat
// or mixed source_type flip it on. Missing meta, code substrate, external
// substrate all stay false so code-grounded writes aren't penalised.
func TestClassifyConversationDerived(t *testing.T) {
	cases := []struct {
		name  string
		input *artifactProposeInput
		want  bool
	}{
		{
			name:  "nil input",
			input: nil,
			want:  false,
		},
		{
			name:  "no meta attached",
			input: &artifactProposeInput{},
			want:  false,
		},
		{
			name: "code source stays false",
			input: &artifactProposeInput{
				ArtifactMeta: &ArtifactMetaInput{SourceType: "code"},
			},
			want: false,
		},
		{
			name: "user_chat flips true",
			input: &artifactProposeInput{
				ArtifactMeta: &ArtifactMetaInput{SourceType: "user_chat"},
			},
			want: true,
		},
		{
			name: "mixed flips true",
			input: &artifactProposeInput{
				ArtifactMeta: &ArtifactMetaInput{SourceType: "mixed"},
			},
			want: true,
		},
		{
			name: "external stays false",
			input: &artifactProposeInput{
				ArtifactMeta: &ArtifactMetaInput{SourceType: "external"},
			},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyConversationDerived(tc.input); got != tc.want {
				t.Fatalf("classifyConversationDerived got=%v want=%v", got, tc.want)
			}
		})
	}
}

// TestApplyConversationDerivedDefaults covers the consent-granted downgrade
// policy. The three interesting cases: (1) caller left completeness blank
// → downgrade to draft; (2) caller set completeness=settled → keep it
// (caller override wins); (3) consent not granted → no downgrade even
// though source_type=user_chat. The function also bumps
// next_context_policy to opt_in when the resolver left it empty.
func TestApplyConversationDerivedDefaults(t *testing.T) {
	t.Run("consent granted with blank completeness downgrades", func(t *testing.T) {
		in := &artifactProposeInput{
			ArtifactMeta: &ArtifactMetaInput{SourceType: "user_chat"},
		}
		meta := ResolvedArtifactMeta{
			SourceType:   "user_chat",
			ConsentState: "granted",
		}
		got := applyConversationDerivedDefaults(in, &meta, "")
		if got != "draft" {
			t.Fatalf("completeness got=%q want=draft", got)
		}
		if meta.NextContextPolicy != "opt_in" {
			t.Fatalf("next_context_policy got=%q want=opt_in", meta.NextContextPolicy)
		}
	})

	t.Run("caller explicit completeness preserved", func(t *testing.T) {
		in := &artifactProposeInput{
			Completeness: "settled",
			ArtifactMeta: &ArtifactMetaInput{SourceType: "user_chat"},
		}
		meta := ResolvedArtifactMeta{
			SourceType:   "user_chat",
			ConsentState: "granted",
		}
		got := applyConversationDerivedDefaults(in, &meta, "settled")
		if got != "settled" {
			t.Fatalf("completeness got=%q want=settled (caller override)", got)
		}
	})

	t.Run("consent missing leaves axes untouched", func(t *testing.T) {
		in := &artifactProposeInput{
			ArtifactMeta: &ArtifactMetaInput{SourceType: "user_chat"},
		}
		meta := ResolvedArtifactMeta{
			SourceType: "user_chat",
			// ConsentState intentionally empty — warning path.
		}
		got := applyConversationDerivedDefaults(in, &meta, "")
		if got != "" {
			t.Fatalf("completeness got=%q want empty (no downgrade without consent)", got)
		}
		// NextContextPolicy still opt_in because resolveArtifactMeta's
		// own user_chat default fires earlier in the real flow, but this
		// helper itself should not touch it when consent absent.
		if meta.NextContextPolicy == "opt_in" {
			t.Fatalf("next_context_policy should not be set by this helper when consent absent")
		}
	})

	t.Run("non-user_chat source is no-op", func(t *testing.T) {
		in := &artifactProposeInput{
			ArtifactMeta: &ArtifactMetaInput{SourceType: "code"},
		}
		meta := ResolvedArtifactMeta{
			SourceType:   "code",
			ConsentState: "granted",
		}
		got := applyConversationDerivedDefaults(in, &meta, "")
		if got != "" {
			t.Fatalf("completeness got=%q want empty (code source unaffected)", got)
		}
	})
}

// TestResolveArtifactMetaWarnsOnInference asserts the resolver emits
// human-readable warnings on heuristic decisions so callers can surface
// "why did you pick that axis" if needed.
func TestResolveArtifactMetaWarnsOnInference(t *testing.T) {
	got := resolveArtifactMeta(nil,
		[]ArtifactPinInput{{Kind: "code", Path: "main.go"}},
		"body",
		false,
	)
	found := false
	for _, w := range got.Warnings {
		if strings.Contains(w, "inferred from code pins") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected inference warning, got warnings=%v", got.Warnings)
	}
}

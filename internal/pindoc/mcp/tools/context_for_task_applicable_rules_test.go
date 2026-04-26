package tools

import "testing"

func TestApplicableRuleAreaMatching(t *testing.T) {
	target := applicableRuleTarget{
		AreaSlug: "ui",
		Type:     "Task",
		Chain: []areaChainEntry{
			{Slug: "ui", Distance: 0},
			{Slug: "experience", Distance: 1},
		},
		Path: "experience/ui",
	}

	cases := []struct {
		name         string
		rule         applicableRuleCandidate
		wantMatch    bool
		wantDistance int
	}{
		{
			name: "default scope applies to own area descendants",
			rule: applicableRuleCandidate{
				AreaSlug: "experience",
				Meta:     ResolvedArtifactMeta{RuleSeverity: "guidance"},
			},
			wantMatch:    true,
			wantDistance: 1,
		},
		{
			name: "explicit exact scope matches target area",
			rule: applicableRuleCandidate{
				AreaSlug: "policies",
				Meta: ResolvedArtifactMeta{
					RuleSeverity:   "binding",
					AppliesToAreas: []string{"ui"},
				},
			},
			wantMatch:    true,
			wantDistance: 0,
		},
		{
			name: "wildcard scope matches area chain parent",
			rule: applicableRuleCandidate{
				AreaSlug: "policies",
				Meta: ResolvedArtifactMeta{
					RuleSeverity:   "binding",
					AppliesToAreas: []string{"experience/*"},
				},
			},
			wantMatch:    true,
			wantDistance: 1,
		},
		{
			name: "cross-cutting rule applies without explicit area scope",
			rule: applicableRuleCandidate{
				AreaSlug:       "accessibility",
				ParentAreaSlug: "cross-cutting",
				IsCrossCutting: true,
				Meta:           ResolvedArtifactMeta{RuleSeverity: "binding"},
			},
			wantMatch:    true,
			wantDistance: crossCuttingRuleDistance,
		},
		{
			name: "unrelated default scope does not match",
			rule: applicableRuleCandidate{
				AreaSlug: "data",
				Meta:     ResolvedArtifactMeta{RuleSeverity: "guidance"},
			},
			wantMatch: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotMatch, gotDistance := ruleMatchesTargetArea(tc.rule, target)
			if gotMatch != tc.wantMatch || gotDistance != tc.wantDistance {
				t.Fatalf("ruleMatchesTargetArea got=(%v,%d) want=(%v,%d)", gotMatch, gotDistance, tc.wantMatch, tc.wantDistance)
			}
		})
	}
}

func TestApplicableRuleTypeMatching(t *testing.T) {
	if !ruleAppliesToTargetType(ResolvedArtifactMeta{}, "Task") {
		t.Fatal("empty applies_to_types should match every target type")
	}
	if !ruleAppliesToTargetType(ResolvedArtifactMeta{AppliesToTypes: []string{"Decision", "Task"}}, "Task") {
		t.Fatal("expected explicit Task scope to match")
	}
	if ruleAppliesToTargetType(ResolvedArtifactMeta{AppliesToTypes: []string{"Decision"}}, "Task") {
		t.Fatal("Decision-only rule should not match Task target")
	}
}

func TestApplicableRuleExcerptDerivation(t *testing.T) {
	body := "# Title\n\n## TL;DR\n\n- Use the shared empty-state component.\n- Keep count text restrained.\n\n## Context\n\nLonger notes."
	got := applicableRuleExcerpt(ResolvedArtifactMeta{}, body)
	want := "Use the shared empty-state component. Keep count text restrained."
	if got != want {
		t.Fatalf("applicableRuleExcerpt got=%q want=%q", got, want)
	}
	explicit := applicableRuleExcerpt(ResolvedArtifactMeta{RuleExcerpt: "  Explicit summary.  "}, body)
	if explicit != "Explicit summary." {
		t.Fatalf("explicit excerpt got=%q", explicit)
	}
}

func TestRuleSeverityRank(t *testing.T) {
	if !(ruleSeverityRank("binding") < ruleSeverityRank("guidance") &&
		ruleSeverityRank("guidance") < ruleSeverityRank("reference")) {
		t.Fatalf("unexpected severity ranking")
	}
	if ruleSeverityRank("unknown") != -1 {
		t.Fatalf("unknown severity should be rejected")
	}
}

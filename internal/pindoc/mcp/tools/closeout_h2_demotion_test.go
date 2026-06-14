package tools

import "testing"

func TestIsCloseoutMissingH2(t *testing.T) {
	closeout := []string{
		"MISSING_H2:Outcome",
		"MISSING_H2:코드 좌표",
		"MISSING_H2:Code coordinates",
		"MISSING_H2:TC / DoD",
		"MISSING_H2:결과",
	}
	for _, code := range closeout {
		if !isCloseoutMissingH2(code) {
			t.Errorf("expected %q to be a closeout MISSING_H2", code)
		}
	}
	nonCloseout := []string{
		"MISSING_H2:Purpose",
		"MISSING_H2:Scope",
		"MISSING_H2:TODO",
		"TITLE_TOO_SHORT",
		"MISSING_H2:",
		"acceptance_unchecked",
	}
	for _, code := range nonCloseout {
		if isCloseoutMissingH2(code) {
			t.Errorf("expected %q to NOT be a closeout MISSING_H2", code)
		}
	}
}

func TestIsDraftAppendUpdate(t *testing.T) {
	draftAppend := artifactProposeInput{Completeness: "draft", BodyPatch: &BodyPatchInput{Mode: "append"}}
	if !isDraftAppendUpdate(ShapeBodyPatch, draftAppend, false) {
		t.Fatal("declared draft append should qualify for demotion")
	}
	// Auto-claiming the Task done preserves the Outcome warning at warn.
	if isDraftAppendUpdate(ShapeBodyPatch, draftAppend, true) {
		t.Fatal("auto-claimed update must NOT demote (Outcome warning is the only signal)")
	}
	// completeness must be explicitly draft — a real closeout append uses
	// partial/settled (or omits it).
	for _, c := range []string{"", "partial", "settled"} {
		in := artifactProposeInput{Completeness: c, BodyPatch: &BodyPatchInput{Mode: "append"}}
		if isDraftAppendUpdate(ShapeBodyPatch, in, false) {
			t.Fatalf("completeness=%q append must not demote", c)
		}
	}
	// Only append mode qualifies — section_replace edits existing structure.
	sectionReplace := artifactProposeInput{Completeness: "draft", BodyPatch: &BodyPatchInput{Mode: "section_replace"}}
	if isDraftAppendUpdate(ShapeBodyPatch, sectionReplace, false) {
		t.Fatal("section_replace must not demote")
	}
	// A full body_markdown update (no BodyPatch) is not a draft append.
	fullBody := artifactProposeInput{Completeness: "draft"}
	if isDraftAppendUpdate(ShapeBodyPatch, fullBody, false) {
		t.Fatal("full-body update must not demote")
	}
}

// TestCloseoutH2DemotionSeverity mirrors the resolver the update path uses:
// closeout MISSING_H2 drops to info and sorts below a real warn, while
// non-closeout warnings keep their severity.
func TestCloseoutH2DemotionSeverity(t *testing.T) {
	severityOf := func(code string) string {
		if isCloseoutMissingH2(code) {
			return SeverityInfo
		}
		return warningSeverity(code)
	}
	in := []string{"MISSING_H2:Outcome", "PIN_PATH_NOT_FOUND:x.go", "MISSING_H2:Purpose"}
	got := sortWarningsBySeverityFunc(in, severityOf)
	// Real warns (PIN_PATH_NOT_FOUND, MISSING_H2:Purpose) rank above the
	// demoted closeout Outcome advisory.
	if got[len(got)-1] != "MISSING_H2:Outcome" {
		t.Fatalf("demoted closeout warning should sort last, got %v", got)
	}
	if severityOf("MISSING_H2:Outcome") != SeverityInfo {
		t.Fatal("closeout Outcome should resolve to info under the demotion resolver")
	}
	if severityOf("MISSING_H2:Purpose") != SeverityWarn {
		t.Fatal("non-closeout Purpose must stay warn")
	}
}

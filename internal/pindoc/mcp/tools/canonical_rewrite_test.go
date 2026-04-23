package tools

import (
	"reflect"
	"testing"
)

// TestDetectCanonicalClaimRewrite covers the per-type canonical section
// rewrite detector (Task `update-of-canonical-claim-rewrite-guard-...`).
// Six cases: Debug root cause flip, Debug unchanged, Decision body flip,
// Decision alternatives-only change (must stay quiet), Analysis conclusion
// absence, whitespace-only change (must stay quiet).
func TestDetectCanonicalClaimRewrite(t *testing.T) {
	cases := []struct {
		name    string
		prev    string
		next    string
		artType string
		want    []string
	}{
		{
			name:    "debug root cause flipped",
			prev:    "## Root cause\nNetwork jitter.\n\n## Resolution\nRetry budget.\n",
			next:    "## Root cause\nExpired TLS certificate.\n\n## Resolution\nRetry budget.\n",
			artType: "Debug",
			want:    []string{"Root cause"},
		},
		{
			name:    "debug unchanged returns nil",
			prev:    "## Root cause\nNetwork jitter.\n\n## Resolution\nRetry budget.\n",
			next:    "## Root cause\nNetwork jitter.\n\n## Resolution\nRetry budget.\n",
			artType: "Debug",
			want:    nil,
		},
		{
			name:    "decision body rewritten",
			prev:    "## Decision\nAdopt Gemma Q4 as default.\n\n## Alternatives considered\nBGE-M3.\n",
			next:    "## Decision\nAdopt BGE-M3 as default.\n\n## Alternatives considered\nGemma Q4.\n",
			artType: "Decision",
			want:    []string{"Decision"},
		},
		{
			name:    "decision alternatives-only change is quiet",
			prev:    "## Decision\nKeep Gemma Q4.\n\n## Alternatives considered\nBGE-M3.\n",
			next:    "## Decision\nKeep Gemma Q4.\n\n## Alternatives considered\nBGE-M3 and Cohere multilingual.\n",
			artType: "Decision",
			want:    nil,
		},
		{
			name:    "analysis conclusion absent in both stays quiet",
			prev:    "## TL;DR\nShort summary.\n",
			next:    "## TL;DR\nUpdated summary.\n",
			artType: "Analysis",
			want:    nil,
		},
		{
			name:    "whitespace-only diff is quiet",
			prev:    "## Root cause\n\nNetwork jitter.\n\n\n",
			next:    "## Root cause\nNetwork jitter.\n",
			artType: "Debug",
			want:    nil,
		},
		{
			name:    "korean synonym still detected",
			prev:    "## 원인\n네트워크 지터\n",
			next:    "## 원인\nTLS 만료\n",
			artType: "Debug",
			want:    []string{"Root cause"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := detectCanonicalClaimRewrite(tc.prev, tc.next, tc.artType)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("detectCanonicalClaimRewrite mismatch\n  got:  %v\n  want: %v", got, tc.want)
			}
		})
	}
}

// TestHasEvidenceDelta covers the three evidence signals: new pins,
// verification bump, and commit_msg keyword. Plus the all-blank case
// where none fire so the guard actually has something to flag.
func TestHasEvidenceDelta(t *testing.T) {
	prevVerified := ResolvedArtifactMeta{VerificationState: "unverified"}

	t.Run("new pins satisfies evidence", func(t *testing.T) {
		in := &artifactProposeInput{
			Pins: []ArtifactPinInput{{Kind: "code", Path: "main.go"}},
		}
		if !hasEvidenceDelta(prevVerified, in) {
			t.Fatal("expected pins to count as evidence")
		}
	})

	t.Run("verification bump satisfies evidence", func(t *testing.T) {
		in := &artifactProposeInput{
			ArtifactMeta: &ArtifactMetaInput{VerificationState: "verified"},
		}
		if !hasEvidenceDelta(prevVerified, in) {
			t.Fatal("expected verification bump to count as evidence")
		}
	})

	t.Run("commit_msg keyword satisfies evidence", func(t *testing.T) {
		in := &artifactProposeInput{CommitMsg: "reproduced and verified"}
		if !hasEvidenceDelta(prevVerified, in) {
			t.Fatal("expected commit_msg keyword to count as evidence")
		}
	})

	t.Run("korean evidence keyword satisfies", func(t *testing.T) {
		in := &artifactProposeInput{CommitMsg: "근거 추가"}
		if !hasEvidenceDelta(prevVerified, in) {
			t.Fatal("expected korean evidence keyword to count")
		}
	})

	t.Run("no signals means no delta", func(t *testing.T) {
		in := &artifactProposeInput{CommitMsg: "wording cleanup"}
		if hasEvidenceDelta(prevVerified, in) {
			t.Fatal("expected plain wording cleanup to fail the delta test")
		}
	})

	t.Run("unverified -> unverified is not a bump", func(t *testing.T) {
		in := &artifactProposeInput{
			ArtifactMeta: &ArtifactMetaInput{VerificationState: "unverified"},
			CommitMsg:    "touch",
		}
		if hasEvidenceDelta(prevVerified, in) {
			t.Fatal("unverified restatement should not count as a bump")
		}
	})
}

// TestParseH2SectionsIgnoresPreamble asserts content above the first H2
// is not mis-attributed. Regression guard: the detector must not pick up
// an opening TL;DR paragraph as part of the first canonical section.
func TestParseH2SectionsIgnoresPreamble(t *testing.T) {
	body := "## TL;DR\nSummary.\n\n## Root cause\nActual cause.\n"
	sections := parseH2Sections(body)
	if _, ok := sections["tl;dr"]; !ok {
		t.Fatal("expected tl;dr section")
	}
	if _, ok := sections["root cause"]; !ok {
		t.Fatal("expected root cause section")
	}
	if got := sections["root cause"]; !reflect.DeepEqual(reflectStringTrim(got), "Actual cause.") {
		t.Fatalf("root cause content unexpected: %q", got)
	}
}

// small helper to keep the assertion readable.
func reflectStringTrim(s string) string {
	return normalizeSectionContent(s)
}

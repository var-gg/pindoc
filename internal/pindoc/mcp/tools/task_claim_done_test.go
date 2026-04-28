package tools

import (
	"strings"
	"testing"
)

// TestMarkUncheckedAsDone covers the body rewrite half of pindoc.task.claim_done.
// Only "- [ ]" markers move to "[x]"; "[x]" / "[X]" / "[~]" / "[-]" are
// preserved because they represent prior judgment calls (already done /
// partial / deferred) that an automatic mass-toggle must not overwrite.
func TestMarkUncheckedAsDone(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		want        string
		wantChanged int
	}{
		{
			name:        "empty body",
			body:        "",
			want:        "",
			wantChanged: 0,
		},
		{
			name:        "no checkboxes",
			body:        "## Purpose\n\nJust prose.\n",
			want:        "## Purpose\n\nJust prose.\n",
			wantChanged: 0,
		},
		{
			name:        "single unchecked",
			body:        "- [ ] item",
			want:        "- [x] item",
			wantChanged: 1,
		},
		{
			name:        "multiple unchecked",
			body:        "- [ ] one\n- [ ] two\n- [ ] three",
			want:        "- [x] one\n- [x] two\n- [x] three",
			wantChanged: 3,
		},
		{
			name:        "mixed states preserved",
			body:        "- [ ] todo\n- [x] done\n- [~] partial\n- [-] deferred\n- [ ] more",
			want:        "- [x] todo\n- [x] done\n- [~] partial\n- [-] deferred\n- [x] more",
			wantChanged: 2,
		},
		{
			name:        "uppercase X is preserved as-is",
			body:        "- [X] capital done\n- [ ] todo",
			want:        "- [X] capital done\n- [x] todo",
			wantChanged: 1,
		},
		{
			name:        "asterisk and plus bullets accepted",
			body:        "* [ ] star\n+ [ ] plus\n- [ ] dash",
			want:        "* [x] star\n+ [x] plus\n- [x] dash",
			wantChanged: 3,
		},
		{
			name:        "indented checkbox",
			body:        "  - [ ] indented\n    - [ ] deeper",
			want:        "  - [x] indented\n    - [x] deeper",
			wantChanged: 2,
		},
		{
			name:        "all already resolved no change",
			body:        "- [x] one\n- [~] two\n- [-] three",
			want:        "- [x] one\n- [~] two\n- [-] three",
			wantChanged: 0,
		},
		{
			name:        "non-checkbox brackets ignored",
			body:        "- not a checkbox\n[ ] no bullet either\n- [ ] real one",
			want:        "- not a checkbox\n[ ] no bullet either\n- [x] real one",
			wantChanged: 1,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, changed := markUncheckedAsDone(c.body)
			if got != c.want {
				t.Fatalf("markUncheckedAsDone body:\n--- got ---\n%s\n--- want ---\n%s", got, c.want)
			}
			if changed != c.wantChanged {
				t.Fatalf("markUncheckedAsDone changed: got %d, want %d", changed, c.wantChanged)
			}
		})
	}
}

// TestValidateClaimDoneCommitSHA covers the commit_sha extension to
// pindoc.task.claim_done. Empty / whitespace input is the legacy "no
// commit attached" path; bad input must short-circuit the handler with
// a stable error code so the Reader never sees a malformed prefix.
func TestValidateClaimDoneCommitSHA(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		wantSHA  string
		wantCode string
	}{
		{name: "empty", raw: "", wantSHA: "", wantCode: ""},
		{name: "whitespace only", raw: "   \t\n  ", wantSHA: "", wantCode: ""},
		{name: "short hex 7 chars", raw: "abc1234", wantSHA: "abc1234", wantCode: ""},
		{name: "git short sha trimmed", raw: "  d4ad2e2  ", wantSHA: "d4ad2e2", wantCode: ""},
		{name: "full sha-1", raw: "0123456789abcdef0123456789abcdef01234567", wantSHA: "0123456789abcdef0123456789abcdef01234567", wantCode: ""},
		{name: "uppercase accepted", raw: "ABCDEF1", wantSHA: "ABCDEF1", wantCode: ""},
		{name: "too short 6 chars", raw: "abc123", wantCode: "CLAIM_DONE_COMMIT_SHA_LENGTH_INVALID"},
		{name: "too long 65 chars", raw: strings.Repeat("a", 65), wantCode: "CLAIM_DONE_COMMIT_SHA_LENGTH_INVALID"},
		{name: "non-hex letters", raw: "ghijklm", wantCode: "CLAIM_DONE_COMMIT_SHA_FORMAT_INVALID"},
		{name: "embedded space", raw: "abc 1234", wantCode: "CLAIM_DONE_COMMIT_SHA_FORMAT_INVALID"},
		{name: "punctuation in middle", raw: "abc-1234", wantCode: "CLAIM_DONE_COMMIT_SHA_FORMAT_INVALID"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotSHA, code, _ := validateClaimDoneCommitSHA(c.raw)
			if code != c.wantCode {
				t.Fatalf("code = %q; want %q", code, c.wantCode)
			}
			if c.wantCode == "" && gotSHA != c.wantSHA {
				t.Fatalf("sha = %q; want %q", gotSHA, c.wantSHA)
			}
		})
	}
}

// TestPrefixClaimDoneCommitMsg pins the "[<short>] ..." commit_msg
// prefix shape: missing commitSHA returns the message untouched,
// otherwise the first 8 hex chars (or the full SHA when shorter) form
// the bracketed prefix that the Reader history view renders verbatim.
func TestPrefixClaimDoneCommitMsg(t *testing.T) {
	cases := []struct {
		name      string
		commitMsg string
		commitSHA string
		want      string
	}{
		{name: "no commit_sha returns msg unchanged", commitMsg: "claim_done: 3 acceptance toggled → [x]", commitSHA: "", want: "claim_done: 3 acceptance toggled → [x]"},
		{name: "short 7-char SHA", commitMsg: "fix layout", commitSHA: "abc1234", want: "[abc1234] fix layout"},
		{name: "8-char SHA used in full", commitMsg: "fix layout", commitSHA: "abc12345", want: "[abc12345] fix layout"},
		{name: "longer SHA truncated to 8", commitMsg: "fix layout", commitSHA: "abc1234567890", want: "[abc12345] fix layout"},
		{name: "full sha-1 truncated to 8", commitMsg: "claim", commitSHA: "0123456789abcdef0123456789abcdef01234567", want: "[01234567] claim"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := prefixClaimDoneCommitMsg(c.commitMsg, c.commitSHA)
			if got != c.want {
				t.Fatalf("prefix = %q; want %q", got, c.want)
			}
		})
	}
}

// TestValidateClaimDonePins exercises pin path / kind / commit
// requirements so a malformed pins[] never reaches the transaction.
// The error code carries CLAIM_DONE_PIN_INVALID:<inner> so callers can
// distinguish surface (claim_done) from cause (the same code add_pin
// would have returned).
func TestValidateClaimDonePins(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		out, code, _ := validateClaimDonePins(nil)
		if code != "" || out != nil {
			t.Fatalf("expected (nil, ''); got (%v, %q)", out, code)
		}
	})
	t.Run("empty path rejected", func(t *testing.T) {
		_, code, _ := validateClaimDonePins([]ArtifactPinInput{
			{Path: "", Kind: "code"},
		})
		if !strings.HasPrefix(code, "CLAIM_DONE_PIN_INVALID:") {
			t.Fatalf("expected CLAIM_DONE_PIN_INVALID prefix; got %q", code)
		}
	})
	t.Run("code pin requires commit_sha", func(t *testing.T) {
		_, code, msg := validateClaimDonePins([]ArtifactPinInput{
			{Path: "internal/pindoc/mcp/tools/task_claim_done.go", Kind: "code"},
		})
		if !strings.HasPrefix(code, "CLAIM_DONE_PIN_INVALID:") {
			t.Fatalf("expected CLAIM_DONE_PIN_INVALID prefix; got %q (msg=%s)", code, msg)
		}
	})
	t.Run("valid code pin passes", func(t *testing.T) {
		out, code, _ := validateClaimDonePins([]ArtifactPinInput{
			{Path: "internal/pindoc/mcp/tools/task_claim_done.go", Kind: "code", CommitSHA: "abc1234"},
		})
		if code != "" {
			t.Fatalf("expected ok; got code=%q", code)
		}
		if len(out) != 1 || out[0].Kind != "code" {
			t.Fatalf("expected 1 normalised code pin; got %#v", out)
		}
	})
	t.Run("kind inferred from path when omitted", func(t *testing.T) {
		out, code, _ := validateClaimDonePins([]ArtifactPinInput{
			{Path: "https://example.com/spec"},
		})
		if code != "" {
			t.Fatalf("expected ok; got code=%q", code)
		}
		if len(out) != 1 || out[0].Kind != "url" {
			t.Fatalf("expected url kind inferred; got %#v", out)
		}
	})
	t.Run("first failing index is reported", func(t *testing.T) {
		_, code, msg := validateClaimDonePins([]ArtifactPinInput{
			{Path: "ok.md", Kind: "doc", CommitSHA: "abc1234"},
			{Path: ""},
		})
		if !strings.HasPrefix(code, "CLAIM_DONE_PIN_INVALID:") {
			t.Fatalf("expected CLAIM_DONE_PIN_INVALID prefix; got %q", code)
		}
		if !strings.Contains(msg, "pins[1]") {
			t.Fatalf("expected pins[1] in msg; got %q", msg)
		}
	})
}

package tools

import "testing"

func TestPinCommitRequirementByLane(t *testing.T) {
	t.Run("add_pin and propose require commit only for code config", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			pin  ArtifactPinInput
			want string
		}{
			{name: "code", pin: ArtifactPinInput{Kind: "code", Path: "internal/pindoc/mcp/tools/artifact_propose.go"}, want: "PIN_COMMIT_REQUIRED"},
			{name: "config", pin: ArtifactPinInput{Kind: "config", Path: "web/tsconfig.json"}, want: "PIN_COMMIT_REQUIRED"},
			{name: "doc", pin: ArtifactPinInput{Kind: "doc", Path: "docs/10-mcp-tools-spec.md"}, want: ""},
			{name: "asset", pin: ArtifactPinInput{Kind: "asset", Path: "docs/diagram.png"}, want: ""},
			{name: "url", pin: ArtifactPinInput{Kind: "url", Path: "https://example.com/spec"}, want: ""},
		} {
			t.Run(tc.name, func(t *testing.T) {
				_, code, msg := normalizeAddPinInput(tc.pin)
				if code != tc.want {
					t.Fatalf("normalizeAddPinInput code=%q, want %q (msg=%q)", code, tc.want, msg)
				}
			})
		}
	})

	t.Run("propose uses add_pin normalization", func(t *testing.T) {
		out, code, msg := normalizePinInputs([]ArtifactPinInput{
			{Kind: "doc", Path: "docs/10-mcp-tools-spec.md"},
			{Kind: "asset", Path: "docs/diagram.png"},
		})
		if code != "" {
			t.Fatalf("doc/asset propose pins should pass without commit_sha; code=%q msg=%q", code, msg)
		}
		if len(out) != 2 || out[0].Kind != "doc" || out[1].Kind != "asset" {
			t.Fatalf("unexpected normalized pins: %#v", out)
		}

		_, code, msg = normalizePinInputs([]ArtifactPinInput{
			{Kind: "config", Path: "web/tsconfig.json"},
		})
		if code != "PIN_COMMIT_REQUIRED" {
			t.Fatalf("config propose pin should require commit_sha; code=%q msg=%q", code, msg)
		}
	})

	t.Run("claim_done evidence pins stay strict", func(t *testing.T) {
		_, code, msg := validateClaimDonePins([]ArtifactPinInput{
			{Kind: "doc", Path: "docs/10-mcp-tools-spec.md"},
		})
		if code != "CLAIM_DONE_PIN_INVALID:PIN_COMMIT_REQUIRED" {
			t.Fatalf("doc claim_done evidence pin should require commit_sha; code=%q msg=%q", code, msg)
		}

		out, code, msg := validateClaimDonePins([]ArtifactPinInput{
			{Kind: "doc", Path: "docs/10-mcp-tools-spec.md", CommitSHA: "abc1234"},
		})
		if code != "" {
			t.Fatalf("doc claim_done evidence pin with commit should pass; code=%q msg=%q", code, msg)
		}
		if len(out) != 1 || out[0].CommitSHA != "abc1234" {
			t.Fatalf("unexpected claim_done pin output: %#v", out)
		}
	})
}

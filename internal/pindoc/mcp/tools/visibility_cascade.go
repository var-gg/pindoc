package tools

import "strings"

// resolveArtifactVisibility implements the cascade documented on the
// Visibility field of artifactProposeInput:
//
//	explicit (in.Visibility, validated) > project default > "org"
//
// Invalid explicit values silently fall through to the project default
// rather than 500ing — the DB-side CHECK constraint still guards
// against bad writes, but a typo on the propose payload shouldn't tank
// an otherwise valid create. The set_visibility tool path is the
// strict-validation route for explicit user intent.
//
// Pure function so the cascade is unit-testable without a DB.
func resolveArtifactVisibility(explicit, projectDefault string) string {
	if v := normalizeVisibility(explicit); v != "" {
		return v
	}
	if v := normalizeVisibility(projectDefault); v != "" {
		return v
	}
	return "org"
}

// normalizeVisibility lowercases and trims a visibility tier, returning
// the canonical value if it matches one of public/org/private and ""
// otherwise. The empty string signals "not specified" so the cascade
// can move to the next layer.
func normalizeVisibility(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "public":
		return "public"
	case "org":
		return "org"
	case "private":
		return "private"
	default:
		return ""
	}
}

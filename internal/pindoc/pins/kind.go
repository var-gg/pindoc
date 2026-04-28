package pins

import (
	"path"
	"strings"
)

const (
	KindCode     = "code"
	KindDoc      = "doc"
	KindConfig   = "config"
	KindAsset    = "asset"
	KindResource = "resource"
	KindURL      = "url"
)

// ValidKind reports whether kind belongs to Pindoc's pin vocabulary.
func ValidKind(kind string) bool {
	switch strings.TrimSpace(strings.ToLower(kind)) {
	case KindCode, KindDoc, KindConfig, KindAsset, KindResource, KindURL:
		return true
	default:
		return false
	}
}

// NormalizeKind preserves an explicit caller kind and only infers when the
// field was omitted. Empty paths still fall back to code; preflight owns the
// "path required" error.
func NormalizeKind(explicit, p string) string {
	k := strings.TrimSpace(strings.ToLower(explicit))
	if k != "" {
		return k
	}
	return InferKind(p)
}

// InferKind maps a pin path to the default kind used when callers omit kind.
// The table intentionally stays extension/path based: it is deterministic,
// cheap, and matches the Reader badge vocabulary.
func InferKind(p string) string {
	raw := strings.TrimSpace(p)
	if raw == "" {
		return KindCode
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return KindURL
	}

	base := strings.ToLower(path.Base(strings.ReplaceAll(raw, "\\", "/")))
	ext := strings.ToLower(path.Ext(base))

	switch {
	case hasAnyPrefix(base, "readme", "changelog", "license", "notice", "contributing"):
		return KindDoc
	case base == "dockerfile" || strings.HasPrefix(base, "dockerfile."):
		return KindConfig
	case base == "docker-compose" || strings.HasPrefix(base, "docker-compose.") ||
		base == "makefile" || strings.HasPrefix(base, ".env") ||
		strings.Contains(base, ".config."):
		return KindConfig
	}

	switch ext {
	case ".md", ".mdx", ".markdown", ".txt", ".rst", ".adoc":
		return KindDoc
	case ".json", ".yaml", ".yml", ".toml", ".ini", ".conf":
		return KindConfig
	case ".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".pdf", ".mp4", ".mp3", ".ttf", ".ico":
		return KindAsset
	}
	if strings.HasPrefix(ext, ".woff") {
		return KindAsset
	}
	return KindCode
}

func hasAnyPrefix(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

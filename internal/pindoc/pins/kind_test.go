package pins

import "testing"

func TestInferKind(t *testing.T) {
	cases := map[string]string{
		"README.md":                  KindDoc,
		"docs/guide.mdx":             KindDoc,
		"CHANGELOG":                  KindDoc,
		"LICENSE.txt":                KindDoc,
		"internal/pindoc/db.go":      KindCode,
		"web/src/App.tsx":            KindCode,
		"scripts/check.py":           KindCode,
		"package.json":               KindConfig,
		"docker-compose.yml":         KindConfig,
		"Dockerfile.prod":            KindConfig,
		".env.local":                 KindConfig,
		"vite.config.ts":             KindConfig,
		"assets/logo.png":            KindAsset,
		"docs/spec.pdf":              KindAsset,
		"public/icon.svg":            KindAsset,
		"font/app.woff2":             KindAsset,
		"https://example.com/spec":   KindURL,
		"http://example.com/spec.md": KindURL,
		"cmd/pindoc-server/main.go":  KindCode,
		"":                           KindCode,
	}
	for path, want := range cases {
		if got := InferKind(path); got != want {
			t.Fatalf("InferKind(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestNormalizeKindPreservesExplicit(t *testing.T) {
	if got := NormalizeKind(KindCode, "README.md"); got != KindCode {
		t.Fatalf("explicit kind was not preserved: %q", got)
	}
	if got := NormalizeKind("", "README.md"); got != KindDoc {
		t.Fatalf("omitted kind did not infer doc: %q", got)
	}
}

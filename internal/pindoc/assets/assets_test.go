package assets

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestLocalFSSaveReadImmutableBlob(t *testing.T) {
	store, err := NewLocalFS(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFS: %v", err)
	}
	content := []byte("hello pindoc asset")
	sha := Hash(content)
	key, err := store.Put(context.Background(), sha, content)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if key == "" || strings.Contains(key, "\\") {
		t.Fatalf("storage key = %q, want slash key", key)
	}
	key2, err := store.Put(context.Background(), sha, []byte("different bytes ignored because key exists"))
	if err != nil {
		t.Fatalf("second Put: %v", err)
	}
	if key2 != key {
		t.Fatalf("second key = %q, want %q", key2, key)
	}
	rc, err := store.Open(context.Background(), key)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("stored bytes = %q, want %q", string(got), string(content))
	}
}

func TestValidateContentMIMEAndLimits(t *testing.T) {
	mimeType, err := ValidateContent([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, "shot.png", "")
	if err != nil {
		t.Fatalf("ValidateContent png: %v", err)
	}
	if mimeType != "image/png" {
		t.Fatalf("png mime = %q", mimeType)
	}

	if _, err := ValidateContent([]byte("<?xml version=\"1.0\"?><svg></svg>"), "diagram.svg", ""); err != nil {
		t.Fatalf("svg by extension should be accepted: %v", err)
	}

	_, err = ValidateContent([]byte{0x00, 0x01, 0x02, 0x03}, "blob.bin", "")
	if err == nil {
		t.Fatal("binary blob should be rejected")
	}
	var assetErr *AssetError
	if !asAssetError(err, &assetErr) || assetErr.Code != "ASSET_MIME_UNSUPPORTED" {
		t.Fatalf("unsupported error = %#v", err)
	}
}

func TestServingContentTypeAndInlineSafety(t *testing.T) {
	if !IsImageMime("image/svg+xml") {
		t.Fatal("SVG should remain an image asset for metadata/projection")
	}
	if IsInlineSafeImageMime("image/svg+xml") {
		t.Fatal("SVG should not be served as an inline-safe browser image")
	}
	if !IsInlineSafeImageMime("image/png") {
		t.Fatal("PNG should be inline-safe")
	}

	cases := map[string]string{
		"text/markdown":    "text/markdown; charset=utf-8",
		"text/plain":       "text/plain; charset=utf-8",
		"text/csv":         "text/csv; charset=utf-8",
		"application/json": "application/json; charset=utf-8",
		"image/png":        "image/png",
		"application/pdf":  "application/pdf",
		"application/zip":  "application/zip",
	}
	for in, want := range cases {
		if got := ContentTypeForServing(in); got != want {
			t.Fatalf("ContentTypeForServing(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAssetRefRoundTrip(t *testing.T) {
	ref := Ref("123")
	if ref != "pindoc-asset://123" {
		t.Fatalf("Ref = %q", ref)
	}
	if got := ParseRef(ref + "#caption"); got != "123" {
		t.Fatalf("ParseRef = %q, want 123", got)
	}
}

func asAssetError(err error, target **AssetError) bool {
	if err == nil {
		return false
	}
	if e, ok := err.(*AssetError); ok {
		*target = e
		return true
	}
	return false
}

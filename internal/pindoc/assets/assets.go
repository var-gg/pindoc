// Package assets owns Pindoc's immutable blob storage contract.
package assets

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
)

const (
	DefaultRoot   = "/var/lib/pindoc/assets"
	DriverLocalFS = "localfs"
	RefScheme     = "pindoc-asset://"

	RoleInlineImage     = "inline_image"
	RoleAttachment      = "attachment"
	RoleEvidence        = "evidence"
	RoleGeneratedOutput = "generated_output"

	MaxImageBytes = 20 << 20
	MaxFileBytes  = 50 << 20
)

var (
	ErrInputEmpty      = errors.New("asset input is empty")
	ErrSizeLimit       = errors.New("asset size limit exceeded")
	ErrMimeUnsupported = errors.New("asset mime type unsupported")
)

type AssetError struct {
	Code    string
	Message string
}

func (e *AssetError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

type Metadata struct {
	ID               string
	ProjectID        string
	SHA256           string
	MimeType         string
	SizeBytes        int64
	OriginalFilename string
	StorageDriver    string
	StorageKey       string
	CreatedBy        string
}

type Projection struct {
	AltText       string `json:"alt_text"`
	Caption       string `json:"caption"`
	OCRText       string `json:"ocr_text"`
	LayoutSummary string `json:"layout_summary"`
}

type Storage interface {
	Driver() string
	Put(ctx context.Context, sha256Hex string, content []byte) (string, error)
	Open(ctx context.Context, storageKey string) (io.ReadCloser, error)
}

func NormalizeRoot(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return DefaultRoot
	}
	return root
}

func Ref(assetID string) string {
	assetID = strings.TrimSpace(assetID)
	if assetID == "" {
		return ""
	}
	return RefScheme + assetID
}

func ParseRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if !strings.HasPrefix(ref, RefScheme) {
		return ""
	}
	id := strings.TrimSpace(strings.TrimPrefix(ref, RefScheme))
	if i := strings.IndexAny(id, "?#"); i >= 0 {
		id = id[:i]
	}
	return strings.TrimSpace(id)
}

func BlobPath(projectSlug, assetID string) string {
	projectSlug = strings.TrimSpace(projectSlug)
	assetID = strings.TrimSpace(assetID)
	if projectSlug == "" || assetID == "" {
		return ""
	}
	return "/api/p/" + urlPathEscape(projectSlug) + "/assets/" + urlPathEscape(assetID) + "/blob"
}

func ValidRole(role string) bool {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case RoleInlineImage, RoleAttachment, RoleEvidence, RoleGeneratedOutput:
		return true
	default:
		return false
	}
}

func NormalizeRole(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	if ValidRole(role) {
		return role
	}
	return ""
}

func IsImageMime(mimeType string) bool {
	return strings.HasPrefix(MimeBase(mimeType), "image/")
}

func IsInlineSafeImageMime(mimeType string) bool {
	switch MimeBase(mimeType) {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

func ContentTypeForServing(mimeType string) string {
	base := MimeBase(mimeType)
	switch base {
	case "text/plain", "text/markdown", "text/csv", "application/json":
		return base + "; charset=utf-8"
	default:
		return base
	}
}

func MimeBase(mimeType string) string {
	if base, _, err := mime.ParseMediaType(strings.TrimSpace(mimeType)); err == nil {
		return strings.ToLower(base)
	}
	if i := strings.IndexByte(mimeType, ';'); i >= 0 {
		mimeType = mimeType[:i]
	}
	return strings.ToLower(strings.TrimSpace(mimeType))
}

func Hash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func ValidateContent(content []byte, filename, declaredMime string) (string, error) {
	if len(content) == 0 {
		return "", &AssetError{Code: "ASSET_INPUT_EMPTY", Message: ErrInputEmpty.Error()}
	}
	mimeType := NormalizeMime(content, filename, declaredMime)
	base := MimeBase(mimeType)
	limit := MaxFileBytes
	if strings.HasPrefix(base, "image/") {
		limit = MaxImageBytes
	}
	if len(content) > limit {
		return "", &AssetError{
			Code:    "ASSET_SIZE_LIMIT_EXCEEDED",
			Message: fmt.Sprintf("asset size %d exceeds limit %d for %s", len(content), limit, base),
		}
	}
	if !mimeAllowed(base) {
		return "", &AssetError{
			Code:    "ASSET_MIME_UNSUPPORTED",
			Message: fmt.Sprintf("asset MIME type %q is not allowed", base),
		}
	}
	return base, nil
}

func NormalizeMime(content []byte, filename, declaredMime string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".svg":
		return "image/svg+xml"
	case ".md", ".markdown":
		return "text/markdown"
	case ".pdf":
		return "application/pdf"
	case ".json":
		return "application/json"
	case ".zip":
		return "application/zip"
	}
	detected := "application/octet-stream"
	if len(content) > 0 {
		detected = http.DetectContentType(content)
	}
	base := MimeBase(detected)
	if base == "text/plain" && ext == ".csv" {
		return "text/csv"
	}
	if mimeAllowed(base) {
		return base
	}
	if declared := MimeBase(declaredMime); compatibleDeclaredTextMime(base, declared) {
		return declared
	}
	return base
}

func ProjectionFor(m Metadata) Projection {
	proj := Projection{}
	if IsImageMime(m.MimeType) && strings.TrimSpace(m.OriginalFilename) != "" {
		proj.AltText = m.OriginalFilename
	}
	return proj
}

func compatibleDeclaredTextMime(detected, declared string) bool {
	if detected != "text/plain" || !mimeAllowed(declared) {
		return false
	}
	switch declared {
	case "text/plain", "text/markdown", "text/csv", "application/json":
		return true
	default:
		return false
	}
}

func mimeAllowed(base string) bool {
	switch base {
	case
		"image/png",
		"image/jpeg",
		"image/gif",
		"image/webp",
		"image/svg+xml",
		"application/pdf",
		"text/plain",
		"text/markdown",
		"text/csv",
		"application/json",
		"application/zip":
		return true
	default:
		return false
	}
}

func urlPathEscape(value string) string {
	replacer := strings.NewReplacer(
		"%", "%25",
		"/", "%2F",
		"?", "%3F",
		"#", "%23",
		" ", "%20",
	)
	return replacer.Replace(value)
}

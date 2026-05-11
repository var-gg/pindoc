package tools

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/assets"
	"github.com/var-gg/pindoc/internal/pindoc/auth"
)

type assetUploadInput struct {
	ProjectSlug   string `json:"project_slug,omitempty" jsonschema:"optional projects.slug to scope this call to; omitted uses explicit session/default resolver"`
	LocalPath     string `json:"local_path,omitempty" jsonschema:"local filesystem path to upload; mutually exclusive with bytes_base64/content_base64"`
	BytesBase64   string `json:"bytes_base64,omitempty" jsonschema:"base64-encoded file bytes; mutually exclusive with local_path"`
	ContentBase64 string `json:"content_base64,omitempty" jsonschema:"alias for bytes_base64"`
	Filename      string `json:"filename,omitempty" jsonschema:"original filename stored as metadata; defaults to local_path basename for local uploads"`
	MimeType      string `json:"mime_type,omitempty" jsonschema:"optional declared MIME type; server verifies against allowlist and extension/detected content"`
}

type assetReadInput struct {
	ProjectSlug string `json:"project_slug,omitempty" jsonschema:"optional projects.slug to scope this call to; omitted uses explicit session/default resolver"`
	AssetID     string `json:"asset_id" jsonschema:"asset UUID, sha256, or pindoc-asset:// UUID reference"`
}

type assetAttachInput struct {
	ProjectSlug  string `json:"project_slug,omitempty" jsonschema:"optional projects.slug to scope this call to; omitted uses explicit session/default resolver"`
	AssetID      string `json:"asset_id" jsonschema:"asset UUID, sha256, or pindoc-asset:// UUID reference"`
	Artifact     string `json:"artifact" jsonschema:"artifact UUID or slug whose current head revision receives the asset relation"`
	Role         string `json:"role" jsonschema:"one of inline_image | attachment | evidence | generated_output"`
	ExpectedHead int    `json:"expected_head,omitempty" jsonschema:"optional current artifact revision_number guard; omitted attaches to current head"`
}

const assetUploadToolDescription = "Create a project-scoped Asset from a local file path or base64 bytes. local_path is evaluated on the MCP server host/container and is loopback-only; non-loopback OAuth callers must send bytes_base64/content_base64. In Docker Desktop on Windows, host paths such as A:\\path\\image.png are not visible inside the Linux container; copy the file with tools/push-asset.ps1 or docker cp to a container path such as /tmp/pindoc-asset-upload/... and pass that container path as local_path. Stores an immutable LocalFS blob under PINDOC_ASSET_ROOT (default /var/lib/pindoc/assets), records metadata only, returns asset.blob_url plus a stable pindoc-asset:// reference, and never exposes storage_key/local paths."

const assetAttachToolDescription = "Attach an Asset to an artifact's current head revision as inline_image, attachment, evidence, or generated_output. The relation is revision-scoped; repeated identical attaches are idempotent. Attachment is metadata only: it does not insert image Markdown into body_markdown. For a Reader-visible inline image, first upload the asset, then include Markdown such as ![alt](asset.blob_url) in the artifact body and attach the same asset with role=inline_image."

type assetToolOutput struct {
	Status           string                 `json:"status"`
	ErrorCode        string                 `json:"error_code,omitempty"`
	ErrorCodes       []string               `json:"error_codes,omitempty"`
	Failed           []string               `json:"failed,omitempty"`
	Checklist        []string               `json:"checklist,omitempty"`
	ChecklistItems   []ErrorChecklistItem   `json:"checklist_items,omitempty"`
	SuggestedActions []string               `json:"suggested_actions,omitempty"`
	MessageLocale    string                 `json:"message_locale,omitempty"`
	ProjectSlug      string                 `json:"project_slug,omitempty"`
	Warnings         []string               `json:"warnings,omitempty"`
	Asset            *assetSummary          `json:"asset,omitempty"`
	AssetRef         string                 `json:"asset_ref,omitempty"`
	Reused           bool                   `json:"reused,omitempty"`
	Attachment       *assetAttachmentResult `json:"attachment,omitempty"`
	Projection       *assets.Projection     `json:"projection,omitempty"`
	ToolsetVersion   string                 `json:"toolset_version,omitempty"`
}

type assetSummary struct {
	ID               string            `json:"id"`
	SHA256           string            `json:"sha256"`
	MimeType         string            `json:"mime_type"`
	SizeBytes        int64             `json:"size_bytes"`
	OriginalFilename string            `json:"original_filename,omitempty"`
	StorageDriver    string            `json:"storage_driver"`
	BlobURL          string            `json:"blob_url"`
	IsImage          bool              `json:"is_image"`
	Projection       assets.Projection `json:"projection"`
	CreatedBy        string            `json:"created_by,omitempty"`
	CreatedAt        time.Time         `json:"created_at,omitzero"`
}

type assetAttachmentResult struct {
	ID             int64  `json:"id"`
	ArtifactID     string `json:"artifact_id"`
	ArtifactSlug   string `json:"artifact_slug"`
	RevisionID     string `json:"revision_id"`
	RevisionNumber int    `json:"revision_number"`
	Role           string `json:"role"`
	DisplayOrder   int    `json:"display_order"`
}

func RegisterAssetUpload(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name:        "pindoc.asset.upload",
			Description: assetUploadToolDescription,
		},
		func(ctx context.Context, p *auth.Principal, in assetUploadInput) (*sdk.CallToolResult, assetToolOutput, error) {
			scope, err := auth.ResolveProject(ctx, deps.DB, p, in.ProjectSlug)
			if err != nil {
				return nil, assetToolOutput{}, fmt.Errorf("asset.upload: %w", err)
			}
			if !scope.Can("write.artifact") {
				return nil, assetNotReady(scope.ProjectSlug, "PROJECT_WRITE_REQUIRED", "write access is required to upload project assets"), nil
			}
			content, filename, out := decodeAssetUploadInput(in, p)
			if out.Status == "not_ready" {
				out.ProjectSlug = scope.ProjectSlug
				return nil, out, nil
			}
			mimeType, err := assets.ValidateContent(content, filename, in.MimeType)
			if err != nil {
				return nil, assetValidationOutput(scope.ProjectSlug, err), nil
			}
			sha := assets.Hash(content)
			if existing, err := loadAssetSummary(ctx, deps, scope.ProjectID, scope.ProjectSlug, sha); err == nil {
				return nil, assetToolOutput{
					Status:      "accepted",
					ProjectSlug: scope.ProjectSlug,
					Asset:       &existing,
					AssetRef:    assets.Ref(existing.ID),
					Projection:  &existing.Projection,
					Reused:      true,
				}, nil
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return nil, assetToolOutput{}, err
			}

			store, err := assetStorage(deps)
			if err != nil {
				return nil, assetToolOutput{}, err
			}
			key, err := store.Put(ctx, sha, content)
			if err != nil {
				return nil, assetToolOutput{}, err
			}
			actor := principalActor(p)
			var assetID string
			var createdAt time.Time
			err = deps.DB.QueryRow(ctx, `
				INSERT INTO assets (
					project_id, sha256, mime_type, size_bytes, original_filename,
					storage_driver, storage_key, created_by, created_by_user_id
				)
				VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, '')::uuid)
				ON CONFLICT (project_id, sha256) DO NOTHING
				RETURNING id::text, created_at
			`, scope.ProjectID, sha, mimeType, int64(len(content)), filename, store.Driver(), key, actor, assetPrincipalUserID(p)).Scan(&assetID, &createdAt)
			reused := false
			if errors.Is(err, pgx.ErrNoRows) {
				reused = true
				existing, loadErr := loadAssetSummary(ctx, deps, scope.ProjectID, scope.ProjectSlug, sha)
				if loadErr != nil {
					return nil, assetToolOutput{}, loadErr
				}
				return nil, assetToolOutput{
					Status:      "accepted",
					ProjectSlug: scope.ProjectSlug,
					Asset:       &existing,
					AssetRef:    assets.Ref(existing.ID),
					Projection:  &existing.Projection,
					Reused:      reused,
				}, nil
			}
			if err != nil {
				return nil, assetToolOutput{}, err
			}
			metadata := assets.Metadata{
				ID:               assetID,
				ProjectID:        scope.ProjectID,
				SHA256:           sha,
				MimeType:         mimeType,
				SizeBytes:        int64(len(content)),
				OriginalFilename: filename,
				StorageDriver:    store.Driver(),
				CreatedBy:        actor,
			}
			summary := assetSummaryFromMetadata(scope.ProjectSlug, metadata, createdAt)
			return nil, assetToolOutput{
				Status:      "accepted",
				ProjectSlug: scope.ProjectSlug,
				Asset:       &summary,
				AssetRef:    assets.Ref(summary.ID),
				Projection:  &summary.Projection,
				Reused:      reused,
			}, nil
		},
	)
}

func RegisterAssetRead(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name:        "pindoc.asset.read",
			Description: "Read project-scoped Asset metadata and the agent-readable projection placeholder. Accepts asset UUID, sha256, or pindoc-asset:// UUID; returns blob_url and metadata but never returns storage_key or a local path.",
		},
		func(ctx context.Context, p *auth.Principal, in assetReadInput) (*sdk.CallToolResult, assetToolOutput, error) {
			scope, err := auth.ResolveProject(ctx, deps.DB, p, in.ProjectSlug)
			if err != nil {
				return nil, assetToolOutput{}, fmt.Errorf("asset.read: %w", err)
			}
			if !scope.Can("read.artifact") {
				return nil, assetNotReady(scope.ProjectSlug, "PROJECT_READ_REQUIRED", "read access is required to inspect project assets"), nil
			}
			assetID := normalizeAssetLookup(in.AssetID)
			if assetID == "" {
				return nil, assetNotReady(scope.ProjectSlug, "ASSET_ID_REQUIRED", "asset_id is required"), nil
			}
			summary, err := loadAssetSummary(ctx, deps, scope.ProjectID, scope.ProjectSlug, assetID)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, assetNotReady(scope.ProjectSlug, "ASSET_NOT_FOUND", "asset not found in this project"), nil
			}
			if err != nil {
				return nil, assetToolOutput{}, err
			}
			return nil, assetToolOutput{
				Status:      "accepted",
				ProjectSlug: scope.ProjectSlug,
				Asset:       &summary,
				AssetRef:    assets.Ref(summary.ID),
				Projection:  &summary.Projection,
			}, nil
		},
	)
}

func RegisterAssetAttach(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name:        "pindoc.asset.attach",
			Description: assetAttachToolDescription,
		},
		func(ctx context.Context, p *auth.Principal, in assetAttachInput) (*sdk.CallToolResult, assetToolOutput, error) {
			scope, err := auth.ResolveProject(ctx, deps.DB, p, in.ProjectSlug)
			if err != nil {
				return nil, assetToolOutput{}, fmt.Errorf("asset.attach: %w", err)
			}
			if !scope.Can("write.artifact") {
				return nil, assetNotReady(scope.ProjectSlug, "PROJECT_WRITE_REQUIRED", "write access is required to attach project assets"), nil
			}
			role := assets.NormalizeRole(in.Role)
			if role == "" {
				return nil, assetNotReady(scope.ProjectSlug, "ASSET_ROLE_INVALID", "role must be inline_image, attachment, evidence, or generated_output"), nil
			}
			assetID := normalizeAssetLookup(in.AssetID)
			if assetID == "" {
				return nil, assetNotReady(scope.ProjectSlug, "ASSET_ID_REQUIRED", "asset_id is required"), nil
			}
			summary, err := loadAssetSummary(ctx, deps, scope.ProjectID, scope.ProjectSlug, assetID)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, assetNotReady(scope.ProjectSlug, "ASSET_NOT_FOUND", "asset not found in this project"), nil
			}
			if err != nil {
				return nil, assetToolOutput{}, err
			}
			artifactRef := strings.TrimSpace(in.Artifact)
			if artifactRef == "" {
				return nil, assetNotReady(scope.ProjectSlug, "ARTIFACT_REQUIRED", "artifact slug or id is required"), nil
			}
			head, err := loadArtifactHeadForAsset(ctx, deps, scope.ProjectID, artifactRef)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, assetNotReady(scope.ProjectSlug, "ARTIFACT_NOT_FOUND", "artifact not found in this project"), nil
			}
			if err != nil {
				return nil, assetToolOutput{}, err
			}
			if in.ExpectedHead > 0 && in.ExpectedHead != head.RevisionNumber {
				return nil, assetNotReady(scope.ProjectSlug, "ARTIFACT_HEAD_CONFLICT", "artifact head revision changed before attach"), nil
			}
			attachment, err := insertAssetAttachment(ctx, deps, head, summary.ID, role, principalActor(p))
			if err != nil {
				return nil, assetToolOutput{}, err
			}
			warnings, err := loadAssetVisibilityWarnings(ctx, deps, scope.ProjectID, summary.ID, head)
			if err != nil {
				return nil, assetToolOutput{}, err
			}
			return nil, assetToolOutput{
				Status:      "accepted",
				ProjectSlug: scope.ProjectSlug,
				Warnings:    warnings,
				Asset:       &summary,
				AssetRef:    assets.Ref(summary.ID),
				Projection:  &summary.Projection,
				Attachment:  &attachment,
			}, nil
		},
	)
}

type artifactHeadForAsset struct {
	ArtifactID     string
	ArtifactSlug   string
	RevisionID     string
	RevisionNumber int
	Visibility     string
}

func decodeAssetUploadInput(in assetUploadInput, principal *auth.Principal) ([]byte, string, assetToolOutput) {
	localPath := strings.TrimSpace(in.LocalPath)
	bytesB64 := strings.TrimSpace(firstNonEmptyString(in.BytesBase64, in.ContentBase64))
	if localPath == "" && bytesB64 == "" {
		return nil, "", assetNotReady("", "ASSET_INPUT_REQUIRED", "provide local_path or bytes_base64")
	}
	if localPath != "" && bytesB64 != "" {
		return nil, "", assetNotReady("", "ASSET_INPUT_CONFLICT", "local_path and bytes_base64 are mutually exclusive")
	}
	filename := strings.TrimSpace(in.Filename)
	if localPath != "" {
		if !principal.IsLoopback() {
			return nil, "", assetNotReady("", "ASSET_LOCAL_PATH_LOOPBACK_ONLY", "local_path is restricted to loopback callers; use bytes_base64 from non-loopback agents")
		}
		content, err := os.ReadFile(localPath)
		if err != nil {
			return nil, "", assetNotReady("", "ASSET_LOCAL_READ_FAILED", "local_path could not be read")
		}
		if filename == "" {
			filename = filepath.Base(localPath)
		}
		return content, filename, assetToolOutput{}
	}
	content, err := base64.StdEncoding.DecodeString(bytesB64)
	if err != nil {
		return nil, "", assetNotReady("", "ASSET_BYTES_BASE64_INVALID", "bytes_base64 must be valid standard base64")
	}
	return content, filename, assetToolOutput{}
}

func assetStorage(deps Deps) (*assets.LocalFS, error) {
	root := deps.AssetRoot
	store, err := assets.NewLocalFS(root)
	if err != nil {
		if deps.Logger != nil {
			deps.Logger.Error("asset storage init failed", "err", err, "root", root)
		} else {
			slog.Default().Error("asset storage init failed", "err", err, "root", root)
		}
		return nil, err
	}
	return store, nil
}

func loadAssetSummary(ctx context.Context, deps Deps, projectID, projectSlug, ref string) (assetSummary, error) {
	ref = normalizeAssetLookup(ref)
	var m assets.Metadata
	var createdAt time.Time
	err := deps.DB.QueryRow(ctx, `
		SELECT id::text, project_id::text, sha256, mime_type, size_bytes,
		       original_filename, storage_driver, created_by, created_at
		  FROM assets
		 WHERE project_id = $1::uuid
		   AND (id::text = $2 OR sha256 = lower($2))
		 LIMIT 1
	`, projectID, ref).Scan(
		&m.ID, &m.ProjectID, &m.SHA256, &m.MimeType, &m.SizeBytes,
		&m.OriginalFilename, &m.StorageDriver, &m.CreatedBy, &createdAt,
	)
	if err != nil {
		return assetSummary{}, err
	}
	return assetSummaryFromMetadata(projectSlug, m, createdAt), nil
}

func assetSummaryFromMetadata(projectSlug string, m assets.Metadata, createdAt time.Time) assetSummary {
	projection := assets.ProjectionFor(m)
	return assetSummary{
		ID:               m.ID,
		SHA256:           m.SHA256,
		MimeType:         m.MimeType,
		SizeBytes:        m.SizeBytes,
		OriginalFilename: m.OriginalFilename,
		StorageDriver:    m.StorageDriver,
		BlobURL:          assets.BlobPath(projectSlug, m.ID),
		IsImage:          assets.IsImageMime(m.MimeType),
		Projection:       projection,
		CreatedBy:        m.CreatedBy,
		CreatedAt:        createdAt,
	}
}

func loadArtifactHeadForAsset(ctx context.Context, deps Deps, projectID, artifactRef string) (artifactHeadForAsset, error) {
	var out artifactHeadForAsset
	err := deps.DB.QueryRow(ctx, `
		SELECT a.id::text, a.slug, r.id::text, r.revision_number, a.visibility
		  FROM artifacts a
		  JOIN artifact_revisions r ON r.artifact_id = a.id
		 WHERE a.project_id = $1::uuid
		   AND a.status <> 'archived'
		   AND (a.id::text = $2 OR a.slug = $2)
		 ORDER BY r.revision_number DESC
		 LIMIT 1
	`, projectID, strings.TrimSpace(artifactRef)).Scan(
		&out.ArtifactID, &out.ArtifactSlug, &out.RevisionID, &out.RevisionNumber, &out.Visibility,
	)
	return out, err
}

func loadAssetVisibilityWarnings(ctx context.Context, deps Deps, projectID, assetID string, head artifactHeadForAsset) ([]string, error) {
	rows, err := deps.DB.Query(ctx, `
		SELECT DISTINCT art.visibility
		  FROM artifact_assets aa
		  JOIN artifact_revisions ar ON ar.id = aa.artifact_revision_id
		  JOIN artifacts art ON art.id = aa.artifact_id
		 WHERE aa.asset_id = $1::uuid
		   AND art.project_id = $2::uuid
		   AND art.id <> $3::uuid
		   AND art.status <> 'archived'
		   AND ar.revision_number = (
				SELECT max(r2.revision_number)
				  FROM artifact_revisions r2
				 WHERE r2.artifact_id = art.id
		   )
	`, assetID, projectID, head.ArtifactID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seen := map[string]bool{}
	for rows.Next() {
		var visibility string
		if err := rows.Scan(&visibility); err != nil {
			return nil, err
		}
		seen[strings.TrimSpace(visibility)] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	switch strings.TrimSpace(head.Visibility) {
	case "public":
		if seen["org"] || seen["private"] {
			return []string{"ASSET_SHARED_PUBLIC: this immutable asset is also attached to less-public artifacts; attaching it to a public artifact makes the shared blob publicly readable."}, nil
		}
	default:
		if seen["public"] {
			return []string{"ASSET_ALREADY_PUBLIC: this immutable asset is already attached to a public artifact, so its blob URL is publicly readable."}, nil
		}
	}
	return nil, nil
}

func insertAssetAttachment(ctx context.Context, deps Deps, head artifactHeadForAsset, assetID, role, actor string) (assetAttachmentResult, error) {
	var out assetAttachmentResult
	err := deps.DB.QueryRow(ctx, `
		WITH next_order AS (
			SELECT COALESCE(max(display_order) + 1, 0) AS n
			  FROM artifact_assets
			 WHERE artifact_revision_id = $1::uuid AND role = $3
		), inserted AS (
			INSERT INTO artifact_assets (
				artifact_id, artifact_revision_id, asset_id, role, display_order, created_by
			)
			SELECT $2::uuid, $1::uuid, $4::uuid, $3, n, $5
			  FROM next_order
			ON CONFLICT (artifact_revision_id, asset_id, role)
			DO UPDATE SET role = EXCLUDED.role
			RETURNING id, display_order
		)
		SELECT id, display_order FROM inserted
	`, head.RevisionID, head.ArtifactID, role, assetID, actor).Scan(&out.ID, &out.DisplayOrder)
	if err != nil {
		return out, err
	}
	out.ArtifactID = head.ArtifactID
	out.ArtifactSlug = head.ArtifactSlug
	out.RevisionID = head.RevisionID
	out.RevisionNumber = head.RevisionNumber
	out.Role = role
	return out, nil
}

func assetValidationOutput(projectSlug string, err error) assetToolOutput {
	var assetErr *assets.AssetError
	if errors.As(err, &assetErr) && assetErr.Code != "" {
		return assetNotReady(projectSlug, assetErr.Code, assetErr.Message)
	}
	return assetNotReady(projectSlug, "ASSET_INVALID", err.Error())
}

func assetNotReady(projectSlug, code, message string) assetToolOutput {
	code = strings.TrimSpace(code)
	if code == "" {
		code = "ASSET_NOT_READY"
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = code
	}
	return assetToolOutput{
		Status:           "not_ready",
		ErrorCode:        code,
		Failed:           []string{code},
		Checklist:        []string{message},
		SuggestedActions: []string{message},
		ProjectSlug:      projectSlug,
	}
}

func normalizeAssetLookup(ref string) string {
	ref = strings.TrimSpace(ref)
	if id := assets.ParseRef(ref); id != "" {
		return id
	}
	return ref
}

func principalActor(p *auth.Principal) string {
	if p == nil {
		return ""
	}
	if strings.TrimSpace(p.AgentID) != "" {
		return strings.TrimSpace(p.AgentID)
	}
	if strings.TrimSpace(p.UserID) != "" {
		return "user:" + strings.TrimSpace(p.UserID)
	}
	return strings.TrimSpace(p.Source)
}

func assetPrincipalUserID(p *auth.Principal) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(p.UserID)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

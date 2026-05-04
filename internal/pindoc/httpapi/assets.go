package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/var-gg/pindoc/internal/pindoc/assets"
	pauth "github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

type assetDetailRow struct {
	ID               string            `json:"id"`
	AssetRef         string            `json:"asset_ref"`
	Role             string            `json:"role"`
	MimeType         string            `json:"mime_type"`
	SizeBytes        int64             `json:"size_bytes"`
	OriginalFilename string            `json:"original_filename,omitempty"`
	BlobURL          string            `json:"blob_url"`
	IsImage          bool              `json:"is_image"`
	Projection       assets.Projection `json:"projection"`
	CrossVisibility  []string          `json:"cross_visibility,omitempty"`
	DisplayOrder     int               `json:"display_order"`
	CreatedBy        string            `json:"created_by,omitempty"`
	CreatedAt        time.Time         `json:"created_at,omitzero"`
}

type assetBlobRow struct {
	ID               string
	ProjectID        string
	SHA256           string
	MimeType         string
	SizeBytes        int64
	OriginalFilename string
	StorageDriver    string
	StorageKey       string
	CreatedAt        time.Time
}

type assetAPIError struct {
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
}

func writeAssetAPIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, assetAPIError{ErrorCode: code, Message: message})
}

type assetBlobAccess string

const (
	assetBlobAccessDenied    assetBlobAccess = ""
	assetBlobAccessPrincipal assetBlobAccess = "principal"
	assetBlobAccessPublic    assetBlobAccess = "public"

	assetBlobCSP = "default-src 'none'; sandbox; img-src 'self'; style-src 'none'; script-src 'none'; base-uri 'none'"
)

func (a assetBlobAccess) canRead() bool {
	return a == assetBlobAccessPrincipal || a == assetBlobAccessPublic
}

func (d Deps) handleAssetBlob(w http.ResponseWriter, r *http.Request) {
	projectSlug := projectSlugFrom(r)
	assetID := strings.TrimSpace(r.PathValue("assetID"))
	if projectSlug == "" || assetID == "" {
		writeAssetAPIError(w, http.StatusBadRequest, "BAD_PATH", "project and assetID are required")
		return
	}
	row, err := d.lookupAssetBlob(r.Context(), projectSlug, assetID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeAssetAPIError(w, http.StatusNotFound, "ASSET_NOT_FOUND", "asset not found")
		return
	}
	if err != nil {
		if d.Logger != nil {
			d.Logger.Error("asset blob lookup", "err", err, "project", projectSlug, "asset_id", assetID)
		}
		writeAssetAPIError(w, http.StatusInternalServerError, "ASSET_LOOKUP_FAILED", "asset lookup failed")
		return
	}
	if access, err := d.canReadAssetBlob(r.Context(), r, projectSlug, row); err != nil {
		if d.Logger != nil {
			d.Logger.Warn("asset blob permission lookup failed", "err", err, "project", projectSlug, "asset_id", assetID)
		}
		writeAssetAPIError(w, http.StatusInternalServerError, "ASSET_ACCESS_CHECK_FAILED", "asset access check failed")
		return
	} else if !access.canRead() {
		writeAssetAPIError(w, http.StatusNotFound, "ASSET_NOT_FOUND", "asset not found")
		return
	} else {
		applyAssetBlobCacheHeaders(w, access)
	}
	if row.StorageDriver != assets.DriverLocalFS {
		if d.Logger != nil {
			d.Logger.Warn("asset blob storage driver unsupported", "project", projectSlug, "asset_id", row.ID, "driver", row.StorageDriver)
		}
		writeAssetAPIError(w, http.StatusNotFound, "ASSET_NOT_FOUND", "asset not found")
		return
	}
	store, err := assets.NewLocalFS(d.AssetRoot)
	if err != nil {
		if d.Logger != nil {
			d.Logger.Error("asset localfs init", "err", err)
		}
		writeAssetAPIError(w, http.StatusInternalServerError, "ASSET_STORAGE_UNAVAILABLE", "asset storage unavailable")
		return
	}
	rc, err := store.Open(r.Context(), row.StorageKey)
	if errors.Is(err, os.ErrNotExist) {
		if d.Logger != nil {
			d.Logger.Warn("asset blob storage key missing", "project", projectSlug, "asset_id", row.ID, "storage_key", row.StorageKey)
		}
		writeAssetAPIError(w, http.StatusNotFound, "ASSET_NOT_FOUND", "asset not found")
		return
	}
	if err != nil {
		if d.Logger != nil {
			d.Logger.Error("asset localfs open", "err", err, "asset_id", row.ID)
		}
		writeAssetAPIError(w, http.StatusInternalServerError, "ASSET_BLOB_OPEN_FAILED", "asset blob open failed")
		return
	}
	defer rc.Close()
	seeker, ok := rc.(io.ReadSeeker)
	if !ok {
		if d.Logger != nil {
			d.Logger.Error("asset localfs open returned non-seeker", "asset_id", row.ID)
		}
		writeAssetAPIError(w, http.StatusInternalServerError, "ASSET_BLOB_OPEN_FAILED", "asset blob open failed")
		return
	}

	w.Header().Set("Content-Type", assets.ContentTypeForServing(row.MimeType))
	w.Header().Set("Content-Disposition", contentDisposition(row.OriginalFilename, assets.IsInlineSafeImageMime(row.MimeType)))
	w.Header().Set("Content-Security-Policy", assetBlobCSP)
	if row.SHA256 != "" {
		w.Header().Set("ETag", `W/"`+row.SHA256+`"`)
	}
	http.ServeContent(w, r, row.OriginalFilename, row.CreatedAt, seeker)
}

func applyAssetBlobCacheHeaders(w http.ResponseWriter, access assetBlobAccess) {
	switch access {
	case assetBlobAccessPublic:
		w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
	case assetBlobAccessPrincipal:
		w.Header().Set("Cache-Control", "private, no-cache")
	}
}

func (d Deps) lookupAssetBlob(ctx context.Context, projectSlug, assetID string) (assetBlobRow, error) {
	var row assetBlobRow
	err := d.DB.QueryRow(ctx, `
		SELECT a.id::text, a.project_id::text, a.sha256, a.mime_type, a.size_bytes,
		       a.original_filename, a.storage_driver, a.storage_key, a.created_at
		  FROM assets a
		  JOIN projects p ON p.id = a.project_id
		 WHERE p.slug = $1
		   AND a.id::text = $2
		 LIMIT 1
	`, projectSlug, assetID).Scan(
		&row.ID, &row.ProjectID, &row.SHA256, &row.MimeType, &row.SizeBytes,
		&row.OriginalFilename, &row.StorageDriver, &row.StorageKey, &row.CreatedAt,
	)
	return row, err
}

func (d Deps) canReadAssetBlob(ctx context.Context, r *http.Request, projectSlug string, row assetBlobRow) (assetBlobAccess, error) {
	principal := d.principalForRequest(r)
	if principal != nil {
		if principal.IsLoopback() {
			return assetBlobAccessPrincipal, nil
		}
		if scope, err := pauth.ResolveProject(ctx, d.DB, principal, projectSlug); err == nil && scope.Can("read.artifact") {
			return assetBlobAccessPrincipal, nil
		} else if err != nil && !errors.Is(err, pauth.ErrProjectAccessDenied) && !errors.Is(err, pauth.ErrProjectNotFound) {
			return assetBlobAccessDenied, err
		}
	}
	var public bool
	err := d.DB.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			  FROM artifact_assets aa
			  JOIN artifact_revisions ar ON ar.id = aa.artifact_revision_id
			  JOIN artifacts art ON art.id = aa.artifact_id
			 WHERE aa.asset_id = $1::uuid
			   AND art.project_id = $2::uuid
			   AND art.status <> 'archived'
			   AND art.visibility = $3
			   AND ar.revision_number = (
					SELECT max(r2.revision_number)
					  FROM artifact_revisions r2
					 WHERE r2.artifact_id = art.id
			   )
		)
	`, row.ID, row.ProjectID, projects.VisibilityPublic).Scan(&public)
	if err != nil {
		return assetBlobAccessDenied, err
	}
	if public {
		return assetBlobAccessPublic, nil
	}
	return assetBlobAccessDenied, nil
}

func (d Deps) loadArtifactAssets(ctx context.Context, projectSlug, artifactID string, revisionNumber int) ([]assetDetailRow, error) {
	if revisionNumber <= 0 {
		return nil, nil
	}
	rows, err := d.DB.Query(ctx, `
		SELECT aa.id, ast.id::text, ast.mime_type, ast.size_bytes,
		       ast.original_filename, aa.role, aa.display_order,
		       ast.created_by, ast.created_at,
		       ARRAY(
					SELECT visibility
					  FROM (
							SELECT DISTINCT other_art.visibility
							  FROM artifact_assets aa_other
							  JOIN artifact_revisions ar_other ON ar_other.id = aa_other.artifact_revision_id
							  JOIN artifacts other_art ON other_art.id = aa_other.artifact_id
							 WHERE aa_other.asset_id = aa.asset_id
							   AND other_art.project_id = cur.project_id
							   AND other_art.id <> cur.id
							   AND other_art.status <> 'archived'
							   AND other_art.visibility <> cur.visibility
							   AND ar_other.revision_number = (
									SELECT max(r2.revision_number)
									  FROM artifact_revisions r2
									 WHERE r2.artifact_id = other_art.id
							   )
					  ) cross_visibility
					 ORDER BY CASE visibility
						 WHEN 'public' THEN 1
						 WHEN 'org' THEN 2
						 WHEN 'private' THEN 3
						 ELSE 4
					 END
		       ) AS cross_visibility
		  FROM artifact_assets aa
		  JOIN assets ast ON ast.id = aa.asset_id
		  JOIN artifact_revisions ar ON ar.id = aa.artifact_revision_id
		  JOIN artifacts cur ON cur.id = aa.artifact_id
		 WHERE aa.artifact_id = $1::uuid
		   AND ar.revision_number = $2
		 ORDER BY
		   CASE aa.role
			 WHEN 'inline_image' THEN 0
			 WHEN 'attachment' THEN 1
			 WHEN 'evidence' THEN 2
			 ELSE 3
		   END,
		   aa.display_order,
		   aa.id
	`, artifactID, revisionNumber)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []assetDetailRow
	for rows.Next() {
		var row assetDetailRow
		var relationID int64
		var createdAt time.Time
		if err := rows.Scan(
			&relationID, &row.ID, &row.MimeType, &row.SizeBytes,
			&row.OriginalFilename, &row.Role, &row.DisplayOrder,
			&row.CreatedBy, &createdAt, &row.CrossVisibility,
		); err != nil {
			return nil, err
		}
		row.AssetRef = assets.Ref(row.ID)
		row.BlobURL = assets.BlobPath(projectSlug, row.ID)
		row.IsImage = assets.IsImageMime(row.MimeType)
		row.Projection = assets.ProjectionFor(assets.Metadata{
			ID:               row.ID,
			MimeType:         row.MimeType,
			SizeBytes:        row.SizeBytes,
			OriginalFilename: row.OriginalFilename,
			CreatedBy:        row.CreatedBy,
		})
		row.CreatedAt = createdAt
		out = append(out, row)
	}
	return out, rows.Err()
}

func contentDisposition(filename string, inline bool) string {
	disposition := "attachment"
	if inline {
		disposition = "inline"
	}
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return disposition
	}
	escaped := strings.ReplaceAll(filename, `\`, `_`)
	escaped = strings.ReplaceAll(escaped, `"`, `_`)
	if asciiOnly(escaped) {
		return fmt.Sprintf(`%s; filename="%s"`, disposition, escaped)
	}
	return fmt.Sprintf(`%s; filename*=UTF-8''%s`, disposition, url.PathEscape(filename))
}

func asciiOnly(value string) bool {
	for _, r := range value {
		if r < 32 || r > 126 {
			return false
		}
	}
	return true
}

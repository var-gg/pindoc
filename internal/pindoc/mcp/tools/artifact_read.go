package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type artifactReadInput struct {
	// One of IDOrSlug (UUID or project-scoped slug) must be set.
	// URLs coming from Wiki Reader share links (pindoc://... or
	// https://pindoc.org/a/<id>) are accepted here too and normalized
	// server-side — agents shouldn't have to parse Pindoc's URL shape.
	IDOrSlug string `json:"id_or_slug" jsonschema:"artifact UUID, slug, or share URL (pindoc://... or https://.../a/ID)"`
}

type artifactReadOutput struct {
	ID              string    `json:"id"`
	ProjectSlug     string    `json:"project_slug"`
	AreaSlug        string    `json:"area_slug"`
	Slug            string    `json:"slug"`
	Type            string    `json:"type"`
	Title           string    `json:"title"`
	BodyMarkdown    string    `json:"body_markdown"`
	Tags            []string  `json:"tags"`
	Completeness    string    `json:"completeness"`
	Status          string    `json:"status"`
	ReviewState     string    `json:"review_state"`
	AuthorKind      string    `json:"author_kind"`
	AuthorID        string    `json:"author_id"`
	AuthorVersion   string    `json:"author_version,omitempty"`
	SupersededBy    string    `json:"superseded_by,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	PublishedAt     time.Time `json:"published_at,omitzero"`
}

// RegisterArtifactRead wires pindoc.artifact.read.
func RegisterArtifactRead(server *sdk.Server, deps Deps) {
	sdk.AddTool(server,
		&sdk.Tool{
			Name:        "pindoc.artifact.read",
			Description: "Fetch a single artifact by UUID, project-scoped slug, or share URL. Use this after pindoc.artifact.search hits to pull the full body, or when a user pastes a Pindoc URL into chat and you need the canonical content.",
		},
		func(ctx context.Context, _ *sdk.CallToolRequest, in artifactReadInput) (*sdk.CallToolResult, artifactReadOutput, error) {
			idOrSlug := normalizeRef(in.IDOrSlug)
			if idOrSlug == "" {
				return nil, artifactReadOutput{}, errors.New("id_or_slug is required")
			}

			var out artifactReadOutput
			var desc, authorVer, superseded *string
			var publishedAt *time.Time
			err := deps.DB.QueryRow(ctx, `
				SELECT
					a.id::text,
					proj.slug,
					area.slug,
					a.slug,
					a.type,
					a.title,
					a.body_markdown,
					a.tags,
					a.completeness,
					a.status,
					a.review_state,
					a.author_kind,
					a.author_id,
					a.author_version,
					a.superseded_by::text,
					a.created_at,
					a.updated_at,
					a.published_at
				FROM artifacts a
				JOIN projects proj ON proj.id = a.project_id
				JOIN areas    area ON area.id = a.area_id
				WHERE proj.slug = $1
				  AND (a.id::text = $2 OR a.slug = $2)
				LIMIT 1
			`, deps.ProjectSlug, idOrSlug).Scan(
				&out.ID, &out.ProjectSlug, &out.AreaSlug, &out.Slug,
				&out.Type, &out.Title, &out.BodyMarkdown, &out.Tags,
				&out.Completeness, &out.Status, &out.ReviewState,
				&out.AuthorKind, &out.AuthorID, &authorVer, &superseded,
				&out.CreatedAt, &out.UpdatedAt, &publishedAt,
			)
			_ = desc // reserved; project.description not part of read response
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, artifactReadOutput{}, fmt.Errorf("artifact %q not found in project %q", in.IDOrSlug, deps.ProjectSlug)
			}
			if err != nil {
				return nil, artifactReadOutput{}, fmt.Errorf("read: %w", err)
			}
			if authorVer != nil {
				out.AuthorVersion = *authorVer
			}
			if superseded != nil {
				out.SupersededBy = *superseded
			}
			if publishedAt != nil {
				out.PublishedAt = *publishedAt
			}
			return nil, out, nil
		},
	)
}

// normalizeRef strips a Pindoc share URL down to the ID/slug the caller
// actually wanted. Plain IDs/slugs pass through unchanged.
//
// Recognised shapes:
//   pindoc://<id_or_slug>
//   https://<host>/a/<id_or_slug>
//   http://<host>/a/<id_or_slug>
//   <id_or_slug>
func normalizeRef(raw string) string {
	s := strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(s, "pindoc://"):
		return strings.TrimPrefix(s, "pindoc://")
	case strings.Contains(s, "://"):
		// http(s)://host/a/<tail>
		if i := strings.LastIndex(s, "/a/"); i >= 0 {
			return s[i+3:]
		}
	}
	return s
}

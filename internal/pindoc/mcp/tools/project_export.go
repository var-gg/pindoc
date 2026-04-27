package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/projectexport"
)

type projectExportInput struct {
	ProjectSlug      string   `json:"project_slug" jsonschema:"projects.slug to scope this call to"`
	Areas            []string `json:"areas,omitempty" jsonschema:"optional area_slug filters"`
	Slugs            []string `json:"slugs,omitempty" jsonschema:"optional artifact slug filters"`
	IncludeRevisions bool     `json:"include_revisions,omitempty" jsonschema:"default false; true adds per-artifact .revisions.md files"`
	Format           string   `json:"format,omitempty" jsonschema:"zip (default) | tar"`
}

type projectExportOutput struct {
	Filename      string `json:"filename"`
	MimeType      string `json:"mime_type"`
	Encoding      string `json:"encoding"`
	Bytes         int    `json:"bytes"`
	ArtifactCount int    `json:"artifact_count"`
	FileCount     int    `json:"file_count"`
	ContentBase64 string `json:"content_base64"`
}

func RegisterProjectExport(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name:        "pindoc.project_export",
			Description: "Export a project or area as a zip/tar markdown bundle. Each artifact is written as <area>/<slug>.md with frontmatter; include_revisions=true adds revision files.",
		},
		func(ctx context.Context, p *auth.Principal, in projectExportInput) (*sdk.CallToolResult, projectExportOutput, error) {
			scope, err := auth.ResolveProject(ctx, deps.DB, p, in.ProjectSlug)
			if err != nil {
				return nil, projectExportOutput{}, fmt.Errorf("project_export: %w", err)
			}
			format := strings.ToLower(strings.TrimSpace(in.Format))
			if format == "" {
				format = "zip"
			}
			if format != "zip" && format != "tar" {
				return nil, projectExportOutput{}, fmt.Errorf("format must be zip or tar")
			}
			archive, err := projectexport.BuildFromDB(ctx, deps.DB, projectexport.Options{
				ProjectSlug:      scope.ProjectSlug,
				Areas:            in.Areas,
				Slugs:            in.Slugs,
				IncludeRevisions: in.IncludeRevisions,
				Format:           format,
			})
			if err != nil {
				return nil, projectExportOutput{}, err
			}
			return nil, projectExportOutput{
				Filename:      archive.Filename,
				MimeType:      archive.MimeType,
				Encoding:      "base64",
				Bytes:         len(archive.Bytes),
				ArtifactCount: archive.ArtifactCount,
				FileCount:     archive.FileCount,
				ContentBase64: base64.StdEncoding.EncodeToString(archive.Bytes),
			}, nil
		},
	)
}

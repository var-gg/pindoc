// Package samplefixtures seeds a small public demo project for first-run
// self-host onboarding.
package samplefixtures

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/organizations"
	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

const (
	ProjectSlug     = "pindoc-tour"
	sampleOwnerSlug = "pindoc-sample"
	authorID        = "pindoc-sample-seed"
	version         = "0.0.1"
)

//go:embed manifest.json *.md
var fixtureFS embed.FS

type Manifest struct {
	Project   ProjectMeta    `json:"project"`
	Artifacts []ArtifactMeta `json:"artifacts"`
}

type ProjectMeta struct {
	Slug            string `json:"slug"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	PrimaryLanguage string `json:"primary_language"`
}

type ArtifactMeta struct {
	Slug  string   `json:"slug"`
	Type  string   `json:"type"`
	Area  string   `json:"area"`
	Title string   `json:"title"`
	Tags  []string `json:"tags"`
	File  string   `json:"file"`
}

type ArtifactFixture struct {
	ArtifactMeta
	Body string
}

type SeedResult struct {
	ProjectSlug       string
	ProjectCreated    bool
	ArtifactsInserted int
	ArtifactsSkipped  int
	FixturesTotal     int
}

func LoadManifest() (Manifest, error) {
	var out Manifest
	raw, err := fixtureFS.ReadFile("manifest.json")
	if err != nil {
		return out, fmt.Errorf("read sample manifest: %w", err)
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("parse sample manifest: %w", err)
	}
	if err := validateManifest(out); err != nil {
		return out, err
	}
	return out, nil
}

func LoadFixtures() ([]ArtifactFixture, error) {
	manifest, err := LoadManifest()
	if err != nil {
		return nil, err
	}
	fixtures := make([]ArtifactFixture, 0, len(manifest.Artifacts))
	for _, meta := range manifest.Artifacts {
		raw, err := fixtureFS.ReadFile(meta.File)
		if err != nil {
			return nil, fmt.Errorf("read sample fixture %s: %w", meta.File, err)
		}
		body := strings.TrimSpace(string(raw))
		if body == "" {
			return nil, fmt.Errorf("sample fixture %s is empty", meta.File)
		}
		fixtures = append(fixtures, ArtifactFixture{ArtifactMeta: meta, Body: body})
	}
	return fixtures, nil
}

func Seed(ctx context.Context, pool *db.Pool, ownerUserID string) (SeedResult, error) {
	var zero SeedResult
	if pool == nil {
		return zero, errors.New("sample fixtures: nil DB pool")
	}
	manifest, err := LoadManifest()
	if err != nil {
		return zero, err
	}
	fixtures, err := LoadFixtures()
	if err != nil {
		return zero, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return zero, fmt.Errorf("begin sample seed: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	projectID, created, err := ensureSampleProject(ctx, tx, manifest.Project, ownerUserID)
	if err != nil {
		return zero, err
	}
	result := SeedResult{
		ProjectSlug:    manifest.Project.Slug,
		ProjectCreated: created,
		FixturesTotal:  len(fixtures),
	}
	for _, fixture := range fixtures {
		inserted, err := seedArtifact(ctx, tx, projectID, fixture, manifest.Project.PrimaryLanguage)
		if err != nil {
			return zero, err
		}
		if inserted {
			result.ArtifactsInserted++
		} else {
			result.ArtifactsSkipped++
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return zero, fmt.Errorf("commit sample seed: %w", err)
	}
	return result, nil
}

func validateManifest(m Manifest) error {
	if strings.TrimSpace(m.Project.Slug) != ProjectSlug {
		return fmt.Errorf("sample manifest project.slug = %q, want %q", m.Project.Slug, ProjectSlug)
	}
	if strings.TrimSpace(m.Project.Name) == "" {
		return errors.New("sample manifest project.name is required")
	}
	if strings.TrimSpace(m.Project.PrimaryLanguage) == "" {
		return errors.New("sample manifest project.primary_language is required")
	}
	if n := len(m.Artifacts); n < 5 || n > 10 {
		return fmt.Errorf("sample manifest should contain 5-10 artifacts, got %d", n)
	}
	seen := map[string]struct{}{}
	for _, a := range m.Artifacts {
		for field, value := range map[string]string{
			"slug":  a.Slug,
			"type":  a.Type,
			"area":  a.Area,
			"title": a.Title,
			"file":  a.File,
		} {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("sample artifact %q missing %s", a.Slug, field)
			}
		}
		if _, ok := seen[a.Slug]; ok {
			return fmt.Errorf("duplicate sample artifact slug %q", a.Slug)
		}
		seen[a.Slug] = struct{}{}
		if len(a.Tags) == 0 {
			return fmt.Errorf("sample artifact %q has no tags", a.Slug)
		}
		if strings.Contains(a.File, "/") || strings.Contains(a.File, "\\") || !strings.HasSuffix(a.File, ".md") {
			return fmt.Errorf("sample artifact %q has invalid file %q", a.Slug, a.File)
		}
	}
	return nil
}

func ensureSampleProject(ctx context.Context, tx pgx.Tx, meta ProjectMeta, ownerUserID string) (projectID string, created bool, err error) {
	slug := strings.TrimSpace(meta.Slug)
	if err := ensureSampleOrganization(ctx, tx, ownerUserID); err != nil {
		return "", false, err
	}
	var ownerID string
	err = tx.QueryRow(ctx, `
		SELECT id::text, owner_id FROM projects WHERE slug = $1
	`, slug).Scan(&projectID, &ownerID)
	if err == nil {
		if ownerID != sampleOwnerSlug {
			return "", false, fmt.Errorf("sample project slug %q already exists outside sample owner %q", slug, sampleOwnerSlug)
		}
		if err := markSampleProjectPublic(ctx, tx, projectID, meta); err != nil {
			return "", false, err
		}
		return projectID, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, fmt.Errorf("lookup sample project: %w", err)
	}

	out, err := projects.CreateProject(ctx, tx, projects.CreateProjectInput{
		Slug:            slug,
		Name:            meta.Name,
		Description:     meta.Description,
		PrimaryLanguage: meta.PrimaryLanguage,
		OwnerID:         sampleOwnerSlug,
		OwnerUserID:     ownerUserID,
	})
	if err != nil {
		return "", false, fmt.Errorf("create sample project: %w", err)
	}
	if err := markSampleProjectPublic(ctx, tx, out.ID, meta); err != nil {
		return "", false, err
	}
	return out.ID, true, nil
}

func ensureSampleOrganization(ctx context.Context, tx pgx.Tx, ownerUserID string) error {
	if _, err := organizations.ResolveBySlug(ctx, tx, sampleOwnerSlug); err == nil {
		return nil
	} else if !errors.Is(err, organizations.ErrNotFound) {
		return fmt.Errorf("lookup sample organization: %w", err)
	}
	if _, err := organizations.Create(ctx, tx, organizations.CreateInput{
		Slug:        sampleOwnerSlug,
		Name:        "Pindoc Sample",
		Kind:        organizations.KindTeam,
		Description: "System-owned namespace for optional Pindoc sample fixtures.",
		OwnerUserID: ownerUserID,
	}); err != nil {
		return fmt.Errorf("create sample organization: %w", err)
	}
	return nil
}

func markSampleProjectPublic(ctx context.Context, tx pgx.Tx, projectID string, meta ProjectMeta) error {
	_, err := tx.Exec(ctx, `
		UPDATE projects
		   SET name = $2,
		       description = $3,
		       visibility = $4,
		       default_artifact_visibility = $4,
		       updated_at = now()
		 WHERE id = $1::uuid
	`, projectID, meta.Name, meta.Description, projects.VisibilityPublic)
	if err != nil {
		return fmt.Errorf("mark sample project public: %w", err)
	}
	return nil
}

func seedArtifact(ctx context.Context, tx pgx.Tx, projectID string, fixture ArtifactFixture, bodyLocale string) (bool, error) {
	areaID, err := areaIDForSlug(ctx, tx, projectID, fixture.Area)
	if err != nil {
		return false, err
	}
	var artifactID string
	err = tx.QueryRow(ctx, `
		INSERT INTO artifacts (
			project_id, area_id, slug, type, title, body_markdown, tags,
			body_locale, completeness, status, review_state,
			author_kind, author_id, author_version, artifact_meta,
			visibility, published_at
		) VALUES (
			$1::uuid, $2::uuid, $3, $4, $5, $6, $7,
			$8, 'settled', 'published', 'auto_published',
			'system', $9, $10,
			'{"source_type":"artifact","confidence":"high","verification_state":"verified"}'::jsonb,
			$11, now()
		)
		ON CONFLICT (project_id, slug) DO NOTHING
		RETURNING id::text
	`, projectID, areaID, fixture.Slug, fixture.Type, fixture.Title, fixture.Body, fixture.Tags,
		bodyLocale, authorID, version, projects.VisibilityPublic).Scan(&artifactID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("insert sample artifact %s: %w", fixture.Slug, err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO artifact_revisions (
			artifact_id, revision_number, title, body_markdown, body_hash,
			tags, completeness, author_kind, author_id, author_version,
			commit_msg, revision_shape
		) VALUES (
			$1::uuid, 1, $2, $3, encode(sha256(convert_to($3, 'UTF8')), 'hex'),
			$4, 'settled', 'system', $5, $6,
			'seed: sample fixture artifact', 'body_patch'
		)
	`, artifactID, fixture.Title, fixture.Body, fixture.Tags, authorID, version); err != nil {
		return false, fmt.Errorf("insert sample artifact revision %s: %w", fixture.Slug, err)
	}
	return true, nil
}

func areaIDForSlug(ctx context.Context, tx pgx.Tx, projectID, areaSlug string) (string, error) {
	var areaID string
	err := tx.QueryRow(ctx, `
		SELECT id::text FROM areas
		 WHERE project_id = $1::uuid AND slug = $2
	`, projectID, areaSlug).Scan(&areaID)
	if err != nil {
		return "", fmt.Errorf("resolve sample area %s: %w", areaSlug, err)
	}
	return areaID, nil
}

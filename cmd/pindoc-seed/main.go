// pindoc-seed imports existing repo docs into the Pindoc project as
// artifacts. Runs once (idempotent via ON CONFLICT DO NOTHING). From that
// point forward, the canonical way to change docs/ is through
// pindoc.artifact.propose — this binary is the last time we touch
// artifacts outside the MCP flow.
//
// Usage:
//   go run ./cmd/pindoc-seed
//   go run ./cmd/pindoc-seed -docs=path/to/docs
//   go run ./cmd/pindoc-seed -dry-run
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/var-gg/pindoc/internal/pindoc/config"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/embed"
)

// seedPlan maps a markdown file in docs/ to the artifact it should become.
// Kept explicit (no convention-over-config) because this is a one-time
// bootstrap — the mapping is easier to audit in a table than to infer.
type seedPlan struct {
	File     string // relative to -docs
	Slug     string
	Type     string
	Area     string
	Title    string
}

var plans = []seedPlan{
	{File: "00-vision.md",                 Slug: "vision",                   Type: "Analysis", Area: "vision",       Title: "Pindoc vision"},
	{File: "01-problem.md",                Slug: "problem-space",            Type: "Analysis", Area: "vision",       Title: "Problem space and failure modes F1–F6"},
	{File: "02-concepts.md",               Slug: "core-concepts",            Type: "Analysis", Area: "architecture", Title: "Core concepts — five primitives"},
	{File: "03-architecture.md",           Slug: "architecture",             Type: "Analysis", Area: "architecture", Title: "System architecture"},
	{File: "04-data-model.md",             Slug: "data-model",               Type: "Analysis", Area: "data-model",   Title: "Data model — Tier A/B, Area, Pin, state"},
	{File: "05-mechanisms.md",             Slug: "mechanisms",               Type: "Analysis", Area: "mechanisms",   Title: "Mechanisms M0–M7"},
	{File: "06-ui-flows.md",               Slug: "ui-flows",                 Type: "Analysis", Area: "ui",           Title: "UI flows and surfaces"},
	{File: "07-roadmap.md",                Slug: "roadmap",                  Type: "Analysis", Area: "roadmap",      Title: "Roadmap V1/V1.x/V2"},
	{File: "08-non-goals.md",              Slug: "non-goals",                Type: "Analysis", Area: "vision",       Title: "Non-goals — what Pindoc is not"},
	{File: "09-pindoc-md-spec.md",         Slug: "pindoc-md-spec",           Type: "Analysis", Area: "architecture", Title: "PINDOC.md spec"},
	{File: "10-mcp-tools-spec.md",         Slug: "mcp-tools-spec",           Type: "Analysis", Area: "architecture", Title: "MCP tools spec"},
	{File: "11-design-system-handoff.md",  Slug: "design-system-handoff",    Type: "Analysis", Area: "ui",           Title: "Design system handoff v0"},
	{File: "12-m1-implementation-plan.md", Slug: "m1-implementation-plan",   Type: "Analysis", Area: "roadmap",      Title: "M1 implementation plan"},
	{File: "glossary.md",                  Slug: "glossary",                 Type: "Glossary", Area: "misc",         Title: "Glossary"},
	{File: "decisions.md",                 Slug: "decisions-log",            Type: "Analysis", Area: "decisions",    Title: "Decisions log and open questions"},
}

func main() {
	var (
		docsDir = flag.String("docs", "docs", "path to docs/ directory")
		dryRun  = flag.Bool("dry-run", false, "print plan without writing")
	)
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config load", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("db open", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	embedder, err := embed.Build(cfg.Embed)
	if err != nil {
		logger.Error("embed build", "err", err)
		os.Exit(1)
	}

	// Resolve project + area IDs up front.
	var projectID string
	if err := pool.QueryRow(ctx,
		`SELECT id::text FROM projects WHERE slug = $1`, cfg.ProjectSlug,
	).Scan(&projectID); err != nil {
		logger.Error("project lookup — have you run pindoc-server once to apply migrations+seed?", "slug", cfg.ProjectSlug, "err", err)
		os.Exit(1)
	}
	areaIDs := map[string]string{}
	rows, err := pool.Query(ctx, `SELECT slug, id::text FROM areas WHERE project_id = $1`, projectID)
	if err != nil {
		logger.Error("area lookup", "err", err)
		os.Exit(1)
	}
	for rows.Next() {
		var slug, id string
		if err := rows.Scan(&slug, &id); err != nil {
			logger.Error("area scan", "err", err)
			os.Exit(1)
		}
		areaIDs[slug] = id
	}
	rows.Close()

	info := embedder.Info()
	logger.Info("seed start",
		"project", cfg.ProjectSlug,
		"embedder", info.Name, "embedder_dim", info.Dimension,
		"docs_dir", *docsDir, "plans", len(plans), "dry_run", *dryRun,
	)

	imported := 0
	skipped := 0
	for _, p := range plans {
		path := filepath.Join(*docsDir, p.File)
		body, err := os.ReadFile(path)
		if err != nil {
			logger.Warn("skip (read failed)", "file", p.File, "err", err)
			skipped++
			continue
		}
		areaID, ok := areaIDs[p.Area]
		if !ok {
			logger.Warn("skip (area not found)", "file", p.File, "area", p.Area)
			skipped++
			continue
		}

		// Idempotency: slug unique per project. Skip if already imported.
		var existingID string
		err = pool.QueryRow(ctx,
			`SELECT id::text FROM artifacts WHERE project_id = $1 AND slug = $2`,
			projectID, p.Slug,
		).Scan(&existingID)
		if err == nil {
			logger.Info("skip (already imported)", "slug", p.Slug, "id", existingID)
			skipped++
			continue
		}
		if err != pgx.ErrNoRows {
			logger.Error("slug lookup", "slug", p.Slug, "err", err)
			os.Exit(1)
		}

		if *dryRun {
			logger.Info("would import", "file", p.File, "slug", p.Slug, "type", p.Type, "area", p.Area, "title", p.Title, "bytes", len(body))
			continue
		}

		if err := insertOne(ctx, pool, embedder, projectID, areaID, p, string(body)); err != nil {
			logger.Error("insert failed", "file", p.File, "err", err)
			os.Exit(1)
		}
		logger.Info("imported", "file", p.File, "slug", p.Slug, "type", p.Type, "area", p.Area)
		imported++
	}

	logger.Info("seed done", "imported", imported, "skipped", skipped)
}

func insertOne(ctx context.Context, pool *db.Pool, provider embed.Provider,
	projectID, areaID string, p seedPlan, body string) error {

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var newID string
	err = tx.QueryRow(ctx, `
		INSERT INTO artifacts (
			project_id, area_id, slug, type, title, body_markdown, tags,
			completeness, status, review_state,
			author_kind, author_id, author_version,
			published_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, 'settled', 'published', 'auto_published',
			'system', 'pindoc-seed', 'M1', now())
		RETURNING id::text
	`, projectID, areaID, p.Slug, p.Type, p.Title, body, []string{"seed", "m1"},
	).Scan(&newID)
	if err != nil {
		return fmt.Errorf("insert artifact: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO events (project_id, kind, subject_id, payload)
		VALUES ($1, 'artifact.published', $2, jsonb_build_object(
			'slug', $3::text, 'type', $4::text, 'source', 'pindoc-seed'
		))
	`, projectID, newID, p.Slug, p.Type); err != nil {
		return fmt.Errorf("event: %w", err)
	}

	info := provider.Info()

	// Title chunk.
	titleRes, err := provider.Embed(ctx, embed.Request{Texts: []string{p.Title}, Kind: embed.KindDocument})
	if err != nil {
		return fmt.Errorf("embed title: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO artifact_chunks (
			artifact_id, kind, chunk_index, heading, span_start, span_end,
			text, embedding, model_name, model_dim
		) VALUES ($1, 'title', 0, NULL, 0, 0, $2, $3::vector, $4, $5)
	`, newID, p.Title,
		embed.VectorString(embed.PadTo768(titleRes.Vectors[0])),
		info.Name+":"+info.ModelID, info.Dimension,
	); err != nil {
		return fmt.Errorf("title chunk: %w", err)
	}

	// Body chunks.
	chunks := embed.ChunkBody(p.Title, body, 600)
	if len(chunks) > 0 {
		texts := make([]string, len(chunks))
		for i, c := range chunks {
			texts[i] = c.Text
		}
		bodyRes, err := provider.Embed(ctx, embed.Request{Texts: texts, Kind: embed.KindDocument})
		if err != nil {
			return fmt.Errorf("embed body: %w", err)
		}
		for i, c := range chunks {
			var heading any = c.Heading
			if c.Heading == "" {
				heading = nil
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO artifact_chunks (
					artifact_id, kind, chunk_index, heading, span_start, span_end,
					text, embedding, model_name, model_dim
				) VALUES ($1, 'body', $2, $3, $4, $5, $6, $7::vector, $8, $9)
			`, newID, c.Index, heading, c.SpanStart, c.SpanEnd, c.Text,
				embed.VectorString(embed.PadTo768(bodyRes.Vectors[i])),
				info.Name+":"+info.ModelID, info.Dimension,
			); err != nil {
				return fmt.Errorf("body chunk %d: %w", c.Index, err)
			}
		}
	}

	return tx.Commit(ctx)
}

// Unused but kept so `go vet` doesn't complain if we stop importing strings
// after a refactor.
var _ = strings.TrimSpace

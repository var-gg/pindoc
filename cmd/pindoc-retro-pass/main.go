package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/var-gg/pindoc/internal/pindoc/artifactlinks"
	"github.com/var-gg/pindoc/internal/pindoc/artifactslug"
	"github.com/var-gg/pindoc/internal/pindoc/config"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/settings"
)

type artifactRow struct {
	ID            string
	ProjectID     string
	ProjectSlug   string
	ProjectLocale string
	Slug          string
	Title         string
	Body          string
	Tags          []string
	Completeness  string
	Revision      int
}

type planItem struct {
	Artifact artifactRow
	NewSlug  string
	NewBody  string
}

func main() {
	var (
		dsn                 string
		projects            string
		apply               bool
		limit               int
		actor               string
		rewriteSlugs        bool
		scanAcceptanceVerbs bool
		acceptanceReport    string
		scanOutcomeMissing  bool
		outcomeReport       string
	)
	flag.StringVar(&dsn, "database-url", "", "Postgres DSN; defaults to PINDOC_DATABASE_URL/config default")
	flag.StringVar(&projects, "projects", "", "comma-separated project slugs; empty means all projects")
	flag.BoolVar(&apply, "apply", false, "write revisions and slug aliases; default is dry-run")
	flag.IntVar(&limit, "limit", 0, "maximum artifacts to scan; 0 means no limit")
	flag.StringVar(&actor, "actor", "pindoc-retro-pass", "author_id for generated revisions")
	flag.BoolVar(&rewriteSlugs, "rewrite-slugs", false, "also regenerate slugs from titles and write old_slug aliases; default only normalizes body links")
	flag.BoolVar(&scanAcceptanceVerbs, "scan-acceptance-verbs", false, "scan Task acceptance checklists for forbidden action verbs and write a report")
	flag.StringVar(&acceptanceReport, "acceptance-report", "artifacts/retro-pass/acceptance-verb-lint.md", "report path for -scan-acceptance-verbs; use '-' for stdout")
	flag.BoolVar(&scanOutcomeMissing, "scan-outcome-missing", false, "scan claimed_done Tasks for missing Outcome sections/content and write a report")
	flag.StringVar(&outcomeReport, "outcome-report", "artifacts/retro-pass/claim-done-outcome-missing.md", "report path for -scan-outcome-missing; use '-' for stdout")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if strings.TrimSpace(dsn) == "" {
		cfg, err := config.Load()
		if err != nil {
			log.Fatalf("load config: %v", err)
		}
		dsn = cfg.DatabaseURL
	}
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool.Pool); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	store, err := settings.New(ctx, pool)
	if err != nil {
		log.Fatalf("settings: %v", err)
	}

	projectFilter := splitCSV(projects)
	if scanAcceptanceVerbs {
		findings, err := loadAcceptanceVerbFindings(ctx, pool, projectFilter, limit)
		if err != nil {
			log.Fatalf("load acceptance verb findings: %v", err)
		}
		if err := writeAcceptanceVerbReport(acceptanceReport, findings); err != nil {
			log.Fatalf("write acceptance verb report: %v", err)
		}
		printAcceptanceVerbReportSummary(acceptanceReport, findings)
		return
	}
	if scanOutcomeMissing {
		findings, err := loadOutcomeMissingFindings(ctx, pool, projectFilter, limit)
		if err != nil {
			log.Fatalf("load outcome missing findings: %v", err)
		}
		if err := writeOutcomeMissingReport(outcomeReport, findings); err != nil {
			log.Fatalf("write outcome missing report: %v", err)
		}
		printOutcomeMissingReportSummary(outcomeReport, findings)
		return
	}

	rows, err := loadArtifacts(ctx, pool, projectFilter, limit)
	if err != nil {
		log.Fatalf("load artifacts: %v", err)
	}

	var planned []planItem
	var skipped []string
	for _, row := range rows {
		newSlug := row.Slug
		if rewriteSlugs {
			newSlug = artifactslug.Slugify(row.Title)
		}
		if strings.TrimSpace(newSlug) == "" {
			newSlug = row.Slug
		}
		var linkErr *artifactlinks.LinkError
		newBody, _, err := artifactlinks.RewritePindocLinks(row.Body, func(ref string) (string, error) {
			ref = artifactlinks.NormalizeRef(ref)
			targetSlug, err := resolveArtifactSlug(ctx, pool, row.ProjectID, ref)
			if err != nil {
				return "", err
			}
			if base := strings.TrimRight(strings.TrimSpace(store.Get().PublicBaseURL), "/"); base != "" {
				return base + "/p/" + row.ProjectSlug + "/wiki/" + targetSlug, nil
			}
			return "/p/" + row.ProjectSlug + "/wiki/" + targetSlug, nil
		})
		if errors.As(err, &linkErr) {
			skipped = append(skipped, fmt.Sprintf("%s/%s invalid ref pindoc://%s", row.ProjectSlug, row.Slug, linkErr.Ref))
			continue
		}
		if err != nil {
			log.Fatalf("rewrite %s/%s: %v", row.ProjectSlug, row.Slug, err)
		}
		if newSlug == row.Slug && newBody == row.Body {
			continue
		}
		planned = append(planned, planItem{Artifact: row, NewSlug: newSlug, NewBody: newBody})
	}

	if !apply {
		printPlan(planned, skipped, false)
		return
	}
	applied := 0
	for _, item := range planned {
		if err := applyPlanItem(ctx, pool, item, actor); err != nil {
			log.Fatalf("apply %s/%s: %v", item.Artifact.ProjectSlug, item.Artifact.Slug, err)
		}
		applied++
	}
	printPlan(planned[:applied], skipped, true)
}

func loadArtifacts(ctx context.Context, pool *db.Pool, projects []string, limit int) ([]artifactRow, error) {
	args := []any{projects}
	limitSQL := ""
	if limit > 0 {
		args = append(args, limit)
		limitSQL = fmt.Sprintf(" LIMIT $%d", len(args))
	}
	query := `
		SELECT a.id::text, a.project_id::text, p.slug,
		       COALESCE(NULLIF(p.primary_language, ''), 'en'),
		       a.slug, a.title, a.body_markdown, a.tags, a.completeness,
		       COALESCE((SELECT max(revision_number) FROM artifact_revisions WHERE artifact_id = a.id), 0)
		  FROM artifacts a
		  JOIN projects p ON p.id = a.project_id
		 WHERE a.status <> 'archived'
		   AND a.status <> 'superseded'
		   AND NOT starts_with(a.slug, '_template_')
		   AND (cardinality($1::text[]) = 0 OR p.slug = ANY($1::text[]))
		 ORDER BY p.slug, a.slug` + limitSQL
	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []artifactRow
	for rows.Next() {
		var row artifactRow
		if err := rows.Scan(&row.ID, &row.ProjectID, &row.ProjectSlug, &row.ProjectLocale, &row.Slug, &row.Title, &row.Body, &row.Tags, &row.Completeness, &row.Revision); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func resolveArtifactSlug(ctx context.Context, pool *db.Pool, projectID, ref string) (string, error) {
	var slug string
	err := pool.QueryRow(ctx, `
		SELECT slug
		  FROM artifacts
		 WHERE project_id = $1::uuid
		   AND (id::text = $2 OR slug = $2)
		   AND status <> 'archived'
		 LIMIT 1
	`, projectID, strings.TrimSpace(ref)).Scan(&slug)
	if err != nil {
		return "", err
	}
	return slug, nil
}

func applyPlanItem(ctx context.Context, pool *db.Pool, item planItem, actor string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	finalSlug := item.NewSlug
	if finalSlug != item.Artifact.Slug {
		finalSlug, err = allocateSlug(ctx, tx, item.Artifact.ProjectID, item.Artifact.ID, item.NewSlug)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO artifact_slug_aliases (project_id, artifact_id, old_slug, created_by, reason)
			VALUES ($1::uuid, $2::uuid, $3, $4, 'retro-pass slug regeneration')
			ON CONFLICT (project_id, old_slug) DO NOTHING
		`, item.Artifact.ProjectID, item.Artifact.ID, item.Artifact.Slug, actor); err != nil {
			return err
		}
	}

	newRev := item.Artifact.Revision + 1
	if newRev < 1 {
		newRev = 1
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO artifact_revisions (
			artifact_id, revision_number, title, body_markdown, body_hash, tags,
			completeness, author_kind, author_id, commit_msg, revision_shape
		) VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, 'system', $8, $9, 'body_patch')
	`, item.Artifact.ID, newRev, item.Artifact.Title, item.NewBody, bodyHash(item.NewBody), item.Artifact.Tags,
		item.Artifact.Completeness, actor, "retro-pass slug/link normalization"); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE artifacts
		   SET slug = $2,
		       body_markdown = $3,
		       updated_at = now()
		 WHERE id = $1::uuid
	`, item.Artifact.ID, finalSlug, item.NewBody); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func allocateSlug(ctx context.Context, tx pgx.Tx, projectID, artifactID, base string) (string, error) {
	base = strings.TrimSpace(base)
	if base == "" {
		return "", errors.New("empty generated slug")
	}
	for attempt := 0; attempt < 100; attempt++ {
		candidate := base
		if attempt > 0 {
			candidate = fmt.Sprintf("%s-%d", base, attempt+1)
		}
		var exists bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM artifacts
				 WHERE project_id = $1::uuid
				   AND slug = $2
				   AND id::text <> $3
				UNION ALL
				SELECT 1 FROM artifact_slug_aliases
				 WHERE project_id = $1::uuid
				   AND old_slug = $2
			)
		`, projectID, candidate, artifactID).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not allocate slug for %q", base)
}

func printPlan(items []planItem, skipped []string, applied bool) {
	mode := "dry_run"
	if applied {
		mode = "applied"
	}
	fmt.Fprintf(os.Stdout, "%s items=%d skipped=%d\n", mode, len(items), len(skipped))
	for _, item := range items {
		changes := []string{}
		if item.NewSlug != item.Artifact.Slug {
			changes = append(changes, "slug:"+item.Artifact.Slug+"->"+item.NewSlug)
		}
		if item.NewBody != item.Artifact.Body {
			changes = append(changes, "body_links")
		}
		fmt.Fprintf(os.Stdout, "%s/%s\t%s\n", item.Artifact.ProjectSlug, item.Artifact.Slug, strings.Join(changes, ","))
	}
	sort.Strings(skipped)
	for _, s := range skipped {
		fmt.Fprintf(os.Stdout, "skipped\t%s\n", s)
	}
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func bodyHash(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

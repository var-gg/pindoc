// pindoc-admin is a tiny CLI for operator-editable server settings. Lives
// between "raw psql" and "Settings UI" — the UI lands in V1.5 with auth;
// until then this is how a self-host operator changes public_base_url or
// other runtime settings without a server restart.
//
// Usage:
//
//	pindoc-admin list
//	pindoc-admin get public_base_url
//	pindoc-admin set public_base_url https://wiki.acme.dev
//	pindoc-admin relabel-artifacts --actor codex --reason "area taxonomy reform" mapping.tsv
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/var-gg/pindoc/internal/pindoc/config"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/projects"
	"github.com/var-gg/pindoc/internal/pindoc/settings"
)

// errProjectValidation tags user-input failures so main() can map to
// exit 2 (vs exit 1 for DB / internal). Wrapped around the underlying
// projects sentinel error so callers can still errors.Is to the
// specific code (ErrSlugInvalid, ErrLangInvalid, ...) when needed.
var errProjectValidation = errors.New("project validation failed")

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: pindoc-admin <command> [args]")
		fmt.Fprintln(os.Stderr, "commands:")
		fmt.Fprintln(os.Stderr, "  list                 — list editable keys")
		fmt.Fprintln(os.Stderr, "  get <key>            — print current value")
		fmt.Fprintln(os.Stderr, "  set <key> <value>    — update a setting (hot, no restart)")
		fmt.Fprintln(os.Stderr, "  relabel-artifacts    — batch move artifact area_slug from a TSV mapping")
		fmt.Fprintln(os.Stderr, "  project create <slug> --name \"...\" --language ko [--description \"...\"] [--color \"#...\"] [--git-remote-url \"...\"]")
		fmt.Fprintln(os.Stderr, "                       — create a new project (no MCP session needed)")
	}
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(2)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config load", "err", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("db open", "err", err, "hint", "is docker compose up -d db running?")
		os.Exit(1)
	}
	defer pool.Close()

	store, err := settings.New(ctx, pool)
	if err != nil {
		// Most likely cause: migration 0007 hasn't run yet (server never
		// started). Point the operator there instead of at psql.
		logger.Error("settings load", "err", err, "hint", "start pindoc-server once to apply migrations 0001-0007, then retry")
		os.Exit(1)
	}

	switch args[0] {
	case "list":
		v := store.Get()
		fmt.Printf("public_base_url = %q\n", v.PublicBaseURL)
		fmt.Printf("updated_at      = %s\n", v.UpdatedAt.Format(time.RFC3339))
		fmt.Println()
		fmt.Println("Keys you can set:")
		for _, k := range settings.AllKeys() {
			fmt.Printf("  %s\n", k)
		}

	case "get":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "get: missing key")
			os.Exit(2)
		}
		v := store.Get()
		switch args[1] {
		case "public_base_url":
			fmt.Println(v.PublicBaseURL)
		default:
			fmt.Fprintf(os.Stderr, "unknown key: %s\n", args[1])
			os.Exit(2)
		}

	case "set":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "set: missing <key> <value>")
			os.Exit(2)
		}
		key := args[1]
		// Allow spaces in values ("set x hello world") by joining
		// everything after the key.
		value := strings.Join(args[2:], " ")
		if err := store.Set(ctx, key, value); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		fmt.Printf("updated %s = %q\n", key, value)
		fmt.Fprintln(os.Stderr, "note: pindoc-api and pindoc-server cache settings at startup. Restart them to pick up this change. (Hot-reload lands in V1.x.)")

	case "relabel-artifacts":
		if err := runRelabelArtifacts(ctx, pool, cfg.ProjectSlug, args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}

	case "project":
		if err := runProjectCommand(ctx, pool, args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, "error: "+err.Error())
			// Validation failures exit 2, infra/DB failures exit 1.
			// runProjectCommand returns errProjectValidation-wrapped
			// errors for the former so we can split here.
			if errors.Is(err, errProjectValidation) {
				os.Exit(2)
			}
			os.Exit(1)
		}

	default:
		flag.Usage()
		os.Exit(2)
	}
}

type relabelMapping struct {
	Ref        string
	TargetArea string
	Reason     string
}

func runRelabelArtifacts(ctx context.Context, pool *db.Pool, defaultProject string, args []string) error {
	fs := flag.NewFlagSet("relabel-artifacts", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	projectSlug := fs.String("project", defaultProject, "project slug")
	actor := fs.String("actor", "pindoc-admin", "operator or agent label stored in the event payload")
	batchReason := fs.String("reason", "", "default reason used when a mapping row omits one")
	dryRun := fs.Bool("dry-run", false, "validate and print changes without writing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("relabel-artifacts: expected mapping TSV path")
	}
	if strings.TrimSpace(*projectSlug) == "" {
		return fmt.Errorf("relabel-artifacts: --project is empty")
	}
	if strings.TrimSpace(*actor) == "" {
		return fmt.Errorf("relabel-artifacts: --actor is empty")
	}

	mappings, err := readRelabelMappings(fs.Arg(0), *batchReason)
	if err != nil {
		return err
	}
	if len(mappings) == 0 {
		return fmt.Errorf("relabel-artifacts: no mapping rows found")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var projectID string
	if err := tx.QueryRow(ctx, `
		SELECT id::text FROM projects WHERE slug = $1
	`, *projectSlug).Scan(&projectID); err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("project %q not found", *projectSlug)
		}
		return fmt.Errorf("resolve project: %w", err)
	}

	moved := 0
	unchanged := 0
	for _, m := range mappings {
		var targetAreaID string
		if err := tx.QueryRow(ctx, `
			SELECT id::text FROM areas
			WHERE project_id = $1 AND slug = $2
		`, projectID, m.TargetArea).Scan(&targetAreaID); err != nil {
			if err == pgx.ErrNoRows {
				return fmt.Errorf("target area %q not found for artifact %q", m.TargetArea, m.Ref)
			}
			return fmt.Errorf("resolve target area %q: %w", m.TargetArea, err)
		}

		var artifactID, slug, title, currentArea string
		if err := tx.QueryRow(ctx, `
			SELECT a.id::text, a.slug, a.title, ar.slug
			FROM artifacts a
			JOIN areas ar ON ar.id = a.area_id
			WHERE a.project_id = $1
			  AND (a.id::text = $2 OR a.slug = $2)
			  AND a.status <> 'archived'
			LIMIT 1
		`, projectID, m.Ref).Scan(&artifactID, &slug, &title, &currentArea); err != nil {
			if err == pgx.ErrNoRows {
				return fmt.Errorf("artifact %q not found or archived", m.Ref)
			}
			return fmt.Errorf("resolve artifact %q: %w", m.Ref, err)
		}

		if currentArea == m.TargetArea {
			unchanged++
			fmt.Printf("unchanged\t%s\t%s\t%s\n", slug, currentArea, title)
			continue
		}
		if *dryRun {
			fmt.Printf("would_move\t%s\t%s\t%s\t%s\n", slug, currentArea, m.TargetArea, title)
			moved++
			continue
		}

		if _, err := tx.Exec(ctx, `
			UPDATE artifacts
			   SET area_id = $2,
			       updated_at = now()
			 WHERE id = $1
		`, artifactID, targetAreaID); err != nil {
			return fmt.Errorf("move artifact %q: %w", slug, err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO events (project_id, kind, subject_id, payload)
			VALUES ($1, 'artifact.area_relabelled', $2, jsonb_build_object(
				'slug',           $3::text,
				'title',          $4::text,
				'from_area_slug', $5::text,
				'to_area_slug',   $6::text,
				'reason',         $7::text,
				'actor',          $8::text
			))
		`, projectID, artifactID, slug, title, currentArea, m.TargetArea, m.Reason, *actor); err != nil {
			return fmt.Errorf("insert area relabel event for %q: %w", slug, err)
		}
		moved++
		fmt.Printf("moved\t%s\t%s\t%s\t%s\n", slug, currentArea, m.TargetArea, title)
	}

	if *dryRun {
		fmt.Printf("dry-run complete: %d moves, %d unchanged\n", moved, unchanged)
		return nil
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit relabels: %w", err)
	}
	fmt.Printf("relabel complete: %d moved, %d unchanged\n", moved, unchanged)
	return nil
}

func readRelabelMappings(path, batchReason string) ([]relabelMapping, error) {
	f := os.Stdin
	if path != "-" {
		var err error
		f, err = os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open mapping: %w", err)
		}
		defer f.Close()
	}

	var out []relabelMapping
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 2 || len(fields) > 3 {
			return nil, fmt.Errorf("mapping line %d: expected slug<TAB>area_slug[<TAB>reason]", lineNo)
		}
		m := relabelMapping{
			Ref:        strings.TrimSpace(fields[0]),
			TargetArea: strings.TrimSpace(fields[1]),
			Reason:     strings.TrimSpace(batchReason),
		}
		if len(fields) == 3 && strings.TrimSpace(fields[2]) != "" {
			m.Reason = strings.TrimSpace(fields[2])
		}
		if m.Ref == "" || m.TargetArea == "" {
			return nil, fmt.Errorf("mapping line %d: artifact ref and area_slug are required", lineNo)
		}
		if m.Reason == "" {
			return nil, fmt.Errorf("mapping line %d: reason is required via row third column or --reason", lineNo)
		}
		out = append(out, m)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read mapping: %w", err)
	}
	return out, nil
}

// runProjectCommand dispatches `pindoc-admin project <subcommand>`. V1
// only carries `create`; future subs (rename, delete, archive) plug in
// here. Mirrors the MCP / REST entrypoints — all three call
// projects.CreateProject so behavior stays identical regardless of
// surface.
func runProjectCommand(ctx context.Context, pool *db.Pool, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: project: missing subcommand (try `project create <slug> ...`)", errProjectValidation)
	}
	switch args[0] {
	case "create":
		return runProjectCreate(ctx, pool, args[1:])
	default:
		return fmt.Errorf("%w: project: unknown subcommand %q", errProjectValidation, args[0])
	}
}

// runProjectCreate implements `pindoc-admin project create <slug> --name
// "..." --language ko [--description "..."] [--color "#..."]`. Begins
// its own tx (the MCP path uses its existing tx; CLI is the boundary
// here). Stable error_code mapping for stderr output mirrors the REST
// envelope so a wrapper script can tee results.
func runProjectCreate(ctx context.Context, pool *db.Pool, args []string) error {
	fs := flag.NewFlagSet("project create", flag.ContinueOnError)
	name := fs.String("name", "", "human-readable display name (required)")
	language := fs.String("language", "", "primary_language: en | ko | ja (required, immutable after create)")
	description := fs.String("description", "", "optional one-line description")
	color := fs.String("color", "", "optional sidebar accent color (hex / oklch / css color)")
	gitRemoteURL := fs.String("git-remote-url", "", "optional git remote URL to store in project_repos")

	// Slug is the first positional arg. Push everything before the
	// first flag into a single positional slot so the FlagSet can
	// parse the rest.
	if len(args) == 0 {
		fs.Usage()
		return fmt.Errorf("%w: project create: missing <slug> positional argument", errProjectValidation)
	}
	slug := args[0]
	if err := fs.Parse(args[1:]); err != nil {
		return fmt.Errorf("%w: %s", errProjectValidation, err)
	}

	if strings.TrimSpace(*name) == "" {
		return fmt.Errorf("%w: project create: --name is required", errProjectValidation)
	}
	if strings.TrimSpace(*language) == "" {
		return fmt.Errorf("%w: project create: --language is required (en | ko | ja). Immutable after create — pick deliberately", errProjectValidation)
	}

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	out, err := projects.CreateProject(ctx, tx, projects.CreateProjectInput{
		Slug:            slug,
		Name:            *name,
		Description:     *description,
		Color:           *color,
		PrimaryLanguage: *language,
		GitRemoteURL:    *gitRemoteURL,
	})
	if err != nil {
		// Map sentinel errors to errProjectValidation so main()
		// returns exit 2. Non-sentinel errors stay bare → exit 1.
		switch {
		case errors.Is(err, projects.ErrSlugInvalid),
			errors.Is(err, projects.ErrSlugReserved),
			errors.Is(err, projects.ErrSlugTaken),
			errors.Is(err, projects.ErrNameRequired),
			errors.Is(err, projects.ErrLangRequired),
			errors.Is(err, projects.ErrLangInvalid),
			errors.Is(err, projects.ErrGitRemoteURLInvalid):
			return fmt.Errorf("%w: %s", errProjectValidation, err)
		default:
			return fmt.Errorf("project create: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	fmt.Printf("created project: %s (id=%s)\n", out.Slug, out.ID)
	fmt.Printf("  name:      %s\n", out.Name)
	fmt.Printf("  language:  %s\n", out.PrimaryLanguage)
	fmt.Printf("  url:       /p/%s/wiki\n", out.Slug)
	fmt.Printf("  areas:     %d\n", out.AreasCreated)
	fmt.Printf("  templates: %d\n", out.TemplatesCreated)
	return nil
}

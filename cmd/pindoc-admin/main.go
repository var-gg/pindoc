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
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/var-gg/pindoc/internal/pindoc/config"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/settings"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: pindoc-admin <command> [args]")
		fmt.Fprintln(os.Stderr, "commands:")
		fmt.Fprintln(os.Stderr, "  list                 — list editable keys")
		fmt.Fprintln(os.Stderr, "  get <key>            — print current value")
		fmt.Fprintln(os.Stderr, "  set <key> <value>    — update a setting (hot, no restart)")
		fmt.Fprintln(os.Stderr, "  relabel-artifacts    — batch move artifact area_slug from a TSV mapping")
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

// pindoc-reembed walks every active artifact in the configured project,
// drops its existing artifact_chunks rows, and re-computes title + body
// chunks against whatever embedding provider the config points at. Used
// when switching providers (e.g. stub → http / TEI) or when the chunking
// algorithm changes.
//
// Safe to re-run: each artifact is handled in its own transaction, so a
// partial failure leaves the rest of the corpus unchanged.
//
// Usage:
//
//	go run ./cmd/pindoc-reembed
//	go run ./cmd/pindoc-reembed -dry-run
//	go run ./cmd/pindoc-reembed -only slug1,slug2
package main

import (
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
	"github.com/var-gg/pindoc/internal/pindoc/embed"
)

func main() {
	var (
		dryRun = flag.Bool("dry-run", false, "list artifacts that would be re-embedded, don't touch chunks")
		only   = flag.String("only", "", "comma-separated slugs to limit the run (empty = all)")
	)
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config load", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("db open", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	provider, err := embed.Build(cfg.Embed)
	if err != nil {
		logger.Error("embed build", "err", err)
		os.Exit(1)
	}
	info := provider.Info()
	logger.Info("provider ready", "name", info.Name, "model", info.ModelID, "dim", info.Dimension)
	if info.Name == "stub" {
		logger.Warn("embed provider is 'stub' — re-embedding with a hash encoder is a no-op for retrieval quality. Set PINDOC_EMBED_PROVIDER=http to use the sidecar.")
	}

	onlySet := map[string]struct{}{}
	for _, s := range strings.Split(*only, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			onlySet[s] = struct{}{}
		}
	}

	rows, err := pool.Query(ctx, `
		SELECT a.id::text, a.slug, a.title, a.body_markdown
		FROM artifacts a
		JOIN projects p ON p.id = a.project_id
		WHERE p.slug = $1 AND a.status <> 'archived'
		ORDER BY a.updated_at
	`, cfg.ProjectSlug)
	if err != nil {
		logger.Error("load artifacts", "err", err)
		os.Exit(1)
	}

	type art struct{ id, slug, title, body string }
	var all []art
	for rows.Next() {
		var a art
		if err := rows.Scan(&a.id, &a.slug, &a.title, &a.body); err != nil {
			logger.Error("scan artifact", "err", err)
			os.Exit(1)
		}
		if len(onlySet) > 0 {
			if _, ok := onlySet[a.slug]; !ok {
				continue
			}
		}
		all = append(all, a)
	}
	rows.Close()

	logger.Info("plan", "project", cfg.ProjectSlug, "artifacts", len(all), "dry_run", *dryRun)

	ok, fail := 0, 0
	for _, a := range all {
		if *dryRun {
			logger.Info("would re-embed", "slug", a.slug)
			continue
		}
		if err := reembedOne(ctx, pool, provider, a.id, a.title, a.body); err != nil {
			logger.Error("re-embed failed", "slug", a.slug, "err", err)
			fail++
			continue
		}
		logger.Info("re-embedded", "slug", a.slug)
		ok++
	}

	logger.Info("done", "ok", ok, "fail", fail, "skipped_dry_run", *dryRun)
	if fail > 0 {
		os.Exit(1)
	}
}

// reembedOne is the per-artifact transaction: drop existing chunks, then
// insert fresh title + body chunks produced by the current provider. Mirrors
// the logic in internal/pindoc/mcp/tools/artifact_propose.go::embedAndStoreChunks
// but lives here because the tools package doesn't export it and we don't
// want a dependency edge tools → cmd.
func reembedOne(ctx context.Context, pool *db.Pool, provider embed.Provider, artifactID, title, body string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM artifact_chunks WHERE artifact_id = $1`, artifactID); err != nil {
		return fmt.Errorf("purge: %w", err)
	}
	if err := embedAndStoreChunks(ctx, tx, provider, artifactID, title, body); err != nil {
		return fmt.Errorf("embed: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func embedAndStoreChunks(ctx context.Context, tx pgx.Tx, provider embed.Provider, artifactID, title, body string) error {
	info := provider.Info()

	titleRes, err := provider.Embed(ctx, embed.Request{Texts: []string{title}, Kind: embed.KindDocument})
	if err != nil {
		return fmt.Errorf("embed title: %w", err)
	}
	if len(titleRes.Vectors) != 1 {
		return fmt.Errorf("embed title: got %d vectors", len(titleRes.Vectors))
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO artifact_chunks (
			artifact_id, kind, chunk_index, heading, span_start, span_end,
			text, embedding, model_name, model_dim
		) VALUES ($1, 'title', 0, NULL, 0, 0, $2, $3::vector, $4, $5)
	`, artifactID, title,
		embed.VectorString(embed.PadTo768(titleRes.Vectors[0])),
		info.Name+":"+info.ModelID, info.Dimension,
	); err != nil {
		return fmt.Errorf("store title chunk: %w", err)
	}

	chunks := embed.ChunkBody(title, body, 600)
	if len(chunks) == 0 {
		return nil
	}
	// Batch requests at TEI's hard max (32 items per HTTP call). Large
	// artifacts can produce 40+ chunks and TEI rejects the whole batch with
	// a 413 if we over-shoot. Keep the batch size conservative so swapping
	// providers later doesn't force re-tuning.
	const batchSize = 32
	allVecs := make([][]float32, 0, len(chunks))
	for start := 0; start < len(chunks); start += batchSize {
		end := start + batchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		batchTexts := make([]string, 0, end-start)
		for _, c := range chunks[start:end] {
			batchTexts = append(batchTexts, c.Text)
		}
		res, err := provider.Embed(ctx, embed.Request{Texts: batchTexts, Kind: embed.KindDocument})
		if err != nil {
			return fmt.Errorf("embed body batch [%d:%d]: %w", start, end, err)
		}
		if len(res.Vectors) != end-start {
			return fmt.Errorf("embed body batch [%d:%d]: got %d want %d", start, end, len(res.Vectors), end-start)
		}
		allVecs = append(allVecs, res.Vectors...)
	}
	bodyRes := &embed.Response{Vectors: allVecs}
	if len(bodyRes.Vectors) != len(chunks) {
		return fmt.Errorf("embed body: got %d want %d", len(bodyRes.Vectors), len(chunks))
	}
	for i, c := range chunks {
		var heading any
		if c.Heading != "" {
			heading = c.Heading
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO artifact_chunks (
				artifact_id, kind, chunk_index, heading, span_start, span_end,
				text, embedding, model_name, model_dim
			) VALUES ($1, 'body', $2, $3, $4, $5, $6, $7::vector, $8, $9)
		`, artifactID, c.Index, heading, c.SpanStart, c.SpanEnd, c.Text,
			embed.VectorString(embed.PadTo768(bodyRes.Vectors[i])),
			info.Name+":"+info.ModelID, info.Dimension,
		); err != nil {
			return fmt.Errorf("store body chunk %d: %w", c.Index, err)
		}
	}
	return nil
}

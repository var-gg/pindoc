// pindoc-reembed refreshes artifact search chunks with the configured
// embedding provider. Every artifact runs in its own transaction. Provider
// failures preserve the last known-good chunks and commit a retryable failed
// state; successful attempts replace chunks and provenance atomically.
//
// Usage:
//
//	go run ./cmd/pindoc-reembed
//	go run ./cmd/pindoc-reembed -dry-run -state needs-refresh
//	go run ./cmd/pindoc-reembed -state failed
//	go run ./cmd/pindoc-reembed -state stale -only slug1,slug2
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/var-gg/pindoc/internal/pindoc/config"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/embed"
	"github.com/var-gg/pindoc/internal/pindoc/indexstate"
)

const (
	stateAll          = "all"
	stateNeedsRefresh = "needs-refresh"
)

type artifactPlan struct {
	ID             string
	Slug           string
	Title          string
	Body           string
	RevisionNumber int
	StoredState    *indexstate.State
	EffectiveState string
}

func main() {
	var (
		dryRun    = flag.Bool("dry-run", false, "list artifacts that would be re-embedded without writing")
		only      = flag.String("only", "", "comma-separated slugs to limit the run (empty = all)")
		stateMode = flag.String("state", stateAll, "select all|needs-refresh|failed|stale|unknown|indexed")
	)
	flag.Parse()

	mode := strings.ToLower(strings.TrimSpace(*stateMode))
	if !validStateMode(mode) {
		fmt.Fprintf(os.Stderr, "invalid -state %q (want all|needs-refresh|failed|stale|unknown|indexed)\n", *stateMode)
		os.Exit(2)
	}

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
	if err := db.Migrate(ctx, pool.Pool); err != nil {
		logger.Error("db migrate", "err", err)
		os.Exit(1)
	}

	provider, err := embed.Build(cfg.Embed)
	if err != nil {
		logger.Error("embed build", "err", err)
		os.Exit(1)
	}
	info := provider.Info()
	logger.Info("provider ready", "name", info.Name, "model", info.ModelID, "dim", info.Dimension)
	if info.Name == "stub" {
		logger.Warn("embed provider is 'stub'; the run repairs state transitions but does not improve semantic retrieval quality")
	}

	onlySet := parseOnly(*only)
	plans, err := loadPlans(ctx, pool, cfg.ProjectSlug, info, onlySet, mode)
	if err != nil {
		logger.Error("load re-embed plan", "err", err)
		os.Exit(1)
	}

	counts := map[string]int{}
	for _, plan := range plans {
		counts[plan.EffectiveState]++
	}
	logger.Info("plan",
		"project", cfg.ProjectSlug,
		"artifacts", len(plans),
		"state_filter", mode,
		"state_counts", stableCounts(counts),
		"dry_run", *dryRun,
	)

	ok, fail := 0, 0
	for _, plan := range plans {
		if *dryRun {
			logger.Info("would re-embed", "slug", plan.Slug, "effective_state", plan.EffectiveState, "revision_number", plan.RevisionNumber)
			continue
		}
		state, err := reembedOne(ctx, pool, provider, plan)
		if err != nil {
			logger.Error("re-embed failed",
				"slug", plan.Slug,
				"status", state.Status,
				"attempt_count", state.AttemptCount,
				"retryable", indexstate.IsRetryable(err),
				"err", err,
			)
			fail++
			continue
		}
		logger.Info("re-embedded", "slug", plan.Slug, "revision_number", state.RevisionNumber, "attempt_count", state.AttemptCount)
		ok++
	}

	logger.Info("done", "ok", ok, "fail", fail, "skipped_dry_run", *dryRun)
	if fail > 0 {
		os.Exit(1)
	}
}

func loadPlans(
	ctx context.Context,
	pool *db.Pool,
	projectSlug string,
	provider embed.Info,
	onlySet map[string]struct{},
	mode string,
) ([]artifactPlan, error) {
	rows, err := pool.Query(ctx, `
		SELECT
			a.id::text,
			a.slug,
			a.title,
			a.body_markdown,
			COALESCE(r.revision_number, 0),
			s.artifact_id IS NOT NULL,
			COALESCE(s.revision_number, 0),
			COALESCE(s.body_hash, ''),
			COALESCE(s.title_hash, ''),
			COALESCE(s.model_name, ''),
			COALESCE(s.model_id, ''),
			COALESCE(s.model_dim, 0),
			COALESCE(s.status, 'unknown'),
			COALESCE(s.attempt_count, 0),
			COALESCE(s.last_error, '')
		FROM artifacts a
		JOIN projects p ON p.id = a.project_id
		LEFT JOIN LATERAL (
			SELECT revision_number
			FROM artifact_revisions
			WHERE artifact_id = a.id
			ORDER BY revision_number DESC
			LIMIT 1
		) r ON TRUE
		LEFT JOIN artifact_index_state s ON s.artifact_id = a.id
		WHERE p.slug = $1 AND a.status <> 'archived'
		ORDER BY a.updated_at, a.slug
	`, projectSlug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var plans []artifactPlan
	for rows.Next() {
		var (
			plan     artifactPlan
			hasState bool
			stored   indexstate.State
		)
		if err := rows.Scan(
			&plan.ID,
			&plan.Slug,
			&plan.Title,
			&plan.Body,
			&plan.RevisionNumber,
			&hasState,
			&stored.RevisionNumber,
			&stored.BodyHash,
			&stored.TitleHash,
			&stored.ModelName,
			&stored.ModelID,
			&stored.ModelDim,
			&stored.Status,
			&stored.AttemptCount,
			&stored.LastError,
		); err != nil {
			return nil, err
		}
		if len(onlySet) > 0 {
			if _, ok := onlySet[plan.Slug]; !ok {
				continue
			}
		}
		if hasState {
			plan.StoredState = &stored
		}
		plan.EffectiveState = indexstate.Classify(plan.Title, plan.Body, plan.StoredState, provider)
		if matchesStateMode(mode, plan.EffectiveState) {
			plans = append(plans, plan)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return plans, nil
}

// reembedOne commits a provider failure so the retryable failed state is
// observable. Storage/database failures still roll the transaction back.
func reembedOne(ctx context.Context, pool *db.Pool, provider embed.Provider, plan artifactPlan) (indexstate.State, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return indexstate.State{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	state, refreshErr := indexstate.Refresh(ctx, tx, provider, indexstate.RefreshInput{
		ArtifactID:     plan.ID,
		RevisionNumber: plan.RevisionNumber,
		Title:          plan.Title,
		Body:           plan.Body,
	})
	if refreshErr != nil && !indexstate.IsRetryable(refreshErr) {
		return indexstate.State{}, refreshErr
	}
	if err := tx.Commit(ctx); err != nil {
		return indexstate.State{}, fmt.Errorf("commit: %w", err)
	}
	return state, refreshErr
}

func validStateMode(mode string) bool {
	switch mode {
	case stateAll, stateNeedsRefresh, indexstate.StatusFailed, indexstate.StatusStale, indexstate.StatusUnknown, indexstate.StatusIndexed:
		return true
	default:
		return false
	}
}

func matchesStateMode(mode, effective string) bool {
	switch mode {
	case stateAll:
		return true
	case stateNeedsRefresh:
		return effective != indexstate.StatusIndexed
	default:
		return effective == mode
	}
}

func parseOnly(raw string) map[string]struct{} {
	result := map[string]struct{}{}
	for _, value := range strings.Split(raw, ",") {
		value = strings.TrimSpace(value)
		if value != "" {
			result[value] = struct{}{}
		}
	}
	return result
}

func stableCounts(counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, counts[key]))
	}
	return strings.Join(parts, ",")
}

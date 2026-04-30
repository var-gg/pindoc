package mcp

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/mcp/tools"
)

const (
	mcpRuntimeStateToolsetVersionKey = "toolset_version"
	toolsetListChangedPollInterval   = 50 * time.Millisecond
)

type toolsetChangeNotice struct {
	Current  string
	Previous string
	Changed  bool
}

func recordToolsetVersion(ctx context.Context, pool *db.Pool, current string) (toolsetChangeNotice, error) {
	current = strings.TrimSpace(current)
	notice := toolsetChangeNotice{Current: current}
	if pool == nil || current == "" {
		return notice, nil
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return notice, err
	}
	defer tx.Rollback(ctx)

	var previous string
	err = tx.QueryRow(ctx, `
		SELECT value
		  FROM mcp_runtime_state
		 WHERE key = $1
		 FOR UPDATE
	`, mcpRuntimeStateToolsetVersionKey).Scan(&previous)
	switch {
	case err == nil:
		previous = strings.TrimSpace(previous)
	case errors.Is(err, pgx.ErrNoRows):
		if last, ok, lastErr := lastTelemetryToolsetVersion(ctx, tx); lastErr != nil {
			return notice, lastErr
		} else if ok {
			previous = last
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO mcp_runtime_state (key, value, updated_at)
			VALUES ($1, $2, now())
		`, mcpRuntimeStateToolsetVersionKey, current); err != nil {
			return notice, err
		}
	default:
		return notice, err
	}

	notice.Previous = previous
	notice.Changed = shouldNotifyToolsetListChanged(previous != "", previous, current)
	if previous != current {
		if _, err := tx.Exec(ctx, `
			UPDATE mcp_runtime_state
			   SET value = $2, updated_at = now()
			 WHERE key = $1
		`, mcpRuntimeStateToolsetVersionKey, current); err != nil {
			return notice, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return notice, err
	}
	return notice, nil
}

type toolsetVersionQueryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func lastTelemetryToolsetVersion(ctx context.Context, q toolsetVersionQueryer) (string, bool, error) {
	var previous string
	err := q.QueryRow(ctx, `
		SELECT toolset_version
		  FROM mcp_tool_calls
		 WHERE COALESCE(toolset_version, '') <> ''
		 ORDER BY started_at DESC, id DESC
		 LIMIT 1
	`).Scan(&previous)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	previous = strings.TrimSpace(previous)
	return previous, previous != "", nil
}

func shouldNotifyToolsetListChanged(hadPrevious bool, previous, current string) bool {
	previous = strings.TrimSpace(previous)
	current = strings.TrimSpace(current)
	return hadPrevious && previous != "" && current != "" && previous != current
}

type toolsetListChangedNotifier struct {
	notice toolsetChangeNotice
	once   sync.Once
}

func (n *toolsetListChangedNotifier) start(ctx context.Context, server *sdk.Server, logger *slog.Logger) {
	if n == nil || !n.notice.Changed || server == nil {
		return
	}
	n.once.Do(func() {
		go n.watch(ctx, server, logger)
	})
}

func (n *toolsetListChangedNotifier) watch(ctx context.Context, server *sdk.Server, logger *slog.Logger) {
	ticker := time.NewTicker(toolsetListChangedPollInterval)
	defer ticker.Stop()
	for {
		if notifyToolsetListChanged(ctx, server, logger, n.notice) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func notifyToolsetListChanged(_ context.Context, server *sdk.Server, logger *slog.Logger, notice toolsetChangeNotice) bool {
	if !hasInitializedServerSession(server) {
		return false
	}
	toolName, ok := tools.ReannounceToolListChanged(server)
	if !ok {
		if logger != nil {
			logger.Warn("toolset list_changed notification skipped; no reannouncable tool registered",
				"previous_toolset_version", notice.Previous,
				"current_toolset_version", notice.Current,
			)
		}
		return true
	}
	if logger != nil {
		logger.Info("mcp tools/list_changed notification emitted",
			"reannounced_tool", toolName,
			"previous_toolset_version", notice.Previous,
			"current_toolset_version", notice.Current,
		)
	}
	return true
}

func hasInitializedServerSession(server *sdk.Server) bool {
	if server == nil {
		return false
	}
	for session := range server.Sessions() {
		if session != nil && session.InitializeParams() != nil {
			return true
		}
	}
	return false
}

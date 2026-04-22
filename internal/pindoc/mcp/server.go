// Package mcp wires the official Go MCP SDK to Pindoc's tool surface.
//
// Tools live in sub-packages under ./tools. This package owns the server
// lifecycle: it constructs the sdk.Server, registers every tool, and
// exposes a single Run entry point the main() binary calls.
package mcp

import (
	"context"
	"log/slog"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/config"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/embed"
	"github.com/var-gg/pindoc/internal/pindoc/mcp/tools"
	"github.com/var-gg/pindoc/internal/pindoc/receipts"
)

type Options struct {
	Name     string
	Version  string
	Logger   *slog.Logger
	Config   *config.Config
	DB       *db.Pool
	Embedder embed.Provider

	// AgentID is the server-issued identity for this subprocess (Phase
	// 12c). Set by the binary entrypoint at startup; empty falls back to
	// "unassigned" which still lets writes proceed but flags the gap in
	// audit logs.
	AgentID string
}

type Server struct {
	sdk    *sdk.Server
	logger *slog.Logger
}

func NewServer(opts Options) *Server {
	impl := &sdk.Implementation{
		Name:    opts.Name,
		Version: opts.Version,
	}
	s := sdk.NewServer(impl, nil)

	// Phase 1: handshake.
	tools.RegisterPing(s, tools.PingDeps{
		Version:      opts.Version,
		UserLanguage: opts.Config.UserLanguage,
	})

	// Phase 2 read-side: project context + scope enumeration + artifact fetch.
	deps := tools.Deps{
		DB:           opts.DB,
		Logger:       opts.Logger,
		Version:      opts.Version,
		ProjectSlug:  opts.Config.ProjectSlug,
		UserLanguage: opts.Config.UserLanguage,
		Embedder:     opts.Embedder,
		MultiProject: opts.Config.MultiProject,
		Receipts:     receipts.New(0), // DefaultTTL = 10 min
		AgentID:      opts.AgentID,
	}
	tools.RegisterProjectCurrent(s, deps)
	tools.RegisterProjectCreate(s, deps)
	tools.RegisterAreaList(s, deps)
	tools.RegisterArtifactRead(s, deps)

	// Phase 2.3 write-side + Phase 3 retrieval.
	tools.RegisterArtifactPropose(s, deps)
	tools.RegisterHarnessInstall(s, deps)
	tools.RegisterArtifactSearch(s, deps)
	tools.RegisterContextForTask(s, deps)

	// Phase 7 revision history.
	tools.RegisterArtifactRevisions(s, deps)
	tools.RegisterArtifactDiff(s, deps)
	tools.RegisterArtifactSummary(s, deps)

	return &Server{
		sdk:    s,
		logger: opts.Logger,
	}
}

// Run blocks until the transport returns (client disconnected, ctx cancelled,
// or fatal error). Graceful shutdown on ctx cancel is handled by the SDK.
func (s *Server) Run(ctx context.Context, transport sdk.Transport) error {
	s.logger.Info("mcp server ready",
		"tools", []string{
			"pindoc.ping",
			"pindoc.project.current",
			"pindoc.project.create",
			"pindoc.area.list",
			"pindoc.artifact.read",
			"pindoc.artifact.propose",
			"pindoc.harness.install",
			"pindoc.artifact.search",
			"pindoc.context.for_task",
			"pindoc.artifact.revisions",
			"pindoc.artifact.diff",
			"pindoc.artifact.summary_since",
		})
	return s.sdk.Run(ctx, transport)
}

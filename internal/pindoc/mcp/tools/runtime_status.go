package tools

import (
	"context"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
)

// runtimeStatusInput is intentionally empty: pindoc.runtime.status is a
// snapshot probe with no filtering. Reserved for future selectors
// (e.g. include=ports,tools) when the response grows large.
type runtimeStatusInput struct{}

// runtimeStatusPort is one entry in the configured-ports list. healthy is
// always true today because the listing process is the same one
// answering this request — the field exists so callers / future
// out-of-process probes can disagree.
type runtimeStatusPort struct {
	Name    string `json:"name"`
	Port    int    `json:"port"`
	Healthy bool   `json:"healthy"`
}

type runtimeStatusOutput struct {
	// Version is deps.Version — the build version the server boots with.
	Version string `json:"version"`

	// ServerCommit is vcs.revision from the Go build info. Empty when the
	// binary was built outside the module (go run ./...) or with VCS
	// stamping disabled.
	ServerCommit string `json:"server_commit,omitempty"`

	// BuildModified is vcs.modified from the build info — true if the
	// repo had uncommitted changes when this binary was built.
	BuildModified bool `json:"build_modified,omitempty"`

	// ToolsetVersion is the catalog hash agents compare across sessions
	// to spot "the server grew a tool I cannot see yet". Same value
	// pindoc.ping returns.
	ToolsetVersion string `json:"toolset_version"`

	// ToolCount is len(RegisteredTools) — a redundant convenience to
	// save callers from parsing toolset_version.
	ToolCount int `json:"tool_count"`

	// AuthMode echoes the resolver that produced the calling Principal.
	// trusted_local in V1; oauth_github / bearer_token become possible
	// once the chain grows.
	AuthMode string `json:"auth_mode,omitempty"`

	// Ports lists the conventional listeners (http=5830, sidecar=5832)
	// with whatever override env vars set them to, so the operator can
	// see at a glance which port their MCP client should be hitting.
	Ports []runtimeStatusPort `json:"ports"`

	// ContainerID is the Docker short id when running under Docker
	// (HOSTNAME is 12 hex chars by default). Empty on non-container
	// hosts so callers can switch on presence.
	ContainerID string `json:"container_id,omitempty"`

	// ImageTag is PINDOC_IMAGE_TAG when the operator pinned it (the
	// Compose / Helm chart sets this). Empty when not set.
	ImageTag string `json:"image_tag,omitempty"`

	// Hostname is os.Hostname() — useful when ContainerID is empty
	// (host process) and the operator wants to confirm which machine
	// answered the request.
	Hostname string `json:"hostname,omitempty"`

	// Transport is "stdio" | "streamable_http". Carried for
	// Diagnostic only; account-level scope removed any handler-side
	// branching on it.
	Transport string `json:"transport,omitempty"`

	// GoVersion is runtime.Version() — included because module drift on
	// pgx / sdk is the kind of thing this snapshot exists to surface.
	GoVersion string `json:"go_version,omitempty"`

	// DBHealthy is the result of a single deps.DB.Ping with the request
	// context. False when the pool was never built (tests) or the DB is
	// unreachable.
	DBHealthy bool `json:"db_healthy"`

	// Notice nudges callers toward the most common interpretation of a
	// toolset_version mismatch — restart the session.
	Notice string `json:"notice,omitempty"`
}

// RegisterRuntimeStatus wires pindoc.runtime.status. Read-only diagnostic
// snapshot consolidating ports / commit / toolset / container / db so
// the operator does not have to grep three places to triage 5830 vs
// 5832 mix-ups or "did the new tool actually land?". Decision mcp-dx-
// 외부-리뷰-codex-1차-피드백-6항목 발견 2 + 6.
func RegisterRuntimeStatus(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name:        "pindoc.runtime.status",
			Description: "Read-only diagnostic snapshot. Returns server version, git commit (when build embedded vcs info), MCP toolset_version + tool_count, configured ports (HTTP + sidecar) with overrides, container_id / image_tag / hostname, auth_mode of the calling principal, transport, Go runtime version, and DB connectivity. Use when triaging port mix-ups (5830 vs 5832), 'restart needed?' after a tool catalog bump, or any quick environment check. No mutations.",
		},
		func(ctx context.Context, p *auth.Principal, _ runtimeStatusInput) (*sdk.CallToolResult, runtimeStatusOutput, error) {
			commit, modified := readBuildVCS()
			authMode := ""
			if p != nil {
				authMode = p.AuthMode
			}
			dbHealthy := false
			if deps.DB != nil {
				if err := deps.DB.Ping(ctx); err == nil {
					dbHealthy = true
				}
			}
			hostname, _ := os.Hostname()
			return nil, runtimeStatusOutput{
				Version:        deps.Version,
				ServerCommit:   commit,
				BuildModified:  modified,
				ToolsetVersion: ToolsetVersion(),
				ToolCount:      len(RegisteredTools),
				AuthMode:       authMode,
				Ports:          configuredPorts(),
				ContainerID:    detectContainerID(),
				ImageTag:       strings.TrimSpace(os.Getenv("PINDOC_IMAGE_TAG")),
				Hostname:       hostname,
				Transport:      deps.Transport,
				GoVersion:      runtime.Version(),
				DBHealthy:      dbHealthy,
				Notice:         "Diagnostic snapshot is read-only. toolset_version mismatch between this response and the client schema cache means the catalog grew — restart the MCP session.",
			}, nil
		},
	)
}

// readBuildVCS reads the vcs.* settings the Go toolchain (1.18+) embeds
// into module builds. Returns ("", false) when the binary was built
// outside the module (go run ./...) or VCS stamping was disabled with
// -buildvcs=false. modified=true means the working tree had uncommitted
// changes at build time — useful signal when the operator suspects a
// dirty deploy.
func readBuildVCS() (string, bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", false
	}
	var revision string
	var modified bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	return revision, modified
}

// configuredPorts returns the conventional listeners with optional env
// overrides. Defaults (http=5830, sidecar=5832) match the Compose /
// daemon documentation; PINDOC_HTTP_PORT / PINDOC_SIDECAR_PORT override
// either. healthy is always true here because the listing process is
// the one answering — the field exists so an out-of-process probe can
// override the value in a future revision.
func configuredPorts() []runtimeStatusPort {
	ports := []runtimeStatusPort{
		resolvePort("http", "PINDOC_HTTP_PORT", 5830),
		resolvePort("sidecar", "PINDOC_SIDECAR_PORT", 5832),
	}
	return ports
}

// resolvePort reads the env var override and falls back to the default
// when it is missing or unparseable. Healthy mirrors the in-process
// assumption documented on configuredPorts.
func resolvePort(name, env string, fallback int) runtimeStatusPort {
	port := fallback
	if v := strings.TrimSpace(os.Getenv(env)); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			port = parsed
		}
	}
	return runtimeStatusPort{Name: name, Port: port, Healthy: true}
}

// detectContainerID returns Docker's short hostname-as-cid when running
// inside a container, otherwise empty. Docker sets HOSTNAME to the
// 12-char shortened container id by default; non-container environments
// have HOSTNAME set to the actual hostname which doesn't match the
// 12-hex shape. Best-effort — operators on Kubernetes / Podman get an
// empty string and should rely on Hostname instead.
func detectContainerID() string {
	h, err := os.Hostname()
	if err != nil {
		return ""
	}
	h = strings.TrimSpace(h)
	if len(h) != 12 {
		return ""
	}
	for _, r := range h {
		isHex := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')
		if !isHex {
			return ""
		}
	}
	return h
}

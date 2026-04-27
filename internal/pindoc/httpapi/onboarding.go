package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

// task: agent-era first-time identity flow.
//
// POST /api/onboarding/identity creates (or rebinds) the loopback
// principal's users.id row from a Reader-side form so a fresh install
// never has to touch env to attribute work. Loopback only, idempotent
// when the email already matches a row.
//
// On success: the form returns the new user_id + the MCP connection
// shapes the Reader's success screen needs to copy-paste into Claude
// Code. Decision agent-only-write-분할 + Decision decision-auth-
// model-loopback-and-providers § 2 cover the loopback Trust the
// handler relies on.

type onboardingIdentityRequest struct {
	DisplayName  string `json:"display_name"`
	Email        string `json:"email"`
	GithubHandle string `json:"github_handle,omitempty"`
}

type onboardingIdentityResponse struct {
	Status     string                  `json:"status"`
	UserID     string                  `json:"user_id"`
	DisplayName string                 `json:"display_name"`
	Email      string                  `json:"email"`
	Project    onboardingProjectRef    `json:"project"`
	MCPConnect onboardingMCPConnect    `json:"mcp_connect"`
}

type onboardingProjectRef struct {
	Slug string `json:"slug"`
	Role string `json:"role"`
	URL  string `json:"url"`
}

// onboardingMCPConnect bundles the three flavours of MCP connection
// payload the success screen offers — bare URL, full `.mcp.json`
// snippet, and an agent-ready prompt — so the FE doesn't have to
// reconstruct any of them. Each field is ready to paste verbatim.
type onboardingMCPConnect struct {
	URL         string `json:"url"`
	MCPJSON     string `json:"mcp_json"`
	AgentPrompt string `json:"agent_prompt"`
}

type onboardingError struct {
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
}

func writeOnboardingError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, onboardingError{ErrorCode: code, Message: message})
}

func (d Deps) handleOnboardingIdentity(w http.ResponseWriter, r *http.Request) {
	if d.DB == nil {
		writeOnboardingError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "database pool not configured")
		return
	}
	if d.Settings == nil {
		writeOnboardingError(w, http.StatusServiceUnavailable, "SETTINGS_UNAVAILABLE", "server settings store not configured")
		return
	}
	if !d.isLoopbackOrTrustedProxy(r) {
		writeOnboardingError(w, http.StatusForbidden, "INSTANCE_OWNER_REQUIRED", "identity setup is loopback only")
		return
	}

	var in onboardingIdentityRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeOnboardingError(w, http.StatusBadRequest, "BAD_JSON", "could not parse request body as JSON")
		return
	}
	displayName := strings.TrimSpace(in.DisplayName)
	if displayName == "" {
		writeOnboardingError(w, http.StatusBadRequest, "DISPLAY_NAME_REQUIRED", "display_name is required")
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" {
		writeOnboardingError(w, http.StatusBadRequest, "EMAIL_REQUIRED", "email is required")
		return
	}
	githubHandle := strings.TrimSpace(in.GithubHandle)
	githubHandle = strings.TrimPrefix(githubHandle, "@")

	userID, err := upsertOnboardingUser(r.Context(), d, displayName, email, githubHandle)
	if err != nil {
		if d.Logger != nil {
			d.Logger.Error("onboarding identity upsert failed", "err", err)
		}
		writeOnboardingError(w, http.StatusInternalServerError, "USER_UPSERT_FAILED", "failed to create or rebind user row")
		return
	}

	if err := d.Settings.SetDefaultLoopbackUserID(r.Context(), userID); err != nil {
		if d.Logger != nil {
			d.Logger.Error("onboarding settings write failed", "err", err)
		}
		writeOnboardingError(w, http.StatusInternalServerError, "SETTINGS_WRITE_FAILED", "failed to bind user to instance")
		return
	}

	projectSlug := strings.TrimSpace(d.DefaultProjectSlug)
	if projectSlug == "" {
		projectSlug = "pindoc"
	}
	if err := projects.EnsureDefaultProjectOwnerMembership(r.Context(), d.DB, projectSlug, userID); err != nil && d.Logger != nil {
		d.Logger.Warn("onboarding owner membership reconcile failed",
			"err", err, "project_slug", projectSlug, "user_id", userID)
	}

	mcpURL := onboardingMCPURL(d, r)
	resp := onboardingIdentityResponse{
		Status:      "ok",
		UserID:      userID,
		DisplayName: displayName,
		Email:       email,
		Project: onboardingProjectRef{
			Slug: projectSlug,
			Role: "owner",
			URL:  "/p/" + projectSlug + "/today",
		},
		MCPConnect: onboardingMCPConnect{
			URL:         mcpURL,
			MCPJSON:     onboardingMCPJSON(mcpURL),
			AgentPrompt: onboardingAgentPrompt(mcpURL, displayName),
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

// upsertOnboardingUser is a thin wrapper that picks the canonical
// user row for (email, display_name). Mirrors UpsertUserFromEnv but
// also honours github_handle when present and sources the row as
// `harness_install` so it shows up alongside env-seeded users in the
// Reader's user list.
func upsertOnboardingUser(ctx context.Context, d Deps, displayName, email, githubHandle string) (string, error) {
	var existing string
	err := d.DB.QueryRow(ctx, `
		SELECT id::text FROM users
		 WHERE lower(email) = $1 AND deleted_at IS NULL
		 LIMIT 1
	`, email).Scan(&existing)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("lookup user by email: %w", err)
	}
	if existing != "" {
		// Sync display_name / github_handle so the form acts as a
		// quick rename even when an existing row matches by email.
		if _, err := d.DB.Exec(ctx, `
			UPDATE users
			   SET display_name  = $1,
			       github_handle = NULLIF($2, ''),
			       updated_at    = now()
			 WHERE id = $3::uuid
		`, displayName, githubHandle, existing); err != nil {
			return "", fmt.Errorf("update user identity: %w", err)
		}
		return existing, nil
	}
	var newID string
	if err := d.DB.QueryRow(ctx, `
		INSERT INTO users (display_name, email, github_handle, source)
		VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), 'harness_install')
		RETURNING id::text
	`, displayName, email, githubHandle).Scan(&newID); err != nil {
		return "", fmt.Errorf("insert user: %w", err)
	}
	return newID, nil
}

// onboardingMCPURL resolves the MCP daemon URL the Reader's success
// screen pastes into agent configs. Prefers the operator-set
// public_base_url so reverse-proxy deployments get the public host;
// otherwise falls back to the daemon's BindAddr (or the README's
// 127.0.0.1:5830 default).
func onboardingMCPURL(d Deps, r *http.Request) string {
	if d.Settings != nil {
		if base := strings.TrimRight(strings.TrimSpace(d.Settings.Get().PublicBaseURL), "/"); base != "" {
			return base + "/mcp"
		}
	}
	host := strings.TrimSpace(r.Host)
	if host == "" {
		host = strings.TrimSpace(d.BindAddr)
	}
	if host == "" {
		host = "127.0.0.1:5830"
	}
	scheme := "http"
	if proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); proto != "" {
		scheme = proto
	}
	return scheme + "://" + host + "/mcp"
}

// onboardingMCPJSON formats the `.mcp.json` snippet with the same
// shape README documents so the user can copy it verbatim into a
// workspace's `.mcp.json` file.
func onboardingMCPJSON(mcpURL string) string {
	var sb strings.Builder
	sb.WriteString("{\n")
	sb.WriteString("  \"mcpServers\": {\n")
	sb.WriteString("    \"pindoc\": {\n")
	sb.WriteString("      \"type\": \"http\",\n")
	sb.WriteString("      \"url\": \"" + mcpURL + "\"\n")
	sb.WriteString("    }\n")
	sb.WriteString("  }\n")
	sb.WriteString("}\n")
	return sb.String()
}

// onboardingAgentPrompt is a single message the operator can paste
// into a Claude Code / Codex / Cursor chat to register the MCP server
// and verify it. The prompt asks the agent to add the entry, restart,
// then call pindoc.ping — covers the full bootstrap loop without the
// operator hand-editing files.
func onboardingAgentPrompt(mcpURL, displayName string) string {
	greeting := "Hi"
	if strings.TrimSpace(displayName) != "" {
		greeting = "Hi, I'm " + displayName + ". "
	}
	return greeting + "Please register the Pindoc MCP server in this workspace. " +
		"Add this entry to ~/.config/claude-code/mcp.json (or the equivalent for your client):\n\n" +
		onboardingMCPJSON(mcpURL) +
		"\nAfter saving, restart your MCP session and call pindoc.ping to confirm the handshake. " +
		"Then run pindoc.harness.install to drop a PINDOC.md harness into the workspace root."
}

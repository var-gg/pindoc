package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
)

// Author identity dual (Decision `decision-author-identity-dual`,
// migration 0014). Two MCP tools + helpers.
//
// pindoc.user.current: return the user row bound to this MCP session.
// pindoc.user.update:  mutate display_name / email / github_handle with
//                       events.user_rename audit row on change.
//
// The artifact.propose path reads Principal.UserID (populated from the
// boot-time upsert via the trusted_local resolver) and writes it into
// artifacts.author_user_id so every revision lands with the dual identity.

// displayNameMin / displayNameMax are the validation range for
// display_name. Phase 14's D-title-rule used 15-80 runes for artifact
// titles; user names follow a shorter range (2-60) because CJK single-
// syllable given names are already 1-2 runes.
const (
	displayNameMin = 2
	displayNameMax = 60
)

// UserRow is the shape we return to agents. `Source` reflects where the
// row originated (harness_install / pindoc_admin / github_oauth) so V1.5
// OAuth can migrate rows in place.
type UserRow struct {
	ID           string    `json:"id"`
	DisplayName  string    `json:"display_name"`
	Email        string    `json:"email,omitempty"`
	GithubHandle string    `json:"github_handle,omitempty"`
	Source       string    `json:"source"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type userCurrentOutput struct {
	Status         string               `json:"status"`
	Code           string               `json:"code,omitempty"`
	ErrorCode      string               `json:"error_code,omitempty"`
	Failed         []string             `json:"failed,omitempty"`
	ErrorCodes     []string             `json:"error_codes,omitempty" jsonschema:"canonical stable SCREAMING_SNAKE_CASE identifiers; branch on these"`
	Checklist      []string             `json:"checklist,omitempty"`
	ChecklistItems []ErrorChecklistItem `json:"checklist_items,omitempty" jsonschema:"localized checklist entries paired with stable codes"`
	MessageLocale  string               `json:"message_locale,omitempty" jsonschema:"locale used for checklist/checklist_items.message after fallback"`
	Hints          []string             `json:"hints,omitempty"`
	User           *UserRow             `json:"user,omitempty"`
	ToolsetVersion string               `json:"toolset_version,omitempty"`
}

// RegisterUserCurrent wires pindoc.user.current. Missing user identity is
// informational: artifact.propose can still use the server-issued agent
// identity and client-reported author_id. Only missing agent identity is a
// real not_ready blocker.
func RegisterUserCurrent(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name:        "pindoc.user.current",
			Description: "Return the user (display_name / email / github_handle / source) bound to this MCP session. Populated at server startup from PINDOC_USER_NAME / PINDOC_USER_EMAIL. Missing user identity returns status=informational with USER_NOT_SET hints; artifact.propose can still use agent author_id.",
		},
		func(ctx context.Context, p *auth.Principal, _ struct{}) (*sdk.CallToolResult, userCurrentOutput, error) {
			if out, handled := classifyUserCurrentIdentity(p); handled {
				return nil, out, nil
			}
			row, err := loadUserByID(ctx, deps, p.UserID)
			if err != nil {
				return nil, userCurrentOutput{}, fmt.Errorf("load user: %w", err)
			}
			return nil, userCurrentOutput{
				Status: "ok",
				User:   row,
			}, nil
		},
	)
}

func classifyUserCurrentIdentity(p *auth.Principal) (userCurrentOutput, bool) {
	if p == nil || strings.TrimSpace(p.AgentID) == "" {
		return userCurrentOutput{
			Status:    "not_ready",
			ErrorCode: "AGENT_NOT_SET",
			Failed:    []string{"AGENT_NOT_SET"},
			Checklist: []string{
				"MCP session has no server-issued agent identity. Restart the MCP server so trusted_local can mint an agent identity before using user.current.",
			},
		}, true
	}
	if strings.TrimSpace(p.UserID) == "" {
		agent := strings.TrimSpace(p.AgentID)
		return userCurrentOutput{
			Status: "informational",
			Code:   "USER_NOT_SET",
			Hints: []string{
				"Server was launched without PINDOC_USER_NAME. Set PINDOC_USER_NAME (and optional PINDOC_USER_EMAIL) and restart to enable user attribution.",
				fmt.Sprintf("Agent identity %q is still available; artifact.propose can proceed with author_id and will leave author_user unset until a user is configured.", agent),
				"pindoc.user.update requires a bound user row, so configure PINDOC_USER_NAME first.",
			},
		}, true
	}
	return userCurrentOutput{}, false
}

type userUpdateInput struct {
	// DisplayName overrides the user's display name. Required: at least
	// one field among display_name / email / github_handle must be set.
	DisplayName string `json:"display_name,omitempty" jsonschema:"new display name (2-60 runes); omit to leave unchanged"`
	// Email is the RFC-relaxed address; '@' substring check only. Omit
	// to leave unchanged; pass empty string to clear.
	Email string `json:"email,omitempty" jsonschema:"new email; empty string clears, omit to leave unchanged"`
	// GithubHandle is optional, filled by V1.5 OAuth flow. V1 manual
	// edits are accepted for early use cases but uniqueness applies.
	GithubHandle string `json:"github_handle,omitempty" jsonschema:"new github handle; empty string clears, omit to leave unchanged"`

	// ProjectSlug is the optional project that should own the
	// user_rename event row. Account-level scope (Decision mcp-scope-
	// account-level-industry-standard) means user identity is
	// instance-wide, but events.project_id is NOT NULL, so we still
	// need a project to attribute the audit event to. Empty falls back
	// to deps.DefaultProjectSlug (PINDOC_PROJECT env); when neither is
	// set the rename succeeds and the event is skipped with a logged
	// warning rather than blocking the user-facing update.
	ProjectSlug string `json:"project_slug,omitempty" jsonschema:"optional project_slug to attribute the user_rename event to; defaults to PINDOC_PROJECT env"`
}

type userUpdateOutput struct {
	Status         string               `json:"status"`
	ErrorCode      string               `json:"error_code,omitempty"`
	Failed         []string             `json:"failed,omitempty"`
	ErrorCodes     []string             `json:"error_codes,omitempty" jsonschema:"canonical stable SCREAMING_SNAKE_CASE identifiers; branch on these"`
	Checklist      []string             `json:"checklist,omitempty"`
	ChecklistItems []ErrorChecklistItem `json:"checklist_items,omitempty" jsonschema:"localized checklist entries paired with stable codes"`
	MessageLocale  string               `json:"message_locale,omitempty" jsonschema:"locale used for checklist/checklist_items.message after fallback"`
	User           *UserRow             `json:"user,omitempty"`
	ChangedFields  []string             `json:"changed_fields,omitempty"`
	Previous       map[string]string    `json:"previous,omitempty"`
	ToolsetVersion string               `json:"toolset_version,omitempty"`
}

// RegisterUserUpdate wires pindoc.user.update. Validates inputs, mutates
// the row, and records a `user_rename` event with before/after payload
// when any field actually changes.
func RegisterUserUpdate(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name:        "pindoc.user.update",
			Description: "Mutate the current session user's display_name / email / github_handle. At least one field required. Validates length and '@' substring on email; unique constraint on email and github_handle. On success returns the new row plus changed_fields[] and previous{} so the agent can echo the diff.",
		},
		func(ctx context.Context, p *auth.Principal, in userUpdateInput) (*sdk.CallToolResult, userUpdateOutput, error) {
			if p.UserID == "" {
				return nil, userUpdateOutput{
					Status:    "not_ready",
					ErrorCode: "USER_NOT_SET",
					Failed:    []string{"USER_NOT_SET"},
					Checklist: []string{
						"MCP session has no bound user. Restart the server with PINDOC_USER_NAME set so this tool can mutate a real row.",
					},
				}, nil
			}

			// Collect validation failures (all at once, so the agent can
			// fix multiple fields in one retry).
			var failed []string
			var checklist []string
			fieldsChanged := []string{}
			prev := map[string]string{}

			existing, err := loadUserByID(ctx, deps, p.UserID)
			if err != nil {
				return nil, userUpdateOutput{}, fmt.Errorf("load user: %w", err)
			}

			newDisplay := existing.DisplayName
			newEmail := existing.Email
			newGithub := existing.GithubHandle

			if in.DisplayName != "" {
				trimmed := strings.TrimSpace(in.DisplayName)
				runes := utf8.RuneCountInString(trimmed)
				if runes < displayNameMin || runes > displayNameMax {
					failed = append(failed, "DISPLAY_NAME_RANGE")
					checklist = append(checklist,
						fmt.Sprintf("display_name must be %d-%d runes (got %d).", displayNameMin, displayNameMax, runes))
				} else if trimmed != existing.DisplayName {
					newDisplay = trimmed
					fieldsChanged = append(fieldsChanged, "display_name")
					prev["display_name"] = existing.DisplayName
				}
			}
			// Email: omitted → no change; passed as non-empty → set/override;
			// passed as "" → would clear, but JSON omitempty elides empty
			// strings on the wire so explicit clearing needs a sentinel.
			// V1 accepts only set/override; clearing is an Open question.
			if in.Email != "" {
				e := canonicalUserEmail(in.Email)
				if !strings.Contains(e, "@") {
					failed = append(failed, "EMAIL_INVALID")
					checklist = append(checklist, "email must contain '@'. Full RFC 5322 validation is intentionally relaxed for V1.")
				} else if e != existing.Email {
					newEmail = e
					fieldsChanged = append(fieldsChanged, "email")
					prev["email"] = existing.Email
				}
			}
			if in.GithubHandle != "" {
				g := strings.TrimSpace(in.GithubHandle)
				if g != existing.GithubHandle {
					newGithub = g
					fieldsChanged = append(fieldsChanged, "github_handle")
					prev["github_handle"] = existing.GithubHandle
				}
			}

			if len(failed) > 0 {
				return nil, userUpdateOutput{
					Status:    "not_ready",
					ErrorCode: failed[0],
					Failed:    failed,
					Checklist: checklist,
				}, nil
			}
			if len(fieldsChanged) == 0 {
				// Nothing actually changed — return current row so agents
				// that call update unconditionally (e.g. from a settings
				// modal) don't get a no-op error.
				return nil, userUpdateOutput{
					Status: "accepted",
					User:   existing,
				}, nil
			}

			// Resolve project_id for the events audit row. events requires
			// project_id; identity is account-level (Decision
			// mcp-scope-account-level-industry-standard) but the audit
			// event needs SOME project to live under. Resolution order:
			// caller-provided in.ProjectSlug > deps.DefaultProjectSlug
			// (PINDOC_PROJECT env) > skip event (log warn). The rename
			// itself proceeds either way so identity work isn't blocked
			// when no project exists yet.
			eventProjectSlug := strings.TrimSpace(in.ProjectSlug)
			if eventProjectSlug == "" {
				eventProjectSlug = strings.TrimSpace(deps.DefaultProjectSlug)
			}
			var projectID string
			if eventProjectSlug != "" {
				err = deps.DB.QueryRow(ctx, `SELECT id::text FROM projects WHERE slug = $1`, eventProjectSlug).Scan(&projectID)
				if err != nil {
					if errors.Is(err, pgx.ErrNoRows) {
						projectID = ""
						if deps.Logger != nil {
							deps.Logger.Warn("user.update: event attribution project missing — skipping event row",
								"requested_slug", eventProjectSlug,
							)
						}
					} else {
						return nil, userUpdateOutput{}, fmt.Errorf("resolve project_id: %w", err)
					}
				}
			}

			tx, err := deps.DB.Begin(ctx)
			if err != nil {
				return nil, userUpdateOutput{}, fmt.Errorf("begin tx: %w", err)
			}
			defer func() { _ = tx.Rollback(ctx) }()

			_, err = tx.Exec(ctx, `
				UPDATE users
				   SET display_name  = $1,
				       email         = NULLIF($2, ''),
				       github_handle = NULLIF($3, ''),
				       updated_at    = now()
				 WHERE id = $4
			`, newDisplay, newEmail, newGithub, p.UserID)
			if err != nil {
				// Uniqueness (email / github_handle) shows up as a Postgres
				// error here. Surface as stable code instead of 500.
				errStr := err.Error()
				if strings.Contains(errStr, "idx_users_email_unique") || strings.Contains(errStr, "users_email_lower_idx") {
					return nil, userUpdateOutput{
						Status:    "not_ready",
						ErrorCode: "EMAIL_TAKEN",
						Failed:    []string{"EMAIL_TAKEN"},
						Checklist: []string{"email already used by another user row."},
					}, nil
				}
				if strings.Contains(errStr, "idx_users_github_handle_unique") {
					return nil, userUpdateOutput{
						Status:    "not_ready",
						ErrorCode: "GITHUB_HANDLE_TAKEN",
						Failed:    []string{"GITHUB_HANDLE_TAKEN"},
						Checklist: []string{"github_handle already used by another user row."},
					}, nil
				}
				return nil, userUpdateOutput{}, fmt.Errorf("update user: %w", err)
			}

			if projectID != "" {
				payload := map[string]any{
					"user_id":        p.UserID,
					"changed_fields": fieldsChanged,
					"previous":       prev,
					"new": map[string]string{
						"display_name":  newDisplay,
						"email":         newEmail,
						"github_handle": newGithub,
					},
				}
				payloadJSON, _ := json.Marshal(payload)
				_, err = tx.Exec(ctx, `
					INSERT INTO events (project_id, kind, subject_id, payload)
					VALUES ($1, 'user_rename', $2::uuid, $3::jsonb)
				`, projectID, p.UserID, payloadJSON)
				if err != nil {
					return nil, userUpdateOutput{}, fmt.Errorf("event insert: %w", err)
				}
			}

			if err := tx.Commit(ctx); err != nil {
				return nil, userUpdateOutput{}, fmt.Errorf("commit: %w", err)
			}

			updated, err := loadUserByID(ctx, deps, p.UserID)
			if err != nil {
				return nil, userUpdateOutput{}, fmt.Errorf("reload: %w", err)
			}

			return nil, userUpdateOutput{
				Status:        "accepted",
				User:          updated,
				ChangedFields: fieldsChanged,
				Previous:      prev,
			}, nil
		},
	)
}

func loadUserByID(ctx context.Context, deps Deps, id string) (*UserRow, error) {
	var u UserRow
	var email, github *string
	err := deps.DB.QueryRow(ctx, `
		SELECT id::text, display_name, email, github_handle, source, created_at, updated_at
		  FROM users
		 WHERE id = $1
	`, id).Scan(&u.ID, &u.DisplayName, &email, &github, &u.Source, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("user %s not found", id)
	}
	if err != nil {
		return nil, err
	}
	if email != nil {
		u.Email = *email
	}
	if github != nil {
		u.GithubHandle = *github
	}
	return &u, nil
}

// UpsertUserFromEnv is called by the MCP binary at startup when
// PINDOC_USER_NAME is set. Idempotent: same (display_name, email) on
// re-run returns the existing id. Returns empty id + nil when env isn't
// set so the boot-time chain wiring can pass an empty UserID into
// TrustedLocalResolver.
func UpsertUserFromEnv(ctx context.Context, deps Deps, userName, userEmail string) (string, error) {
	if strings.TrimSpace(userName) == "" {
		return "", nil
	}
	name := strings.TrimSpace(userName)
	email := canonicalUserEmail(userEmail)

	// Prefer email as the uniqueness anchor when both are set — otherwise
	// the same user running with two different display_names would mint a
	// fresh row each restart. Fall back to display_name when email is
	// absent (V1 operator with no email yet).
	var existingID string
	if email != "" {
		err := deps.DB.QueryRow(ctx,
			`SELECT id::text FROM users WHERE lower(email) = $1 AND deleted_at IS NULL LIMIT 1`, email,
		).Scan(&existingID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("lookup user by email: %w", err)
		}
	}
	if existingID == "" {
		err := deps.DB.QueryRow(ctx,
			`SELECT id::text FROM users WHERE display_name = $1 AND email IS NOT DISTINCT FROM NULLIF($2, '') AND deleted_at IS NULL LIMIT 1`,
			name, email,
		).Scan(&existingID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("lookup user by name: %w", err)
		}
	}
	if existingID != "" {
		// Keep display_name in sync with the latest env if it drifted.
		_, err := deps.DB.Exec(ctx,
			`UPDATE users SET display_name = $1, updated_at = now() WHERE id = $2 AND display_name <> $1`,
			name, existingID,
		)
		if err != nil {
			return "", fmt.Errorf("sync display_name: %w", err)
		}
		return existingID, nil
	}

	var newID string
	err := deps.DB.QueryRow(ctx, `
		INSERT INTO users (display_name, email, source)
		VALUES ($1, NULLIF($2, ''), 'harness_install')
		RETURNING id::text
	`, name, email).Scan(&newID)
	if err != nil {
		return "", fmt.Errorf("insert user: %w", err)
	}
	return newID, nil
}

func canonicalUserEmail(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

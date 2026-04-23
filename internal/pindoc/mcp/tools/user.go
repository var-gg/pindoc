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
)

// Author identity dual (Decision `decision-author-identity-dual`,
// migration 0014). Two MCP tools + helpers.
//
// pindoc.user.current: return the user row bound to this MCP session.
// pindoc.user.update:  mutate display_name / email / github_handle with
//                       events.user_rename audit row on change.
//
// The artifact.propose path reads deps.UserID (populated from the boot-
// time upsert) and writes it into artifacts.author_user_id so every
// revision lands with the dual identity.

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
	ID            string    `json:"id"`
	DisplayName   string    `json:"display_name"`
	Email         string    `json:"email,omitempty"`
	GithubHandle  string    `json:"github_handle,omitempty"`
	Source        string    `json:"source"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type userCurrentOutput struct {
	Status    string   `json:"status"`
	ErrorCode string   `json:"error_code,omitempty"`
	Failed    []string `json:"failed,omitempty"`
	Checklist []string `json:"checklist,omitempty"`
	User      *UserRow `json:"user,omitempty"`
}

// RegisterUserCurrent wires pindoc.user.current. Fails open with a
// stable NOT_READY code when the session was launched without
// PINDOC_USER_NAME so the agent can surface "identity not configured"
// rather than a silent null.
func RegisterUserCurrent(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name:        "pindoc.user.current",
			Description: "Return the user (display_name / email / github_handle / source) bound to this MCP session. Populated at server startup from PINDOC_USER_NAME / PINDOC_USER_EMAIL; returns USER_NOT_SET when the operator hasn't configured identity yet.",
		},
		func(ctx context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, userCurrentOutput, error) {
			if deps.UserID == "" {
				return nil, userCurrentOutput{
					Status:    "not_ready",
					ErrorCode: "USER_NOT_SET",
					Failed:    []string{"USER_NOT_SET"},
					Checklist: []string{
						"Server was launched without PINDOC_USER_NAME. Set the env var (and optional PINDOC_USER_EMAIL) and restart the MCP server, or call pindoc.user.update to create a user row from this session.",
					},
				}, nil
			}
			row, err := loadUserByID(ctx, deps, deps.UserID)
			if err != nil {
				return nil, userCurrentOutput{}, fmt.Errorf("load user: %w", err)
			}
			return nil, userCurrentOutput{
				Status: "accepted",
				User:   row,
			}, nil
		},
	)
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
}

type userUpdateOutput struct {
	Status        string              `json:"status"`
	ErrorCode     string              `json:"error_code,omitempty"`
	Failed        []string            `json:"failed,omitempty"`
	Checklist     []string            `json:"checklist,omitempty"`
	User          *UserRow            `json:"user,omitempty"`
	ChangedFields []string            `json:"changed_fields,omitempty"`
	Previous      map[string]string   `json:"previous,omitempty"`
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
		func(ctx context.Context, _ *sdk.CallToolRequest, in userUpdateInput) (*sdk.CallToolResult, userUpdateOutput, error) {
			if deps.UserID == "" {
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

			existing, err := loadUserByID(ctx, deps, deps.UserID)
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
				e := strings.TrimSpace(in.Email)
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
			// project_id; identity is per-server today but we write the
			// event under the active project scope so audit queries stay
			// project-bounded.
			var projectID string
			err = deps.DB.QueryRow(ctx, `SELECT id::text FROM projects WHERE slug = $1`, deps.ProjectSlug).Scan(&projectID)
			if err != nil {
				return nil, userUpdateOutput{}, fmt.Errorf("resolve project_id: %w", err)
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
			`, newDisplay, newEmail, newGithub, deps.UserID)
			if err != nil {
				// Uniqueness (email / github_handle) shows up as a Postgres
				// error here. Surface as stable code instead of 500.
				errStr := err.Error()
				if strings.Contains(errStr, "idx_users_email_unique") {
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

			payload := map[string]any{
				"user_id":        deps.UserID,
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
			`, projectID, deps.UserID, payloadJSON)
			if err != nil {
				return nil, userUpdateOutput{}, fmt.Errorf("event insert: %w", err)
			}

			if err := tx.Commit(ctx); err != nil {
				return nil, userUpdateOutput{}, fmt.Errorf("commit: %w", err)
			}

			updated, err := loadUserByID(ctx, deps, deps.UserID)
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
// set so the caller can leave deps.UserID empty.
func UpsertUserFromEnv(ctx context.Context, deps Deps, userName, userEmail string) (string, error) {
	if strings.TrimSpace(userName) == "" {
		return "", nil
	}
	name := strings.TrimSpace(userName)
	email := strings.TrimSpace(userEmail)

	// Prefer email as the uniqueness anchor when both are set — otherwise
	// the same user running with two different display_names would mint a
	// fresh row each restart. Fall back to display_name when email is
	// absent (V1 operator with no email yet).
	var existingID string
	if email != "" {
		err := deps.DB.QueryRow(ctx,
			`SELECT id::text FROM users WHERE email = $1 LIMIT 1`, email,
		).Scan(&existingID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("lookup user by email: %w", err)
		}
	}
	if existingID == "" {
		err := deps.DB.QueryRow(ctx,
			`SELECT id::text FROM users WHERE display_name = $1 AND email IS NOT DISTINCT FROM NULLIF($2, '') LIMIT 1`,
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

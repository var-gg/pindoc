package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	pauth "github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/invites"
	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

// Phase D — Reader UI permission management plane.
//
// Decision oauth-github-immediate carries the OSS-launch contract that
// the first external user can land on a Pindoc instance and round-trip
// invite issue → signup → membership without dropping into psql. Phase
// C 4/4 (task-reader-ui-team-invite) shipped issue + signup. This file
// closes the loop — list, remove, revoke — so an owner who issues a
// wrong invite or wants to evict a leaked secondary account can do so
// from the UI alone.
//
// All four handlers run through d.principalForInvite, which delegates
// to auth.PrincipalFromRequest: loopback callers get Source=loopback +
// auto-trusted owner, non-loopback callers must present a Pindoc AS
// browser session. project_members decides editor / viewer for OAuth
// callers. Decision `decision-auth-model-loopback-and-providers`
// retired the auth_mode enum these handlers previously branched on.

type memberRowResponse struct {
	UserID       string    `json:"user_id"`
	DisplayName  string    `json:"display_name,omitempty"`
	GitHubHandle string    `json:"github_handle,omitempty"`
	Role         string    `json:"role"`
	InvitedByID  string    `json:"invited_by_id,omitempty"`
	JoinedAt     time.Time `json:"joined_at"`
	IsSelf       bool      `json:"is_self,omitempty"`
}

type membersListResponse struct {
	ProjectSlug string              `json:"project_slug"`
	ViewerRole  string              `json:"viewer_role"`
	ViewerID    string              `json:"viewer_id,omitempty"`
	Members     []memberRowResponse `json:"members"`
}

type inviteRowResponse struct {
	TokenHash  string     `json:"token_hash"`
	Role       string     `json:"role"`
	IssuedByID string     `json:"issued_by_id,omitempty"`
	IssuedAt   time.Time  `json:"issued_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
}

type invitesListResponse struct {
	ProjectSlug string              `json:"project_slug"`
	Invites     []inviteRowResponse `json:"invites"`
}

type membersOpResponse struct {
	Status string `json:"status"`
}

type inviteExtendRequest struct {
	ExtendTo string `json:"extend_to"`
}

func (d Deps) handleMembersList(w http.ResponseWriter, r *http.Request) {
	if d.DB == nil {
		writeInviteError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "database pool not configured")
		return
	}
	principal, ok := d.principalForInvite(w, r, false)
	if !ok {
		return
	}
	scope, err := pauth.ResolveProject(r.Context(), d.DB, principal, projectSlugFrom(r))
	if err != nil {
		d.writeProjectAuthError(w, err)
		return
	}
	rows, err := projects.ListProjectMembers(r.Context(), d.DB, scope.ProjectID)
	if err != nil {
		writeInviteError(w, http.StatusInternalServerError, "MEMBERS_LIST_FAILED", "failed to list project members")
		return
	}
	resp := membersListResponse{
		ProjectSlug: scope.ProjectSlug,
		ViewerRole:  scope.Role,
		ViewerID:    principal.UserID,
		Members:     make([]memberRowResponse, 0, len(rows)),
	}
	for _, m := range rows {
		resp.Members = append(resp.Members, memberRowResponse{
			UserID:       m.UserID,
			DisplayName:  m.DisplayName,
			GitHubHandle: m.GitHubHandle,
			Role:         m.Role,
			InvitedByID:  m.InvitedByID,
			JoinedAt:     m.JoinedAt,
			IsSelf:       principal.UserID != "" && m.UserID == principal.UserID,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (d Deps) handleMemberRemove(w http.ResponseWriter, r *http.Request) {
	if d.DB == nil {
		writeInviteError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "database pool not configured")
		return
	}
	principal, ok := d.principalForInvite(w, r, false)
	if !ok {
		return
	}
	scope, err := pauth.ResolveProject(r.Context(), d.DB, principal, projectSlugFrom(r))
	if err != nil {
		d.writeProjectAuthError(w, err)
		return
	}
	targetUserID := strings.TrimSpace(r.PathValue("user_id"))
	if targetUserID == "" {
		writeInviteError(w, http.StatusBadRequest, "USER_ID_REQUIRED", "user_id path segment is required")
		return
	}
	// Permission: project owner can remove anyone; any member can
	// remove themself. trusted_local has implicit owner so this
	// reduces to "owner OR self == self" anyway.
	isSelf := principal.UserID != "" && targetUserID == principal.UserID
	if !isSelf && scope.Role != pauth.RoleOwner {
		writeInviteError(w, http.StatusForbidden, "PROJECT_OWNER_REQUIRED", "only project owners can remove other members")
		return
	}
	if err := projects.RemoveProjectMember(r.Context(), d.DB, scope.ProjectID, targetUserID); err != nil {
		switch {
		case errors.Is(err, projects.ErrLastOwner):
			writeInviteError(w, http.StatusUnprocessableEntity, "LAST_OWNER", "cannot remove the last owner — transfer ownership first")
		case errors.Is(err, projects.ErrMemberNotFound):
			writeInviteError(w, http.StatusNotFound, "MEMBER_NOT_FOUND", "no project member matches this user_id")
		default:
			writeInviteError(w, http.StatusInternalServerError, "MEMBER_REMOVE_FAILED", "failed to remove project member")
		}
		return
	}
	writeJSON(w, http.StatusOK, membersOpResponse{Status: "removed"})
}

func (d Deps) handleInvitesList(w http.ResponseWriter, r *http.Request) {
	if d.DB == nil {
		writeInviteError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "database pool not configured")
		return
	}
	principal, ok := d.principalForInvite(w, r, false)
	if !ok {
		return
	}
	scope, err := pauth.ResolveProject(r.Context(), d.DB, principal, projectSlugFrom(r))
	if err != nil {
		d.writeProjectAuthError(w, err)
		return
	}
	if scope.Role != pauth.RoleOwner {
		writeInviteError(w, http.StatusForbidden, "PROJECT_OWNER_REQUIRED", "only project owners can list invites")
		return
	}
	rows, err := invites.ListActive(r.Context(), d.DB, scope.ProjectID, time.Now().UTC())
	if err != nil {
		writeInviteError(w, http.StatusInternalServerError, "INVITES_LIST_FAILED", "failed to list active invites")
		return
	}
	resp := invitesListResponse{
		ProjectSlug: scope.ProjectSlug,
		Invites:     make([]inviteRowResponse, 0, len(rows)),
	}
	for _, inv := range rows {
		resp.Invites = append(resp.Invites, inviteRowResponse{
			TokenHash:  inv.TokenHash,
			Role:       inv.Role,
			IssuedByID: inv.IssuedByID,
			IssuedAt:   inv.CreatedAt,
			ExpiresAt:  inv.ExpiresAt,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (d Deps) handleInviteRevoke(w http.ResponseWriter, r *http.Request) {
	if d.DB == nil {
		writeInviteError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "database pool not configured")
		return
	}
	principal, ok := d.principalForInvite(w, r, false)
	if !ok {
		return
	}
	scope, err := pauth.ResolveProject(r.Context(), d.DB, principal, projectSlugFrom(r))
	if err != nil {
		d.writeProjectAuthError(w, err)
		return
	}
	if scope.Role != pauth.RoleOwner {
		writeInviteError(w, http.StatusForbidden, "PROJECT_OWNER_REQUIRED", "only project owners can revoke invites")
		return
	}
	tokenHash := strings.TrimSpace(r.PathValue("token_hash"))
	if tokenHash == "" {
		writeInviteError(w, http.StatusBadRequest, "INVITE_HASH_REQUIRED", "token_hash path segment is required")
		return
	}
	if err := invites.Revoke(r.Context(), d.DB, scope.ProjectID, tokenHash, principal.UserID, time.Now().UTC()); err != nil {
		switch {
		case errors.Is(err, invites.ErrTokenNotFound):
			writeInviteError(w, http.StatusNotFound, "INVITE_TOKEN_NOT_FOUND", "invite token not found for this project")
		case errors.Is(err, invites.ErrTokenInactive):
			writeInviteError(w, http.StatusGone, "INVITE_TOKEN_INACTIVE", "invite token is already revoked or consumed")
		default:
			writeInviteError(w, http.StatusInternalServerError, "INVITE_REVOKE_FAILED", "failed to revoke invite")
		}
		return
	}
	writeJSON(w, http.StatusOK, membersOpResponse{Status: "revoked"})
}

func (d Deps) handleInviteExtend(w http.ResponseWriter, r *http.Request) {
	if d.DB == nil {
		writeInviteError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "database pool not configured")
		return
	}
	principal, ok := d.principalForInvite(w, r, false)
	if !ok {
		return
	}
	scope, err := pauth.ResolveProject(r.Context(), d.DB, principal, projectSlugFrom(r))
	if err != nil {
		d.writeProjectAuthError(w, err)
		return
	}
	if scope.Role != pauth.RoleOwner {
		writeInviteError(w, http.StatusForbidden, "PROJECT_OWNER_REQUIRED", "only project owners can extend invites")
		return
	}
	tokenHash := strings.TrimSpace(r.PathValue("token_hash"))
	if tokenHash == "" {
		writeInviteError(w, http.StatusBadRequest, "INVITE_HASH_REQUIRED", "token_hash path segment is required")
		return
	}
	var in inviteExtendRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeInviteError(w, http.StatusBadRequest, "BAD_JSON", "could not parse request body as JSON")
		return
	}
	rec, err := invites.Extend(r.Context(), d.DB, scope.ProjectID, tokenHash, in.ExtendTo, principal.UserID, time.Now().UTC())
	if err != nil {
		switch {
		case errors.Is(err, invites.ErrTokenNotFound):
			writeInviteError(w, http.StatusNotFound, "INVITE_TOKEN_NOT_FOUND", "invite token not found for this project")
		case errors.Is(err, invites.ErrTokenInactive):
			writeInviteError(w, http.StatusGone, "INVITE_TOKEN_INACTIVE", "invite token is expired or already consumed")
		case errors.Is(err, invites.ErrExtendInvalid):
			writeInviteError(w, http.StatusBadRequest, "INVITE_EXTEND_INVALID", "extend_to must be +7d, +30d, or permanent")
		default:
			writeInviteError(w, http.StatusInternalServerError, "INVITE_EXTEND_FAILED", "failed to extend invite")
		}
		return
	}
	if err := d.recordInviteAuditEvent(r.Context(), scope.ProjectID, "invite.extended", rec.TokenHash, rec.Role, in.ExtendTo, principal.UserID, rec.ExpiresAt); err != nil {
		writeInviteError(w, http.StatusInternalServerError, "INVITE_AUDIT_FAILED", "failed to record invite audit event")
		return
	}
	writeJSON(w, http.StatusOK, membersOpResponse{Status: "extended"})
}

func (d Deps) recordInviteAuditEvent(ctx context.Context, projectID, kind, tokenHash, role, action, actorUserID string, expiresAt *time.Time) error {
	var actor any
	if v := strings.TrimSpace(actorUserID); v != "" {
		actor = v
	}
	var expiresAtArg any
	if expiresAt != nil {
		expiresAtArg = *expiresAt
	}
	_, err := d.DB.Exec(ctx, `
		INSERT INTO events (project_id, kind, subject_id, payload)
		VALUES ($1::uuid, $2, $3::uuid, jsonb_build_object(
			'token_hash', $4::text,
			'role',       $5::text,
			'action',     $6::text,
			'actor_id',   COALESCE($7::text, ''),
			'expires_at', $8::timestamptz
		))
	`, projectID, kind, actor, tokenHash, role, action, actor, expiresAtArg)
	return err
}

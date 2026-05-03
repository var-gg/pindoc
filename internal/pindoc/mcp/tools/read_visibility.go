package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
	"github.com/var-gg/pindoc/internal/pindoc/db"
	"github.com/var-gg/pindoc/internal/pindoc/projects"
)

// mcpReadProjectScope is the read-side sibling of auth.ProjectScope.
// auth.ResolveProject intentionally denies OAuth callers without a
// project_members row; public artifact reads need a softer scope so they
// can return public rows while still filtering org/private rows.
type mcpReadProjectScope struct {
	*auth.ProjectScope

	UserID     string
	TrustedAll bool
	Member     bool
}

func resolveMCPReadProjectScope(ctx context.Context, pool *db.Pool, p *auth.Principal, slug string) (*mcpReadProjectScope, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil, auth.ErrProjectSlugRequired
	}
	if pool == nil {
		return nil, errors.New("auth: ResolveProject called with nil DB pool")
	}

	var projectID, primaryLanguage, organizationID string
	err := pool.QueryRow(ctx, `
		SELECT id::text, COALESCE(NULLIF(primary_language, ''), 'en'), organization_id::text
		  FROM projects
		 WHERE slug = $1
		 LIMIT 1
	`, slug).Scan(&projectID, &primaryLanguage, &organizationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: %q", auth.ErrProjectNotFound, slug)
	}
	if err != nil {
		return nil, fmt.Errorf("auth: lookup project %q: %w", slug, err)
	}

	userID := ""
	if p != nil {
		userID = strings.TrimSpace(p.UserID)
	}

	role := ""
	trustedAll := mcpReadTrustedAll(p)
	member := trustedAll
	if trustedAll {
		role = auth.RoleOwner
	} else if p != nil && p.IsOAuth() && userID != "" {
		projectRole, err := mcpReadProjectMemberRole(ctx, pool, userID, projectID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("auth: lookup project membership %q: %w", slug, err)
		}
		role = projectRole
		member = projectRole != ""
		if !member {
			orgMember, err := mcpReadOrgMember(ctx, pool, userID, organizationID)
			if err != nil {
				return nil, fmt.Errorf("auth: lookup organization membership %q: %w", slug, err)
			}
			member = orgMember
			if member {
				role = auth.RoleViewer
			}
		}
	}

	return &mcpReadProjectScope{
		ProjectScope: &auth.ProjectScope{
			ProjectID:     projectID,
			ProjectSlug:   slug,
			ProjectLocale: primaryLanguage,
			Role:          role,
		},
		UserID:     userID,
		TrustedAll: trustedAll,
		Member:     member,
	}, nil
}

func mcpReadTrustedAll(p *auth.Principal) bool {
	if p == nil {
		return false
	}
	source := strings.TrimSpace(p.Source)
	return source == "" || source == auth.SourceLoopback
}

func mcpReadProjectMemberRole(ctx context.Context, pool *db.Pool, userID, projectID string) (string, error) {
	var role string
	err := pool.QueryRow(ctx, `
		SELECT role
		  FROM project_members
		 WHERE project_id = $1::uuid AND user_id = $2::uuid
		 LIMIT 1
	`, projectID, userID).Scan(&role)
	return role, err
}

func mcpReadOrgMember(ctx context.Context, pool *db.Pool, userID, organizationID string) (bool, error) {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(organizationID) == "" {
		return false, nil
	}
	var ok bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			  FROM organization_members
			 WHERE organization_id = $1::uuid
			   AND user_id = $2::uuid
		)
	`, organizationID, userID).Scan(&ok)
	return ok, err
}

func mcpReadArtifactVisibilityWhere(scope *mcpReadProjectScope, alias string, startPlaceholder int) (string, []any) {
	if scope != nil && scope.TrustedAll {
		return "TRUE", nil
	}
	if startPlaceholder <= 0 {
		startPlaceholder = 1
	}

	visibilityCol := mcpReadColumn(alias, "visibility")
	authorCol := mcpReadColumn(alias, "author_user_id")
	next := startPlaceholder
	clauses := []string{fmt.Sprintf("%s = $%d", visibilityCol, next)}
	args := []any{projects.VisibilityPublic}
	next++

	if scope != nil && scope.Member {
		clauses = append(clauses, fmt.Sprintf("%s = $%d", visibilityCol, next))
		args = append(args, projects.VisibilityOrg)
		next++
	}
	if scope != nil && strings.TrimSpace(scope.UserID) != "" {
		if scope.ProjectScope != nil && scope.Role == auth.RoleOwner {
			clauses = append(clauses, fmt.Sprintf("%s = $%d", visibilityCol, next))
			args = append(args, projects.VisibilityPrivate)
			next++
			return "(" + strings.Join(clauses, " OR ") + ")", args
		}
		clauses = append(clauses, fmt.Sprintf("(%s = $%d AND %s::text = $%d)", visibilityCol, next, authorCol, next+1))
		args = append(args, projects.VisibilityPrivate, strings.TrimSpace(scope.UserID))
	}

	return "(" + strings.Join(clauses, " OR ") + ")", args
}

func mcpReadColumn(alias, column string) string {
	alias = strings.TrimSpace(alias)
	column = strings.TrimSpace(column)
	if alias == "" {
		return column
	}
	return alias + "." + column
}

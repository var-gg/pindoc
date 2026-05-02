// Package taskassignee owns the canonical Task assignee string rules shared by
// the MCP tool lane and the Reader HTTP bridge.
package taskassignee

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
)

const (
	CodeFormatInvalid = "ASSIGNEE_FORMAT_INVALID"
	CodeUnresolved    = "ASSIGNEE_UNRESOLVED"
)

type Problem struct {
	Code    string
	Message string
}

type User struct {
	ID           string
	DisplayName  string
	GitHubHandle string
}

type LookupFunc func(ctx context.Context, value string) (User, bool, error)

type QueryRower interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

var (
	assigneePattern = regexp.MustCompile(`^(agent:[a-zA-Z0-9_\-:.]+|user:[^\r\n]+|@[a-zA-Z0-9_\-.]+)$`)
	uuidPattern     = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

func ValidFormat(assignee string) (string, bool) {
	a := strings.TrimSpace(assignee)
	if a == "" {
		return "", true
	}
	if !assigneePattern.MatchString(a) {
		return "", false
	}
	return a, true
}

func NormalizeWithDB(ctx context.Context, q QueryRower, assignee string) (string, *Problem, error) {
	return Normalize(ctx, assignee, func(ctx context.Context, value string) (User, bool, error) {
		if q == nil {
			return User{}, false, fmt.Errorf("taskassignee: user lookup unavailable")
		}
		return LookupUser(ctx, q, value)
	})
}

func Normalize(ctx context.Context, assignee string, lookup LookupFunc) (string, *Problem, error) {
	a := strings.TrimSpace(assignee)
	if a == "" {
		return "", nil, nil
	}

	if uuidPattern.MatchString(a) {
		return normalizeUserReference(ctx, a, lookup)
	}

	if strings.HasPrefix(a, "user:") {
		value := strings.TrimSpace(strings.TrimPrefix(a, "user:"))
		if value == "" {
			return "", formatProblem(), nil
		}
		if uuidPattern.MatchString(value) {
			return normalizeUserReference(ctx, value, lookup)
		}
		if _, ok := ValidFormat(a); !ok {
			return "", formatProblem(), nil
		}
		if lookup == nil {
			return a, nil, nil
		}
		user, found, err := lookup(ctx, value)
		if err != nil {
			return "", nil, err
		}
		if !found {
			return "", unresolvedProblem(value), nil
		}
		canonical, ok := CanonicalUserAssignee(user)
		if !ok {
			return "", unresolvedProblem(value), nil
		}
		return canonical, nil, nil
	}

	if _, ok := ValidFormat(a); !ok {
		return "", formatProblem(), nil
	}
	return a, nil, nil
}

func LookupUser(ctx context.Context, q QueryRower, value string) (User, bool, error) {
	v := strings.TrimSpace(value)
	if v == "" {
		return User{}, false, nil
	}
	var user User
	err := q.QueryRow(ctx, `
		SELECT id::text, display_name, COALESCE(github_handle, '')
		  FROM users
		 WHERE deleted_at IS NULL
		   AND (
		       id::text = $1
		       OR display_name = $1
		       OR github_handle = $1
		       OR lower(ltrim(COALESCE(github_handle, ''), '@')) = lower(ltrim($1, '@'))
		   )
		 ORDER BY CASE
		              WHEN id::text = $1 THEN 0
		              WHEN display_name = $1 THEN 1
		              ELSE 2
		          END
		 LIMIT 1
	`, v).Scan(&user.ID, &user.DisplayName, &user.GitHubHandle)
	if err == nil {
		return user, true, nil
	}
	if err == pgx.ErrNoRows {
		return User{}, false, nil
	}
	return User{}, false, err
}

func CanonicalUserAssignee(user User) (string, bool) {
	handle := strings.TrimSpace(user.GitHubHandle)
	handle = strings.TrimLeft(handle, "@")
	if handle != "" {
		candidate := "@" + handle
		if _, ok := ValidFormat(candidate); ok {
			return candidate, true
		}
	}

	name := strings.TrimSpace(user.DisplayName)
	if name != "" {
		candidate := "user:" + name
		if _, ok := ValidFormat(candidate); ok {
			return candidate, true
		}
	}

	return "", false
}

func normalizeUserReference(ctx context.Context, value string, lookup LookupFunc) (string, *Problem, error) {
	if lookup == nil {
		return "", unresolvedProblem(value), nil
	}
	user, found, err := lookup(ctx, value)
	if err != nil {
		return "", nil, err
	}
	if !found {
		return "", unresolvedProblem(value), nil
	}
	canonical, ok := CanonicalUserAssignee(user)
	if !ok {
		return "", unresolvedProblem(value), nil
	}
	return canonical, nil, nil
}

func formatProblem() *Problem {
	return &Problem{
		Code:    CodeFormatInvalid,
		Message: "assignee must match agent:<id> | user:<display_name-or-id> | @<handle>, a bare user UUID, or be empty string to clear",
	}
}

func unresolvedProblem(value string) *Problem {
	return &Problem{
		Code:    CodeUnresolved,
		Message: fmt.Sprintf("assignee user %q did not resolve to an active users row with a valid display label", value),
	}
}

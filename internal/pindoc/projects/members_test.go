package projects

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

type recordingMembershipExec struct {
	sql  string
	args []any
}

func (r *recordingMembershipExec) Exec(_ context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	r.sql = sql
	r.args = arguments
	return pgconn.CommandTag{}, nil
}

func TestEnsureProjectOwnerMembership(t *testing.T) {
	rec := &recordingMembershipExec{}
	err := EnsureProjectOwnerMembership(context.Background(), rec,
		"11111111-1111-1111-1111-111111111111",
		"22222222-2222-2222-2222-222222222222",
	)
	if err != nil {
		t.Fatalf("EnsureProjectOwnerMembership returned error: %v", err)
	}
	for _, want := range []string{"INSERT INTO project_members", "ON CONFLICT (project_id, user_id) DO NOTHING"} {
		if !strings.Contains(rec.sql, want) {
			t.Fatalf("membership SQL missing %q: %s", want, rec.sql)
		}
	}
	if len(rec.args) != 3 || rec.args[2] != ProjectRoleOwner {
		t.Fatalf("membership args = %#v, want owner role as third arg", rec.args)
	}
}

func TestEnsureProjectOwnerMembershipEmptyUserNoop(t *testing.T) {
	rec := &recordingMembershipExec{}
	if err := EnsureProjectOwnerMembership(context.Background(), rec, "project-id", " "); err != nil {
		t.Fatalf("empty user should be a no-op, got %v", err)
	}
	if rec.sql != "" {
		t.Fatalf("empty user should not execute SQL, got %s", rec.sql)
	}
}

func TestEnsureDefaultProjectOwnerMembership(t *testing.T) {
	rec := &recordingMembershipExec{}
	err := EnsureDefaultProjectOwnerMembership(context.Background(), rec,
		"pindoc",
		"22222222-2222-2222-2222-222222222222",
	)
	if err != nil {
		t.Fatalf("EnsureDefaultProjectOwnerMembership returned error: %v", err)
	}
	for _, want := range []string{"SELECT p.id", "FROM projects p", "WHERE p.slug = $1", "ON CONFLICT (project_id, user_id) DO NOTHING"} {
		if !strings.Contains(rec.sql, want) {
			t.Fatalf("bootstrap SQL missing %q: %s", want, rec.sql)
		}
	}
	if len(rec.args) != 3 || rec.args[0] != "pindoc" || rec.args[2] != ProjectRoleOwner {
		t.Fatalf("bootstrap args = %#v", rec.args)
	}
}

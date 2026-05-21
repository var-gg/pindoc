package projects

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/var-gg/pindoc/internal/pindoc/db"
)

// TestCreateTopLevelAreaValidation covers the input checks that run before
// any DB access, so a nil querier is never dereferenced.
func TestCreateTopLevelAreaValidation(t *testing.T) {
	if _, err := CreateTopLevelArea(context.Background(), nil, "p",
		TopLevelAreaSpec{Slug: "Bad Slug", Name: "Ok name"}, "", ""); !errors.Is(err, ErrTopLevelAreaSlugInvalid) {
		t.Fatalf("bad slug err = %v, want ErrTopLevelAreaSlugInvalid", err)
	}
	if _, err := CreateTopLevelArea(context.Background(), nil, "p",
		TopLevelAreaSpec{Slug: "combat", Name: "x"}, "", ""); !errors.Is(err, ErrTopLevelAreaNameInvalid) {
		t.Fatalf("short name err = %v, want ErrTopLevelAreaNameInvalid", err)
	}
}

// TestCreateTopLevelAreaIntegration covers the runtime creation primitive:
// a top-level row (parent_id NULL, lifecycle active, origin recorded),
// slug-collision rejection, and the active top-level cap.
//
// Calls run against the pool, not a shared tx: a unique-violation inside a
// postgres transaction poisons every later statement, and the collision
// case deliberately triggers one. Pool statements auto-commit, so the
// created areas persist and are cleaned up by the project cascade delete.
func TestCreateTopLevelAreaIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PINDOC_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set PINDOC_TEST_DATABASE_URL to run CreateTopLevelArea integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool.Pool); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	slug := fmt.Sprintf("t9-toplevel-%d", time.Now().UnixNano())
	var projectID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO projects (organization_id, slug, name, primary_language)
		VALUES ((SELECT id FROM organizations WHERE slug = 'default' LIMIT 1), $1, $2, 'en')
		RETURNING id::text
	`, slug, "T9 "+slug).Scan(&projectID); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM projects WHERE id = $1::uuid`, projectID)
	}()

	id, err := CreateTopLevelArea(ctx, pool, projectID, TopLevelAreaSpec{
		Slug: "combat", Name: "Combat", Description: "Combat systems",
		Fileable: true, MaxDepth: 2,
	}, "game-narrative", "")
	if err != nil {
		t.Fatalf("create top-level area: %v", err)
	}
	var parentID *string
	var lifecycle, originProfile string
	var fileable bool
	var maxDepth int
	if err := pool.QueryRow(ctx, `
		SELECT parent_id::text, lifecycle, COALESCE(origin_profile_slug, ''), fileable, max_depth
		  FROM areas WHERE id = $1::uuid
	`, id).Scan(&parentID, &lifecycle, &originProfile, &fileable, &maxDepth); err != nil {
		t.Fatalf("read created area: %v", err)
	}
	if parentID != nil || lifecycle != "active" || originProfile != "game-narrative" || !fileable || maxDepth != 2 {
		t.Fatalf("created area = parent=%v lifecycle=%q origin=%q fileable=%v maxDepth=%d",
			parentID, lifecycle, originProfile, fileable, maxDepth)
	}

	if _, err := CreateTopLevelArea(ctx, pool, projectID, TopLevelAreaSpec{
		Slug: "combat", Name: "Combat duplicate", Fileable: true,
	}, "", ""); !errors.Is(err, ErrTopLevelAreaSlugTaken) {
		t.Fatalf("slug collision err = %v, want ErrTopLevelAreaSlugTaken", err)
	}

	// One area exists (combat). Fill to the cap of 12, then the 13th fails.
	for i := 0; i < MaxActiveTopLevelAreas-1; i++ {
		if _, err := CreateTopLevelArea(ctx, pool, projectID, TopLevelAreaSpec{
			Slug: fmt.Sprintf("cap-area-%d", i), Name: fmt.Sprintf("Cap area %d", i), Fileable: true,
		}, "", ""); err != nil {
			t.Fatalf("cap fill %d: %v", i, err)
		}
	}
	if _, err := CreateTopLevelArea(ctx, pool, projectID, TopLevelAreaSpec{
		Slug: "cap-overflow", Name: "Cap overflow", Fileable: true,
	}, "", ""); !errors.Is(err, ErrTopLevelAreaCapExceeded) {
		t.Fatalf("cap overflow err = %v, want ErrTopLevelAreaCapExceeded", err)
	}
}

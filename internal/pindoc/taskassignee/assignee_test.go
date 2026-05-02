package taskassignee

import (
	"context"
	"errors"
	"testing"
)

func TestNormalizeResolvesRawUserUUID(t *testing.T) {
	const id = "72947d93-6866-4b05-9f5e-f0059db8b91f"
	lookup := func(_ context.Context, value string) (User, bool, error) {
		if value != id {
			t.Fatalf("lookup value = %q, want %q", value, id)
		}
		return User{ID: id, DisplayName: "curioustore", GitHubHandle: "curioustore"}, true, nil
	}

	for _, in := range []string{id, "user:" + id} {
		t.Run(in, func(t *testing.T) {
			got, problem, err := Normalize(context.Background(), in, lookup)
			if err != nil {
				t.Fatalf("Normalize() error = %v", err)
			}
			if problem != nil {
				t.Fatalf("Normalize() problem = %+v", problem)
			}
			if got != "@curioustore" {
				t.Fatalf("Normalize() = %q, want @curioustore", got)
			}
		})
	}
}

func TestNormalizeUserDisplayName(t *testing.T) {
	lookup := func(_ context.Context, value string) (User, bool, error) {
		if value != "Alice Smith" {
			t.Fatalf("lookup value = %q", value)
		}
		return User{DisplayName: "Alice Smith"}, true, nil
	}

	got, problem, err := Normalize(context.Background(), "user:Alice Smith", lookup)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if problem != nil {
		t.Fatalf("Normalize() problem = %+v", problem)
	}
	if got != "user:Alice Smith" {
		t.Fatalf("Normalize() = %q, want user:Alice Smith", got)
	}
}

func TestNormalizeUserLookupMiss(t *testing.T) {
	got, problem, err := Normalize(context.Background(), "user:missing", func(context.Context, string) (User, bool, error) {
		return User{}, false, nil
	})
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if got != "" {
		t.Fatalf("Normalize() = %q, want empty", got)
	}
	if problem == nil || problem.Code != CodeUnresolved {
		t.Fatalf("problem = %+v, want %s", problem, CodeUnresolved)
	}
}

func TestNormalizePassesAgentAndHandleWithoutLookup(t *testing.T) {
	lookup := func(context.Context, string) (User, bool, error) {
		return User{}, false, errors.New("lookup should not run")
	}
	for _, in := range []string{"agent:codex", "@curioustore", ""} {
		t.Run(in, func(t *testing.T) {
			got, problem, err := Normalize(context.Background(), in, lookup)
			if err != nil {
				t.Fatalf("Normalize() error = %v", err)
			}
			if problem != nil {
				t.Fatalf("Normalize() problem = %+v", problem)
			}
			if got != in {
				t.Fatalf("Normalize() = %q, want %q", got, in)
			}
		})
	}
}

func TestNormalizeRejectsBadFormat(t *testing.T) {
	got, problem, err := Normalize(context.Background(), "http://example.com", nil)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if got != "" {
		t.Fatalf("Normalize() = %q, want empty", got)
	}
	if problem == nil || problem.Code != CodeFormatInvalid {
		t.Fatalf("problem = %+v, want %s", problem, CodeFormatInvalid)
	}
}

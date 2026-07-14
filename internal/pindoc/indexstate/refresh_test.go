package indexstate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/var-gg/pindoc/internal/pindoc/embed"
)

type fakeProvider struct {
	info       embed.Info
	failOnCall int
	calls      int
	events     *[]string
}

func (p *fakeProvider) Info() embed.Info { return p.info }

func (p *fakeProvider) Embed(_ context.Context, request embed.Request) (*embed.Response, error) {
	p.calls++
	if p.events != nil {
		*p.events = append(*p.events, fmt.Sprintf("embed:%d", p.calls))
	}
	if p.failOnCall == p.calls {
		return nil, errors.New("provider unavailable")
	}
	vectors := make([][]float32, len(request.Texts))
	for i := range request.Texts {
		vectors[i] = []float32{0.25, 0.75}
	}
	return &embed.Response{Vectors: vectors}, nil
}

type fakeStore struct {
	statements []string
	events     *[]string
	attempt    int
}

func (s *fakeStore) Exec(_ context.Context, query string, _ ...any) (pgconn.CommandTag, error) {
	s.statements = append(s.statements, query)
	if s.events != nil {
		*s.events = append(*s.events, "exec:"+statementKind(query))
	}
	return pgconn.CommandTag{}, nil
}

func (s *fakeStore) QueryRow(_ context.Context, query string, _ ...any) pgx.Row {
	s.statements = append(s.statements, query)
	if s.events != nil {
		*s.events = append(*s.events, "query:"+statementKind(query))
	}
	if s.attempt == 0 {
		s.attempt = 1
	}
	return fakeRow{value: s.attempt}
}

type fakeRow struct {
	value int
}

func (r fakeRow) Scan(dest ...any) error {
	if len(dest) != 1 {
		return fmt.Errorf("fake row expected one destination, got %d", len(dest))
	}
	value, ok := dest[0].(*int)
	if !ok {
		return fmt.Errorf("fake row destination is %T, want *int", dest[0])
	}
	*value = r.value
	return nil
}

func TestRefreshProviderFailurePreservesChunksAndRecordsRetryableState(t *testing.T) {
	t.Parallel()
	store := &fakeStore{attempt: 3}
	provider := &fakeProvider{
		info:       embed.Info{Name: "test", ModelID: "v1", Dimension: 2},
		failOnCall: 2,
	}

	state, err := Refresh(context.Background(), store, provider, RefreshInput{
		ArtifactID:     "11111111-1111-1111-1111-111111111111",
		RevisionNumber: 7,
		Title:          "Title",
		Body:           "Body",
	})
	if !IsRetryable(err) {
		t.Fatalf("Refresh() error = %v, want RetryableError", err)
	}
	if state.Status != StatusFailed || !state.Retryable {
		t.Fatalf("Refresh() state = %+v, want retryable failed", state)
	}
	if state.AttemptCount != 3 {
		t.Fatalf("attempt_count = %d, want 3", state.AttemptCount)
	}
	if !strings.Contains(state.LastError, "provider unavailable") {
		t.Fatalf("last_error = %q", state.LastError)
	}
	for _, statement := range store.statements {
		if strings.Contains(statement, "DELETE FROM artifact_chunks") {
			t.Fatalf("provider failure deleted existing chunks: %s", statement)
		}
	}
}

func TestRefreshPreparesAllEmbeddingsBeforeReplacingChunks(t *testing.T) {
	t.Parallel()
	events := []string{}
	store := &fakeStore{events: &events}
	provider := &fakeProvider{
		info:   embed.Info{Name: "test", ModelID: "v1", Dimension: 2},
		events: &events,
	}

	state, err := Refresh(context.Background(), store, provider, RefreshInput{
		ArtifactID:     "22222222-2222-2222-2222-222222222222",
		RevisionNumber: 4,
		Title:          "Title",
		Body:           "Body",
	})
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if state.Status != StatusIndexed || state.ModelID != "v1" {
		t.Fatalf("Refresh() state = %+v", state)
	}
	if len(events) < 5 {
		t.Fatalf("events = %v", events)
	}
	if events[0] != "embed:1" || events[1] != "embed:2" || events[2] != "exec:delete" {
		t.Fatalf("operation order = %v, want all embeds before delete", events)
	}
	if got := countStatements(store.statements, "DELETE FROM artifact_chunks"); got != 1 {
		t.Fatalf("delete statements = %d, want 1", got)
	}
	if got := countStatements(store.statements, "INSERT INTO artifact_chunks"); got != 2 {
		t.Fatalf("chunk inserts = %d, want title + body", got)
	}
}

func TestMarkUnknownDoesNotTouchChunks(t *testing.T) {
	t.Parallel()
	store := &fakeStore{attempt: 2}
	state, err := MarkUnknown(context.Background(), store, RefreshInput{
		ArtifactID:     "33333333-3333-3333-3333-333333333333",
		RevisionNumber: 9,
		Title:          "Title",
		Body:           "Body",
	})
	if err != nil {
		t.Fatalf("MarkUnknown() error = %v", err)
	}
	if state.Status != StatusUnknown || state.AttemptCount != 2 {
		t.Fatalf("MarkUnknown() state = %+v", state)
	}
	if got := countStatements(store.statements, "artifact_chunks"); got != 0 {
		t.Fatalf("MarkUnknown() touched chunks in %d statements", got)
	}
}

func TestClassifyDetectsContentAndProviderDrift(t *testing.T) {
	t.Parallel()
	info := embed.Info{Name: "test", ModelID: "v1", Dimension: 2}
	state := &State{
		BodyHash:  HashText("Body"),
		TitleHash: HashText("Title"),
		ModelName: "test",
		ModelID:   "v1",
		ModelDim:  2,
		Status:    StatusIndexed,
	}
	if got := Classify("Title", "Body", state, info); got != StatusIndexed {
		t.Fatalf("Classify(current) = %q", got)
	}
	if got := Classify("Title", "changed", state, info); got != StatusStale {
		t.Fatalf("Classify(body drift) = %q", got)
	}
	changedProvider := info
	changedProvider.ModelID = "v2"
	if got := Classify("Title", "Body", state, changedProvider); got != StatusStale {
		t.Fatalf("Classify(provider drift) = %q", got)
	}
	failed := *state
	failed.Status = StatusFailed
	if got := Classify("Title", "changed", &failed, changedProvider); got != StatusFailed {
		t.Fatalf("Classify(failed) = %q", got)
	}
}

func statementKind(query string) string {
	query = strings.TrimSpace(query)
	switch {
	case strings.HasPrefix(query, "DELETE"):
		return "delete"
	case strings.Contains(query, "INSERT INTO artifact_chunks"):
		return "chunk-insert"
	case strings.Contains(query, "INSERT INTO artifact_index_state"):
		return "state-upsert"
	default:
		return "other"
	}
}

func countStatements(statements []string, needle string) int {
	count := 0
	for _, statement := range statements {
		if strings.Contains(statement, needle) {
			count++
		}
	}
	return count
}

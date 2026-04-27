package telemetry

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"
)

// TestRecordNeverBlocks is the core invariant — even with a tiny buffer
// and a slow (absent) writer, Record() must return immediately and
// increment the drop counter rather than stall the caller. This is the
// load-bearing promise of the package: zero response-latency impact.
func TestRecordNeverBlocks(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	// Pool-less Store exercises the "no DB" path; flusher still spins.
	s := New(context.Background(), nil, logger, Options{BufferSize: 4, BatchSize: 4, FlushEvery: time.Hour})
	defer s.Close()

	start := time.Now()
	for i := 0; i < 1000; i++ {
		s.Record(Entry{ToolName: "stress.test", StartedAt: time.Now()})
	}
	elapsed := time.Since(start)
	if elapsed > 50*time.Millisecond {
		t.Fatalf("Record()×1000 took %v — should be <<50ms under the zero-latency promise", elapsed)
	}
	// Verify some were dropped (buffer=4, inserted 1000) — the drop
	// counter is the "we tried to back-pressure and chose to drop"
	// signal an operator would monitor.
	_, dropped := s.Stats()
	if dropped == 0 {
		t.Fatalf("expected drop counter to move under buffer=4 / 1000 sends, got 0")
	}
}

// TestNilStoreIsSafe asserts the nil-receiver contract. Handler
// wrappers call store.Record() unconditionally; a nil Store needs to
// swallow the call so tests that skip telemetry don't crash.
func TestNilStoreIsSafe(t *testing.T) {
	var s *Store
	// None of these should panic.
	s.Record(Entry{ToolName: "nil.probe"})
	_ = s.EstimateTokens("hello world")
	w, d := s.Stats()
	if w != 0 || d != 0 {
		t.Fatalf("nil stats non-zero: wrote=%d dropped=%d", w, d)
	}
	s.Close()
}

// TestEstimateTokensShape sanity-checks the tiktoken integration —
// not exact match, just "returns a positive number smaller than byte
// count for English text".
func TestEstimateTokensShape(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	s := New(context.Background(), nil, logger, Options{})
	defer s.Close()

	text := "the quick brown fox jumps over the lazy dog"
	est := s.EstimateTokens(text)
	if est <= 0 {
		t.Fatalf("expected positive token estimate, got %d", est)
	}
	// English BPE typically produces roughly 1 token per word on
	// common text; the sentence is 9 words.
	if est > len(text) {
		t.Fatalf("token estimate (%d) should not exceed byte count (%d)", est, len(text))
	}
}

// TestEmptyTextIsZero — empty string shouldn't enter the tokenizer.
func TestEmptyTextIsZero(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	s := New(context.Background(), nil, logger, Options{})
	defer s.Close()
	if got := s.EstimateTokens(""); got != 0 {
		t.Fatalf("empty string should yield 0 tokens, got %d", got)
	}
}

// TestPgxBatchShape locks down the SQL-template generator. A regression
// that inflates parameter positions (e.g., $1..$15 on row 1 but $16..$30
// on row 2 going sideways) would silently corrupt bulk inserts.
func TestPgxBatchShape(t *testing.T) {
	entries := []Entry{
		{ToolName: "a"}, {ToolName: "b"}, {ToolName: "c"},
	}
	b := pgxBatch(entries)
	// 15 columns × 3 rows = 45 placeholders (column 15 is metadata,
	// added in migration 0024).
	if len(b.args) != 15*3 {
		t.Fatalf("expected 45 args, got %d", len(b.args))
	}
	// Must contain $45 and not $46 — cheap sanity check.
	if !contains(b.sql, "$45") {
		t.Fatalf("sql missing final placeholder $45:\n%s", b.sql)
	}
	if contains(b.sql, "$46") {
		t.Fatalf("sql leaked extra placeholder $46:\n%s", b.sql)
	}
	// metadata is the only column that needs a JSONB cast — every batch
	// row must carry one. With 3 rows we expect exactly 3 occurrences.
	if got := countOccurrences(b.sql, "::jsonb"); got != 3 {
		t.Fatalf("expected 3 jsonb casts, got %d:\n%s", got, b.sql)
	}
}

// TestMetadataArgDefaults asserts the empty-payload contract: nil and
// zero-length RawMessage both produce '{}' so the NOT NULL column stays
// happy without forcing every wrapper to remember the default.
func TestMetadataArgDefaults(t *testing.T) {
	if got := metadataArg(nil); got != "{}" {
		t.Fatalf("nil metadata: got %v, want \"{}\"", got)
	}
	if got := metadataArg([]byte{}); got != "{}" {
		t.Fatalf("empty metadata: got %v, want \"{}\"", got)
	}
	payload := []byte(`{"via":"git"}`)
	if got := metadataArg(payload); got != string(payload) {
		t.Fatalf("non-empty metadata: got %v, want %q", got, string(payload))
	}
}

func countOccurrences(s, sub string) int {
	if len(sub) == 0 {
		return 0
	}
	count := 0
	start := 0
	for {
		idx := indexOf(s[start:], sub)
		if idx < 0 {
			return count
		}
		count++
		start += idx + len(sub)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	n := len(sub)
	if n == 0 {
		return 0
	}
	for i := 0; i+n <= len(s); i++ {
		if s[i:i+n] == sub {
			return i
		}
	}
	return -1
}

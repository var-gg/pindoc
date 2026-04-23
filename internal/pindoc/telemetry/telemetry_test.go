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
// that inflates parameter positions (e.g., $1..$14 on row 1 but $15..$29
// on row 2 going sideways) would silently corrupt bulk inserts.
func TestPgxBatchShape(t *testing.T) {
	entries := []Entry{
		{ToolName: "a"}, {ToolName: "b"}, {ToolName: "c"},
	}
	b := pgxBatch(entries)
	// 14 columns × 3 rows = 42 placeholders.
	if len(b.args) != 14*3 {
		t.Fatalf("expected 42 args, got %d", len(b.args))
	}
	// Must contain $42 and not $43 — cheap sanity check.
	if !contains(b.sql, "$42") {
		t.Fatalf("sql missing final placeholder $42:\n%s", b.sql)
	}
	if contains(b.sql, "$43") {
		t.Fatalf("sql leaked extra placeholder $43:\n%s", b.sql)
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

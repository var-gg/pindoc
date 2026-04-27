// Package telemetry is Pindoc's async MCP tool-call logger.
//
// Every tool handler is wrapped by Instrument (see
// internal/pindoc/mcp/tools/telemetry_wrap.go) which records the
// start/end envelope and ships an Entry through a buffered channel.
// A background flusher batches inserts into the mcp_tool_calls
// table on a size / interval trigger. Agents see zero latency cost
// — the send is a non-blocking channel push that drops when the
// buffer is full, trading rare-case observability gaps for
// guaranteed response-time stability.
//
// Design choices:
//
//   - In-tree, not external (prom/OTLP). Self-host deployments
//     shouldn't need a sidecar to answer "which tool costs me the
//     most tokens this week". Querying mcp_tool_calls directly is
//     enough for trend observation.
//   - Drop-on-full is the right default. A back-pressure strategy
//     (block until space) would defeat the "zero response impact"
//     promise; the dropped counter is surfaced via Stats() for
//     operators who want to know they're undersized.
//   - Token counts are approximations via tiktoken cl100k_base.
//     Claude's tokenizer is not public; BPE approximation is within
//     roughly ±20% for CJK / ±5% for English which is enough for
//     relative comparison between tools and across time.
package telemetry

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pkoukk/tiktoken-go"
)

// Entry is one tool call record. Sent from the handler wrapper to the
// flusher over a buffered channel.
type Entry struct {
	StartedAt       time.Time
	DurationMs      int64
	ToolName        string
	AgentID         string
	UserID          string // empty when server has no user bound
	ProjectSlug     string
	InputBytes      int
	OutputBytes     int
	InputChars      int
	OutputChars     int
	InputTokensEst  int
	OutputTokensEst int
	ErrorCode       string
	ToolsetVersion  string

	// Metadata is the tool-specific result-attribute payload (Decision
	// mcp-dx-외부-리뷰-codex-1차-피드백-6항목 발견 4). The wrapper in
	// internal/pindoc/mcp/tools/telemetry_wrap.go fills this from a
	// per-tool extractor; tools without an extractor leave it nil and
	// the row defaults to '{}'::jsonb in the DB. Use json.RawMessage so
	// the writer can pass through pre-serialised bytes without a second
	// json.Marshal.
	Metadata json.RawMessage
}

// Store is the live async logger. Safe for concurrent Record() calls.
// Close() drains the channel and flushes outstanding entries before
// returning — call it during graceful shutdown.
type Store struct {
	ch          chan Entry
	pool        *pgxpool.Pool
	logger      *slog.Logger
	batchSize   int
	flushEvery  time.Duration
	dropped     atomic.Uint64
	wrote       atomic.Uint64
	tokenizer   *tiktoken.Tiktoken
	tokenizerMu sync.Mutex // tiktoken-go's encoder is not documented safe for concurrent use
	done        chan struct{}
}

// Options tunes buffer size / batch size / flush cadence. Zero values use
// sensible defaults: 1024 channel buffer, 64 batch, 2s flush.
type Options struct {
	BufferSize int
	BatchSize  int
	FlushEvery time.Duration
}

// New starts the background flusher and returns a ready-to-use Store.
// The context governs the flusher's lifetime; cancel it (or call
// Close) to drain and exit. A nil pool disables DB writes but keeps
// Record() functional — useful for tests or ops modes that want the
// logger interface without persistence.
func New(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, opts Options) *Store {
	if opts.BufferSize <= 0 {
		opts.BufferSize = 1024
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = 64
	}
	if opts.FlushEvery <= 0 {
		opts.FlushEvery = 2 * time.Second
	}
	// cl100k_base is the GPT-4 tokenizer — closest public BPE proxy for
	// Anthropic Claude. Used only for trend observation, not billing.
	tk, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		logger.Warn("tiktoken init failed — token estimates will be 0", "err", err)
	}
	s := &Store{
		ch:         make(chan Entry, opts.BufferSize),
		pool:       pool,
		logger:     logger,
		batchSize:  opts.BatchSize,
		flushEvery: opts.FlushEvery,
		tokenizer:  tk,
		done:       make(chan struct{}),
	}
	go s.runFlusher(ctx)
	return s
}

// Record enqueues an entry. Non-blocking: returns immediately whether
// the channel had room or not. Full buffer increments the Dropped()
// counter instead of blocking the caller, preserving the "zero
// response-time impact" promise.
func (s *Store) Record(e Entry) {
	if s == nil {
		return
	}
	select {
	case s.ch <- e:
	default:
		s.dropped.Add(1)
	}
}

// EstimateTokens counts an approximate token length for the given UTF-8
// text via tiktoken cl100k_base. Thread-safe (guarded by tokenizerMu
// because tiktoken-go doesn't document goroutine safety). Returns 0 if
// the encoder failed to initialise.
func (s *Store) EstimateTokens(text string) int {
	if s == nil || s.tokenizer == nil || text == "" {
		return 0
	}
	s.tokenizerMu.Lock()
	defer s.tokenizerMu.Unlock()
	return len(s.tokenizer.Encode(text, nil, nil))
}

// Stats returns cumulative counters — wrote=successful DB rows,
// dropped=channel-full events. Operators can poll these to detect
// sustained overload (dropped climbing = buffer undersized).
func (s *Store) Stats() (wrote, dropped uint64) {
	if s == nil {
		return 0, 0
	}
	return s.wrote.Load(), s.dropped.Load()
}

// Close blocks until outstanding entries have been flushed. Call before
// process exit.
func (s *Store) Close() {
	if s == nil {
		return
	}
	close(s.ch)
	<-s.done
}

// runFlusher pulls entries off the channel, batches them, and bulk-
// inserts. Flush triggers: batch full OR ticker OR channel closed.
func (s *Store) runFlusher(ctx context.Context) {
	defer close(s.done)
	buf := make([]Entry, 0, s.batchSize)
	tick := time.NewTicker(s.flushEvery)
	defer tick.Stop()

	flush := func() {
		if len(buf) == 0 {
			return
		}
		if err := s.writeBatch(ctx, buf); err != nil {
			s.logger.Warn("telemetry flush failed", "err", err, "entries", len(buf))
		} else {
			s.wrote.Add(uint64(len(buf)))
		}
		buf = buf[:0]
	}

	for {
		select {
		case <-ctx.Done():
			// Drain anything still in the channel (non-blocking).
			for {
				select {
				case e, ok := <-s.ch:
					if !ok {
						flush()
						return
					}
					buf = append(buf, e)
					if len(buf) >= s.batchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		case e, ok := <-s.ch:
			if !ok {
				flush()
				return
			}
			buf = append(buf, e)
			if len(buf) >= s.batchSize {
				flush()
			}
		case <-tick.C:
			flush()
		}
	}
}

// writeBatch is pulled out for testability. Pool-less Store skips
// persistence entirely (useful for tests) while still ticking
// counters.
func (s *Store) writeBatch(ctx context.Context, entries []Entry) error {
	if s.pool == nil {
		return nil
	}
	// Use a child context so a cancelled parent during Close still lets
	// the final batch land.
	writeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	batch := pgxBatch(entries)
	_, err := s.pool.Exec(writeCtx, batch.sql, batch.args...)
	return err
}

// sqlBatch packs N entries into a single multi-VALUES INSERT statement.
// pgx supports parameter arrays but a templated multi-row INSERT is
// cheaper for a hot telemetry path and avoids round-trip setup cost.
type sqlBatch struct {
	sql  string
	args []any
}

const insertCols = `
	started_at, duration_ms, tool_name, agent_id, user_id, project_slug,
	input_bytes, output_bytes, input_chars, output_chars,
	input_tokens_est, output_tokens_est, error_code, toolset_version,
	metadata
`

func pgxBatch(entries []Entry) sqlBatch {
	const cols = 15
	args := make([]any, 0, len(entries)*cols)
	sb := make([]byte, 0, 256+64*len(entries))
	sb = append(sb, "INSERT INTO mcp_tool_calls ("+insertCols+") VALUES "...)
	for i, e := range entries {
		if i > 0 {
			sb = append(sb, ',')
		}
		sb = append(sb, '(')
		for j := 0; j < cols; j++ {
			if j > 0 {
				sb = append(sb, ',')
			}
			idx := i*cols + j + 1
			sb = append(sb, '$')
			sb = appendInt(sb, idx)
			// metadata column needs a JSONB cast so pgx ships bytes/text
			// to the right type. It is the last column.
			if j == cols-1 {
				sb = append(sb, "::jsonb"...)
			}
		}
		sb = append(sb, ')')

		args = append(args,
			e.StartedAt,
			e.DurationMs,
			e.ToolName,
			nullIfEmpty(e.AgentID),
			nullIfEmptyUUID(e.UserID),
			nullIfEmpty(e.ProjectSlug),
			e.InputBytes,
			e.OutputBytes,
			e.InputChars,
			e.OutputChars,
			e.InputTokensEst,
			e.OutputTokensEst,
			nullIfEmpty(e.ErrorCode),
			nullIfEmpty(e.ToolsetVersion),
			metadataArg(e.Metadata),
		)
	}
	return sqlBatch{sql: string(sb), args: args}
}

// metadataArg returns the SQL argument for the metadata column. Nil and
// empty payloads default to '{}' so the column's NOT NULL contract is
// preserved without forcing every tool wrapper to remember to pass a
// non-nil RawMessage.
func metadataArg(raw json.RawMessage) any {
	if len(raw) == 0 {
		return "{}"
	}
	return string(raw)
}

func appendInt(b []byte, n int) []byte {
	if n == 0 {
		return append(b, '0')
	}
	var tmp [20]byte
	pos := len(tmp)
	for n > 0 {
		pos--
		tmp[pos] = byte('0' + n%10)
		n /= 10
	}
	return append(b, tmp[pos:]...)
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// nullIfEmptyUUID separates empty-string UUID handling because pgx
// interprets a bare string as text and would fail the uuid column cast.
func nullIfEmptyUUID(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Package receipts is the in-memory search-receipt ledger.
//
// The contract: every call to pindoc.artifact.search or
// pindoc.context.for_task returns a short opaque token (the "receipt").
// pindoc.artifact.propose (create path) requires a valid, not-yet-expired
// receipt for the same project before it will accept the write. This is
// the server-side teeth behind the PINDOC.md "search before propose"
// rule — an agent that skips retrieval cannot fake its way past the gate.
//
// Design choices for V1:
//
//   - In-memory only. A restart wipes receipts, which is fine: MCP
//     subprocess lifetime is bounded to an agent session, and restarts are
//     rare. Persistence belongs in V1.5 alongside agent tokens.
//   - TTL is short (10 minutes default). An agent that searches once and
//     then thinks for hours has to re-search — which is the correct
//     behaviour because by then the corpus may have changed.
//   - Keyed by project slug so cross-project leakage is impossible.
//   - Background goroutine sweeps expired entries so the map doesn't grow
//     unbounded under stub-embedder stress tests.
package receipts

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// DefaultTTL is how long a receipt stays usable after issue. 30 minutes
// balances "agent takes a coding loop of many tool calls" against "corpus
// may have meaningfully changed during a long pause". Extended from the
// original 10 minutes after 3rd-round peer review feedback that the
// shorter window forced re-search at the end of most real coding loops.
const DefaultTTL = 30 * time.Minute

type entry struct {
	project   string
	query     string
	issuedAt  time.Time
	expiresAt time.Time
}

// Store is safe for concurrent Issue/Verify.
type Store struct {
	mu  sync.Mutex
	buf map[string]entry
	ttl time.Duration
}

// New constructs a Store with the given TTL. A zero ttl uses DefaultTTL.
// Call Close() on shutdown to stop the sweeper goroutine.
func New(ttl time.Duration) *Store {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	s := &Store{
		buf: make(map[string]entry),
		ttl: ttl,
	}
	go s.sweep()
	return s
}

// Issue returns a new receipt string for the given (project, query) pair.
// The query is stored only so debug logs and VERBOSE mode can echo it back
// on verification — it's not part of the verification check.
func (s *Store) Issue(project, query string) string {
	if s == nil {
		return ""
	}
	now := time.Now()
	buf := make([]byte, 12) // 24 hex chars, plenty of entropy for 10-min TTL
	_, _ = rand.Read(buf)
	id := "sr_" + hex.EncodeToString(buf)

	s.mu.Lock()
	s.buf[id] = entry{
		project:   project,
		query:     query,
		issuedAt:  now,
		expiresAt: now.Add(s.ttl),
	}
	s.mu.Unlock()
	return id
}

// VerifyResult carries the outcome of Verify. Valid=true means the receipt
// existed, matched the project, and hadn't expired. The other booleans
// discriminate failure reasons so the caller can emit a specific code
// (NO_SRCH / RECEIPT_EXPIRED / RECEIPT_WRONG_PROJECT / RECEIPT_UNKNOWN).
type VerifyResult struct {
	Valid          bool
	Unknown        bool // receipt never issued (or already swept)
	Expired        bool
	WrongProject   bool
	IssuedQuery    string
}

// Verify looks up a receipt. After a successful verify the receipt stays
// usable until expiry — multiple propose calls in one session can reuse
// the same receipt (common: search once, then several connected writes).
func (s *Store) Verify(receipt, project string) VerifyResult {
	if s == nil || receipt == "" {
		return VerifyResult{Unknown: true}
	}
	s.mu.Lock()
	e, ok := s.buf[receipt]
	s.mu.Unlock()
	if !ok {
		return VerifyResult{Unknown: true}
	}
	if time.Now().After(e.expiresAt) {
		return VerifyResult{Expired: true, IssuedQuery: e.query}
	}
	if e.project != project {
		return VerifyResult{WrongProject: true, IssuedQuery: e.query}
	}
	return VerifyResult{Valid: true, IssuedQuery: e.query}
}

// sweep drops expired entries every TTL/4. Skipping this leaks memory
// under the happy path because Verify doesn't delete — deletion on verify
// would force agents to re-search after a single successful write, which
// is the wrong trade-off (see package doc).
func (s *Store) sweep() {
	t := time.NewTicker(s.ttl / 4)
	defer t.Stop()
	for range t.C {
		now := time.Now()
		s.mu.Lock()
		for id, e := range s.buf {
			if now.After(e.expiresAt) {
				delete(s.buf, id)
			}
		}
		s.mu.Unlock()
	}
}

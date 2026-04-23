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
//   - Phase E shift: primary staleness signal is "any snapshot artifact's
//     revision number has moved since issue" (RECEIPT_SUPERSEDED), not a
//     clock TTL. The clock TTL stays as a very-long memory-pressure
//     fallback (24h) because the in-memory map would otherwise grow
//     unbounded under stub-embedder stress tests. The user-visible TTL
//     became "until the corpus moves" instead of "until 30 minutes elapse".
//   - Keyed by project slug so cross-project leakage is impossible.
//   - Background goroutine sweeps expired entries on the long TTL so the
//     map doesn't grow unbounded.
package receipts

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// DefaultTTL is the fallback clock-based expiry (Phase E: demoted to a
// memory-pressure relief, not the primary staleness signal). 24h is long
// enough that most agent sessions never hit it; those that do have
// already seen the corpus drift and should re-search anyway.
const DefaultTTL = 24 * time.Hour

// ArtifactRef is a point-in-time snapshot of one artifact's revision
// head. Receipts store the list of refs observed at issue time; the
// verifier (artifact.propose call site, DB-aware) flags refs whose
// revision_number has advanced since and treats "all advanced" as
// RECEIPT_SUPERSEDED.
type ArtifactRef struct {
	ArtifactID     string `json:"artifact_id"`
	RevisionNumber int    `json:"revision_number"`
}

type entry struct {
	project   string
	query     string
	issuedAt  time.Time
	expiresAt time.Time
	snapshots []ArtifactRef
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

// Issue returns a new receipt string for the given (project, query,
// snapshots) tuple. Snapshots carry the revision numbers the search saw
// so the propose-time verifier can detect corpus drift (Phase E). Pass
// nil snapshots to fall back to project-scope-only validity (backward
// compat for legacy callers).
func (s *Store) Issue(project, query string, snapshots []ArtifactRef) string {
	if s == nil {
		return ""
	}
	now := time.Now()
	buf := make([]byte, 12) // 24 hex chars
	_, _ = rand.Read(buf)
	id := "sr_" + hex.EncodeToString(buf)

	snapCopy := make([]ArtifactRef, len(snapshots))
	copy(snapCopy, snapshots)

	s.mu.Lock()
	s.buf[id] = entry{
		project:   project,
		query:     query,
		issuedAt:  now,
		expiresAt: now.Add(s.ttl),
		snapshots: snapCopy,
	}
	s.mu.Unlock()
	return id
}

// VerifyResult carries the outcome of Verify. Valid=true means the receipt
// existed, matched the project, and hadn't hit the long fallback TTL.
// Snapshots returns the refs the receipt was issued with so the call site
// can run the DB-aware staleness check and emit RECEIPT_SUPERSEDED when
// every referenced artifact has moved past the snapshotted revision.
type VerifyResult struct {
	Valid        bool
	Unknown      bool // receipt never issued (or already swept)
	Expired      bool // long fallback TTL tripped (memory pressure relief only)
	WrongProject bool
	IssuedQuery  string
	Snapshots    []ArtifactRef
}

// Verify looks up a receipt. After a successful verify the receipt stays
// usable until expiry — multiple propose calls in one session can reuse
// the same receipt (common: search once, then several connected writes).
// Corpus-drift staleness is NOT checked here — that requires DB access
// and lives at the call site so receipts stays dependency-free.
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
	snapCopy := make([]ArtifactRef, len(e.snapshots))
	copy(snapCopy, e.snapshots)
	return VerifyResult{Valid: true, IssuedQuery: e.query, Snapshots: snapCopy}
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

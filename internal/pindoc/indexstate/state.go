// Package indexstate owns artifact search-index provenance and refresh
// classification. Artifact persistence can depend on this package without
// pulling MCP tool orchestration into re-embedding commands.
package indexstate

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/var-gg/pindoc/internal/pindoc/embed"
)

const (
	StatusUnknown = "unknown"
	StatusIndexed = "indexed"
	StatusFailed  = "failed"
	StatusStale   = "stale"
)

// State is the durable provenance for the last index attempt. Retryable is a
// response hint and is not stored in artifact_index_state.
type State struct {
	RevisionNumber int    `json:"revision_number"`
	BodyHash       string `json:"body_hash"`
	TitleHash      string `json:"title_hash"`
	ModelName      string `json:"model_name,omitempty"`
	ModelID        string `json:"model_id,omitempty"`
	ModelDim       int    `json:"model_dim,omitempty"`
	Status         string `json:"status"`
	AttemptCount   int    `json:"attempt_count"`
	LastError      string `json:"last_error,omitempty"`
	Retryable      bool   `json:"retryable,omitempty"`
}

// HashText matches the SQL sha256(convert_to(text, 'UTF8')) representation
// used by artifact_revisions and artifact_index_health.
func HashText(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// Classify returns the effective index state for the current artifact content
// and provider. Revision numbers are provenance, while title/body hashes avoid
// false staleness after metadata-only revisions.
func Classify(title, body string, state *State, provider embed.Info) string {
	if state == nil {
		return StatusUnknown
	}
	if state.Status == StatusFailed {
		return StatusFailed
	}
	if state.Status != StatusIndexed {
		return StatusUnknown
	}
	if state.TitleHash != HashText(title) || state.BodyHash != HashText(body) {
		return StatusStale
	}
	if state.ModelName != provider.Name || state.ModelID != provider.ModelID || state.ModelDim != provider.Dimension {
		return StatusStale
	}
	return StatusIndexed
}

// RetryableError means artifact persistence may commit while this index
// attempt is recorded as failed. A later pindoc-reembed run can recover it.
type RetryableError struct {
	Cause error
}

func (e *RetryableError) Error() string {
	if e == nil || e.Cause == nil {
		return "artifact index refresh failed"
	}
	return fmt.Sprintf("artifact index refresh failed: %v", e.Cause)
}

func (e *RetryableError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func IsRetryable(err error) bool {
	var target *RetryableError
	return errors.As(err, &target)
}

package indexstate

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/var-gg/pindoc/internal/pindoc/embed"
)

const (
	bodyChunkSize  = 600
	embedBatchSize = 32
	maxLastError   = 1000
)

// Store is the transaction surface used by Refresh and MarkUnknown. pgx.Tx
// satisfies it, while tests can verify statement ordering without Postgres.
type Store interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type RefreshInput struct {
	ArtifactID     string
	RevisionNumber int
	Title          string
	Body           string
}

type preparedChunk struct {
	Kind      string
	Index     int
	Heading   any
	SpanStart int
	SpanEnd   int
	Text      string
	Vector    string
}

type preparedIndex struct {
	Info   embed.Info
	Chunks []preparedChunk
}

// Refresh prepares every embedding before it touches artifact_chunks. A
// provider failure therefore records a retryable failed state while leaving
// the last known-good chunks intact. Successful replacement and provenance
// update happen through the caller's transaction.
func Refresh(ctx context.Context, tx Store, provider embed.Provider, in RefreshInput) (State, error) {
	prepared, err := prepare(ctx, provider, in.Title, in.Body)
	if err != nil {
		state, markErr := markFailed(ctx, tx, provider.Info(), in, err)
		if markErr != nil {
			return State{}, fmt.Errorf("record failed index attempt: %w", markErr)
		}
		state.Retryable = true
		return state, &RetryableError{Cause: err}
	}

	state, err := replace(ctx, tx, in, prepared)
	if err != nil {
		return State{}, err
	}
	return state, nil
}

// MarkUnknown advances provenance when no embedding provider is available.
// Existing chunks are preserved and explicitly treated as unverified.
func MarkUnknown(ctx context.Context, tx Store, in RefreshInput) (State, error) {
	state := baseState(in)
	state.Status = StatusUnknown
	err := tx.QueryRow(ctx, `
		INSERT INTO artifact_index_state (
			artifact_id, revision_number, body_hash, title_hash, status,
			model_name, model_id, model_dim, last_error, updated_at
		) VALUES ($1::uuid, $2, $3, $4, 'unknown', '', '', 0, NULL, now())
		ON CONFLICT (artifact_id) DO UPDATE SET
			revision_number = EXCLUDED.revision_number,
			body_hash       = EXCLUDED.body_hash,
			title_hash      = EXCLUDED.title_hash,
			model_name      = '',
			model_id        = '',
			model_dim       = 0,
			status          = 'unknown',
			last_error      = NULL,
			updated_at      = now()
		RETURNING attempt_count
	`, in.ArtifactID, in.RevisionNumber, state.BodyHash, state.TitleHash).Scan(&state.AttemptCount)
	if err != nil {
		return State{}, fmt.Errorf("mark index unknown: %w", err)
	}
	return state, nil
}

func prepare(ctx context.Context, provider embed.Provider, title, body string) (preparedIndex, error) {
	if provider == nil {
		return preparedIndex{}, fmt.Errorf("embedding provider is nil")
	}
	info := provider.Info()
	if info.Dimension <= 0 {
		return preparedIndex{}, fmt.Errorf("provider %q reports invalid dimension %d", info.Name, info.Dimension)
	}

	titleResult, err := provider.Embed(ctx, embed.Request{Texts: []string{title}, Kind: embed.KindDocument})
	if err != nil {
		return preparedIndex{}, fmt.Errorf("embed title: %w", err)
	}
	if titleResult == nil || len(titleResult.Vectors) != 1 {
		got := 0
		if titleResult != nil {
			got = len(titleResult.Vectors)
		}
		return preparedIndex{}, fmt.Errorf("embed title: got %d vectors, want 1", got)
	}
	if err := validateVector(info, titleResult.Vectors[0]); err != nil {
		return preparedIndex{}, fmt.Errorf("embed title: %w", err)
	}

	prepared := preparedIndex{
		Info: info,
		Chunks: []preparedChunk{{
			Kind:    "title",
			Index:   0,
			Heading: nil,
			Text:    title,
			Vector:  embed.VectorString(embed.PadTo768(titleResult.Vectors[0])),
		}},
	}

	bodyChunks := embed.ChunkBody(title, body, bodyChunkSize)
	for start := 0; start < len(bodyChunks); start += embedBatchSize {
		end := start + embedBatchSize
		if end > len(bodyChunks) {
			end = len(bodyChunks)
		}
		texts := make([]string, 0, end-start)
		for _, chunk := range bodyChunks[start:end] {
			texts = append(texts, chunk.Text)
		}
		result, err := provider.Embed(ctx, embed.Request{Texts: texts, Kind: embed.KindDocument})
		if err != nil {
			return preparedIndex{}, fmt.Errorf("embed body batch [%d:%d]: %w", start, end, err)
		}
		if result == nil || len(result.Vectors) != end-start {
			got := 0
			if result != nil {
				got = len(result.Vectors)
			}
			return preparedIndex{}, fmt.Errorf("embed body batch [%d:%d]: got %d vectors, want %d", start, end, got, end-start)
		}
		for offset, chunk := range bodyChunks[start:end] {
			vector := result.Vectors[offset]
			if err := validateVector(info, vector); err != nil {
				return preparedIndex{}, fmt.Errorf("embed body chunk %d: %w", chunk.Index, err)
			}
			var heading any
			if chunk.Heading != "" {
				heading = chunk.Heading
			}
			prepared.Chunks = append(prepared.Chunks, preparedChunk{
				Kind:      "body",
				Index:     chunk.Index,
				Heading:   heading,
				SpanStart: chunk.SpanStart,
				SpanEnd:   chunk.SpanEnd,
				Text:      chunk.Text,
				Vector:    embed.VectorString(embed.PadTo768(vector)),
			})
		}
	}
	return prepared, nil
}

func validateVector(info embed.Info, vector []float32) error {
	if len(vector) != info.Dimension {
		return fmt.Errorf("%w: got %d, provider declares %d", embed.ErrDimensionMismatch, len(vector), info.Dimension)
	}
	return nil
}

func replace(ctx context.Context, tx Store, in RefreshInput, prepared preparedIndex) (State, error) {
	if _, err := tx.Exec(ctx, `DELETE FROM artifact_chunks WHERE artifact_id = $1::uuid`, in.ArtifactID); err != nil {
		return State{}, fmt.Errorf("purge old chunks: %w", err)
	}

	modelName := prepared.Info.Name + ":" + prepared.Info.ModelID
	for _, chunk := range prepared.Chunks {
		if _, err := tx.Exec(ctx, `
			INSERT INTO artifact_chunks (
				artifact_id, kind, chunk_index, heading, span_start, span_end,
				text, embedding, model_name, model_dim
			) VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8::vector, $9, $10)
		`, in.ArtifactID, chunk.Kind, chunk.Index, chunk.Heading, chunk.SpanStart,
			chunk.SpanEnd, chunk.Text, chunk.Vector, modelName, prepared.Info.Dimension); err != nil {
			return State{}, fmt.Errorf("store %s chunk %d: %w", chunk.Kind, chunk.Index, err)
		}
	}

	state := baseState(in)
	state.ModelName = prepared.Info.Name
	state.ModelID = prepared.Info.ModelID
	state.ModelDim = prepared.Info.Dimension
	state.Status = StatusIndexed
	err := tx.QueryRow(ctx, `
		INSERT INTO artifact_index_state (
			artifact_id, revision_number, body_hash, title_hash,
			model_name, model_id, model_dim, status, attempt_count,
			last_attempt_at, indexed_at, last_error, updated_at
		) VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, 'indexed', 1, now(), now(), NULL, now())
		ON CONFLICT (artifact_id) DO UPDATE SET
			revision_number = EXCLUDED.revision_number,
			body_hash       = EXCLUDED.body_hash,
			title_hash      = EXCLUDED.title_hash,
			model_name      = EXCLUDED.model_name,
			model_id        = EXCLUDED.model_id,
			model_dim       = EXCLUDED.model_dim,
			status          = 'indexed',
			attempt_count   = artifact_index_state.attempt_count + 1,
			last_attempt_at = now(),
			indexed_at      = now(),
			last_error      = NULL,
			updated_at      = now()
		RETURNING attempt_count
	`, in.ArtifactID, in.RevisionNumber, state.BodyHash, state.TitleHash,
		state.ModelName, state.ModelID, state.ModelDim).Scan(&state.AttemptCount)
	if err != nil {
		return State{}, fmt.Errorf("record indexed state: %w", err)
	}
	return state, nil
}

func markFailed(ctx context.Context, tx Store, info embed.Info, in RefreshInput, cause error) (State, error) {
	state := baseState(in)
	state.ModelName = info.Name
	state.ModelID = info.ModelID
	state.ModelDim = info.Dimension
	state.Status = StatusFailed
	state.LastError = compactError(cause)
	err := tx.QueryRow(ctx, `
		INSERT INTO artifact_index_state (
			artifact_id, revision_number, body_hash, title_hash,
			model_name, model_id, model_dim, status, attempt_count,
			last_attempt_at, last_error, updated_at
		) VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, 'failed', 1, now(), $8, now())
		ON CONFLICT (artifact_id) DO UPDATE SET
			revision_number = EXCLUDED.revision_number,
			body_hash       = EXCLUDED.body_hash,
			title_hash      = EXCLUDED.title_hash,
			model_name      = EXCLUDED.model_name,
			model_id        = EXCLUDED.model_id,
			model_dim       = EXCLUDED.model_dim,
			status          = 'failed',
			attempt_count   = artifact_index_state.attempt_count + 1,
			last_attempt_at = now(),
			last_error      = EXCLUDED.last_error,
			updated_at      = now()
		RETURNING attempt_count
	`, in.ArtifactID, in.RevisionNumber, state.BodyHash, state.TitleHash,
		state.ModelName, state.ModelID, state.ModelDim, state.LastError).Scan(&state.AttemptCount)
	if err != nil {
		return State{}, err
	}
	return state, nil
}

func baseState(in RefreshInput) State {
	return State{
		RevisionNumber: in.RevisionNumber,
		BodyHash:       HashText(in.Body),
		TitleHash:      HashText(in.Title),
	}
}

func compactError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if len(message) > maxLastError {
		message = message[:maxLastError]
	}
	return message
}

package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/diff"
)

// ---------------------------------------------------------------------------
// pindoc.artifact.revisions — list
// ---------------------------------------------------------------------------

type artifactRevisionsInput struct {
	IDOrSlug string `json:"id_or_slug" jsonschema:"artifact UUID, slug, or pindoc:// URL"`
	Limit    int    `json:"limit,omitempty" jsonschema:"max rows; default 30, cap 200"`
}

type RevisionMeta struct {
	RevisionNumber int       `json:"revision_number"`
	Title          string    `json:"title"`
	BodyHash       string    `json:"body_hash"`
	AuthorID       string    `json:"author_id"`
	AuthorVersion  string    `json:"author_version,omitempty"`
	CommitMsg      string    `json:"commit_msg,omitempty"`
	Completeness   string    `json:"completeness"`
	RevisionShape  string    `json:"revision_shape,omitempty"`
	RevisionType   string    `json:"revision_type,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type artifactRevisionsOutput struct {
	ArtifactID string         `json:"artifact_id"`
	Slug       string         `json:"slug"`
	Title      string         `json:"title"` // current head title
	Revisions  []RevisionMeta `json:"revisions"`
}

func RegisterArtifactRevisions(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name:        "pindoc.artifact.revisions",
			Description: "List every revision of an artifact (newest first). Returns metadata only — call pindoc.artifact.diff for actual body diffs. Use this to answer 'how many times has this been edited and why?'",
		},
		func(ctx context.Context, _ *sdk.CallToolRequest, in artifactRevisionsInput) (*sdk.CallToolResult, artifactRevisionsOutput, error) {
			ref := normalizeRef(in.IDOrSlug)
			if ref == "" {
				return nil, artifactRevisionsOutput{}, errors.New("id_or_slug is required")
			}
			limit := in.Limit
			if limit <= 0 {
				limit = 30
			}
			if limit > 200 {
				limit = 200
			}

			var artifactID, slug, title string
			err := deps.DB.QueryRow(ctx, `
				SELECT a.id::text, a.slug, a.title
				FROM artifacts a
				JOIN projects p ON p.id = a.project_id
				WHERE p.slug = $1 AND (a.id::text = $2 OR a.slug = $2)
			`, deps.ProjectSlug, ref).Scan(&artifactID, &slug, &title)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, artifactRevisionsOutput{}, fmt.Errorf("artifact %q not found", in.IDOrSlug)
			}
			if err != nil {
				return nil, artifactRevisionsOutput{}, err
			}

			rows, err := deps.DB.Query(ctx, `
				SELECT revision_number, title, body_hash, author_id, author_version,
				       commit_msg, completeness, tags, revision_shape, shape_payload, created_at
				FROM artifact_revisions
				WHERE artifact_id = $1
				ORDER BY revision_number ASC
			`, artifactID)
			if err != nil {
				return nil, artifactRevisionsOutput{}, err
			}
			defer rows.Close()

			revs := []RevisionMeta{}
			var prevSnapshot diff.RevisionMetaSnapshot
			var prevBodyHash string
			for rows.Next() {
				var r RevisionMeta
				var authorVer, commitMsg *string
				var tags []string
				var shapePayload []byte
				if err := rows.Scan(&r.RevisionNumber, &r.Title, &r.BodyHash, &r.AuthorID,
					&authorVer, &commitMsg, &r.Completeness, &tags, &r.RevisionShape,
					&shapePayload, &r.CreatedAt); err != nil {
					return nil, artifactRevisionsOutput{}, err
				}
				if authorVer != nil {
					r.AuthorVersion = *authorVer
				}
				if commitMsg != nil {
					r.CommitMsg = *commitMsg
				}
				snapshot := diff.RevisionMetaSnapshot{
					RevisionNumber: r.RevisionNumber,
					Tags:           tags,
					Completeness:   r.Completeness,
					Shape:          r.RevisionShape,
					ShapePayload:   json.RawMessage(shapePayload),
				}
				bodyChanged := prevSnapshot.RevisionNumber == 0 || r.BodyHash != prevBodyHash
				metaChanged := diff.MetaChangedBetween(prevSnapshot, snapshot)
				r.RevisionType = diff.ClassifyRevisionType(r.RevisionShape, r.CommitMsg, bodyChanged, metaChanged)
				revs = append(revs, r)
				prevSnapshot = snapshot
				prevBodyHash = r.BodyHash
			}
			for i, j := 0, len(revs)-1; i < j; i, j = i+1, j-1 {
				revs[i], revs[j] = revs[j], revs[i]
			}
			if len(revs) > limit {
				revs = revs[:limit]
			}
			return nil, artifactRevisionsOutput{
				ArtifactID: artifactID,
				Slug:       slug,
				Title:      title,
				Revisions:  revs,
			}, rows.Err()
		},
	)
}

// ---------------------------------------------------------------------------
// pindoc.artifact.diff — compare two revisions
// ---------------------------------------------------------------------------

type artifactDiffInput struct {
	IDOrSlug string `json:"id_or_slug"`
	FromRev  int    `json:"from_rev,omitempty" jsonschema:"optional; default = to_rev - 1"`
	ToRev    int    `json:"to_rev,omitempty" jsonschema:"optional; default = latest revision"`
}

type artifactDiffOutput struct {
	ArtifactID    string                `json:"artifact_id"`
	Slug          string                `json:"slug"`
	From          RevisionMeta          `json:"from"`
	To            RevisionMeta          `json:"to"`
	Stats         diff.Stats            `json:"stats"`
	MetaDelta     []diff.MetaDeltaEntry `json:"meta_delta"`
	RevisionType  string                `json:"revision_type"`
	SectionDeltas []diff.SectionDelta   `json:"section_deltas"`
	UnifiedDiff   string                `json:"unified_diff"`
}

func RegisterArtifactDiff(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name:        "pindoc.artifact.diff",
			Description: "Compute the diff between two revisions of an artifact. Returns revision_type, meta_delta, and per-section change summary (section_deltas) before unified_diff; prefer reading those summaries before consuming the full unified_diff. from_rev defaults to latest-1, to_rev to latest.",
		},
		func(ctx context.Context, _ *sdk.CallToolRequest, in artifactDiffInput) (*sdk.CallToolResult, artifactDiffOutput, error) {
			ref := normalizeRef(in.IDOrSlug)
			if ref == "" {
				return nil, artifactDiffOutput{}, errors.New("id_or_slug is required")
			}

			from, to, artifactID, slug, err := resolveDiffRevs(ctx, deps, ref, in.FromRev, in.ToRev)
			if err != nil {
				return nil, artifactDiffOutput{}, err
			}

			stats, deltas := diff.Summary(from.body, to.body)
			unified := diff.Unified(slug, from.body, to.body)
			metaSnapshots, err := loadMetaSnapshots(ctx, deps, artifactID, to.meta.RevisionNumber)
			if err != nil {
				return nil, artifactDiffOutput{}, err
			}
			metaDelta := diff.MetaDeltaForRange(from.meta.RevisionNumber, to.meta.RevisionNumber, metaSnapshots)
			revisionType := diff.ClassifyRevisionType(
				to.snapshot.Shape,
				to.meta.CommitMsg,
				from.meta.BodyHash != to.meta.BodyHash,
				len(metaDelta) > 0,
			)
			to.meta.RevisionType = revisionType

			return nil, artifactDiffOutput{
				ArtifactID:    artifactID,
				Slug:          slug,
				From:          from.meta,
				To:            to.meta,
				Stats:         stats,
				MetaDelta:     metaDelta,
				RevisionType:  revisionType,
				SectionDeltas: deltas,
				UnifiedDiff:   unified,
			}, nil
		},
	)
}

// ---------------------------------------------------------------------------
// pindoc.artifact.summary_since — multiple diffs since a point in time
// ---------------------------------------------------------------------------

// summaryStep is the per-revision piece of pindoc.artifact.summary_since.
// Lives here so both files see the same type.
type summaryStep struct {
	From          RevisionMeta          `json:"from"`
	To            RevisionMeta          `json:"to"`
	Stats         diff.Stats            `json:"stats"`
	MetaDelta     []diff.MetaDeltaEntry `json:"meta_delta,omitempty"`
	RevisionType  string                `json:"revision_type,omitempty"`
	SectionDeltas []diff.SectionDelta   `json:"section_deltas"`
}

// resolveDiffRevs loads the two revisions for a diff, defaulting from/to.
type loadedRev struct {
	meta     RevisionMeta
	body     string
	snapshot diff.RevisionMetaSnapshot
}

func resolveDiffRevs(ctx context.Context, deps Deps, ref string, fromRev, toRev int) (loadedRev, loadedRev, string, string, error) {
	var artifactID, slug string
	var latest int
	err := deps.DB.QueryRow(ctx, `
		SELECT a.id::text, a.slug,
		       COALESCE((SELECT max(revision_number) FROM artifact_revisions WHERE artifact_id = a.id), 0)
		FROM artifacts a
		JOIN projects p ON p.id = a.project_id
		WHERE p.slug = $1 AND (a.id::text = $2 OR a.slug = $2)
	`, deps.ProjectSlug, ref).Scan(&artifactID, &slug, &latest)
	if errors.Is(err, pgx.ErrNoRows) {
		return loadedRev{}, loadedRev{}, "", "", fmt.Errorf("artifact %q not found", ref)
	}
	if err != nil {
		return loadedRev{}, loadedRev{}, "", "", err
	}
	if latest == 0 {
		return loadedRev{}, loadedRev{}, "", "", errors.New("artifact has no revisions")
	}

	if toRev == 0 {
		toRev = latest
	}
	if fromRev == 0 {
		fromRev = toRev - 1
		if fromRev < 1 {
			fromRev = 1
		}
	}
	if fromRev == toRev && latest > 1 {
		// common agent mistake: they asked for diff of rev N vs rev N.
		// coerce to "since previous" so the call returns something useful.
		fromRev = toRev - 1
	}

	from, err := loadRev(ctx, deps, artifactID, fromRev)
	if err != nil {
		return loadedRev{}, loadedRev{}, "", "", fmt.Errorf("from rev %d: %w", fromRev, err)
	}
	to, err := loadRev(ctx, deps, artifactID, toRev)
	if err != nil {
		return loadedRev{}, loadedRev{}, "", "", fmt.Errorf("to rev %d: %w", toRev, err)
	}
	return from, to, artifactID, slug, nil
}

func loadRev(ctx context.Context, deps Deps, artifactID string, rev int) (loadedRev, error) {
	var out loadedRev
	var authorVer, commitMsg *string
	var tags []string
	var shapePayload []byte
	err := deps.DB.QueryRow(ctx, `
		SELECT r.revision_number, r.title,
		       COALESCE(
		           r.body_markdown,
		           (
		               SELECT prev.body_markdown
		               FROM artifact_revisions prev
		               WHERE prev.artifact_id = r.artifact_id
		                 AND prev.revision_number < r.revision_number
		                 AND prev.body_markdown IS NOT NULL
		               ORDER BY prev.revision_number DESC
		               LIMIT 1
		           ),
		           ''
		       ) AS body_markdown,
		       r.body_hash, r.author_id, r.author_version, r.commit_msg,
		       r.completeness, r.tags, r.revision_shape, r.shape_payload, r.created_at
		FROM artifact_revisions r
		WHERE r.artifact_id = $1 AND r.revision_number = $2
	`, artifactID, rev).Scan(
		&out.meta.RevisionNumber, &out.meta.Title, &out.body, &out.meta.BodyHash,
		&out.meta.AuthorID, &authorVer, &commitMsg, &out.meta.Completeness, &tags,
		&out.meta.RevisionShape, &shapePayload, &out.meta.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, fmt.Errorf("revision %d not found", rev)
	}
	if err != nil {
		return out, err
	}
	if authorVer != nil {
		out.meta.AuthorVersion = *authorVer
	}
	if commitMsg != nil {
		out.meta.CommitMsg = *commitMsg
	}
	out.snapshot = diff.RevisionMetaSnapshot{
		RevisionNumber: out.meta.RevisionNumber,
		Tags:           tags,
		Completeness:   out.meta.Completeness,
		Shape:          out.meta.RevisionShape,
		ShapePayload:   json.RawMessage(shapePayload),
	}
	return out, nil
}

func loadMetaSnapshots(ctx context.Context, deps Deps, artifactID string, toRev int) ([]diff.RevisionMetaSnapshot, error) {
	rows, err := deps.DB.Query(ctx, `
		SELECT revision_number, tags, completeness, revision_shape, shape_payload
		FROM artifact_revisions
		WHERE artifact_id = $1 AND revision_number <= $2
		ORDER BY revision_number ASC
	`, artifactID, toRev)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []diff.RevisionMetaSnapshot
	for rows.Next() {
		var snap diff.RevisionMetaSnapshot
		var payload []byte
		if err := rows.Scan(&snap.RevisionNumber, &snap.Tags, &snap.Completeness, &snap.Shape, &payload); err != nil {
			return nil, err
		}
		snap.ShapePayload = json.RawMessage(payload)
		out = append(out, snap)
	}
	return out, rows.Err()
}

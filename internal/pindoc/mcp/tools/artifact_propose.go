package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/embed"
	"github.com/var-gg/pindoc/internal/pindoc/i18n"
)

// ValidArtifactTypes are the types Phase 2 accepts. Tier A (7) + Tier B
// Web-SaaS pack (4). When a Tier B pack is activated for a project in
// Phase 4+ this set becomes project-scoped, but for M1 a single flat
// whitelist is enough.
var validArtifactTypes = map[string]struct{}{
	// Tier A core
	"Decision": {}, "Analysis": {}, "Debug": {}, "Flow": {},
	"Task": {}, "TC": {}, "Glossary": {},
	// Tier B Web SaaS
	"Feature": {}, "APIEndpoint": {}, "Screen": {}, "DataModel": {},
}

type artifactProposeInput struct {
	Type          string   `json:"type" jsonschema:"one of Decision|Analysis|Debug|Flow|Task|TC|Glossary|Feature|APIEndpoint|Screen|DataModel"`
	AreaSlug      string   `json:"area_slug" jsonschema:"slug from pindoc.area.list; use 'misc' or '_unsorted' if unsure"`
	Title         string   `json:"title"`
	BodyMarkdown  string   `json:"body_markdown" jsonschema:"main content in markdown"`
	Slug          string   `json:"slug,omitempty" jsonschema:"optional; auto-generated from title if absent"`
	Tags          []string `json:"tags,omitempty"`
	Completeness  string   `json:"completeness,omitempty" jsonschema:"draft|partial|settled; default partial"`
	AuthorID      string   `json:"author_id" jsonschema:"'claude-code', 'cursor', 'codex', etc."`
	AuthorVersion string   `json:"author_version,omitempty" jsonschema:"e.g. 'opus-4.7'"`

	// UpdateOf switches this call from "create a new artifact" to "append a
	// revision to an existing one". Accepts artifact UUID, project-scoped
	// slug, or pindoc://slug URL. When set, exact-title conflict is
	// skipped and a new artifact_revisions row is written.
	UpdateOf string `json:"update_of,omitempty" jsonschema:"id, slug, or pindoc:// URL of the artifact to revise"`

	// CommitMsg is a short one-liner stored on the revision row so diff
	// views and history lists can explain why the body changed. Required
	// when update_of is set; ignored otherwise.
	CommitMsg string `json:"commit_msg,omitempty" jsonschema:"required for updates; one line rationale"`

	// ExpectedVersion gates an update_of call against concurrent revision
	// writers. If set, the server compares against the artifact's current
	// max(revision_number); mismatch → not_ready with VER_CONFLICT. Leave
	// unset to accept whatever head is (legacy, optimistic-lock off).
	ExpectedVersion int `json:"expected_version,omitempty" jsonschema:"optional optimistic lock for update_of"`

	// SupersedeOf marks the target artifact as superseded by this new one.
	// Creates a NEW artifact (like a no-update_of call), then flips the
	// target's status to 'superseded' and sets superseded_by to the new id.
	// Different from update_of: update appends a revision to the same
	// artifact; supersede creates a replacement and archives the old one.
	SupersedeOf string `json:"supersede_of,omitempty" jsonschema:"id, slug, or pindoc:// URL of the artifact being replaced"`

	// Pins attach code references to the artifact. All optional — the
	// server stores whatever is provided. path is the only required field
	// in each pin (enforced by DB check). Phase 11a stores them; stale
	// detection (comparing commit_sha to current HEAD) lands V1.x.
	Pins []ArtifactPinInput `json:"pins,omitempty" jsonschema:"code references tying this artifact to files/commits"`

	// RelatesTo records typed edges to other artifacts in the same project.
	// Valid relations: implements | references | blocks | relates_to.
	// Target may be id, slug, or pindoc:// URL — resolved server-side.
	// Unknown targets fail the whole call with RELATES_TARGET_NOT_FOUND.
	RelatesTo []ArtifactRelationInput `json:"relates_to,omitempty" jsonschema:"typed edges to other artifacts"`
}

// ArtifactPinInput is the agent-facing shape for a single code pin. `path`
// is mandatory (enforced at DB level); everything else is optional because
// agents frequently don't know the commit or lines precisely at write time.
type ArtifactPinInput struct {
	Repo       string `json:"repo,omitempty" jsonschema:"'origin' default; named remote when multi-repo"`
	CommitSHA  string `json:"commit_sha,omitempty"`
	Path       string `json:"path"`
	LinesStart int    `json:"lines_start,omitempty"`
	LinesEnd   int    `json:"lines_end,omitempty"`
}

// ArtifactRelationInput is the agent-facing shape for one edge.
type ArtifactRelationInput struct {
	TargetID string `json:"target_id" jsonschema:"id, slug, or pindoc:// URL of the related artifact"`
	Relation string `json:"relation" jsonschema:"one of implements|references|blocks|relates_to"`
}

var validRelations = map[string]struct{}{
	"implements": {}, "references": {}, "blocks": {}, "relates_to": {},
}

type artifactProposeOutput struct {
	Status    string `json:"status"` // "accepted" | "not_ready"
	ErrorCode string `json:"error_code,omitempty"`
	// Failed is the Phase 12-style stable code list. Populated alongside
	// the legacy natural-language Checklist during Phase 11a so agents can
	// start branching on codes now; Checklist becomes optional in Phase 12.
	Failed           []string `json:"failed,omitempty"`
	Checklist        []string `json:"checklist,omitempty"`
	SuggestedActions []string `json:"suggested_actions,omitempty"`

	// Only set on Status == "accepted".
	ArtifactID string `json:"artifact_id,omitempty"`
	Slug       string `json:"slug,omitempty"`
	// AgentRef is the pindoc://<slug> URL an agent re-feeds to
	// artifact.read or embeds in other artifact bodies. Stable across UI
	// route changes.
	AgentRef string `json:"agent_ref,omitempty"`
	// HumanURL is the /p/:project/wiki/:slug path an agent pastes into
	// chat so the user can open the Reader in a browser. Relative because
	// the external origin belongs to the user's deployment.
	HumanURL       string    `json:"human_url,omitempty"`
	PublishedAt    time.Time `json:"published_at,omitzero"`
	Created        bool      `json:"created"`         // false on updates
	RevisionNumber int       `json:"revision_number"` // 1 on create, N+1 on update

	// Phase 11a: surface what was actually persisted so agents get
	// confirmation of edge/pin storage without a second read.
	PinsStored   int  `json:"pins_stored,omitempty"`
	EdgesStored  int  `json:"edges_stored,omitempty"`
	Superseded   bool `json:"superseded,omitempty"` // true if supersede_of was processed
}

// RegisterArtifactPropose wires pindoc.artifact.propose — the only write
// tool in Phase 2. Implements the Pre-flight Check mechanism (M0.5 in
// docs/05): on failing checks the tool returns Status=not_ready with a
// checklist telling the agent what to fix, not a hard error. The agent
// re-submits after addressing the checklist.
//
// Accepted propose calls auto-publish (review_state=auto_published,
// status=published). Review Queue (sensitive ops) lands in Phase 2.x+.
func RegisterArtifactPropose(server *sdk.Server, deps Deps) {
	sdk.AddTool(server,
		&sdk.Tool{
			Name:        "pindoc.artifact.propose",
			Description: "Propose a new artifact (the only write path humans use — always via an agent). Returns Status=accepted + artifact_id on success, or Status=not_ready + checklist + suggested_actions if Pre-flight fails. Always read the checklist; never surface the raw error to the user without trying the suggested actions first.",
		},
		func(ctx context.Context, _ *sdk.CallToolRequest, in artifactProposeInput) (*sdk.CallToolResult, artifactProposeOutput, error) {
			// --- Pre-flight ----------------------------------------------
			lang := deps.UserLanguage
			checklist, failed, code := preflight(&in, lang)
			if len(checklist) > 0 {
				return nil, artifactProposeOutput{
					Status:    "not_ready",
					ErrorCode: code,
					Failed:    failed,
					Checklist: checklist,
					SuggestedActions: []string{
						i18n.T(lang, "suggested.fix_all"),
						i18n.T(lang, "suggested.confirm_types"),
						i18n.T(lang, "suggested.use_misc"),
					},
				}, nil
			}

			// Mutual exclusion: update_of and supersede_of are two different
			// revisions-of-truth paths. Agent must pick one.
			if strings.TrimSpace(in.UpdateOf) != "" && strings.TrimSpace(in.SupersedeOf) != "" {
				return nil, artifactProposeOutput{
					Status:           "not_ready",
					ErrorCode:        "UPDATE_SUPERSEDE_EXCLUSIVE",
					Failed:           []string{"UPDATE_SUPERSEDE_EXCLUSIVE"},
					Checklist:        []string{i18n.T(lang, "preflight.update_supersede_exclusive")},
					SuggestedActions: []string{i18n.T(lang, "suggested.pick_one_mode")},
				}, nil
			}

			// --- Update path (update_of set) -----------------------------
			if strings.TrimSpace(in.UpdateOf) != "" {
				return handleUpdate(ctx, deps, in, lang)
			}

			// --- Supersede path (supersede_of set) -----------------------
			// Creates a fresh artifact via the same insert flow as "new",
			// then flips the target artifact's status to 'superseded' and
			// writes superseded_by. We reuse the create path below and do
			// the supersede bookkeeping just before commit.

			// --- Resolve area + project ----------------------------------
			var projectID, areaID string
			err := deps.DB.QueryRow(ctx, `
				SELECT proj.id::text, area.id::text
				FROM projects proj
				JOIN areas area ON area.project_id = proj.id
				WHERE proj.slug = $1 AND area.slug = $2
			`, deps.ProjectSlug, in.AreaSlug).Scan(&projectID, &areaID)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, artifactProposeOutput{
					Status:    "not_ready",
					ErrorCode: "AREA_NOT_FOUND",
					Checklist: []string{
						fmt.Sprintf(i18n.T(lang, "preflight.area_not_found"), in.AreaSlug, deps.ProjectSlug),
					},
					SuggestedActions: []string{
						i18n.T(lang, "suggested.list_areas"),
						i18n.T(lang, "suggested.area_or_misc"),
					},
				}, nil
			}
			if err != nil {
				return nil, artifactProposeOutput{}, fmt.Errorf("resolve scope: %w", err)
			}

			// --- Resolve supersede_of target first (if set) --------------
			// The supersede path creates a new artifact and flips the old
			// one's status to 'superseded'. We resolve the target upfront
			// so we can skip the exact-title conflict check when the
			// replacement keeps the same title (common pattern).
			var supersedeTargetID string
			if strings.TrimSpace(in.SupersedeOf) != "" {
				ref := normalizeRef(in.SupersedeOf)
				err := deps.DB.QueryRow(ctx, `
					SELECT a.id::text FROM artifacts a
					JOIN projects p ON p.id = a.project_id
					WHERE p.slug = $1 AND (a.id::text = $2 OR a.slug = $2)
					  AND a.status <> 'archived'
					LIMIT 1
				`, deps.ProjectSlug, ref).Scan(&supersedeTargetID)
				if errors.Is(err, pgx.ErrNoRows) {
					return nil, artifactProposeOutput{
						Status:    "not_ready",
						ErrorCode: "SUPERSEDE_TARGET_NOT_FOUND",
						Failed:    []string{"SUPERSEDE_TARGET_NOT_FOUND"},
						Checklist: []string{
							fmt.Sprintf(i18n.T(lang, "preflight.supersede_target_missing"), in.SupersedeOf),
						},
					}, nil
				}
				if err != nil {
					return nil, artifactProposeOutput{}, fmt.Errorf("resolve supersede target: %w", err)
				}
			}

			// --- Exact-title conflict check (embedding-based lands Phase 3) ---
			// Skipped when superseding: the replacement commonly keeps the
			// same title; the old one is about to be archived anyway, so it
			// would no longer match status<>'archived' either.
			if supersedeTargetID == "" {
				var existingID, existingSlug string
				err = deps.DB.QueryRow(ctx, `
					SELECT id::text, slug FROM artifacts
					WHERE project_id = $1
					  AND lower(title) = lower($2)
					  AND status <> 'archived'
					LIMIT 1
				`, projectID, in.Title).Scan(&existingID, &existingSlug)
				if err == nil {
					return nil, artifactProposeOutput{
						Status:    "not_ready",
						ErrorCode: "CONFLICT_EXACT_TITLE",
						Failed:    []string{"CONFLICT_EXACT_TITLE"},
						Checklist: []string{
							fmt.Sprintf(i18n.T(lang, "preflight.conflict_exact"), existingID, existingSlug),
						},
						SuggestedActions: []string{
							fmt.Sprintf(i18n.T(lang, "suggested.update_of_hint"), existingSlug),
							fmt.Sprintf(i18n.T(lang, "suggested.read_existing"), existingSlug),
							i18n.T(lang, "suggested.pick_title"),
						},
					}, nil
				}
				if !errors.Is(err, pgx.ErrNoRows) {
					return nil, artifactProposeOutput{}, fmt.Errorf("conflict check: %w", err)
				}
			}

			// --- Slug: either the explicit one or a generated one. Retry on
			//     unique-constraint violation with a -N suffix until we settle.
			baseSlug := in.Slug
			if baseSlug == "" {
				baseSlug = slugify(in.Title)
			}
			if baseSlug == "" {
				baseSlug = strings.ToLower(in.Type) + "-" + time.Now().UTC().Format("20060102150405")
			}

			completeness := in.Completeness
			if completeness == "" {
				completeness = "partial"
			}
			if in.Tags == nil {
				in.Tags = []string{}
			}

			// --- INSERT + event in one tx --------------------------------
			tx, err := deps.DB.Begin(ctx)
			if err != nil {
				return nil, artifactProposeOutput{}, fmt.Errorf("begin tx: %w", err)
			}
			defer func() { _ = tx.Rollback(ctx) }()

			finalSlug := baseSlug
			var newID string
			var publishedAt time.Time
			for attempt := 0; attempt < 10; attempt++ {
				err = tx.QueryRow(ctx, `
					INSERT INTO artifacts (
						project_id, area_id, slug, type, title, body_markdown, tags,
						completeness, status, review_state,
						author_kind, author_id, author_version,
						published_at
					) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'published', 'auto_published', 'agent', $9, $10, now())
					RETURNING id::text, published_at
				`, projectID, areaID, finalSlug, in.Type, in.Title, in.BodyMarkdown, in.Tags,
					completeness, in.AuthorID, nullIfEmpty(in.AuthorVersion)).Scan(&newID, &publishedAt)
				if err == nil {
					break
				}
				if isUniqueViolation(err, "artifacts_project_id_slug_key") {
					finalSlug = fmt.Sprintf("%s-%d", baseSlug, attempt+2)
					continue
				}
				return nil, artifactProposeOutput{}, fmt.Errorf("insert: %w", err)
			}
			if newID == "" {
				return nil, artifactProposeOutput{}, errors.New("could not allocate a unique slug after 10 attempts")
			}

			if _, err := tx.Exec(ctx, `
				INSERT INTO events (project_id, kind, subject_id, payload)
				VALUES ($1, 'artifact.published', $2, jsonb_build_object(
					'area_slug', $3::text,
					'type', $4::text,
					'slug', $5::text,
					'author_id', $6::text
				))
			`, projectID, newID, in.AreaSlug, in.Type, finalSlug, in.AuthorID); err != nil {
				return nil, artifactProposeOutput{}, fmt.Errorf("event insert: %w", err)
			}

			// Embed title + body chunks in the same transaction so search
			// never observes a half-indexed artifact. If the embedder fails
			// we still keep the artifact — search becomes keyword-only for
			// that row until re-embedding lands in Phase 3.x.
			if deps.Embedder != nil {
				if err := embedAndStoreChunks(ctx, tx, deps.Embedder, newID, in.Title, in.BodyMarkdown); err != nil {
					deps.Logger.Warn("chunk/embed failed — artifact saved without vectors",
						"artifact_id", newID, "err", err)
				}
			}

			// --- relates_to: resolve targets, then insert edges ----------
			relTargets, relErr := resolveRelatesTo(ctx, tx, deps.ProjectSlug, in.RelatesTo, lang)
			if relErr != nil {
				return nil, *relErr, nil
			}
			edgesStored, err := insertEdges(ctx, tx, newID, relTargets, in.RelatesTo)
			if err != nil {
				return nil, artifactProposeOutput{}, err
			}

			// --- pins ---------------------------------------------------
			pinsStored, err := insertPins(ctx, tx, newID, in.Pins)
			if err != nil {
				return nil, artifactProposeOutput{}, err
			}

			// --- supersede bookkeeping: archive target + record edge ----
			supersededFlag := false
			if supersedeTargetID != "" {
				if _, err := tx.Exec(ctx, `
					UPDATE artifacts
					   SET status         = 'superseded',
					       superseded_by  = $2::uuid,
					       updated_at     = now()
					 WHERE id = $1
				`, supersedeTargetID, newID); err != nil {
					return nil, artifactProposeOutput{}, fmt.Errorf("supersede update: %w", err)
				}
				if _, err := tx.Exec(ctx, `
					INSERT INTO events (project_id, kind, subject_id, payload)
					VALUES ($1, 'artifact.superseded', $2::uuid, jsonb_build_object(
						'superseded_by', $3::text,
						'author_id', $4::text
					))
				`, projectID, supersedeTargetID, newID, in.AuthorID); err != nil {
					return nil, artifactProposeOutput{}, fmt.Errorf("supersede event: %w", err)
				}
				supersededFlag = true
			}

			if err := tx.Commit(ctx); err != nil {
				return nil, artifactProposeOutput{}, fmt.Errorf("commit: %w", err)
			}

			// First revision — keep the invariant that every artifact has
			// at least one artifact_revisions row. Done outside the tx
			// (commit just happened) which is safe because a missing
			// revision row still has the artifact intact; a background
			// backfill (future) can repair it.
			if _, err := deps.DB.Exec(ctx, `
				INSERT INTO artifact_revisions (
					artifact_id, revision_number, title, body_markdown, body_hash, tags,
					completeness, author_kind, author_id, author_version, commit_msg
				) VALUES ($1, 1, $2, $3, $4, $5, $6, 'agent', $7, $8, 'initial')
			`, newID, in.Title, in.BodyMarkdown, bodyHash(in.BodyMarkdown), in.Tags,
				completeness, in.AuthorID, nullIfEmpty(in.AuthorVersion)); err != nil {
				deps.Logger.Warn("initial revision insert failed — head row still present",
					"artifact_id", newID, "err", err)
			}

			return nil, artifactProposeOutput{
				Status:         "accepted",
				ArtifactID:     newID,
				Slug:           finalSlug,
				AgentRef:       "pindoc://" + finalSlug,
				HumanURL:       HumanURL(deps.ProjectSlug, finalSlug),
				PublishedAt:    publishedAt,
				Created:        true,
				RevisionNumber: 1,
				PinsStored:     pinsStored,
				EdgesStored:    edgesStored,
				Superseded:     supersededFlag,
			}, nil
		},
	)
}

// handleUpdate writes a new revision for an existing artifact, updates the
// head row, re-chunks embeddings, and emits an event. Runs in a single
// transaction so search never sees a half-indexed update.
func handleUpdate(ctx context.Context, deps Deps, in artifactProposeInput, lang string) (*sdk.CallToolResult, artifactProposeOutput, error) {
	if strings.TrimSpace(in.CommitMsg) == "" {
		return nil, artifactProposeOutput{
			Status:    "not_ready",
			ErrorCode: "MISSING_COMMIT_MSG",
			Checklist: []string{i18n.T(lang, "preflight.update_needs_commit")},
			SuggestedActions: []string{
				i18n.T(lang, "suggested.commit_msg_hint"),
			},
		}, nil
	}

	ref := normalizeRef(in.UpdateOf)

	var artifactID, projectID, currentBody, currentTitle string
	var lastRev int
	err := deps.DB.QueryRow(ctx, `
		SELECT a.id::text, a.project_id::text, a.body_markdown, a.title,
		       COALESCE((SELECT max(revision_number) FROM artifact_revisions WHERE artifact_id = a.id), 0)
		FROM artifacts a
		JOIN projects p ON p.id = a.project_id
		WHERE p.slug = $1 AND (a.id::text = $2 OR a.slug = $2)
		LIMIT 1
	`, deps.ProjectSlug, ref).Scan(&artifactID, &projectID, &currentBody, &currentTitle, &lastRev)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, artifactProposeOutput{
			Status:    "not_ready",
			ErrorCode: "UPDATE_TARGET_NOT_FOUND",
			Failed:    []string{"UPDATE_TARGET_NOT_FOUND"},
			Checklist: []string{
				fmt.Sprintf(i18n.T(lang, "preflight.update_target_missing"), in.UpdateOf),
			},
			SuggestedActions: []string{
				i18n.T(lang, "suggested.list_areas"),
			},
		}, nil
	}
	if err != nil {
		return nil, artifactProposeOutput{}, fmt.Errorf("resolve update target: %w", err)
	}

	// Optimistic lock: if the agent asserted a version, fail fast when
	// another writer has already advanced the head. Unset = trust whatever
	// head is (legacy behaviour).
	if in.ExpectedVersion > 0 && in.ExpectedVersion != lastRev {
		return nil, artifactProposeOutput{
			Status:    "not_ready",
			ErrorCode: "VER_CONFLICT",
			Failed:    []string{"VER_CONFLICT"},
			Checklist: []string{
				fmt.Sprintf(i18n.T(lang, "preflight.ver_conflict"), in.ExpectedVersion, lastRev),
			},
			SuggestedActions: []string{
				i18n.T(lang, "suggested.reread_before_update"),
			},
		}, nil
	}

	// No-op detection: identical body + title → reject so history stays
	// clean. Agents that hit this should stop retrying.
	if currentBody == in.BodyMarkdown && currentTitle == in.Title {
		return nil, artifactProposeOutput{
			Status:    "not_ready",
			ErrorCode: "NO_CHANGES",
			Checklist: []string{i18n.T(lang, "preflight.no_changes")},
			SuggestedActions: []string{
				i18n.T(lang, "suggested.verify_diff"),
			},
		}, nil
	}

	// Area is preserved on update — moving across areas is a supersede,
	// not a revision. Treat area_slug as a reconfirm-of-current.
	var areaID string
	if err := deps.DB.QueryRow(ctx, `
		SELECT area.id::text FROM areas area
		JOIN projects p ON p.id = area.project_id
		WHERE p.slug = $1 AND area.slug = $2
	`, deps.ProjectSlug, in.AreaSlug).Scan(&areaID); err != nil {
		return nil, artifactProposeOutput{
			Status:    "not_ready",
			ErrorCode: "AREA_NOT_FOUND",
			Checklist: []string{
				fmt.Sprintf(i18n.T(lang, "preflight.area_not_found"), in.AreaSlug, deps.ProjectSlug),
			},
		}, nil
	}

	if in.Tags == nil {
		in.Tags = []string{}
	}
	completeness := in.Completeness
	if completeness == "" {
		completeness = "partial"
	}

	tx, err := deps.DB.Begin(ctx)
	if err != nil {
		return nil, artifactProposeOutput{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	newRev := lastRev + 1
	var revID string
	err = tx.QueryRow(ctx, `
		INSERT INTO artifact_revisions (
			artifact_id, revision_number, title, body_markdown, body_hash, tags,
			completeness, author_kind, author_id, author_version, commit_msg
		) VALUES ($1, $2, $3, $4, $5, $6, $7, 'agent', $8, $9, $10)
		RETURNING id::text
	`, artifactID, newRev, in.Title, in.BodyMarkdown, bodyHash(in.BodyMarkdown),
		in.Tags, completeness, in.AuthorID, nullIfEmpty(in.AuthorVersion), in.CommitMsg,
	).Scan(&revID)
	if err != nil {
		return nil, artifactProposeOutput{}, fmt.Errorf("insert revision: %w", err)
	}

	var publishedAt time.Time
	var slug string
	err = tx.QueryRow(ctx, `
		UPDATE artifacts
		   SET title          = $2,
		       body_markdown  = $3,
		       tags           = $4,
		       completeness   = $5,
		       author_id      = $6,
		       author_version = $7,
		       updated_at     = now()
		 WHERE id = $1
		RETURNING slug, COALESCE(published_at, now())
	`, artifactID, in.Title, in.BodyMarkdown, in.Tags, completeness,
		in.AuthorID, nullIfEmpty(in.AuthorVersion),
	).Scan(&slug, &publishedAt)
	if err != nil {
		return nil, artifactProposeOutput{}, fmt.Errorf("update head: %w", err)
	}

	// Re-chunk: drop old chunks, generate new.
	if _, err := tx.Exec(ctx, `DELETE FROM artifact_chunks WHERE artifact_id = $1`, artifactID); err != nil {
		return nil, artifactProposeOutput{}, fmt.Errorf("purge chunks: %w", err)
	}
	if deps.Embedder != nil {
		if err := embedAndStoreChunks(ctx, tx, deps.Embedder, artifactID, in.Title, in.BodyMarkdown); err != nil {
			deps.Logger.Warn("re-embed failed — artifact updated without vectors",
				"artifact_id", artifactID, "err", err)
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO events (project_id, kind, subject_id, payload)
		VALUES ($1, 'artifact.revised', $2, jsonb_build_object(
			'revision_number', $3::int,
			'slug',            $4::text,
			'author_id',       $5::text,
			'commit_msg',      $6::text
		))
	`, projectID, artifactID, newRev, slug, in.AuthorID, in.CommitMsg); err != nil {
		return nil, artifactProposeOutput{}, fmt.Errorf("event: %w", err)
	}

	// pins and edges are additive on update (append new pins; edges are
	// idempotent on (source, target, relation)). If an agent needs to
	// "replace" all pins they should supersede rather than update.
	relTargets, relErr := resolveRelatesTo(ctx, tx, deps.ProjectSlug, in.RelatesTo, lang)
	if relErr != nil {
		return nil, *relErr, nil
	}
	edgesStored, err := insertEdges(ctx, tx, artifactID, relTargets, in.RelatesTo)
	if err != nil {
		return nil, artifactProposeOutput{}, err
	}
	pinsStored, err := insertPins(ctx, tx, artifactID, in.Pins)
	if err != nil {
		return nil, artifactProposeOutput{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, artifactProposeOutput{}, fmt.Errorf("commit: %w", err)
	}

	return nil, artifactProposeOutput{
		Status:         "accepted",
		ArtifactID:     artifactID,
		Slug:           slug,
		AgentRef:       "pindoc://" + slug,
		HumanURL:       HumanURL(deps.ProjectSlug, slug),
		PublishedAt:    publishedAt,
		Created:        false,
		RevisionNumber: newRev,
		PinsStored:     pinsStored,
		EdgesStored:    edgesStored,
	}, nil
}

func bodyHash(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

// preflight runs the cheap synchronous checks. Returns a list of ✗-prefixed
// lines (legacy natural-language) + parallel stable-code list + short
// ErrorCode for the first failure. Empty lists mean clean.
//
// Phase 11a change: every check now emits both a natural-language line
// (legacy) AND a stable code (Phase 12-style). Phase 12 will make codes
// primary and prose optional.
func preflight(in *artifactProposeInput, lang string) (checklist []string, failed []string, code string) {
	push := func(line, failCode string) {
		checklist = append(checklist, line)
		failed = append(failed, failCode)
		if code == "" {
			code = failCode
		}
	}

	if _, ok := validArtifactTypes[in.Type]; !ok {
		push(fmt.Sprintf(i18n.T(lang, "preflight.type_invalid"), in.Type), "TYPE_INVALID")
	}
	if strings.TrimSpace(in.Title) == "" {
		push(i18n.T(lang, "preflight.title_empty"), "TITLE_EMPTY")
	}
	if strings.TrimSpace(in.BodyMarkdown) == "" {
		push(i18n.T(lang, "preflight.body_empty"), "BODY_EMPTY")
	}
	if strings.TrimSpace(in.AreaSlug) == "" {
		push(i18n.T(lang, "preflight.area_empty"), "AREA_EMPTY")
	}
	if strings.TrimSpace(in.AuthorID) == "" {
		push(i18n.T(lang, "preflight.author_empty"), "AUTHOR_EMPTY")
	}
	if in.Completeness != "" {
		switch in.Completeness {
		case "draft", "partial", "settled":
		default:
			push(fmt.Sprintf(i18n.T(lang, "preflight.completeness_invalid"), in.Completeness), "COMPLETENESS_INVALID")
		}
	}

	// Type-specific guardrails.
	switch in.Type {
	case "Task":
		if !strings.Contains(strings.ToLower(in.BodyMarkdown), "acceptance") {
			push(i18n.T(lang, "preflight.task_acceptance"), "TASK_NO_ACCEPTANCE")
		}
	case "Decision":
		lower := strings.ToLower(in.BodyMarkdown)
		if !strings.Contains(lower, "decision") || !strings.Contains(lower, "context") {
			push(i18n.T(lang, "preflight.adr_sections"), "DEC_NO_SECTIONS")
		}
	}

	// Phase 11a: shape-check pins + relates_to. Hard-blocks only on
	// structurally invalid input (empty path, unknown relation). Missing
	// entirely = soft (Phase 11b will escalate NEED_PIN for code-linked
	// types once search_receipt is in place).
	for i, p := range in.Pins {
		if strings.TrimSpace(p.Path) == "" {
			push(fmt.Sprintf(i18n.T(lang, "preflight.pin_path_empty"), i), "PIN_PATH_EMPTY")
		}
		if p.LinesStart < 0 || p.LinesEnd < 0 {
			push(fmt.Sprintf(i18n.T(lang, "preflight.pin_lines_invalid"), i), "PIN_LINES_INVALID")
		}
		if p.LinesStart > 0 && p.LinesEnd > 0 && p.LinesEnd < p.LinesStart {
			push(fmt.Sprintf(i18n.T(lang, "preflight.pin_lines_range"), i), "PIN_LINES_INVALID")
		}
	}
	for i, r := range in.RelatesTo {
		if strings.TrimSpace(r.TargetID) == "" {
			push(fmt.Sprintf(i18n.T(lang, "preflight.rel_target_empty"), i), "REL_TARGET_EMPTY")
		}
		if _, ok := validRelations[strings.TrimSpace(r.Relation)]; !ok {
			push(fmt.Sprintf(i18n.T(lang, "preflight.rel_invalid"), i, r.Relation), "REL_INVALID")
		}
	}
	if in.ExpectedVersion < 0 {
		push(i18n.T(lang, "preflight.expected_version_negative"), "VER_INVALID")
	}

	return checklist, failed, code
}

var slugRegex = regexp.MustCompile(`[^a-z0-9]+`)

// slugify lowercases, replaces runs of non-alnum with '-', trims dashes,
// and caps at 60 chars. Keeps ASCII only — Korean characters drop out,
// which is fine because slug is a URL/path component and the real human
// label lives in title. If the title has no ASCII letters the caller
// falls back to a type+timestamp slug.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugRegex.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 60 {
		s = strings.Trim(s[:60], "-")
	}
	return s
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// embedAndStoreChunks computes vectors for the title and each body chunk
// and inserts them into artifact_chunks inside the caller's transaction.
// All vectors pad to the DB column width (768) — see embed/vector.go for
// the rationale.
func embedAndStoreChunks(ctx context.Context, tx pgx.Tx, provider embed.Provider, artifactID, title, body string) error {
	info := provider.Info()

	// Title vector (always one, kind='title').
	titleRes, err := provider.Embed(ctx, embed.Request{Texts: []string{title}, Kind: embed.KindDocument})
	if err != nil {
		return fmt.Errorf("embed title: %w", err)
	}
	if len(titleRes.Vectors) != 1 {
		return fmt.Errorf("embed title: got %d vectors", len(titleRes.Vectors))
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO artifact_chunks (
			artifact_id, kind, chunk_index, heading, span_start, span_end,
			text, embedding, model_name, model_dim
		) VALUES ($1, 'title', 0, NULL, 0, 0, $2, $3::vector, $4, $5)
	`,
		artifactID,
		title,
		embed.VectorString(embed.PadTo768(titleRes.Vectors[0])),
		info.Name+":"+info.ModelID,
		info.Dimension,
	); err != nil {
		return fmt.Errorf("store title chunk: %w", err)
	}

	// Body chunks (kind='body').
	chunks := embed.ChunkBody(title, body, 600)
	if len(chunks) == 0 {
		return nil
	}
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Text
	}
	bodyRes, err := provider.Embed(ctx, embed.Request{Texts: texts, Kind: embed.KindDocument})
	if err != nil {
		return fmt.Errorf("embed body: %w", err)
	}
	if len(bodyRes.Vectors) != len(chunks) {
		return fmt.Errorf("embed body: got %d vectors want %d", len(bodyRes.Vectors), len(chunks))
	}
	for i, c := range chunks {
		if _, err := tx.Exec(ctx, `
			INSERT INTO artifact_chunks (
				artifact_id, kind, chunk_index, heading, span_start, span_end,
				text, embedding, model_name, model_dim
			) VALUES ($1, 'body', $2, $3, $4, $5, $6, $7::vector, $8, $9)
		`,
			artifactID,
			c.Index,
			nullIfEmpty(c.Heading),
			c.SpanStart, c.SpanEnd,
			c.Text,
			embed.VectorString(embed.PadTo768(bodyRes.Vectors[i])),
			info.Name+":"+info.ModelID,
			info.Dimension,
		); err != nil {
			return fmt.Errorf("store body chunk %d: %w", c.Index, err)
		}
	}
	return nil
}

// isUniqueViolation is a best-effort check against pgx's error message.
// The typed error route requires importing pgconn; until we add that we
// string-match on the known constraint name. Good enough for one retry
// loop.
func isUniqueViolation(err error, constraint string) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "23505") &&
		(constraint == "" || strings.Contains(err.Error(), constraint))
}

// resolveRelatesTo looks up each relates_to target (by id, slug, or
// pindoc:// URL) inside the given project and returns the resolved UUIDs
// aligned with the input order. Returns (nil, notReadyOutput, true) when
// a target can't be found — the caller surfaces it as REL_TARGET_NOT_FOUND.
func resolveRelatesTo(ctx context.Context, tx pgx.Tx, projectSlug string, relates []ArtifactRelationInput, lang string) ([]string, *artifactProposeOutput) {
	if len(relates) == 0 {
		return nil, nil
	}
	resolved := make([]string, len(relates))
	for i, r := range relates {
		ref := normalizeRef(r.TargetID)
		var id string
		err := tx.QueryRow(ctx, `
			SELECT a.id::text FROM artifacts a
			JOIN projects p ON p.id = a.project_id
			WHERE p.slug = $1 AND (a.id::text = $2 OR a.slug = $2)
			LIMIT 1
		`, projectSlug, ref).Scan(&id)
		if err != nil {
			return nil, &artifactProposeOutput{
				Status:    "not_ready",
				ErrorCode: "REL_TARGET_NOT_FOUND",
				Failed:    []string{"REL_TARGET_NOT_FOUND"},
				Checklist: []string{
					fmt.Sprintf(i18n.T(lang, "preflight.rel_target_missing"), i, r.TargetID),
				},
				SuggestedActions: []string{
					i18n.T(lang, "suggested.read_existing_rel"),
				},
			}
		}
		resolved[i] = id
	}
	return resolved, nil
}

// insertPins writes each validated pin to artifact_pins. Returns how many
// rows landed (caller echoes this in the output).
func insertPins(ctx context.Context, tx pgx.Tx, artifactID string, pins []ArtifactPinInput) (int, error) {
	n := 0
	for _, p := range pins {
		repo := strings.TrimSpace(p.Repo)
		if repo == "" {
			repo = "origin"
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO artifact_pins (artifact_id, repo, commit_sha, path, lines_start, lines_end)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, artifactID, repo, nullIfEmpty(p.CommitSHA), p.Path,
			nullIfZero(p.LinesStart), nullIfZero(p.LinesEnd),
		); err != nil {
			return n, fmt.Errorf("pin insert: %w", err)
		}
		n++
	}
	return n, nil
}

// insertEdges writes artifact_edges rows for each resolved (target, relation)
// pair. Idempotent on (source, target, relation) via UNIQUE index; duplicates
// are silently treated as success so re-propose with the same relates_to
// list doesn't blow up.
func insertEdges(ctx context.Context, tx pgx.Tx, sourceID string, targetIDs []string, relates []ArtifactRelationInput) (int, error) {
	n := 0
	for i, tgt := range targetIDs {
		if tgt == sourceID {
			// DB check also catches this, but skip silently to give a
			// cleaner error path.
			continue
		}
		rel := strings.TrimSpace(relates[i].Relation)
		tag, err := tx.Exec(ctx, `
			INSERT INTO artifact_edges (source_id, target_id, relation)
			VALUES ($1, $2, $3)
			ON CONFLICT (source_id, target_id, relation) DO NOTHING
		`, sourceID, tgt, rel)
		if err != nil {
			return n, fmt.Errorf("edge insert: %w", err)
		}
		n += int(tag.RowsAffected())
	}
	return n, nil
}

func nullIfZero(n int) any {
	if n == 0 {
		return nil
	}
	return n
}

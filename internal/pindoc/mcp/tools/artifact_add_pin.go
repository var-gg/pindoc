package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
	pgit "github.com/var-gg/pindoc/internal/pindoc/git"
	pinmodel "github.com/var-gg/pindoc/internal/pindoc/pins"
)

type artifactAddPinInput struct {
	ProjectSlug     string           `json:"project_slug" jsonschema:"projects.slug to scope this call to"`
	SlugOrID        string           `json:"slug_or_id" jsonschema:"target artifact UUID, slug, or pindoc:// URL"`
	Pin             ArtifactPinInput `json:"pin" jsonschema:"required; same pin item shape as artifact.propose pins[]"`
	Reason          string           `json:"reason" jsonschema:"required, 2-200 runes; stored as commit_msg with add_pin prefix"`
	ExpectedVersion *int             `json:"expected_version" jsonschema:"required optimistic lock; current artifact revision number"`
	AuthorID        string           `json:"author_id,omitempty" jsonschema:"override author display label; defaults to server agent_id"`
	AuthorVersion   string           `json:"author_version,omitempty" jsonschema:"e.g. 'gpt-5'"`
}

type addPinTarget struct {
	ArtifactID          string
	ProjectID           string
	BodyMarkdown        string
	BodyHash            string
	Title               string
	Type                string
	Slug                string
	Tags                []string
	Completeness        string
	LastRevisionNumber  int
	PublishedAtFallback string
}

// RegisterArtifactAddPin wires pindoc.artifact.add_pin. It is the narrow
// post-hoc pin lane: no body_markdown, no search_receipt, no semantic body
// rewrite. The revision records a meta_patch-shaped audit entry whose
// body_hash equals the current artifact body.
func RegisterArtifactAddPin(server *sdk.Server, deps Deps) {
	AddInstrumentedTool(server, deps,
		&sdk.Tool{
			Name: "pindoc.artifact.add_pin",
			Description: strings.TrimSpace(`
Add one pin to an existing artifact without resending or changing body_markdown. Requires slug_or_id, pin, reason, and expected_version; bypasses search_receipt because targeting an existing artifact is the context proof. Example: pindoc.artifact.add_pin({project_slug:"pindoc", slug_or_id:"task-slug", expected_version:3, reason:"implementation commit", pin:{commit_sha:"abc1234", path:"internal/pindoc/mcp/tools/artifact_add_pin.go"}}). Omitted pin.kind is inferred from path, omitted pin.repo_id is auto-mapped through project_repos. Duplicate coordinates (repo_id, commit_sha, path, lines_start, lines_end) are rejected with PIN_DUPLICATE. This tool only adds one pin; pin removal, line-range edits, and bulk pinning remain outside this lane.
`),
		},
		func(ctx context.Context, p *auth.Principal, in artifactAddPinInput) (*sdk.CallToolResult, artifactProposeOutput, error) {
			scope, err := auth.ResolveProject(ctx, deps.DB, p, in.ProjectSlug)
			if err != nil {
				return nil, artifactProposeOutput{}, fmt.Errorf("artifact.add_pin: %w", err)
			}
			out, err := addPinToArtifact(ctx, deps, p, scope, in)
			if err != nil {
				return nil, artifactProposeOutput{}, err
			}
			return nil, out, nil
		},
	)
}

func addPinToArtifact(ctx context.Context, deps Deps, p *auth.Principal, scope *auth.ProjectScope, in artifactAddPinInput) (artifactProposeOutput, error) {
	if strings.TrimSpace(in.SlugOrID) == "" {
		return addPinNotReady("ADD_PIN_TARGET_REQUIRED", "slug_or_id is required", "slug_or_id"), nil
	}
	if in.ExpectedVersion == nil {
		return addPinNotReady("NEED_VER", "expected_version is required", "expected_version"), nil
	}
	reason := strings.TrimSpace(in.Reason)
	reasonLen := utf8.RuneCountInString(reason)
	if reasonLen < reasonMinLen || reasonLen > reasonMaxLen {
		return addPinNotReady("ADD_PIN_REASON_INVALID", fmt.Sprintf("reason must be %d-%d runes (got %d)", reasonMinLen, reasonMaxLen, reasonLen), "reason"), nil
	}
	pin, code, msg := normalizeAddPinInput(in.Pin)
	if code != "" {
		return addPinNotReady(code, msg, "pin"), nil
	}

	target, err := loadAddPinTarget(ctx, deps, scope.ProjectSlug, in.SlugOrID)
	if errors.Is(err, pgx.ErrNoRows) {
		return addPinNotReady("UPDATE_TARGET_NOT_FOUND", fmt.Sprintf("artifact %q not found", in.SlugOrID), "slug_or_id"), nil
	}
	if err != nil {
		return artifactProposeOutput{}, fmt.Errorf("resolve add_pin target: %w", err)
	}
	if *in.ExpectedVersion != target.LastRevisionNumber {
		return artifactProposeOutput{
			Status:          "not_ready",
			ErrorCode:       "VER_CONFLICT",
			Failed:          []string{"VER_CONFLICT"},
			ErrorCodes:      []string{"VER_CONFLICT"},
			Checklist:       []string{fmt.Sprintf("expected_version=%d is stale; current head=%d.", *in.ExpectedVersion, target.LastRevisionNumber)},
			PatchableFields: []string{"expected_version"},
			NextTools:       defaultNextTools("VER_CONFLICT"),
			Related: []RelatedRef{
				makeRelated(deps, scope, normalizeRef(in.SlugOrID), target.ArtifactID, "", target.Title, fmt.Sprintf("current revision = %d, not %d", target.LastRevisionNumber, *in.ExpectedVersion)),
			},
		}, nil
	}

	authorID := strings.TrimSpace(in.AuthorID)
	if authorID == "" {
		authorID = stripAgentPrefix(taskAttentionCallerID(p))
	}
	if authorID == "" {
		authorID = "codex"
	}
	commitMsg := "add_pin: " + reason

	tx, err := deps.DB.Begin(ctx)
	if err != nil {
		return artifactProposeOutput{}, fmt.Errorf("begin add_pin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	resolvedRepoID, _, err := pgit.ResolvePinRepoID(ctx, tx, target.ProjectID, pin.RepoID, pin.Repo, pin.Path, deps.RepoRoot)
	if err != nil {
		return artifactProposeOutput{}, fmt.Errorf("resolve add_pin repo: %w", err)
	}
	if resolvedRepoID != "" {
		pin.RepoID = resolvedRepoID
	}
	dup, err := pinDuplicateExists(ctx, tx, target.ArtifactID, resolvedRepoID, pin)
	if err != nil {
		return artifactProposeOutput{}, fmt.Errorf("check duplicate pin: %w", err)
	}
	if dup {
		return addPinNotReady("PIN_DUPLICATE", "a pin with the same repo_id, commit_sha, path, lines_start, and lines_end already exists", "pin"), nil
	}

	newRev := target.LastRevisionNumber + 1
	shapePayloadJSON, err := json.Marshal(map[string]any{
		"fields": []string{"pin"},
		"pin":    pin,
	})
	if err != nil {
		return artifactProposeOutput{}, fmt.Errorf("marshal add_pin shape payload: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO artifact_revisions (
			artifact_id, revision_number, title, body_markdown, body_hash,
			tags, completeness, author_kind, author_id, author_version,
			commit_msg, source_session_ref, revision_shape, shape_payload
		) VALUES ($1, $2, $3, NULL, $4, $5, $6, 'agent', $7, $8, $9, $10, 'meta_patch', $11::jsonb)
	`, target.ArtifactID, newRev, target.Title, target.BodyHash, target.Tags, target.Completeness,
		authorID, nullIfEmpty(in.AuthorVersion), commitMsg,
		buildSourceSessionRef(p, artifactProposeInput{AuthorID: authorID, CommitMsg: commitMsg, AddPin: true}),
		string(shapePayloadJSON),
	); err != nil {
		return artifactProposeOutput{}, fmt.Errorf("insert add_pin revision: %w", err)
	}

	var publishedAt any
	if err := tx.QueryRow(ctx, `
		UPDATE artifacts
		   SET author_id = $2,
		       author_version = $3,
		       updated_at = now()
		 WHERE id = $1
		RETURNING COALESCE(published_at, now())
	`, target.ArtifactID, authorID, nullIfEmpty(in.AuthorVersion)).Scan(&publishedAt); err != nil {
		return artifactProposeOutput{}, fmt.Errorf("update artifact add_pin head: %w", err)
	}

	pinsStored, repoWarnings, err := insertPins(ctx, tx, target.ProjectID, target.ArtifactID, []ArtifactPinInput{pin}, deps.RepoRoot)
	if err != nil {
		return artifactProposeOutput{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO events (project_id, kind, subject_id, payload)
		VALUES ($1, 'artifact.pin_added', $2, jsonb_build_object(
			'revision_number', $3::int,
			'slug',            $4::text,
			'author_id',       $5::text,
			'commit_msg',      $6::text,
			'pin',             $7::jsonb
		))
	`, target.ProjectID, target.ArtifactID, newRev, target.Slug, authorID, commitMsg, string(shapePayloadJSON)); err != nil {
		return artifactProposeOutput{}, fmt.Errorf("event add_pin: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return artifactProposeOutput{}, fmt.Errorf("commit add_pin: %w", err)
	}

	warnings := append([]string{"ADD_PIN_APPLIED"}, repoWarnings...)
	warnings = sortWarningsBySeverity(warnings)
	severities := make([]string, len(warnings))
	for i, w := range warnings {
		severities[i] = warningSeverity(w)
	}

	out := artifactProposeOutput{
		Status:            "accepted",
		ArtifactID:        target.ArtifactID,
		Slug:              target.Slug,
		AgentRef:          "pindoc://" + target.Slug,
		HumanURL:          HumanURL(scope.ProjectSlug, scope.ProjectLocale, target.Slug),
		HumanURLAbs:       AbsHumanURL(deps.Settings, scope.ProjectSlug, scope.ProjectLocale, target.Slug),
		Created:           false,
		RevisionNumber:    newRev,
		PinsStored:        pinsStored,
		Warnings:          warnings,
		WarningSeverities: severities,
		ToolsetVersion:    ToolsetVersion(),
	}
	_ = publishedAt
	return out, nil
}

func addPinNotReady(code, msg string, fields ...string) artifactProposeOutput {
	return artifactProposeOutput{
		Status:          "not_ready",
		ErrorCode:       code,
		Failed:          []string{code},
		ErrorCodes:      []string{code},
		Checklist:       []string{msg},
		PatchableFields: fields,
	}
}

func normalizeAddPinInput(pin ArtifactPinInput) (ArtifactPinInput, string, string) {
	pin.Kind = pinmodel.NormalizeKind(pin.Kind, pin.Path)
	pin.RepoID = strings.TrimSpace(pin.RepoID)
	pin.Repo = strings.TrimSpace(pin.Repo)
	if pin.Repo == "" {
		pin.Repo = "origin"
	}
	pin.CommitSHA = strings.TrimSpace(pin.CommitSHA)
	pin.Path = strings.TrimSpace(pin.Path)
	if !pinmodel.ValidKind(pin.Kind) {
		return pin, "PIN_KIND_INVALID", fmt.Sprintf("pin.kind %q is not valid", pin.Kind)
	}
	if pin.Path == "" {
		return pin, "PIN_PATH_EMPTY", "pin.path is required"
	}
	if pin.Kind == "url" && !strings.Contains(pin.Path, "://") {
		return pin, "PIN_URL_INVALID", "kind=url requires an absolute URL path"
	}
	if pin.LinesStart < 0 || pin.LinesEnd < 0 || (pin.LinesStart > 0 && pin.LinesEnd > 0 && pin.LinesEnd < pin.LinesStart) {
		return pin, "PIN_LINES_INVALID", "pin line range is invalid"
	}
	if addPinUsesGitCoordinate(pin.Kind) && pin.CommitSHA == "" {
		return pin, "PIN_COMMIT_REQUIRED", "pin.commit_sha is required for code, doc, config, and asset pins"
	}
	return pin, "", ""
}

func addPinUsesGitCoordinate(kind string) bool {
	switch kind {
	case "code", "doc", "config", "asset":
		return true
	default:
		return false
	}
}

func loadAddPinTarget(ctx context.Context, deps Deps, projectSlug, slugOrID string) (addPinTarget, error) {
	ref := normalizeRef(slugOrID)
	var target addPinTarget
	err := deps.DB.QueryRow(ctx, `
		SELECT a.id::text, a.project_id::text, a.body_markdown, a.title, a.type, a.slug,
		       a.tags, a.completeness,
		       COALESCE((SELECT max(revision_number) FROM artifact_revisions WHERE artifact_id = a.id), 0)
		  FROM artifacts a
		  JOIN projects p ON p.id = a.project_id
		 WHERE p.slug = $1
		   AND (a.id::text = $2 OR a.slug = $2)
		   AND a.status <> 'archived'
		 LIMIT 1
	`, projectSlug, ref).Scan(
		&target.ArtifactID, &target.ProjectID, &target.BodyMarkdown, &target.Title, &target.Type, &target.Slug,
		&target.Tags, &target.Completeness, &target.LastRevisionNumber,
	)
	target.BodyHash = bodyHash(target.BodyMarkdown)
	return target, err
}

func pinDuplicateExists(ctx context.Context, q pgx.Tx, artifactID, repoID string, pin ArtifactPinInput) (bool, error) {
	kind := pinmodel.NormalizeKind(pin.Kind, pin.Path)
	var repoIDArg any
	if strings.TrimSpace(repoID) != "" {
		repoIDArg = strings.TrimSpace(repoID)
	}
	var commitArg any = strings.TrimSpace(pin.CommitSHA)
	var linesStartArg any = nullIfZero(pin.LinesStart)
	var linesEndArg any = nullIfZero(pin.LinesEnd)
	if !addPinUsesGitCoordinate(kind) {
		commitArg, linesStartArg, linesEndArg = nil, nil, nil
	}
	var exists bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			  FROM artifact_pins
			 WHERE artifact_id = $1::uuid
			   AND repo_id IS NOT DISTINCT FROM $2::uuid
			   AND commit_sha IS NOT DISTINCT FROM $3::text
			   AND path = $4
			   AND lines_start IS NOT DISTINCT FROM $5::int
			   AND lines_end IS NOT DISTINCT FROM $6::int
			 LIMIT 1
		)
	`, artifactID, repoIDArg, commitArg, pin.Path, linesStartArg, linesEndArg).Scan(&exists)
	return exists, err
}

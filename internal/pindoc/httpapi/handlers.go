package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/var-gg/pindoc/internal/pindoc/embed"
)

type projectInfo struct {
	ID              string        `json:"id"`
	Slug            string        `json:"slug"`
	Name            string        `json:"name"`
	Description     string        `json:"description,omitempty"`
	Color           string        `json:"color,omitempty"`
	PrimaryLanguage string        `json:"primary_language"`
	AreasCount      int           `json:"areas_count"`
	ArtifactsCount  int           `json:"artifacts_count"`
	CreatedAt       time.Time     `json:"created_at"`
	Rendering       RenderingCaps `json:"rendering"`
}

// RenderingCaps tells an agent which markdown features actually render in
// Pindoc's Wiki Reader. Anything not listed may round-trip correctly but
// will not visually render — agents should stick to this set when
// proposing artifact bodies.
type RenderingCaps struct {
	MarkdownFlavor string   `json:"markdown_flavor"`
	Extensions     []string `json:"extensions"`
	CodeLanguages  []string `json:"code_languages"`
	Notes          string   `json:"notes,omitempty"`
}

var pindocRenderingCaps = RenderingCaps{
	MarkdownFlavor: "gfm",
	Extensions: []string{
		"tables",
		"task_lists",
		"strikethrough",
		"autolink",
		"mermaid", // fenced ```mermaid blocks render as SVG
	},
	CodeLanguages: []string{"any"}, // plain monospace rendering for all
	Notes:         "Headings H1–H6, ordered/unordered lists, blockquotes, inline code, fenced code, links. Mermaid via ```mermaid fence. Math/KaTeX not supported (M1.x).",
}

func (d Deps) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"default_project_slug": d.DefaultProjectSlug,
		"multi_project":        d.MultiProject,
		"version":              d.Version,
	})
}

type projectListRow struct {
	ID              string    `json:"id"`
	Slug            string    `json:"slug"`
	Name            string    `json:"name"`
	Description     string    `json:"description,omitempty"`
	Color           string    `json:"color,omitempty"`
	PrimaryLanguage string    `json:"primary_language"`
	ArtifactsCount  int       `json:"artifacts_count"`
	CreatedAt       time.Time `json:"created_at"`
}

func (d Deps) handleProjectList(w http.ResponseWriter, r *http.Request) {
	rows, err := d.DB.Query(r.Context(), `
		SELECT
			p.id::text, p.slug, p.name, p.description, p.color,
			p.primary_language, p.created_at,
			(SELECT count(*) FROM artifacts WHERE project_id = p.id AND status <> 'archived')
		FROM projects p
		ORDER BY p.created_at
	`)
	if err != nil {
		d.Logger.Error("project list", "err", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	out := []projectListRow{}
	for rows.Next() {
		var p projectListRow
		var desc, color *string
		if err := rows.Scan(
			&p.ID, &p.Slug, &p.Name, &desc, &color,
			&p.PrimaryLanguage, &p.CreatedAt, &p.ArtifactsCount,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		if desc != nil {
			p.Description = *desc
		}
		if color != nil {
			p.Color = *color
		}
		out = append(out, p)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"projects":             out,
		"default_project_slug": d.DefaultProjectSlug,
		"multi_project":        d.MultiProject,
	})
}

func (d Deps) handleProjectCurrent(w http.ResponseWriter, r *http.Request) {
	slug := projectSlugFrom(r)
	var out projectInfo
	var desc, color *string
	err := d.DB.QueryRow(r.Context(), `
		SELECT
			p.id::text, p.slug, p.name, p.description, p.color,
			p.primary_language, p.created_at,
			(SELECT count(*) FROM areas     WHERE project_id = p.id),
			(SELECT count(*) FROM artifacts WHERE project_id = p.id AND status <> 'archived')
		FROM projects p WHERE p.slug = $1
	`, slug).Scan(
		&out.ID, &out.Slug, &out.Name, &desc, &color,
		&out.PrimaryLanguage, &out.CreatedAt,
		&out.AreasCount, &out.ArtifactsCount,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		d.Logger.Error("project query", "err", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	if desc != nil {
		out.Description = *desc
	}
	if color != nil {
		out.Color = *color
	}
	out.Rendering = pindocRenderingCaps
	writeJSON(w, http.StatusOK, out)
}

type areaRow struct {
	ID             string   `json:"id"`
	Slug           string   `json:"slug"`
	Name           string   `json:"name"`
	Description    string   `json:"description,omitempty"`
	ParentSlug     string   `json:"parent_slug,omitempty"`
	IsCrossCutting bool     `json:"is_cross_cutting"`
	ArtifactCount  int      `json:"artifact_count"`
	ChildrenSlugs  []string `json:"children_slugs,omitempty"`
}

func (d Deps) handleAreas(w http.ResponseWriter, r *http.Request) {
	slug := projectSlugFrom(r)
	rows, err := d.DB.Query(r.Context(), `
		WITH p AS (SELECT id FROM projects WHERE slug = $1)
		SELECT
			a.id::text, a.slug, a.name, a.description,
			parent.slug, a.is_cross_cutting,
			(SELECT count(*) FROM artifacts x
			  WHERE x.area_id = a.id AND x.status <> 'archived'),
			COALESCE(ARRAY(
			  SELECT c.slug FROM areas c
			  WHERE c.parent_id = a.id
			  ORDER BY c.slug
			), ARRAY[]::text[])
		FROM areas a
		JOIN p ON a.project_id = p.id
		LEFT JOIN areas parent ON parent.id = a.parent_id
		ORDER BY a.is_cross_cutting, a.slug
	`, slug)
	if err != nil {
		d.Logger.Error("areas query", "err", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	out := []areaRow{}
	for rows.Next() {
		var a areaRow
		var desc, parent *string
		if err := rows.Scan(
			&a.ID, &a.Slug, &a.Name, &desc,
			&parent, &a.IsCrossCutting,
			&a.ArtifactCount, &a.ChildrenSlugs,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		if desc != nil {
			a.Description = *desc
		}
		if parent != nil {
			a.ParentSlug = *parent
		}
		out = append(out, a)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"project_slug": slug,
		"areas":        out,
	})
}

type artifactRow struct {
	ID           string    `json:"id"`
	Slug         string    `json:"slug"`
	Type         string    `json:"type"`
	Title        string    `json:"title"`
	AreaSlug     string    `json:"area_slug"`
	Completeness string    `json:"completeness"`
	Status       string    `json:"status"`
	ReviewState  string    `json:"review_state"`
	AuthorID     string    `json:"author_id"`
	PublishedAt  time.Time `json:"published_at,omitzero"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (d Deps) handleArtifactList(w http.ResponseWriter, r *http.Request) {
	slug := projectSlugFrom(r)
	areaSlug := r.URL.Query().Get("area")
	typeFilter := r.URL.Query().Get("type")

	rows, err := d.DB.Query(r.Context(), `
		SELECT
			a.id::text, a.slug, a.type, a.title, ar.slug,
			a.completeness, a.status, a.review_state,
			a.author_id, a.published_at, a.updated_at
		FROM artifacts a
		JOIN projects p  ON p.id  = a.project_id
		JOIN areas    ar ON ar.id = a.area_id
		WHERE p.slug = $1
		  AND a.status <> 'archived'
		  AND ($2 = '' OR ar.slug = $2)
		  AND ($3 = '' OR a.type  = $3)
		ORDER BY a.updated_at DESC
		LIMIT 200
	`, slug, areaSlug, typeFilter)
	if err != nil {
		d.Logger.Error("artifact list", "err", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	out := []artifactRow{}
	for rows.Next() {
		var a artifactRow
		var publishedAt *time.Time
		if err := rows.Scan(
			&a.ID, &a.Slug, &a.Type, &a.Title, &a.AreaSlug,
			&a.Completeness, &a.Status, &a.ReviewState,
			&a.AuthorID, &publishedAt, &a.UpdatedAt,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		if publishedAt != nil {
			a.PublishedAt = *publishedAt
		}
		out = append(out, a)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"project_slug": slug,
		"artifacts":    out,
	})
}

type artifactDetail struct {
	artifactRow
	BodyMarkdown  string    `json:"body_markdown"`
	Tags          []string  `json:"tags"`
	AuthorVersion string    `json:"author_version,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

func (d Deps) handleArtifactGet(w http.ResponseWriter, r *http.Request) {
	slug := projectSlugFrom(r)
	ref := r.PathValue("idOrSlug")
	if ref == "" {
		writeError(w, http.StatusBadRequest, "missing id or slug")
		return
	}

	var a artifactDetail
	var publishedAt *time.Time
	var authorVer *string
	err := d.DB.QueryRow(r.Context(), `
		SELECT
			a.id::text, a.slug, a.type, a.title, ar.slug,
			a.completeness, a.status, a.review_state,
			a.author_id, a.published_at, a.updated_at,
			a.body_markdown, a.tags, a.author_version, a.created_at
		FROM artifacts a
		JOIN projects p  ON p.id  = a.project_id
		JOIN areas    ar ON ar.id = a.area_id
		WHERE p.slug = $1 AND (a.id::text = $2 OR a.slug = $2)
		LIMIT 1
	`, slug, ref).Scan(
		&a.ID, &a.Slug, &a.Type, &a.Title, &a.AreaSlug,
		&a.Completeness, &a.Status, &a.ReviewState,
		&a.AuthorID, &publishedAt, &a.UpdatedAt,
		&a.BodyMarkdown, &a.Tags, &authorVer, &a.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "artifact not found")
		return
	}
	if err != nil {
		d.Logger.Error("artifact get", "err", err, "ref", ref)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	if publishedAt != nil {
		a.PublishedAt = *publishedAt
	}
	if authorVer != nil {
		a.AuthorVersion = *authorVer
	}
	writeJSON(w, http.StatusOK, a)
}

func (d Deps) handleSearch(w http.ResponseWriter, r *http.Request) {
	slug := projectSlugFrom(r)
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "q is required")
		return
	}
	if d.Embedder == nil {
		writeJSON(w, http.StatusOK, map[string]any{"query": q, "hits": []any{}, "notice": "embedder not configured"})
		return
	}
	res, err := d.Embedder.Embed(r.Context(), embed.Request{Texts: []string{q}, Kind: embed.KindQuery})
	if err != nil {
		d.Logger.Error("embed query", "err", err)
		writeError(w, http.StatusInternalServerError, "embed failed")
		return
	}
	qVec := embed.VectorString(embed.PadTo768(res.Vectors[0]))

	rows, err := d.DB.Query(r.Context(), `
		WITH scored AS (
			SELECT DISTINCT ON (c.artifact_id)
				c.artifact_id, c.heading, c.text,
				c.embedding <=> $1::vector AS distance
			FROM artifact_chunks c
			JOIN artifacts a ON a.id = c.artifact_id
			JOIN projects p ON p.id = a.project_id
			WHERE p.slug = $2 AND a.status <> 'archived'
			ORDER BY c.artifact_id, distance
		)
		SELECT
			s.artifact_id::text, a.slug, a.type, a.title, ar.slug,
			COALESCE(s.heading, '') , s.text, s.distance
		FROM scored s
		JOIN artifacts a  ON a.id  = s.artifact_id
		JOIN areas     ar ON ar.id = a.area_id
		ORDER BY s.distance
		LIMIT 10
	`, qVec, slug)
	if err != nil {
		d.Logger.Error("search", "err", err)
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}
	defer rows.Close()

	type hit struct {
		ArtifactID string  `json:"artifact_id"`
		Slug       string  `json:"slug"`
		Type       string  `json:"type"`
		Title      string  `json:"title"`
		AreaSlug   string  `json:"area_slug"`
		Heading    string  `json:"heading,omitempty"`
		Snippet    string  `json:"snippet"`
		Distance   float64 `json:"distance"`
	}
	out := []hit{}
	for rows.Next() {
		var h hit
		if err := rows.Scan(&h.ArtifactID, &h.Slug, &h.Type, &h.Title, &h.AreaSlug, &h.Heading, &h.Snippet, &h.Distance); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		if len(h.Snippet) > 280 {
			h.Snippet = h.Snippet[:280] + "..."
		}
		out = append(out, h)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"query":        q,
		"project_slug": slug,
		"hits":         out,
	})
}

func (d Deps) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := d.DB.Ping(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "db unreachable")
		return
	}
	info := map[string]any{
		"ok":                   true,
		"version":              d.Version,
		"default_project_slug": d.DefaultProjectSlug,
		"multi_project":        d.MultiProject,
	}
	if d.Embedder != nil {
		ei := d.Embedder.Info()
		info["embedder"] = map[string]any{"name": ei.Name, "model": ei.ModelID, "dim": ei.Dimension}
	}
	writeJSON(w, http.StatusOK, info)
}

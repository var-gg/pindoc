package projectexport

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/var-gg/pindoc/internal/pindoc/db"
)

type Options struct {
	ProjectSlug      string
	Areas            []string
	Slugs            []string
	IncludeRevisions bool
	Format           string
}

type Archive struct {
	Filename      string
	MimeType      string
	Bytes         []byte
	ArtifactCount int
	FileCount     int
}

type Artifact struct {
	ID             string
	Slug           string
	Type           string
	Title          string
	AreaSlug       string
	Tags           []string
	Completeness   string
	AuthorID       string
	ArtifactMeta   map[string]string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	RevisionNumber int
	BodyMarkdown   string
	RelatesTo      []Edge
	Revisions      []Revision
}

type Edge struct {
	Relation string
	Slug     string
	Type     string
	Title    string
}

type Revision struct {
	RevisionNumber int
	Title          string
	AuthorID       string
	CommitMsg      string
	CreatedAt      time.Time
	BodyMarkdown   string
}

func BuildFromDB(ctx context.Context, pool *db.Pool, opts Options) (Archive, error) {
	if opts.ProjectSlug == "" {
		return Archive{}, fmt.Errorf("project slug is required")
	}
	artifacts, err := loadArtifacts(ctx, pool, opts)
	if err != nil {
		return Archive{}, err
	}
	if err := loadEdges(ctx, pool, artifacts); err != nil {
		return Archive{}, err
	}
	if opts.IncludeRevisions {
		if err := loadRevisions(ctx, pool, artifacts); err != nil {
			return Archive{}, err
		}
	}
	return BuildArchive(opts.ProjectSlug, artifacts, opts)
}

func BuildArchive(projectSlug string, artifacts []Artifact, opts Options) (Archive, error) {
	files := map[string]string{}
	for _, a := range artifacts {
		base := path.Join(safePath(a.AreaSlug), safePath(a.Slug)+".md")
		files[base] = artifactMarkdown(a)
		if opts.IncludeRevisions {
			files[path.Join(safePath(a.AreaSlug), safePath(a.Slug)+".revisions.md")] = revisionsMarkdown(a)
		}
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	format := strings.ToLower(strings.TrimSpace(opts.Format))
	if format == "" {
		format = "zip"
	}
	var data []byte
	var mime, ext string
	var err error
	switch format {
	case "tar":
		data, err = buildTar(names, files)
		mime = "application/x-tar"
		ext = "tar"
	default:
		data, err = buildZip(names, files)
		mime = "application/zip"
		ext = "zip"
	}
	if err != nil {
		return Archive{}, err
	}
	return Archive{
		Filename:      "pindoc-" + safePath(projectSlug) + "-" + time.Now().UTC().Format("20060102") + "." + ext,
		MimeType:      mime,
		Bytes:         data,
		ArtifactCount: len(artifacts),
		FileCount:     len(files),
	}, nil
}

func loadArtifacts(ctx context.Context, pool *db.Pool, opts Options) ([]Artifact, error) {
	var areasArg, slugsArg any
	if len(opts.Areas) > 0 {
		areasArg = opts.Areas
	}
	if len(opts.Slugs) > 0 {
		slugsArg = opts.Slugs
	}
	rows, err := pool.Query(ctx, `
		SELECT
			a.id::text, a.slug, a.type, a.title, ar.slug,
			a.tags, a.completeness, a.author_id, a.artifact_meta,
			a.created_at, a.updated_at,
			COALESCE((SELECT max(r.revision_number) FROM artifact_revisions r WHERE r.artifact_id = a.id), 0),
			a.body_markdown
		FROM artifacts a
		JOIN projects p ON p.id = a.project_id
		JOIN areas ar ON ar.id = a.area_id
		WHERE p.slug = $1
		  AND a.status <> 'archived'
		  AND ($2::text[] IS NULL OR ar.slug = ANY($2))
		  AND ($3::text[] IS NULL OR a.slug = ANY($3))
		ORDER BY ar.slug, a.slug
	`, opts.ProjectSlug, areasArg, slugsArg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Artifact{}
	for rows.Next() {
		var a Artifact
		var metaRaw []byte
		if err := rows.Scan(
			&a.ID,
			&a.Slug,
			&a.Type,
			&a.Title,
			&a.AreaSlug,
			&a.Tags,
			&a.Completeness,
			&a.AuthorID,
			&metaRaw,
			&a.CreatedAt,
			&a.UpdatedAt,
			&a.RevisionNumber,
			&a.BodyMarkdown,
		); err != nil {
			return nil, err
		}
		a.ArtifactMeta = flattenMeta(metaRaw)
		out = append(out, a)
	}
	return out, rows.Err()
}

func loadEdges(ctx context.Context, pool *db.Pool, artifacts []Artifact) error {
	if len(artifacts) == 0 {
		return nil
	}
	ids := make([]string, 0, len(artifacts))
	index := map[string]int{}
	for i, a := range artifacts {
		ids = append(ids, a.ID)
		index[a.ID] = i
	}
	rows, err := pool.Query(ctx, `
		SELECT e.source_id::text, e.relation, target.slug, target.type, target.title
		FROM artifact_edges e
		JOIN artifacts target ON target.id = e.target_id
		WHERE e.source_id::text = ANY($1::text[])
		ORDER BY e.created_at
	`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var sourceID string
		var edge Edge
		if err := rows.Scan(&sourceID, &edge.Relation, &edge.Slug, &edge.Type, &edge.Title); err != nil {
			return err
		}
		if i, ok := index[sourceID]; ok {
			artifacts[i].RelatesTo = append(artifacts[i].RelatesTo, edge)
		}
	}
	return rows.Err()
}

func loadRevisions(ctx context.Context, pool *db.Pool, artifacts []Artifact) error {
	for i := range artifacts {
		rows, err := pool.Query(ctx, `
			SELECT revision_number, title, author_id, COALESCE(commit_msg, ''), created_at,
			       COALESCE(
			         body_markdown,
			         (
			           SELECT prev.body_markdown
			           FROM artifact_revisions prev
			           WHERE prev.artifact_id = artifact_revisions.artifact_id
			             AND prev.revision_number < artifact_revisions.revision_number
			             AND prev.body_markdown IS NOT NULL
			           ORDER BY prev.revision_number DESC
			           LIMIT 1
			         ),
			         ''
			       )
			FROM artifact_revisions
			WHERE artifact_id::text = $1
			ORDER BY revision_number
		`, artifacts[i].ID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var rev Revision
			if err := rows.Scan(&rev.RevisionNumber, &rev.Title, &rev.AuthorID, &rev.CommitMsg, &rev.CreatedAt, &rev.BodyMarkdown); err != nil {
				rows.Close()
				return err
			}
			artifacts[i].Revisions = append(artifacts[i].Revisions, rev)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
	}
	return nil
}

func artifactMarkdown(a Artifact) string {
	var b strings.Builder
	b.WriteString("---\n")
	writeKV(&b, "title", a.Title)
	writeKV(&b, "type", a.Type)
	writeKV(&b, "area", a.AreaSlug)
	writeList(&b, "tags", a.Tags)
	writeKV(&b, "completeness", a.Completeness)
	writeKV(&b, "slug", a.Slug)
	writeKV(&b, "agent_ref", "pindoc://"+a.Slug)
	if len(a.ArtifactMeta) > 0 {
		keys := make([]string, 0, len(a.ArtifactMeta))
		for key := range a.ArtifactMeta {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		b.WriteString("artifact_meta:\n")
		for _, key := range keys {
			writeNestedKV(&b, key, a.ArtifactMeta[key])
		}
	}
	writeKV(&b, "created_at", a.CreatedAt.Format(time.RFC3339))
	writeKV(&b, "updated_at", a.UpdatedAt.Format(time.RFC3339))
	b.WriteString(fmt.Sprintf("revision_number: %d\n", a.RevisionNumber))
	if len(a.RelatesTo) > 0 {
		b.WriteString("relates_to:\n")
		for _, e := range a.RelatesTo {
			b.WriteString("  - relation: " + quoteYAML(e.Relation) + "\n")
			b.WriteString("    slug: " + quoteYAML(e.Slug) + "\n")
			b.WriteString("    type: " + quoteYAML(e.Type) + "\n")
			b.WriteString("    title: " + quoteYAML(e.Title) + "\n")
		}
	} else {
		b.WriteString("relates_to: []\n")
	}
	b.WriteString("---\n\n")
	b.WriteString(a.BodyMarkdown)
	if !strings.HasSuffix(a.BodyMarkdown, "\n") {
		b.WriteString("\n")
	}
	return b.String()
}

func revisionsMarkdown(a Artifact) string {
	var b strings.Builder
	b.WriteString("# Revisions: " + a.Title + "\n\n")
	for _, rev := range a.Revisions {
		b.WriteString(fmt.Sprintf("## rev %d - %s\n\n", rev.RevisionNumber, rev.CreatedAt.Format(time.RFC3339)))
		if rev.CommitMsg != "" {
			b.WriteString("Commit: " + rev.CommitMsg + "\n\n")
		}
		b.WriteString("Author: " + rev.AuthorID + "\n\n")
		b.WriteString(rev.BodyMarkdown)
		if !strings.HasSuffix(rev.BodyMarkdown, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func buildZip(names []string, files map[string]string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range names {
		w, err := zw.Create(name)
		if err != nil {
			_ = zw.Close()
			return nil, err
		}
		if _, err := io.WriteString(w, files[name]); err != nil {
			_ = zw.Close()
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func buildTar(names []string, files map[string]string) ([]byte, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, name := range names {
		data := []byte(files[name])
		if err := tw.WriteHeader(&tar.Header{
			Name:    name,
			Mode:    0o644,
			Size:    int64(len(data)),
			ModTime: time.Now(),
		}); err != nil {
			_ = tw.Close()
			return nil, err
		}
		if _, err := tw.Write(data); err != nil {
			_ = tw.Close()
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func flattenMeta(raw []byte) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	var values map[string]any
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil
	}
	out := map[string]string{}
	for key, value := range values {
		if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
			out[key] = strings.TrimSpace(s)
		}
	}
	return out
}

func writeKV(b *strings.Builder, key, value string) {
	b.WriteString(key + ": " + quoteYAML(value) + "\n")
}

func writeNestedKV(b *strings.Builder, key, value string) {
	b.WriteString("  " + key + ": " + quoteYAML(value) + "\n")
}

func writeList(b *strings.Builder, key string, values []string) {
	if len(values) == 0 {
		b.WriteString(key + ": []\n")
		return
	}
	b.WriteString(key + ":\n")
	for _, value := range values {
		b.WriteString("  - " + quoteYAML(value) + "\n")
	}
}

func quoteYAML(s string) string {
	replacer := strings.NewReplacer("\\", "\\\\", `"`, `\"`, "\n", "\\n")
	return `"` + replacer.Replace(s) + `"`
}

func safePath(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return "_"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if (r == '-' || r == '_' || r == '.') && !lastDash {
			b.WriteRune(r)
			lastDash = r == '-'
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-.")
	if out == "" {
		return "_"
	}
	return out
}

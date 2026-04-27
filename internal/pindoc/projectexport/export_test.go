package projectexport

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
	"time"
)

func TestBuildArchiveFrontmatterAndBody(t *testing.T) {
	archive, err := BuildArchive("pindoc", []Artifact{{
		ID:             "a1",
		Slug:           "task-export",
		Type:           "Task",
		Title:          "Export Task",
		AreaSlug:       "mcp",
		Tags:           []string{"export"},
		Completeness:   "partial",
		AuthorID:       "codex",
		ArtifactMeta:   map[string]string{"source_type": "code", "verification_state": "verified"},
		CreatedAt:      time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, 4, 26, 10, 5, 0, 0, time.UTC),
		RevisionNumber: 3,
		BodyMarkdown:   "## Context\n\nBody\n",
		RelatesTo: []Edge{{
			Relation: "references",
			Slug:     "other",
			Type:     "Decision",
			Title:    "Other",
		}},
	}}, Options{Format: "zip"})
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(archive.Bytes), int64(len(archive.Bytes)))
	if err != nil {
		t.Fatal(err)
	}
	if len(zr.File) != 1 {
		t.Fatalf("files=%d want 1", len(zr.File))
	}
	rc, err := zr.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		`title: "Export Task"`,
		`agent_ref: "pindoc://task-export"`,
		"artifact_meta:",
		`relation: "references"`,
		"## Context",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("export missing %q in:\n%s", want, text)
		}
	}
}

func TestBuildArchiveIncludesRevisions(t *testing.T) {
	archive, err := BuildArchive("pindoc", []Artifact{{
		Slug:           "task-export",
		Type:           "Task",
		Title:          "Export Task",
		AreaSlug:       "mcp",
		Completeness:   "partial",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		RevisionNumber: 1,
		BodyMarkdown:   "Body\n",
		Revisions: []Revision{{
			RevisionNumber: 1,
			Title:          "Export Task",
			AuthorID:       "codex",
			CommitMsg:      "create",
			CreatedAt:      time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC),
			BodyMarkdown:   "Body\n",
		}},
	}}, Options{Format: "zip", IncludeRevisions: true})
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(archive.Bytes), int64(len(archive.Bytes)))
	if err != nil {
		t.Fatal(err)
	}
	if len(zr.File) != 2 || archive.FileCount != 2 {
		t.Fatalf("revision export files=%d archive.FileCount=%d want 2", len(zr.File), archive.FileCount)
	}
}

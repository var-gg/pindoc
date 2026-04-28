package git

import (
	"context"
	"errors"
)

const BlobSizeCap = 1024 * 1024

var (
	ErrNoProviderForRepo = errors.New("no_provider_for_repo")
	ErrCommitNotFound    = errors.New("commit_not_found")
	ErrPathNotFound      = errors.New("path_not_found")
	ErrPathRejected      = errors.New("path_rejected")
	ErrCommitTooShort    = errors.New("commit_too_short")
	ErrSizeCapExceeded   = errors.New("size_cap_exceeded")
)

type Repo struct {
	ID                string
	Name              string
	GitRemoteURL      string
	GitRemoteOriginal string
	DefaultBranch     string
	LocalPaths        []string
	URLs              []string
}

type ChangedFile struct {
	Path      string `json:"path"`
	Status    string `json:"status"`
	Additions int    `json:"additions,omitempty"`
	Deletions int    `json:"deletions,omitempty"`
	Binary    bool   `json:"binary,omitempty"`
}

type CommitInfo struct {
	SHA         string `json:"sha"`
	Author      string `json:"author"`
	AuthorEmail string `json:"author_email,omitempty"`
	AuthorTime  string `json:"author_time"`
	Summary     string `json:"summary"`
}

type RecentCommit struct {
	SHA     string        `json:"sha"`
	Summary string        `json:"summary"`
	Files   []ChangedFile `json:"files"`
}

type Blob struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	Text   string `json:"text,omitempty"`
	Binary bool   `json:"binary,omitempty"`
}

type GitContentProvider interface {
	Available(ctx context.Context, repo Repo) bool
	ChangedFiles(ctx context.Context, repo Repo, commit string) ([]ChangedFile, error)
	Blob(ctx context.Context, repo Repo, commit, path string, sizeCap int64) (Blob, error)
	Diff(ctx context.Context, repo Repo, commit, path string) (string, error)
}

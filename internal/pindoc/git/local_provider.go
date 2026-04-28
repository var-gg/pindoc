package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type LocalGitProvider struct{}

func (LocalGitProvider) Available(ctx context.Context, repo Repo) bool {
	root := firstUsableRoot(ctx, repo)
	return root != ""
}

func (p LocalGitProvider) ChangedFiles(ctx context.Context, repo Repo, commit string) ([]ChangedFile, error) {
	root := firstUsableRoot(ctx, repo)
	if root == "" {
		return nil, ErrNoProviderForRepo
	}
	commit, err := validateCommit(ctx, root, commit)
	if err != nil {
		return nil, err
	}
	out, err := gitOutput(ctx, root, "show", "--format=", "--name-status", "--no-renames", commit)
	if err != nil {
		if isUnknownRevision(err) {
			return nil, ErrCommitNotFound
		}
		return nil, err
	}
	var files []ChangedFile
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		files = append(files, ChangedFile{Status: fields[0], Path: fields[len(fields)-1]})
	}
	return files, nil
}

func (p LocalGitProvider) RecentCommitFiles(ctx context.Context, repo Repo, author string, limit int) ([]RecentCommit, error) {
	root := firstUsableRoot(ctx, repo)
	if root == "" {
		return nil, ErrNoProviderForRepo
	}
	author = strings.TrimSpace(author)
	if author == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 5
	}
	out, err := gitOutput(ctx, root,
		"log",
		"--no-merges",
		"--author="+author,
		fmt.Sprintf("-n%d", limit),
		"--pretty=format:%H%x09%s",
	)
	if err != nil {
		return nil, err
	}
	var commits []RecentCommit
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		sha, summary, ok := strings.Cut(line, "\t")
		if !ok {
			sha = line
			summary = ""
		}
		files, err := p.ChangedFiles(ctx, repo, sha)
		if err != nil {
			return nil, err
		}
		commits = append(commits, RecentCommit{
			SHA:     strings.TrimSpace(sha),
			Summary: strings.TrimSpace(summary),
			Files:   files,
		})
	}
	return commits, nil
}

func (p LocalGitProvider) Blob(ctx context.Context, repo Repo, commit, filePath string, sizeCap int64) (Blob, error) {
	root := firstUsableRoot(ctx, repo)
	if root == "" {
		return Blob{}, ErrNoProviderForRepo
	}
	commit, err := validateCommit(ctx, root, commit)
	if err != nil {
		return Blob{}, err
	}
	clean, err := cleanRepoPath(filePath)
	if err != nil {
		return Blob{}, err
	}
	sizeRaw, err := gitOutput(ctx, root, "cat-file", "-s", commit+":"+clean)
	if err != nil {
		if isUnknownRevision(err) {
			return Blob{}, ErrPathNotFound
		}
		return Blob{}, err
	}
	var size int64
	if _, err := fmt.Sscanf(strings.TrimSpace(string(sizeRaw)), "%d", &size); err != nil {
		return Blob{}, err
	}
	if sizeCap <= 0 {
		sizeCap = BlobSizeCap
	}
	if size > sizeCap {
		return Blob{Path: clean, Size: size}, ErrSizeCapExceeded
	}
	out, err := gitOutput(ctx, root, "show", commit+":"+clean)
	if err != nil {
		if isUnknownRevision(err) {
			return Blob{}, ErrPathNotFound
		}
		return Blob{}, err
	}
	return Blob{Path: clean, Size: size, Text: string(out), Binary: bytes.IndexByte(out, 0) >= 0}, nil
}

func (p LocalGitProvider) Diff(ctx context.Context, repo Repo, commit, filePath string) (string, error) {
	root := firstUsableRoot(ctx, repo)
	if root == "" {
		return "", ErrNoProviderForRepo
	}
	commit, err := validateCommit(ctx, root, commit)
	if err != nil {
		return "", err
	}
	clean, err := cleanRepoPath(filePath)
	if err != nil {
		return "", err
	}
	out, err := gitOutput(ctx, root, "show", "--format=", "--no-ext-diff", commit, "--", clean)
	if err != nil {
		if isUnknownRevision(err) {
			return "", ErrPathNotFound
		}
		return "", err
	}
	if len(out) == 0 {
		if _, err := gitOutput(ctx, root, "cat-file", "-e", commit+":"+clean); err != nil {
			return "", ErrPathNotFound
		}
	}
	return string(out), nil
}

func validateCommit(ctx context.Context, root, commit string) (string, error) {
	c := strings.TrimSpace(commit)
	if c == "" || c == "HEAD" {
		c = "HEAD"
	} else if len(c) < 7 {
		return "", ErrCommitTooShort
	}
	out, err := gitOutput(ctx, root, "rev-parse", "--verify", c+"^{commit}")
	if err != nil {
		return "", ErrCommitNotFound
	}
	return strings.TrimSpace(string(out)), nil
}

func firstUsableRoot(ctx context.Context, repo Repo) string {
	for _, raw := range repo.LocalPaths {
		root := strings.TrimSpace(raw)
		if root == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
			continue
		}
		if _, err := gitOutput(ctx, root, "rev-parse", "--is-inside-work-tree"); err == nil {
			return root
		}
	}
	return ""
}

func cleanRepoPath(p string) (string, error) {
	p = strings.TrimSpace(strings.ReplaceAll(p, "\\", "/"))
	if p == "" || strings.HasPrefix(p, "/") {
		return "", ErrPathRejected
	}
	clean := filepath.ToSlash(filepath.Clean(p))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return "", ErrPathRejected
	}
	return clean, nil
}

func gitOutput(ctx context.Context, root string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-c", "safe.directory=*", "-C", root}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return nil, fmt.Errorf("%w: %s", err, msg)
		}
		return nil, err
	}
	return out, nil
}

func isUnknownRevision(err error) bool {
	if err == nil {
		return false
	}
	var exit *exec.ExitError
	return errors.As(err, &exit) || strings.Contains(strings.ToLower(err.Error()), "not exist")
}

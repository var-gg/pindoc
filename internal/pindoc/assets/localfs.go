package assets

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type LocalFS struct {
	root string
}

func NewLocalFS(root string) (*LocalFS, error) {
	root = NormalizeRoot(root)
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("asset root abs: %w", err)
	}
	return &LocalFS{root: filepath.Clean(abs)}, nil
}

func (s *LocalFS) Driver() string { return DriverLocalFS }

func (s *LocalFS) Root() string {
	if s == nil {
		return ""
	}
	return s.root
}

func (s *LocalFS) Put(ctx context.Context, sha256Hex string, content []byte) (string, error) {
	if s == nil {
		return "", errors.New("localfs storage is nil")
	}
	sha256Hex = strings.ToLower(strings.TrimSpace(sha256Hex))
	if len(sha256Hex) != 64 {
		return "", fmt.Errorf("invalid sha256 %q", sha256Hex)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	key := sha256Hex[:2] + "/" + sha256Hex
	target, err := s.resolveKey(key)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(target); err == nil {
		return key, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", fmt.Errorf("create asset dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".asset-*")
	if err != nil {
		return "", fmt.Errorf("create asset temp: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("write asset temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close asset temp: %w", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		if _, statErr := os.Stat(target); statErr == nil {
			return key, nil
		}
		return "", fmt.Errorf("commit asset blob: %w", err)
	}
	cleanup = false
	return key, nil
}

func (s *LocalFS) Open(ctx context.Context, storageKey string) (io.ReadCloser, error) {
	if s == nil {
		return nil, errors.New("localfs storage is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := s.resolveKey(storageKey)
	if err != nil {
		return nil, err
	}
	return os.Open(path)
}

func (s *LocalFS) resolveKey(storageKey string) (string, error) {
	storageKey = strings.TrimSpace(storageKey)
	if storageKey == "" {
		return "", errors.New("asset storage key is empty")
	}
	clean := filepath.Clean(filepath.FromSlash(storageKey))
	if filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", fmt.Errorf("asset storage key rejected: %q", storageKey)
	}
	target := filepath.Join(s.root, clean)
	rel, err := filepath.Rel(s.root, target)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("asset storage key escapes root: %q", storageKey)
	}
	return target, nil
}

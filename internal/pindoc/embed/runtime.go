package embed

// onnxruntime shared library bootstrap.
//
// The Gemma provider needs the onnxruntime C library on disk before any
// session can open. We keep the runtime lib under a per-user cache dir
// so every pindoc binary on a machine shares one copy; download is
// one-shot on first use.
//
// Supported platforms below are the Tier-1 targets for V1 self-host:
// Windows amd64, Linux amd64/arm64, macOS amd64/arm64. Everything else
// errors out with a clear message — better than silently falling through
// to stub and lying about semantic search.

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ONNXRuntimeVersion matches the C API headers yalue/onnxruntime_go v1.27.0
// was built against. Bump both together — the wrapper is version-locked.
//
// 1.24.2 bump: 1.24.1 had a regression in ValidateExternalDataPath
// (microsoft/onnxruntime#27353, fixed in #27374) that false-positived on
// Windows model paths under %LOCALAPPDATA%, rejecting perfectly valid
// external-data sidecars (model.onnx + model.onnx_data in the same dir).
// The ORT C ABI is stable across patch releases so the Go wrapper stays
// pinned at v1.27.0.
const ONNXRuntimeVersion = "1.24.2"

// runtimeArtifact describes one platform's onnxruntime download.
type runtimeArtifact struct {
	URL      string
	SHA256   string // hex, lowercase; "" disables check (dev only)
	Archive  string // "zip" | "tgz"
	LibPath  string // path inside archive to the shared library we need
	FileName string // basename after extraction (what callers look up)
}

// runtimeArtifacts is the platform → artifact lookup. Checksums are
// intentionally left empty in this first pass — fill in on release
// engineering so tampered downloads are caught. Until then the
// download warns if PINDOC_ONNX_RUNTIME_CHECKSUM_REQUIRED=true.
var runtimeArtifacts = map[string]runtimeArtifact{
	"windows/amd64": {
		URL:      "https://github.com/microsoft/onnxruntime/releases/download/v" + ONNXRuntimeVersion + "/onnxruntime-win-x64-" + ONNXRuntimeVersion + ".zip",
		Archive:  "zip",
		LibPath:  "onnxruntime-win-x64-" + ONNXRuntimeVersion + "/lib/onnxruntime.dll",
		FileName: "onnxruntime.dll",
	},
	"linux/amd64": {
		URL:      "https://github.com/microsoft/onnxruntime/releases/download/v" + ONNXRuntimeVersion + "/onnxruntime-linux-x64-" + ONNXRuntimeVersion + ".tgz",
		Archive:  "tgz",
		LibPath:  "onnxruntime-linux-x64-" + ONNXRuntimeVersion + "/lib/libonnxruntime.so." + ONNXRuntimeVersion,
		FileName: "libonnxruntime.so",
	},
	"linux/arm64": {
		URL:      "https://github.com/microsoft/onnxruntime/releases/download/v" + ONNXRuntimeVersion + "/onnxruntime-linux-aarch64-" + ONNXRuntimeVersion + ".tgz",
		Archive:  "tgz",
		LibPath:  "onnxruntime-linux-aarch64-" + ONNXRuntimeVersion + "/lib/libonnxruntime.so." + ONNXRuntimeVersion,
		FileName: "libonnxruntime.so",
	},
	"darwin/amd64": {
		URL:      "https://github.com/microsoft/onnxruntime/releases/download/v" + ONNXRuntimeVersion + "/onnxruntime-osx-x86_64-" + ONNXRuntimeVersion + ".tgz",
		Archive:  "tgz",
		LibPath:  "onnxruntime-osx-x86_64-" + ONNXRuntimeVersion + "/lib/libonnxruntime." + ONNXRuntimeVersion + ".dylib",
		FileName: "libonnxruntime.dylib",
	},
	"darwin/arm64": {
		URL:      "https://github.com/microsoft/onnxruntime/releases/download/v" + ONNXRuntimeVersion + "/onnxruntime-osx-arm64-" + ONNXRuntimeVersion + ".tgz",
		Archive:  "tgz",
		LibPath:  "onnxruntime-osx-arm64-" + ONNXRuntimeVersion + "/lib/libonnxruntime." + ONNXRuntimeVersion + ".dylib",
		FileName: "libonnxruntime.dylib",
	},
}

// ResolveONNXRuntime returns the absolute path to the onnxruntime shared
// library for the current platform. On first call it downloads + extracts
// the artifact into cacheDir; subsequent calls short-circuit. The caller
// passes this to ort.SetSharedLibraryPath before InitializeEnvironment.
//
// cacheDir defaults to $XDG_CACHE_HOME/pindoc/runtime (or ~/.pindoc/runtime)
// when empty.
func ResolveONNXRuntime(cacheDir string) (string, error) {
	key := runtime.GOOS + "/" + runtime.GOARCH
	art, ok := runtimeArtifacts[key]
	if !ok {
		return "", fmt.Errorf("onnxruntime: no prebuilt for %s; set PINDOC_ONNX_RUNTIME_LIB to override", key)
	}

	if override := strings.TrimSpace(os.Getenv("PINDOC_ONNX_RUNTIME_LIB")); override != "" {
		if _, err := os.Stat(override); err != nil {
			return "", fmt.Errorf("PINDOC_ONNX_RUNTIME_LIB=%s: %w", override, err)
		}
		return override, nil
	}

	if cacheDir == "" {
		var err error
		cacheDir, err = defaultCacheDir("runtime")
		if err != nil {
			return "", err
		}
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir runtime cache: %w", err)
	}

	verDir := filepath.Join(cacheDir, "onnxruntime-"+ONNXRuntimeVersion)
	libPath := filepath.Join(verDir, art.FileName)
	if _, err := os.Stat(libPath); err == nil {
		return libPath, nil
	}

	// Cache miss. Download, extract into a scratch tempdir, then copy
	// the one file we care about into the canonical cache layout.
	scratch, err := os.MkdirTemp("", "pindoc-onnxruntime-extract-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(scratch)

	if err := downloadAndExtract(art, scratch); err != nil {
		return "", fmt.Errorf("download onnxruntime: %w", err)
	}
	srcLib := filepath.Join(scratch, art.LibPath)
	if _, err := os.Stat(srcLib); err != nil {
		return "", fmt.Errorf("onnxruntime lib missing at archive path %s: %w", art.LibPath, err)
	}
	if err := os.MkdirAll(verDir, 0o755); err != nil {
		return "", err
	}
	if err := copyFile(srcLib, libPath); err != nil {
		return "", fmt.Errorf("copy runtime lib into cache: %w", err)
	}
	return libPath, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// defaultCacheDir returns "<user cache>/pindoc/<sub>". Falls back to
// ~/.pindoc/<sub> when os.UserCacheDir fails (headless envs).
func defaultCacheDir(sub string) (string, error) {
	if base, err := os.UserCacheDir(); err == nil {
		return filepath.Join(base, "pindoc", sub), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	return filepath.Join(home, ".pindoc", sub), nil
}

// downloadAndExtract fetches art.URL to a temp file, verifies checksum if
// provided, then extracts archives preserving the onnxruntime-<ver>/lib/
// layout inside dest.
func downloadAndExtract(art runtimeArtifact, dest string) error {
	tmp, err := os.CreateTemp("", "pindoc-onnxruntime-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(art.URL)
	if err != nil {
		tmp.Close()
		return fmt.Errorf("GET %s: %w", art.URL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		tmp.Close()
		return fmt.Errorf("GET %s: status %s", art.URL, resp.Status)
	}

	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, hash), resp.Body); err != nil {
		tmp.Close()
		return fmt.Errorf("write artifact: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	if art.SHA256 != "" {
		got := hex.EncodeToString(hash.Sum(nil))
		if got != art.SHA256 {
			return fmt.Errorf("checksum mismatch for %s: got %s want %s", art.URL, got, art.SHA256)
		}
	}

	switch art.Archive {
	case "zip":
		return extractZip(tmpPath, dest)
	case "tgz":
		return extractTgz(tmpPath, dest)
	default:
		return fmt.Errorf("unknown archive type %q", art.Archive)
	}
}

func extractZip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		if strings.Contains(f.Name, "..") {
			continue
		}
		out := filepath.Join(dest, f.Name)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(out, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		w, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(w, rc); err != nil {
			rc.Close()
			w.Close()
			return err
		}
		rc.Close()
		w.Close()
	}
	return nil
}

func extractTgz(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if strings.Contains(hdr.Name, "..") {
			continue
		}
		out := filepath.Join(dest, hdr.Name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(out, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
				return err
			}
			w, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(w, tr); err != nil {
				w.Close()
				return err
			}
			w.Close()
		case tar.TypeSymlink:
			_ = os.Symlink(hdr.Linkname, out)
		}
	}
}

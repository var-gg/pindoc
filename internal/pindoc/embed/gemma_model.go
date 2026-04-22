package embed

// EmbeddingGemma model asset bootstrap.
//
// We ship with Google's embeddinggemma-300m in Q4-quantized ONNX form
// (~197 MB weights + ~3 MB tokenizer), the smallest variant that keeps
// a production-usable quality floor. Matryoshka truncation keeps the
// output at 768 dim so the existing pgvector column layout stays valid.
//
// On first use the assets are pulled from HuggingFace Hub to a per-user
// cache dir, then reused. No container, no pip install, no Docker compose
// — just a pindoc binary and a first-run download.
//
// Upgrade path: run a higher-quality variant by setting
//   PINDOC_EMBED_GEMMA_VARIANT=fp16   (~617 MB)  or  =quantized   (~309 MB)
// The filenames below stay 1:1 with the HuggingFace repo so the override
// is a direct mapping.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// GemmaHFRepo is the canonical source of record. Fork it only if the
// upstream layout changes; swapping repos behind users' backs silently
// would be a supply-chain footgun.
const GemmaHFRepo = "onnx-community/embeddinggemma-300m-ONNX"

// GemmaVariant enumerates the four ONNX builds we support. Q4 is the
// bundled default because docs/03 Embedding Layer commits us to a
// footprint that fits a small EC2 instance.
type GemmaVariant string

const (
	GemmaQ4        GemmaVariant = "q4"        // ~197 MB weights  (default)
	GemmaQ4F16     GemmaVariant = "q4f16"     // ~175 MB weights
	GemmaQuantized GemmaVariant = "quantized" // ~309 MB weights (INT8)
	GemmaFP16      GemmaVariant = "fp16"      // ~617 MB weights
)

// gemmaAsset describes the two files we pull for each variant. The
// .onnx file is a thin metadata wrapper; the .onnx_data holds the
// actual weights (ONNX external-data format).
type gemmaAsset struct {
	ModelFile    string // e.g. "onnx/model_q4.onnx"
	ModelData    string // e.g. "onnx/model_q4.onnx_data"
	ModelSHA256  string // optional; enforced when non-empty
	DataSHA256   string // optional; enforced when non-empty
	ApproxSizeMB int
}

var gemmaAssets = map[GemmaVariant]gemmaAsset{
	GemmaQ4:        {ModelFile: "onnx/model_q4.onnx", ModelData: "onnx/model_q4.onnx_data", ApproxSizeMB: 197},
	GemmaQ4F16:     {ModelFile: "onnx/model_q4f16.onnx", ModelData: "onnx/model_q4f16.onnx_data", ApproxSizeMB: 175},
	GemmaQuantized: {ModelFile: "onnx/model_quantized.onnx", ModelData: "onnx/model_quantized.onnx_data", ApproxSizeMB: 309},
	GemmaFP16:      {ModelFile: "onnx/model_fp16.onnx", ModelData: "onnx/model_fp16.onnx_data", ApproxSizeMB: 617},
}

// Shared tokenizer + config files. Same filenames across variants.
// tokenizer.model is the SentencePiece proto that eliben/go-sentencepiece
// reads; tokenizer.json / tokenizer_config.json / special_tokens_map.json
// are kept for parity with HF tooling and potential future swap to a
// HF-tokenizers Go binding.
var gemmaSharedFiles = []string{
	"tokenizer.model",
	"tokenizer.json",
	"tokenizer_config.json",
	"special_tokens_map.json",
	"config.json",
}

// GemmaAssets is everything a gemma session needs on disk to run.
// ModelPath points at the .onnx metadata file; onnxruntime follows the
// external-data reference to the sibling .onnx_data automatically.
type GemmaAssets struct {
	Variant       GemmaVariant
	ModelPath     string
	TokenizerPath string // tokenizer.model (SentencePiece proto)
	Dir           string // cache directory containing all of the above
}

// ResolveGemmaAssets returns the on-disk paths, downloading any missing
// files on first use. cacheDir defaults to the platform cache.
// variant defaults to GemmaQ4 (docs/03 footprint commitment).
func ResolveGemmaAssets(cacheDir string, variant GemmaVariant) (*GemmaAssets, error) {
	if variant == "" {
		variant = GemmaQ4
	}
	asset, ok := gemmaAssets[variant]
	if !ok {
		return nil, fmt.Errorf("unknown gemma variant %q (valid: q4, q4f16, quantized, fp16)", variant)
	}

	if cacheDir == "" {
		var err error
		cacheDir, err = defaultCacheDir("models/embeddinggemma-300m")
		if err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir model cache: %w", err)
	}

	// Download all required files. Each helper is idempotent — skips if
	// the local file exists and (optionally) the sha256 matches.
	files := []struct {
		remote string
		sha    string
	}{
		{asset.ModelFile, asset.ModelSHA256},
		{asset.ModelData, asset.DataSHA256},
	}
	for _, f := range gemmaSharedFiles {
		files = append(files, struct {
			remote string
			sha    string
		}{remote: f})
	}
	for _, f := range files {
		if err := ensureHFFile(GemmaHFRepo, f.remote, cacheDir, f.sha); err != nil {
			return nil, fmt.Errorf("fetch %s: %w", f.remote, err)
		}
	}

	return &GemmaAssets{
		Variant:       variant,
		ModelPath:     filepath.Join(cacheDir, asset.ModelFile),
		TokenizerPath: filepath.Join(cacheDir, "tokenizer.model"),
		Dir:           cacheDir,
	}, nil
}

// ensureHFFile downloads "https://huggingface.co/{repo}/resolve/main/{path}"
// into cacheDir, preserving the subdirectory layout ("onnx/model_q4.onnx"
// stays under cacheDir/onnx/). No-op when the file already exists and
// either (a) no sha256 is configured, or (b) sha256 matches.
func ensureHFFile(repo, path, cacheDir, wantSHA string) error {
	dst := filepath.Join(cacheDir, filepath.FromSlash(path))
	if st, err := os.Stat(dst); err == nil && st.Size() > 0 {
		if wantSHA == "" {
			return nil
		}
		got, err := sha256File(dst)
		if err == nil && got == wantSHA {
			return nil
		}
		// checksum mismatch or read error → re-download
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	url := "https://huggingface.co/" + repo + "/resolve/main/" + path
	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: status %s", url, resp.Status)
	}

	// Write to a temp file first; rename once we're sure.
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".partial-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		if _, err := os.Stat(tmpPath); err == nil {
			_ = os.Remove(tmpPath)
		}
	}()

	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, hash), resp.Body); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	if wantSHA != "" {
		got := hex.EncodeToString(hash.Sum(nil))
		if got != wantSHA {
			return fmt.Errorf("checksum mismatch for %s: got %s want %s", path, got, wantSHA)
		}
	}
	return os.Rename(tmpPath, dst)
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// variantFromEnv reads PINDOC_EMBED_GEMMA_VARIANT with sane fallbacks.
func variantFromEnv(raw string) GemmaVariant {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "q4":
		return GemmaQ4
	case "q4f16":
		return GemmaQ4F16
	case "quantized", "int8":
		return GemmaQuantized
	case "fp16", "half":
		return GemmaFP16
	default:
		return GemmaQ4
	}
}

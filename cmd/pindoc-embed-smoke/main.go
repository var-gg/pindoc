// pindoc-embed-smoke is a throwaway verification tool for Phase 17 —
// constructs the default embedder (gemma) end-to-end and prints one
// query + one document vector so we can eyeball that:
//   1. onnxruntime shared lib auto-download works
//   2. gemma model + tokenizer assets land in the cache dir
//   3. ONNX session opens
//   4. cosine similarity is plausible (query-document pair > random pair)
//
// Not a unit test — the asset download is multi-hundred-MB and too slow
// for CI. Run once locally when validating Phase 17 changes.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/var-gg/pindoc/internal/pindoc/config"
	"github.com/var-gg/pindoc/internal/pindoc/embed"
)

func main() {
	cfg, _ := config.Load()
	fmt.Printf("provider=%q gemma_variant=%q\n", cfg.Embed.Provider, cfg.Embed.GemmaVariant)

	t0 := time.Now()
	fmt.Println("Building embedder (first run downloads ~300MB onnxruntime + ~200MB gemma)...")
	p, err := embed.Build(cfg.Embed)
	if err != nil {
		fmt.Fprintln(os.Stderr, "build failed:", err)
		os.Exit(1)
	}
	info := p.Info()
	fmt.Printf("ok: name=%s model=%s dim=%d max_tokens=%d (%.1fs)\n",
		info.Name, info.ModelID, info.Dimension, info.MaxTokens, time.Since(t0).Seconds())

	ctx := context.Background()

	t1 := time.Now()
	q, err := p.Embed(ctx, embed.Request{
		Texts: []string{"how do I retry a failed payment?"},
		Kind:  embed.KindQuery,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "embed query failed:", err)
		os.Exit(1)
	}
	fmt.Printf("query vec len=%d first4=%.4f %.4f %.4f %.4f (%.1fs)\n",
		len(q.Vectors[0]), q.Vectors[0][0], q.Vectors[0][1], q.Vectors[0][2], q.Vectors[0][3],
		time.Since(t1).Seconds())

	t2 := time.Now()
	d, err := p.Embed(ctx, embed.Request{
		Texts: []string{
			"On payment failure, implement exponential backoff with jitter and cap retry attempts.",
			"Banana bread is a moist, sweet, cake-like quick bread made with mashed ripe bananas.",
		},
		Kind: embed.KindDocument,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "embed docs failed:", err)
		os.Exit(1)
	}
	fmt.Printf("docs embedded=%d (%.1fs)\n", len(d.Vectors), time.Since(t2).Seconds())

	sim1 := dot(q.Vectors[0], d.Vectors[0])
	sim2 := dot(q.Vectors[0], d.Vectors[1])
	fmt.Printf("cosine(query, payment-retry doc) = %.4f\n", sim1)
	fmt.Printf("cosine(query, banana bread doc)  = %.4f\n", sim2)
	if sim1 <= sim2 {
		fmt.Println("WARN: payment retry doc is NOT closer to query than banana bread. Something's off.")
		os.Exit(2)
	}
	fmt.Println("semantic sanity: payment doc wins as expected ✓")
}

func dot(a, b []float32) float32 {
	var s float32
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}

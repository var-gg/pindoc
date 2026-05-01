//go:build cgo

package embed

// GemmaProvider — on-device embedding via EmbeddingGemma-300m ONNX.
//
// This is the product default per docs/03's EC2-medium footprint target
// (~200 MB weights in Q4 form, fits alongside Postgres and the app on a
// 4 GB instance). No sidecar container, no pip install; first-run fetches
// the model + onnxruntime shared library into a user cache dir and then
// everything is local.
//
// The HTTP provider remains available as an explicit upgrade path for
// users who want TEI / bge-m3 / OpenAI / Cohere / Vertex in front of a
// Pindoc instance — set PINDOC_EMBED_PROVIDER=http.

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"

	sentencepiece "github.com/eliben/go-sentencepiece"
	ort "github.com/yalue/onnxruntime_go"
)

// Gemma's Sentence-Transformer head was trained with task-specific
// instruction prefixes. Retrieval uses these two. Applied transparently
// inside the provider so callers don't need to know the prompt engine.
const (
	gemmaQueryPrefix    = "task: search result | query: "
	gemmaDocumentPrefix = "title: none | text: "
)

// Padding id for Gemma tokenizer (shared <pad> token). The ONNX export
// expects an attention_mask to zero these out during attention so their
// hidden states don't pollute the mean-pool.
const gemmaPadID = int64(0)

// ortOnce guards the process-wide onnxruntime environment initialization.
// Calling InitializeEnvironment twice is an error, so every GemmaProvider
// constructor funnels through this.
var ortOnce sync.Once
var ortInitErr error

func ensureORTInitialized(runtimeLib string) error {
	ortOnce.Do(func() {
		if runtimeLib != "" {
			ort.SetSharedLibraryPath(runtimeLib)
		}
		if err := ort.InitializeEnvironment(); err != nil {
			ortInitErr = fmt.Errorf("initialize onnxruntime environment: %w", err)
		}
	})
	return ortInitErr
}

// GemmaProvider implements Provider using EmbeddingGemma ONNX.
//
// Thread model: Provider.Embed is safe for concurrent callers because
// each call creates its own input/output tensors; the underlying
// DynamicAdvancedSession is mutex-guarded in onnxruntime_go. If that
// proves a bottleneck we can add a session pool later.
type GemmaProvider struct {
	info       Info
	tokenizer  *sentencepiece.Processor
	session    *ort.DynamicAdvancedSession
	inputName  string
	maskName   string
	outputName string
	mu         sync.Mutex // serializes session.Run for now
}

// NewGemma constructs a Provider backed by local ONNX inference.
// Resolves any missing asset paths by hitting the cache/downloader.
func NewGemma(cfg GemmaConfig) (Provider, error) {
	if cfg.Variant == "" {
		cfg.Variant = GemmaQ4
	}

	// 1. onnxruntime shared library.
	runtimeLib := cfg.RuntimeLib
	if runtimeLib == "" {
		resolved, err := ResolveONNXRuntime(cfg.RuntimeDir)
		if err != nil {
			return nil, err
		}
		runtimeLib = resolved
	}
	if err := ensureORTInitialized(runtimeLib); err != nil {
		return nil, err
	}

	// 2. Gemma model + tokenizer assets.
	modelPath := cfg.ModelPath
	tokPath := cfg.TokenizerPath
	if modelPath == "" || tokPath == "" {
		assets, err := ResolveGemmaAssets(cfg.ModelDir, cfg.Variant)
		if err != nil {
			return nil, err
		}
		if modelPath == "" {
			modelPath = assets.ModelPath
		}
		if tokPath == "" {
			tokPath = assets.TokenizerPath
		}
	}

	// 3. Tokenizer.
	tokFile, err := os.Open(tokPath)
	if err != nil {
		return nil, fmt.Errorf("open tokenizer %s: %w", tokPath, err)
	}
	defer tokFile.Close()
	processor, err := sentencepiece.NewProcessor(tokFile)
	if err != nil {
		return nil, fmt.Errorf("load sentencepiece proto: %w", err)
	}

	// 4. Discover input/output names from the graph so we don't guess.
	inputs, outputs, err := ort.GetInputOutputInfo(modelPath)
	if err != nil {
		return nil, fmt.Errorf("inspect onnx graph %s: %w", modelPath, err)
	}
	inputName, maskName := pickGemmaInputs(inputs)
	outputName := pickGemmaOutput(outputs)
	if inputName == "" {
		return nil, fmt.Errorf("onnx graph has no recognisable input_ids (saw %d inputs)", len(inputs))
	}
	if outputName == "" {
		return nil, fmt.Errorf("onnx graph has no recognisable sentence output (saw %d outputs)", len(outputs))
	}

	// 5. Build a reusable dynamic session. Dynamic shape lets us rebind
	// tensors per call (batch and seq both vary).
	outputs1 := []string{outputName}
	inputs1 := []string{inputName}
	if maskName != "" {
		inputs1 = append(inputs1, maskName)
	}
	session, err := ort.NewDynamicAdvancedSession(modelPath, inputs1, outputs1, nil)
	if err != nil {
		return nil, fmt.Errorf("open onnx session: %w", err)
	}

	return &GemmaProvider{
		info: Info{
			Name:         "embeddinggemma",
			ModelID:      "google/embeddinggemma-300m/" + string(cfg.Variant),
			Dimension:    GemmaDimension,
			MaxTokens:    GemmaMaxTokens,
			Distance:     "cosine",
			Multilingual: true,
		},
		tokenizer:  processor,
		session:    session,
		inputName:  inputName,
		maskName:   maskName,
		outputName: outputName,
	}, nil
}

func (g *GemmaProvider) Info() Info { return g.info }

// Close releases the ONNX session. Call from server shutdown.
func (g *GemmaProvider) Close() error {
	if g.session == nil {
		return nil
	}
	err := g.session.Destroy()
	g.session = nil
	return err
}

// Embed tokenizes each text with the task-specific Gemma prefix, runs
// one batched ONNX forward pass, mean-pools the last_hidden_state over
// the attention mask, and L2-normalizes so cosine==dot.
func (g *GemmaProvider) Embed(ctx context.Context, req Request) (*Response, error) {
	if len(req.Texts) == 0 {
		return &Response{Vectors: [][]float32{}}, nil
	}

	prefix := gemmaDocumentPrefix
	if req.Kind == KindQuery {
		prefix = gemmaQueryPrefix
	}

	// 1. Tokenize each input into int64 IDs; compute seq length per row.
	batch := len(req.Texts)
	rawIDs := make([][]int64, batch)
	maxLen := 0
	for i, text := range req.Texts {
		tokens := g.tokenizer.Encode(prefix + text)
		if len(tokens) > GemmaMaxTokens {
			tokens = tokens[:GemmaMaxTokens]
		}
		ids := make([]int64, len(tokens))
		for j, tok := range tokens {
			ids[j] = int64(tok.ID)
		}
		rawIDs[i] = ids
		if len(ids) > maxLen {
			maxLen = len(ids)
		}
	}
	if maxLen == 0 {
		// All inputs empty after tokenization — return zero vectors.
		zeros := make([][]float32, batch)
		for i := range zeros {
			zeros[i] = make([]float32, GemmaDimension)
		}
		return &Response{Vectors: zeros}, nil
	}

	// 2. Pad to rectangular [batch, maxLen] with gemmaPadID; build mask.
	flatIDs := make([]int64, batch*maxLen)
	flatMask := make([]int64, batch*maxLen)
	for i, ids := range rawIDs {
		for j := 0; j < maxLen; j++ {
			if j < len(ids) {
				flatIDs[i*maxLen+j] = ids[j]
				flatMask[i*maxLen+j] = 1
			} else {
				flatIDs[i*maxLen+j] = gemmaPadID
				flatMask[i*maxLen+j] = 0
			}
		}
	}

	// 3. Build input tensors.
	inputShape := ort.NewShape(int64(batch), int64(maxLen))
	inputTensor, err := ort.NewTensor(inputShape, flatIDs)
	if err != nil {
		return nil, fmt.Errorf("input_ids tensor: %w", err)
	}
	defer inputTensor.Destroy()

	inputs := []ort.Value{inputTensor}
	if g.maskName != "" {
		maskTensor, err := ort.NewTensor(inputShape, flatMask)
		if err != nil {
			return nil, fmt.Errorf("attention_mask tensor: %w", err)
		}
		defer maskTensor.Destroy()
		inputs = append(inputs, maskTensor)
	}

	// 4. Run. DynamicAdvancedSession with nil output allocates for us.
	g.mu.Lock()
	outputs := []ort.Value{nil}
	err = g.session.Run(inputs, outputs)
	g.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("onnx run: %w", err)
	}
	if outputs[0] == nil {
		return nil, fmt.Errorf("onnx run returned nil output")
	}
	defer outputs[0].Destroy()

	outTensor, ok := outputs[0].(*ort.Tensor[float32])
	if !ok {
		return nil, fmt.Errorf("unexpected output tensor type %T", outputs[0])
	}
	outShape := outTensor.GetShape()
	data := outTensor.GetData()

	// 5. Shape-adaptive pool.
	// Case A: [batch, hidden] — already pooled. L2 normalize only.
	// Case B: [batch, seq, hidden] — mean-pool over seq with mask.
	vectors := make([][]float32, batch)
	switch len(outShape) {
	case 2:
		if int(outShape[0]) != batch || int(outShape[1]) != GemmaDimension {
			return nil, fmt.Errorf("unexpected [batch,hidden] output shape %v (want [%d,%d])", outShape, batch, GemmaDimension)
		}
		for i := 0; i < batch; i++ {
			v := make([]float32, GemmaDimension)
			copy(v, data[i*GemmaDimension:(i+1)*GemmaDimension])
			l2Normalize(v)
			vectors[i] = v
		}
	case 3:
		seq := int(outShape[1])
		hidden := int(outShape[2])
		if int(outShape[0]) != batch || seq != maxLen || hidden != GemmaDimension {
			return nil, fmt.Errorf("unexpected [batch,seq,hidden] shape %v (want [%d,%d,%d])", outShape, batch, maxLen, GemmaDimension)
		}
		for i := 0; i < batch; i++ {
			v := make([]float32, hidden)
			var count float32
			for j := 0; j < seq; j++ {
				if flatMask[i*seq+j] == 0 {
					continue
				}
				count++
				row := data[(i*seq+j)*hidden : (i*seq+j+1)*hidden]
				for k := 0; k < hidden; k++ {
					v[k] += row[k]
				}
			}
			if count > 0 {
				for k := range v {
					v[k] /= count
				}
			}
			l2Normalize(v)
			vectors[i] = v
		}
	default:
		return nil, fmt.Errorf("unsupported output rank %d (shape %v)", len(outShape), outShape)
	}

	return &Response{Vectors: vectors}, nil
}

// l2Normalize scales vec to unit length in-place. Zero vectors stay zero.
func l2Normalize(vec []float32) {
	var sum float64
	for _, v := range vec {
		sum += float64(v) * float64(v)
	}
	if sum == 0 {
		return
	}
	norm := float32(math.Sqrt(sum))
	for i := range vec {
		vec[i] /= norm
	}
}

// pickGemmaInputs finds the "input_ids" + "attention_mask" names from the
// ONNX graph. The ONNX-community export uses those canonical names but
// other conversions may differ, so we match case-insensitively on common
// substrings.
func pickGemmaInputs(infos []ort.InputOutputInfo) (inputName, maskName string) {
	for _, in := range infos {
		n := in.Name
		if inputName == "" && (n == "input_ids" || containsAny(n, "input_ids", "input-ids", "inputIds")) {
			inputName = n
			continue
		}
		if maskName == "" && containsAny(n, "attention_mask", "attention-mask", "attentionMask") {
			maskName = n
			continue
		}
	}
	// Fallbacks: first input is almost always the token IDs.
	if inputName == "" && len(infos) > 0 {
		inputName = infos[0].Name
	}
	return inputName, maskName
}

// pickGemmaOutput prefers a sentence_embedding-style output when the
// model has pre-pooled; falls back to last_hidden_state otherwise.
func pickGemmaOutput(infos []ort.InputOutputInfo) string {
	preferred := []string{"sentence_embedding", "pooler_output", "embeddings", "last_hidden_state"}
	for _, want := range preferred {
		for _, o := range infos {
			if o.Name == want {
				return o.Name
			}
		}
	}
	// Fallback: single-output models — just use whatever is there.
	if len(infos) > 0 {
		return infos[0].Name
	}
	return ""
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if sub != "" && strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

package embed

// GemmaDimension is the native hidden size of embeddinggemma-300m.
// Matryoshka truncation to 512/256/128 is possible but we use the full
// 768 to keep the existing pgvector column unchanged.
const GemmaDimension = 768

// GemmaMaxTokens is the model's effective sequence budget. Gemma 3's
// embedding tower supports 2048 positions; we expose 512 as a
// conservative default that matches the sentence-transformer training
// recipe and keeps per-embed latency bounded on CPU.
const GemmaMaxTokens = 512

// GemmaConfig bundles the options resolved from env / defaults before
// NewGemma is called. Caller is responsible for populating ModelPath +
// TokenizerPath + RuntimeLib or leaving them blank to trigger the
// automatic resolver.
type GemmaConfig struct {
	Variant       GemmaVariant // default Q4
	ModelDir      string       // cache dir for gemma assets (optional)
	RuntimeDir    string       // cache dir for onnxruntime lib (optional)
	RuntimeLib    string       // explicit onnxruntime lib path (optional override)
	ModelPath     string       // explicit .onnx file (optional override)
	TokenizerPath string       // explicit tokenizer.model (optional override)
}

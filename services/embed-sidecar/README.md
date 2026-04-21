# embed-sidecar — reference Python embedding service

Tiny FastAPI service the Go MCP server talks to via the `http` embedding
provider. Ships real semantic embeddings without requiring a vendor API key.

## Default model

`sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2` (118M params,
~400MB download, 384-dim, multilingual including Korean). Cheap, runs
anywhere `pip install sentence-transformers` succeeds.

Swap via `PINDOC_EMBED_MODEL` env. The HuggingFace repo ID must resolve at
import time. Notable alternatives:

| Model | Dim | RAM | Notes |
|---|---|---|---|
| `paraphrase-multilingual-MiniLM-L12-v2` (default) | 384 | ~600MB | multilingual, fast |
| `paraphrase-multilingual-mpnet-base-v2` | 768 | ~1.1GB | better quality |
| `google/embeddinggemma-300m` | 768 | ~600MB | needs HF token; Gemma license |
| `BAAI/bge-m3` | 1024 | ~3-5GB | quality leader, heavy |

## Run

```
python -m venv .venv
.venv\Scripts\activate       # Windows
pip install -r requirements.txt
python main.py
```

Server binds to `127.0.0.1:5860` by default (`EMBED_PORT` env to change).

## Wire to Pindoc MCP

```
PINDOC_EMBED_PROVIDER=http
PINDOC_EMBED_ENDPOINT=http://127.0.0.1:5860/v1/embeddings
PINDOC_EMBED_MODEL=paraphrase-multilingual-MiniLM-L12-v2
PINDOC_EMBED_DIM=384
PINDOC_EMBED_MAX_TOKENS=512
PINDOC_EMBED_MULTILINGUAL=true
```

Put these in `.mcp.json` env under the `pindoc` server and restart Claude
Code.

## Contract

```
POST /v1/embeddings
{"model": "...", "input": ["text a", "text b"], "kind": "query|document"}

→ 200
{"data": [{"embedding": [0.1, 0.2, ...]}, ...]}

→ 4xx
{"error": {"message": "..."}}
```

`kind` is optional and currently ignored; `main.py` uses the same prefix
scheme the Sentence-Transformers model was trained with (query/passage
prefixes for models that expect them, or none for others).

"""Embedding sidecar for Pindoc MCP.

Exposes an OpenAI-compatible /v1/embeddings endpoint so the Go server's
http embedding provider can talk to it without knowing which sentence-
transformers model is loaded.

Intentionally minimal: one model, one endpoint, CPU-first. Scale-out
happens at a completely different layer (Phase 5+), not here.
"""

from __future__ import annotations

import logging
import os

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field
from sentence_transformers import SentenceTransformer

MODEL_ID = os.getenv("PINDOC_EMBED_MODEL", "sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2")
PORT = int(os.getenv("EMBED_PORT", "5860"))

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger("embed-sidecar")

log.info("loading model %s", MODEL_ID)
model = SentenceTransformer(MODEL_ID)
log.info(
    "model ready: dim=%s max_seq=%s",
    model.get_sentence_embedding_dimension(),
    getattr(model, "max_seq_length", "?"),
)

app = FastAPI(title="pindoc embed-sidecar", version="0.1.0")


class EmbedRequest(BaseModel):
    model: str | None = None
    input: list[str] = Field(min_length=1)
    kind: str | None = None


class EmbedDatum(BaseModel):
    embedding: list[float]


class EmbedResponse(BaseModel):
    data: list[EmbedDatum]
    model: str
    dim: int


@app.get("/health")
def health() -> dict[str, object]:
    return {
        "ok": True,
        "model": MODEL_ID,
        "dim": model.get_sentence_embedding_dimension(),
    }


@app.post("/v1/embeddings", response_model=EmbedResponse)
def embed(req: EmbedRequest) -> EmbedResponse:
    if not req.input:
        raise HTTPException(status_code=400, detail="input is empty")

    # Some multilingual retrieval models expect "query:"/"passage:" prefixes.
    # For the default MiniLM-L12-v2 this is a no-op; for E5-family models we
    # add the prefix when kind is set. Gemma-family models have their own
    # task-aware API that lives behind a different code path — upgrade later.
    texts = req.input
    if (req.model or MODEL_ID).lower().find("e5") != -1 and req.kind:
        prefix = "query: " if req.kind == "query" else "passage: "
        texts = [prefix + t for t in texts]

    try:
        embeddings = model.encode(texts, normalize_embeddings=True, show_progress_bar=False)
    except Exception as e:
        log.exception("encode failed")
        raise HTTPException(status_code=500, detail=f"encode failed: {e}") from e

    return EmbedResponse(
        data=[EmbedDatum(embedding=e.tolist()) for e in embeddings],
        model=MODEL_ID,
        dim=model.get_sentence_embedding_dimension(),
    )


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(app, host="127.0.0.1", port=PORT, log_level="info")

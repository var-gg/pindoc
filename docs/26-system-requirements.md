# System Requirements

<p>
  <a href="./26-system-requirements.md"><img alt="English system requirements" src="https://img.shields.io/badge/lang-English-2563eb.svg?style=flat-square"></a>
  <a href="./26-system-requirements-ko.md"><img alt="Korean system requirements" src="https://img.shields.io/badge/lang-%ED%95%9C%EA%B5%AD%EC%96%B4-6b7280.svg?style=flat-square"></a>
</p>

Pindoc uses semantic search as part of the normal product path. The default
Docker stack runs Postgres with pgvector, the Pindoc daemon, the Reader UI, and
a bundled EmbeddingGemma Q4 ONNX provider. No embedding sidecar is required for
day-to-day use.

## Recommended Profiles

| Profile | CPU | Memory | Disk | Notes |
| --- | --- | --- | --- | --- |
| Local dogfood or small team | 2 cores | 4 GB recommended, 2 GB minimum for light use | 5 GB recommended, 2 GB fresh-clone minimum | Best default for Docker Compose on a laptop or small VM. |
| Read-only public demo | 1 vCPU minimum, 2 vCPU recommended | 2 GB minimum, 4 GB recommended if the demo grows | 5 GB recommended | Keep `/mcp` and mutating routes blocked at the proxy. |
| Host-native development | host dependent | 4 GB+ recommended | 5 GB+ plus build cache | Go, Node, pnpm, and a local or Docker Postgres/pgvector are required. |
| TEI/http embedding sidecar | sidecar dependent | add sidecar model memory | add model cache | Optional. The bundled provider remains the default. |

These are practical OSS launch recommendations, not hard scheduler limits. A
larger artifact corpus, heavier concurrent indexing, or a custom embedding
model will need more memory and disk.

## Default Embedding Cache

The Docker Compose daemon stores embedding assets in the `pindoc_cache` volume
mounted at `/var/lib/pindoc/cache`.

| Path | Purpose |
| --- | --- |
| `/var/lib/pindoc/cache/models/embeddinggemma-300m` | Bundled EmbeddingGemma model cache |
| `/var/lib/pindoc/cache/runtime` | ONNX Runtime shared library cache |
| `/var/lib/postgresql/data` | Postgres and pgvector data in the `pindoc_db` volume |

The default model is `google/embeddinggemma-300m` Q4 with 768 dimensions. The
model weights are about 197 MB and runtime assets are downloaded on first run,
then reused from cache. Keep at least 1 GB of cache headroom even for a small
instance so container rebuilds and model/runtime updates do not crowd the
database.

First run requires outbound HTTPS access to download the model and runtime
assets. Offline deployments should pre-seed the cache directories or use an
operator-managed embedding endpoint.

## Docker Quick Start

Required:

- Docker 27+
- outbound HTTPS on first run
- enough disk for Docker images, `pindoc_db`, and `pindoc_cache`

Default startup:

```bash
docker compose up -d --build
```

The default Compose file intentionally does not pass a host
`PINDOC_EMBED_PROVIDER` into the daemon. Use `PINDOC_COMPOSE_EMBED_PROVIDER`
for Compose-level embedding overrides so a stray local test value cannot make a
real instance index with stub embeddings.

## Host-native Development

Host-native development outside Docker needs:

- Go 1.25+
- Node 20.15+ and pnpm 10+ for `web/`
- Postgres 16 with pgvector, either local or through Docker
- a local C toolchain for native Go tests on platforms that need it, or the
  Docker-based Go test path documented in the release checklist

The same embedding cache behavior applies when running the daemon directly, but
the cache location follows the host user cache directory instead of the Docker
volume.

## Optional TEI Or HTTP Provider

The `tei` Compose profile runs Hugging Face Text Embeddings Inference as an
OpenAI-style `/v1/embeddings` endpoint. Compose exposes it on host port `5860`;
the daemon container reaches it through the `embed` service name.

```bash
PINDOC_COMPOSE_EMBED_PROVIDER=http \
PINDOC_COMPOSE_EMBED_ENDPOINT=http://embed/v1/embeddings \
PINDOC_COMPOSE_EMBED_MODEL=multilingual-e5-base \
docker compose --profile tei up -d --build
```

The TEI cache is stored in `pindoc_embed_cache`. Its memory and disk usage are
model-specific and are in addition to the Pindoc daemon and Postgres. Use this
profile only when you intentionally want to test or operate a non-default
embedding provider.

## Public Exposure

Recommended system requirements do not change the trust model. The default
daemon is intended for loopback development. A public read-only demo should be
served through a reverse proxy that blocks `/mcp`, public non-`GET` methods, and
unapproved git preview routes. See [Public Demo Plan](22-public-demo.md) and
[Security Policy](../SECURITY.md).

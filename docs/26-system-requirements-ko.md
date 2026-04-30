# 권장 사양

<p>
  <a href="./26-system-requirements.md"><img alt="English system requirements" src="https://img.shields.io/badge/lang-English-6b7280.svg?style=flat-square"></a>
  <a href="./26-system-requirements-ko.md"><img alt="Korean system requirements" src="https://img.shields.io/badge/lang-%ED%95%9C%EA%B5%AD%EC%96%B4-2563eb.svg?style=flat-square"></a>
</p>

Pindoc은 의미 검색을 기본 product path로 사용합니다. 기본 Docker stack은
Postgres와 pgvector, Pindoc daemon, Reader UI, bundled EmbeddingGemma Q4 ONNX
provider를 함께 실행합니다. 일반적인 개발과 dogfood에는 별도 embedding
sidecar가 필요하지 않습니다.

## 권장 Profile

| Profile | CPU | Memory | Disk | 비고 |
| --- | --- | --- | --- | --- |
| 로컬 dogfood 또는 소규모 팀 | 2 core | 4 GB 권장, 가벼운 사용은 2 GB minimum | 5 GB 권장, fresh-clone minimum 2 GB | laptop 또는 작은 VM의 Docker Compose 기본값. |
| Read-only public demo | 1 vCPU minimum, 2 vCPU 권장 | 2 GB minimum, demo가 커지면 4 GB 권장 | 5 GB 권장 | proxy에서 `/mcp`와 mutation route를 차단합니다. |
| Host-native 개발 | host 의존 | 4 GB+ 권장 | 5 GB+ 및 build cache | Go, Node, pnpm, local 또는 Docker Postgres/pgvector 필요. |
| TEI/http embedding sidecar | sidecar 의존 | sidecar model memory 추가 | model cache 추가 | 선택 사항. 기본값은 bundled provider입니다. |

이 표는 OSS launch를 위한 현실적인 권장값이며 hard scheduler limit은
아닙니다. Artifact corpus가 커지거나 동시 indexing이 많거나 custom embedding
model을 쓰면 memory와 disk를 더 잡아야 합니다.

## 기본 Embedding Cache

Docker Compose daemon은 embedding asset을 `/var/lib/pindoc/cache`에 mount된
`pindoc_cache` volume에 저장합니다.

| Path | 용도 |
| --- | --- |
| `/var/lib/pindoc/cache/models/embeddinggemma-300m` | Bundled EmbeddingGemma model cache |
| `/var/lib/pindoc/cache/runtime` | ONNX Runtime shared library cache |
| `/var/lib/postgresql/data` | `pindoc_db` volume의 Postgres 및 pgvector data |

기본 model은 768 dimension의 `google/embeddinggemma-300m` Q4입니다. Model
weight는 약 197 MB이고 runtime asset은 첫 실행 때 download된 뒤 cache에서
재사용됩니다. 작은 instance에서도 container rebuild와 model/runtime update가
database 공간을 압박하지 않도록 cache headroom을 최소 1 GB 정도 남기는 것을
권장합니다.

첫 실행에는 model과 runtime asset download를 위한 outbound HTTPS가
필요합니다. Offline 배포는 cache directory를 미리 채우거나 operator가 관리하는
embedding endpoint를 사용해야 합니다.

## Docker Quick Start

필수:

- Docker 27+
- 첫 실행 시 outbound HTTPS
- Docker image, `pindoc_db`, `pindoc_cache`를 위한 disk 여유

기본 실행:

```bash
docker compose up -d --build
```

기본 Compose file은 host의 `PINDOC_EMBED_PROVIDER`를 daemon에 전달하지
않습니다. Compose 수준의 embedding override에는
`PINDOC_COMPOSE_EMBED_PROVIDER`를 사용합니다. 이렇게 해야 local test에서 남은
stub 값이 실제 instance에 흘러 들어가 hash embedding으로 indexing되는 사고를
막을 수 있습니다.

## Host-native 개발

Docker 밖에서 host-native로 개발하려면 다음이 필요합니다.

- Go 1.25+
- `web/` 개발을 위한 Node 20.15+ 및 pnpm 10+
- local 또는 Docker 기반 Postgres 16 + pgvector
- platform에 따라 native Go test에 필요한 local C toolchain, 또는 release
  checklist에 문서화된 Docker 기반 Go test 경로

Daemon을 직접 실행할 때도 embedding cache 동작은 같습니다. 다만 cache 위치는
Docker volume이 아니라 host user cache directory를 따릅니다.

## 선택 TEI 또는 HTTP Provider

`tei` Compose profile은 Hugging Face Text Embeddings Inference를 OpenAI-style
`/v1/embeddings` endpoint로 띄웁니다. Compose는 host port `5860`으로
노출하고, daemon container는 `embed` service name으로 접근합니다.

```bash
PINDOC_COMPOSE_EMBED_PROVIDER=http \
PINDOC_COMPOSE_EMBED_ENDPOINT=http://embed/v1/embeddings \
PINDOC_COMPOSE_EMBED_MODEL=multilingual-e5-base \
docker compose --profile tei up -d --build
```

TEI cache는 `pindoc_embed_cache`에 저장됩니다. Memory와 disk 사용량은 model에
따라 달라지며 Pindoc daemon과 Postgres 사용량에 추가됩니다. 이 profile은
기본 provider가 아닌 embedding provider를 의도적으로 테스트하거나 운영할 때만
사용합니다.

## Public Exposure

권장 사양은 trust model을 바꾸지 않습니다. 기본 daemon은 loopback 개발을
전제로 합니다. Public read-only demo는 reverse proxy에서 `/mcp`, public
non-`GET` method, 승인되지 않은 git preview route를 차단해야 합니다.
[공개 데모 운영안](22-public-demo-ko.md)과 [보안 정책](../SECURITY-ko.md)을
같이 봅니다.

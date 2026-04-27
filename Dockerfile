FROM node:20-bookworm-slim AS web-builder

WORKDIR /src/web

COPY web/package.json web/pnpm-lock.yaml ./
RUN corepack enable && pnpm install --frozen-lockfile

COPY web/ ./
RUN pnpm build

FROM golang:1.25-bookworm AS go-builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/

ARG VERSION=0.0.1-dev
ARG COMMIT=unknown
RUN CGO_ENABLED=1 go build \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /out/pindoc-server ./cmd/pindoc-server
RUN CGO_ENABLED=1 go build \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /out/pindoc-api ./cmd/pindoc-api

FROM debian:bookworm-slim AS runtime

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl libgomp1 \
    && rm -rf /var/lib/apt/lists/*

RUN groupadd --system pindoc \
    && useradd --system --gid pindoc --home-dir /var/lib/pindoc --create-home pindoc \
    && mkdir -p /var/lib/pindoc/cache /app/web/dist \
    && chown -R pindoc:pindoc /var/lib/pindoc /app

WORKDIR /app

COPY --from=go-builder /out/pindoc-server /usr/local/bin/pindoc-server
COPY --from=go-builder /out/pindoc-api /usr/local/bin/pindoc-api
COPY --from=web-builder /src/web/dist /app/web/dist

ENV PINDOC_DATABASE_URL=postgres://pindoc:pindoc_dev@db:5432/pindoc?sslmode=disable
ENV PINDOC_HTTP_MCP_ADDR=0.0.0.0:5830
ENV PINDOC_PROJECT=pindoc
ENV PINDOC_SPA_DIST=/app/web/dist
ENV XDG_CACHE_HOME=/var/lib/pindoc/cache

EXPOSE 5830
VOLUME ["/var/lib/pindoc/cache"]

HEALTHCHECK --interval=10s --timeout=5s --start-period=30s --retries=12 \
    CMD curl -fsS http://127.0.0.1:5830/health || exit 1

USER pindoc

ENTRYPOINT ["pindoc-server"]

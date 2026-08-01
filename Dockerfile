FROM golang:1.25-bookworm AS api-builder
WORKDIR /app
ARG APP_VERSION

COPY api-go/go.mod api-go/go.sum ./
RUN go mod download
COPY api-go/ ./
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o openposterdb ./cmd/server/

FROM node:22-bookworm AS web-builder
WORKDIR /app
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ .
ARG APP_VERSION
RUN if [ -n "${APP_VERSION}" ]; then sed -i "s/\"version\": \"[^\"]*\"/\"version\": \"${APP_VERSION}\"/" package.json; fi
RUN npm run build-only

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y ca-certificates curl && rm -rf /var/lib/apt/lists/* \
    && groupadd --system opdb && useradd --system --gid opdb --create-home opdb
COPY --from=api-builder /app/openposterdb /usr/local/bin/openposterdb
COPY --from=api-builder /app/assets /app/assets
COPY --from=web-builder /app/dist /app/dist

RUN mkdir -p /data/cache /data/db && chown -R opdb:opdb /data
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENV STATIC_DIR=/app/dist
ENV CACHE_DIR=/data/cache
ENV DB_DIR=/data/db
WORKDIR /app
EXPOSE 3000
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s \
    CMD curl -sf http://localhost:3000/api/auth/status || exit 1
ENTRYPOINT ["/entrypoint.sh"]
CMD ["openposterdb"]

FROM rust:1-bookworm AS api-builder
RUN apt-get update && apt-get install -y mold clang && rm -rf /var/lib/apt/lists/*
RUN cargo install cargo-chef --locked
ENV RUSTFLAGS="-C linker=clang -C link-arg=-fuse-ld=mold"
WORKDIR /app

ARG CARGO_FEATURES=""
ARG APP_VERSION

COPY api/Cargo.toml api/Cargo.lock ./
RUN mkdir -p src && echo "fn main() {}" > src/main.rs && echo "" > src/lib.rs
RUN cargo chef prepare --recipe-path recipe.json
RUN cargo chef cook --release --features "${CARGO_FEATURES}" --recipe-path recipe.json

COPY api/ .
RUN if [ -n "${APP_VERSION}" ]; then sed -i "s/^version = \"[^\"]*\"/version = \"${APP_VERSION}\"/" Cargo.toml; fi
RUN cargo build --release --features "${CARGO_FEATURES}"

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
COPY --from=api-builder /app/target/release/openposterdb-api /usr/local/bin/openposterdb
COPY --from=web-builder /app/dist /app/dist

RUN mkdir -p /data/cache /data/db && chown -R opdb:opdb /data
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENV STATIC_DIR=/app/dist
ENV CACHE_DIR=/data/cache
ENV DB_DIR=/data/db
EXPOSE 3000
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s \
    CMD curl -sf http://localhost:3000/api/auth/status || exit 1
ENTRYPOINT ["/entrypoint.sh"]
CMD ["openposterdb"]

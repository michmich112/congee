# syntax=docker/dockerfile:1

# Build the static admin UI on the builder's native platform. npm/esbuild must
# not run under qemu (linux/arm64 multi-arch CI hits ETXTBSY in esbuild's
# postinstall). The output is architecture-independent.
FROM --platform=$BUILDPLATFORM node:24-bookworm AS admin-ui
WORKDIR /src/web/admin
COPY web/admin/package.json web/admin/package-lock.json ./
RUN npm ci
COPY web/admin/ ./
RUN npm run build

FROM rust:1.88-bookworm AS turso-fts
RUN apt-get update \
	&& apt-get install -y --no-install-recommends git ca-certificates \
	&& rm -rf /var/lib/apt/lists/*
WORKDIR /src
ARG TURSO_FTS_REF=36da5b2e435cb07bba3bed2c7e7eef236b6b2e64
RUN git clone --depth 1 https://github.com/tursodatabase/turso.git turso \
	&& git -C turso fetch --depth 1 origin ${TURSO_FTS_REF} \
	&& git -C turso checkout --detach ${TURSO_FTS_REF} \
	&& cargo build --manifest-path turso/Cargo.toml --profile lib-release --package turso_sync_sdk_kit --features fts \
	&& mkdir -p /out \
	&& cp turso/target/lib-release/libturso_sync_sdk_kit.so /out/libturso_sync_sdk_kit.so

FROM golang:1.24-bookworm AS go-build
WORKDIR /src
RUN apt-get update \
	&& apt-get install -y --no-install-recommends gcc \
	&& rm -rf /var/lib/apt/lists/*
COPY go.mod go.sum ./
COPY sdk/plugin/go.mod sdk/plugin/go.sum ./sdk/plugin/
RUN go mod download
COPY scripts/overlay-turso-fts.sh ./scripts/overlay-turso-fts.sh
COPY --from=turso-fts /out/libturso_sync_sdk_kit.so /tmp/turso-fts-lib.so
RUN chmod +x ./scripts/overlay-turso-fts.sh \
	&& TURSO_FTS_LIB=/tmp/turso-fts-lib.so ./scripts/overlay-turso-fts.sh
COPY . .
COPY --from=admin-ui /src/web/admin/build ./web/admin/build
ENV CGO_ENABLED=1
ARG VERSION=0.0.0-dev
RUN go build -ldflags "-X github.com/michmich112/congee/internal/version.Version=${VERSION}" -o /out/congee ./cmd/congee

FROM debian:bookworm-slim
ARG VERSION=0.0.0-dev
ARG GIT_REVISION=
LABEL org.opencontainers.image.title="Congee" \
	org.opencontainers.image.description="Nostr relay" \
	org.opencontainers.image.version="${VERSION}" \
	org.opencontainers.image.revision="${GIT_REVISION}" \
	org.opencontainers.image.source="https://github.com/michmich112/congee"
RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates libgomp1 libstdc++6 \
	&& rm -rf /var/lib/apt/lists/*
WORKDIR /
# Admin UI is served from web/admin/build relative to the process working directory (WORKDIR /).
COPY --from=go-build /src/web/admin/build /web/admin/build
COPY --from=go-build /out/congee /usr/local/bin/congee
ENV CONGEE_DATA_DIR=/data
EXPOSE 3334 3335
VOLUME ["/data"]
ENTRYPOINT ["congee"]

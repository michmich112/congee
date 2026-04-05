# syntax=docker/dockerfile:1

FROM node:24-bookworm AS admin-ui
WORKDIR /src/web/admin
COPY web/admin/package.json web/admin/package-lock.json ./
RUN npm ci
COPY web/admin/ ./
RUN npm run build

FROM golang:1.24-bookworm AS go-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=admin-ui /src/web/admin/build ./web/admin/build
ENV CGO_ENABLED=0
ARG VERSION=0.0.0-dev
RUN go build -ldflags "-X github.com/michmich112/congee/internal/version.Version=${VERSION}" -o /out/congee ./cmd/congee

FROM debian:bookworm-slim
RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates \
	&& rm -rf /var/lib/apt/lists/*
COPY --from=go-build /out/congee /usr/local/bin/congee
EXPOSE 3334 3335
VOLUME ["/config"]
ENTRYPOINT ["congee"]

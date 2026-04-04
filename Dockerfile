# Admin UI (static export for embedding or separate hosting)
FROM node:24-bookworm-slim AS ui
WORKDIR /ui
COPY web/admin/package.json web/admin/package-lock.json ./
RUN npm ci
COPY web/admin/ ./
RUN npm run build

# Go relay binary
FROM golang:1.24-bookworm AS gobuild
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui /ui/build ./web/admin/build
ENV CGO_ENABLED=0
RUN mkdir -p /out && go build -trimpath -ldflags="-s -w" -o /out/congee ./cmd/congee

FROM debian:bookworm-slim AS runtime
RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates \
	&& rm -rf /var/lib/apt/lists/*
COPY --from=gobuild /out/congee /usr/local/bin/congee
WORKDIR /app
EXPOSE 3334 3335
VOLUME ["/etc/congee"]
ENTRYPOINT ["congee"]

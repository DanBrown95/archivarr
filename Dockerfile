# syntax=docker/dockerfile:1

# --- Stage 1: build the Vue frontend ---
FROM node:22-alpine AS web
WORKDIR /web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# --- Stage 2: build the Go binary with the frontend embedded ---
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Replace the committed placeholder dist with the freshly built assets.
COPY --from=web /web/dist ./web/dist
ARG VERSION=docker
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/archivarr ./cmd/archivarr

# --- Optional: run the Go test suite on Linux (docker build --target test .) ---
FROM build AS test
RUN go test ./...

# --- Stage 3: minimal runtime ---
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -g 1000 archivarr && \
    adduser -D -u 1000 -G archivarr archivarr
WORKDIR /app
COPY --from=build /out/archivarr /app/archivarr
VOLUME ["/config"]
EXPOSE 7979
ENV ARCHIVARR_CONFIG_DIR=/config
# NOTE: PUID/PGID-style runtime remapping (linuxserver convention) is a later
# enhancement. For now the image runs as a fixed uid/gid 1000.
USER archivarr
ENTRYPOINT ["/app/archivarr"]

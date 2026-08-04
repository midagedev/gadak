# Multi-stage build: web UI → Go binary → minimal runtime.
# The mirror has no auth; --allow-remote is required for a non-loopback bind.

# ── 1. Build the web UI ──────────────────────────────────────────────────────
FROM node:20-bookworm AS web
WORKDIR /src

COPY package.json package-lock.json ./
COPY svelte.config.js vite.config.ts tsconfig.json ./
RUN npm ci

COPY web/ web/
RUN npm run build

# ── 2. Build a static Go binary ──────────────────────────────────────────────
FROM golang:1.25-bookworm AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=web /src/dist/app ./dist/app

ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags="-s -w" -o /out/scry ./cmd/scry

# ── 3. Runtime ───────────────────────────────────────────────────────────────
# distroless/static: no shell, no package manager; ca-certificates included.
FROM gcr.io/distroless/static-debian12

# The web UI is embedded in the binary (go:embed picks up dist/app during the
# build stage), so the runtime image is just the binary.
COPY --from=build /out/scry /usr/bin/scry

# Persist config + scry.db outside the container filesystem.
ENV SCRY_HOME=/data
VOLUME ["/data"]

EXPOSE 7777

ENTRYPOINT ["scry"]
CMD ["serve", "--addr", "0.0.0.0:7777", "--allow-remote"]

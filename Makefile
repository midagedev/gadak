.PHONY: build test vet typecheck bench scan docker plugins-test \
	media media-web media-agent media-prep media-deps brand \
	hosted-demo hosted-demo-test

build:
	CGO_ENABLED=0 go build -trimpath -o bin/gadak ./cmd/gadak

vet:
	go vet ./...

test:
	go test ./...

typecheck:
	npm run typecheck

# Zero-install hosted demo (static UI + demo.db snapshot for GitHub Pages).
# Output: dist/hosted/. Does not touch dist/app (go:embed).
hosted-demo: build
	node tools/hosted-demo/build.mjs

# Playwright smoke against dist/hosted (not in CI). Requires hosted-demo first.
hosted-demo-test: hosted-demo
	./node_modules/.bin/playwright install chromium
	./node_modules/.bin/playwright test --config e2e/hosted/playwright.config.ts

# Latency benches over a synthetic 10k fixture (T6.7 / G5).
# Not a CI fail gate — machine variance is too high; record output in gates.md.
bench:
	go test -bench='Benchmark(Bootstrap|Search)10k' -benchmem -count=1 -benchtime=3x ./internal/server/

# Secret / internal-string scan (T7.4). Also runs in CI.
scan:
	bash scripts/scan-internal.sh

docker:
	docker build -t gadak .

# Example enrichment plugins (examples/plugins/*) — self-tests on temp DBs.
plugins-test:
	bash examples/plugins/github-prs/test.sh
	bash examples/plugins/deploy-status/test.sh
	bash examples/plugins/csv-import/test.sh

# ── Demo media (docs/media/*) ──────────────────────────────────────────────
# Regenerates GIF/MP4 assets for README and social posts. See docs/MEDIA.md.
# Requires: ffmpeg, vhs (charmbracelet), Node 20+, Playwright chromium, Go.
MEDIA_DIR := docs/media

brand:
	node tools/brand/render.mjs

media: media-web media-agent
	@echo "media: done → $(MEDIA_DIR)/"
	@ls -lh $(MEDIA_DIR)/web-demo.gif $(MEDIA_DIR)/web-demo.mp4 \
		$(MEDIA_DIR)/agent.gif

media-deps:
	@command -v ffmpeg >/dev/null || { echo "media: ffmpeg required" >&2; exit 1; }
	@command -v vhs >/dev/null || { echo "media: vhs required (brew install vhs)" >&2; exit 1; }
	@command -v go >/dev/null || { echo "media: go required" >&2; exit 1; }
	@if [ ! -x node_modules/.bin/playwright ]; then \
		echo "media: npm ci…"; \
		npm ci; \
	fi
	@echo "media: ensuring Playwright chromium…"
	./node_modules/.bin/playwright install chromium

media-prep: media-deps
	bash tools/tapes/prepare.sh

media-web: media-deps
	@mkdir -p $(MEDIA_DIR)
	@echo "media-web: recording Playwright demo…"
	rm -rf e2e/demo/test-results
	GADAK_MEDIA=1 ./node_modules/.bin/playwright test --config e2e/demo/playwright.config.ts
	bash e2e/demo/export-video.sh

media-agent: media-prep
	@mkdir -p $(MEDIA_DIR)
	@echo "media-agent: recording VHS tape…"
	vhs tools/tapes/agent.tape

.PHONY: build test vet typecheck theme-check bench scan docker plugins-test \
	media media-web media-search media-agent media-groupby media-scale media-mcp media-prep media-deps brand \
	hosted-demo hosted-demo-test

build:
	CGO_ENABLED=0 go build -trimpath -o bin/gadak ./cmd/gadak

vet:
	go vet ./...

test:
	go test ./... -count=1

typecheck:
	npm run typecheck

theme-check:
	node tools/theme-check.mjs
	node tools/token-catalog.mjs --check

# demo-fixture is circular: snapshot --from examples/demo.db writes
# examples/demo.db. Content is the committed file, not a seed; --spread
# redistributes timestamps so this is not a schema-only migrate. A schema
# bump that must keep issue/comment bytes stable should Open-migrate a copy
# (`GADAK_HOME=<tmp> gadak status` after copying the file to <tmp>/gadak.db)
# and run scripts/scrub-demo-db.py onto examples/demo.db — do not use this
# target for that (GDK-671; seed→synthesis is a later round).
#
# The committed file's PRAGMA user_version must equal this binary's
# migration level. That gate is
# `go test ./internal/store -run TestCommittedDemoDBMatchesCurrentSchema`;
# this target does not claim to land "the current schema" by itself.
# `bash scripts/demo-schema.sh` prints the stamp + row counts.
demo-fixture:
	go run ./cmd/gadak snapshot examples/demo.db.new --from examples/demo.db --spread 90d --seed 1
	# The committed fixture is opened raw by Datasette Lite (GDK-101): the
	# scrub re-checks fictional values and rebuilds items_fts without
	# contentless_delete. Skipping it is exactly how a regen went red on CI
	# (2026-08-21: Lite gate + empty FTS).
	python3 scripts/scrub-demo-db.py examples/demo.db.new examples/demo.db
	rm examples/demo.db.new
	bash scripts/demo-schema.sh examples/demo.db
	bash scripts/scan-internal.sh
	bash tools/doc-checks.sh

# Zero-install hosted demo (static UI + demo.db snapshot for GitHub Pages).
# Output: dist/hosted/. Does not touch dist/app (go:embed).
hosted-demo: build
	node tools/hosted-demo/build.mjs

# Apex site (site/, Astro) over the hosted tree: / and /install/ come from
# the site, /demo/ and /backlog/ stay owned by hosted-demo above.
# Output: dist/hosted/ with the Astro dist merged at the root.
site: hosted-demo
	GADAK_LANDING=skip node tools/hosted-demo/build.mjs
	mkdir -p site/public && ln -sfn ../../docs/media site/public/media
	npm ci --prefix site
	npm run build --prefix site
	cp -R site/dist/. dist/hosted/

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
# Regenerates GIF/MP4 assets for README and social posts. See docs/project/MEDIA.md.
# Requires: ffmpeg, vhs (charmbracelet), Node 20+, Playwright chromium, Go.
MEDIA_DIR := docs/media

# Every surface's mark comes from docs/media/logo.png: the desktop resizes
# it at build time (desktop/build-app.sh), the web icons are rendered
# beside it, and the phone app's set is generated here. Needs the mobile
# workspace installed (npm ci --prefix mobile) for the tauri CLI.
brand:
	node tools/brand/render.mjs
	bash tools/brand/mobile-icons.sh
	bash tools/check-brand-icons.sh

# media-mcp is deliberately not here: it needs vhs and a Claude Code login,
# and every run spends the operator's own model quota. Re-take it on purpose.
media: media-web media-search media-agent media-groupby media-history
	@echo "media: done → $(MEDIA_DIR)/  (mcp.gif: make media-mcp)"
	@ls -lh $(MEDIA_DIR)/web-demo.gif $(MEDIA_DIR)/web-demo.mp4 \
		$(MEDIA_DIR)/search.gif $(MEDIA_DIR)/search.mp4 \
		$(MEDIA_DIR)/agent.gif $(MEDIA_DIR)/agent.mp4 \
		$(MEDIA_DIR)/groupby.gif $(MEDIA_DIR)/groupby.mp4

media-deps:
	@command -v ffmpeg >/dev/null || { echo "media: ffmpeg required" >&2; exit 1; }
	@command -v go >/dev/null || { echo "media: go required" >&2; exit 1; }
	@command -v gifsicle >/dev/null || echo "media: gifsicle not found — mcp/claude-drive GIFs skip their -O3 pass (brew install gifsicle)" >&2
	@if [ ! -x node_modules/.bin/playwright ]; then \
		echo "media: npm ci…"; \
		npm ci; \
	fi
	@echo "media: ensuring Playwright chromium…"
	./node_modules/.bin/playwright install chromium

media-prep: media-deps
	@command -v vhs >/dev/null || { echo "media-prep: vhs required (brew install vhs)" >&2; exit 1; }
	bash tools/tapes/prepare.sh

media-web: media-deps
	@mkdir -p $(MEDIA_DIR)
	@echo "media-web: recording Playwright demo…"
	rm -rf e2e/demo/test-results
	GADAK_MEDIA=1 ./node_modules/.bin/playwright test --config e2e/demo/playwright.config.ts
	bash e2e/demo/export-video.sh

media-search: media-deps
	@mkdir -p $(MEDIA_DIR)
	@echo "media-search: recording unified-search palette demo…"
	rm -rf e2e/demo/test-results-search
	GADAK_MEDIA=1 ./node_modules/.bin/playwright test --config e2e/demo/search.config.ts
	bash e2e/demo/export-search.sh

media-agent: media-deps
	@mkdir -p $(MEDIA_DIR)
	@echo "media-agent: recording agent-focus demo…"
	rm -rf e2e/demo/test-results-agent
	GADAK_MEDIA=1 ./node_modules/.bin/playwright test --config e2e/demo/agent.config.ts
	bash e2e/demo/export-agent.sh

media-groupby: media-deps
	@mkdir -p $(MEDIA_DIR)
	@echo "media-groupby: recording group-by demo…"
	rm -rf e2e/demo/test-results-groupby
	GADAK_MEDIA=1 ./node_modules/.bin/playwright test --config e2e/demo/groupby.config.ts
	bash e2e/demo/export-groupby.sh

# History clip (F2): one issue thread read end to end (committed fixture).
media-history: media-deps
	@mkdir -p $(MEDIA_DIR)
	@echo "media-history: recording history demo…"
	rm -rf e2e/demo/test-results-history
	GADAK_MEDIA=1 ./node_modules/.bin/playwright test --config e2e/demo/history.config.ts
	bash e2e/demo/export-history.sh

# Terminal hero (0.18): the shell inside the window. Retires the composite
# the agent/tokens clips use — those draw a paper terminal beside an app
# iframe because gadak had no terminal of its own.
# Output is scratch/terminal-hero.mp4, not $(MEDIA_DIR): the pane ships Beta
# in 0.18 and is not announced on the site or in the README, and docs/media is
# symlinked into the website's public root. See e2e/demo/export-terminal.sh.
media-terminal: media-deps
	@echo "media-terminal: recording terminal-pane hero…"
	rm -rf e2e/demo/test-results-terminal
	GADAK_MEDIA=1 ./node_modules/.bin/playwright test --config e2e/demo/terminal.config.ts
	bash e2e/demo/export-terminal.sh

# Scale flagship: the 20k-issue mirror (site hero). Deterministic — the
# snapshot is regenerated from examples/demo.db (seed 1) each take, never
# committed (300+ MB). Not in the `media` aggregate for the same size reason;
# the committed artifacts (scale.gif/mp4) are what the site ships.
media-scale: media-deps
	@mkdir -p $(MEDIA_DIR) e2e/.tmp
	@echo "media-scale: generating 20k snapshot from examples/demo.db…"
	GADAK_HOME=e2e/.tmp/home ./e2e/.tmp/gadak snapshot e2e/.tmp/demo-scale.db \
		--from examples/demo.db --scale 20000 --spread 180d --force >/dev/null
	@echo "media-scale: recording scale flagship…"
	rm -rf e2e/demo/test-results-scale
	GADAK_MEDIA=1 GADAK_SEED_DB="$$(pwd)/e2e/.tmp/demo-scale.db" \
		./node_modules/.bin/playwright test --config e2e/demo/scale.config.ts
	bash e2e/demo/export-scale.sh

# Live Claude Code + gadak MCP (VHS). Requires vhs and a Claude Code login;
# prepare-agent.sh copies credentials into /private/tmp/gadak-demo.
# Always finish with: bash tools/tapes/prepare-agent.sh --clean
#
# pin-demo.sh / strip-oauth.py are written into that isolated HOME (not the
# repo). VHS cannot parse escaped $/" inside Type, so the tape runs these
# by absolute path under Hide.
define MCP_PIN_DEMO
#!/usr/bin/env bash
set -euo pipefail
: "$${GADAK_HOME:?}"
mkdir -p "$$HOME/.gadak" "$$HOME/bin"
cp "$$GADAK_HOME/gadak.db" "$$HOME/.gadak/gadak.db"
cp "$$GADAK_HOME/config.json" "$$HOME/.gadak/config.json"
cp "$$(command -v gadak)" "$$HOME/bin/gadak"
chmod 755 "$$HOME/bin/gadak"
export PATH="$$HOME/bin:$$PATH"
unset NO_COLOR
export NODE_NO_WARNINGS=1
python3 -c 'import json,os;p=os.path.expanduser("~/.claude/settings.json");d=json.load(open(p));d["disableClaudeAiConnectors"]=True;json.dump(d,open(p,"w"),indent=2)'
endef
export MCP_PIN_DEMO
define MCP_STRIP_OAUTH
import json, os
p = os.path.expanduser("~/.claude.json")
d = json.load(open(p))
acc = d.get("oauthAccount") or {}
keep = ("accountUuid", "organizationUuid", "billingType", "seatTier")
d["oauthAccount"] = {k: acc[k] for k in keep if k in acc}
d["hasAvailableSubscription"] = True
ws = os.path.expanduser("~/workspace")
projects = d.setdefault("projects", {})
pr = projects.get(ws) or {}
mcp = (pr.get("mcpServers") or {}).get("gadak") or {}
mcp["command"] = os.path.expanduser("~/bin/gadak")
mcp["args"] = ["mcp"]
mcp["type"] = mcp.get("type") or "stdio"
mcp["env"] = {"GADAK_HOME": os.path.expanduser("~/.gadak")}
pr.setdefault("mcpServers", {})["gadak"] = mcp
projects[ws] = pr
json.dump(d, open(p, "w"), indent=2)
endef
export MCP_STRIP_OAUTH
media-mcp:
	@command -v vhs >/dev/null || { echo "media-mcp: vhs required (brew install vhs)" >&2; exit 1; }
	@command -v go >/dev/null || { echo "media-mcp: go required" >&2; exit 1; }
	@mkdir -p $(MEDIA_DIR)
	bash tools/tapes/prepare.sh
	bash tools/tapes/prepare-agent.sh
	@printf '%s\n' "$$MCP_PIN_DEMO" > /private/tmp/gadak-demo/pin-demo.sh
	@chmod +x /private/tmp/gadak-demo/pin-demo.sh
	@printf '%s\n' "$$MCP_STRIP_OAUTH" > /private/tmp/gadak-demo/strip-oauth.py
	@echo "media-mcp: recording VHS tape (live Claude Code + MCP)…"
	vhs tools/tapes/mcp.tape
	@if command -v gifsicle >/dev/null; then \
		echo "media-mcp: gifsicle -O3 --colors 64"; \
		gifsicle -O3 --colors 64 $(MEDIA_DIR)/mcp.gif -o $(MEDIA_DIR)/mcp.gif; \
	fi
	@echo "media-mcp: run  bash tools/tapes/prepare-agent.sh --clean  when finished"

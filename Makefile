.PHONY: build test vet typecheck bench scan docker plugins-test

build:
	CGO_ENABLED=0 go build -trimpath -o bin/scry ./cmd/scry

vet:
	go vet ./...

test:
	go test ./...

typecheck:
	npm run typecheck

# Latency benches over a synthetic 10k fixture (T6.7 / G5).
# Not a CI fail gate — machine variance is too high; record output in gates.md.
bench:
	go test -bench='Benchmark(Bootstrap|Search)10k' -benchmem -count=1 -benchtime=3x ./internal/server/

# Secret / internal-string scan (T7.4). Also runs in CI.
scan:
	bash scripts/scan-internal.sh

docker:
	docker build -t scry .

# Example enrichment plugins (examples/plugins/*) — self-tests on temp DBs.
plugins-test:
	bash examples/plugins/github-prs/test.sh
	bash examples/plugins/deploy-status/test.sh
	bash examples/plugins/csv-import/test.sh

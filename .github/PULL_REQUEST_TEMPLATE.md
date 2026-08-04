## What / why

-

## How to test

```bash
# commands you ran
```

## Checks

- [ ] `go build ./... && go vet ./... && go test ./...`
- [ ] `npm run typecheck && npm run build` (if UI touched)
- [ ] `bash scripts/scan-internal.sh` (or `make scan`) passes
- [ ] Nothing installation-specific (site URL, token, real issue data, company string)

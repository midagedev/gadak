# Local Development

## Prerequisites

- Go 1.25+
- Node.js 20+
- A Jira Cloud API token: <https://id.atlassian.com/manage-profile/security/api-tokens>
  (use "Create API token"; organization API keys from admin.atlassian.com do
  **not** work against product APIs)

## First run

```bash
npm ci
npm run build                    # web/ -> dist/app
go build -o gadak ./cmd/gadak
./gadak serve                     # http://127.0.0.1:7777
```

`gadak serve` works before any sync exists: it serves the UI and `config.json`,
and the UI reports that nothing is configured.

## Frontend iteration

```bash
./gadak serve &                   # API + config on :7777
npm run dev                      # Vite on :5173, proxies /api and /config.json
npm run typecheck                # svelte-check
```

## Populating a demo Jira

`go run ./tools/seed-demo` creates releases, components, issues, transition
history, comments, and links in a throwaway Jira site.

```bash
export JIRA_SITE=https://your-site.atlassian.net
export JIRA_EMAIL=you@example.com
export JIRA_TOKEN=...
go run ./tools/seed-demo --projects NMB,NMA,NMS --issues 300
go run ./tools/seed-demo --data examples/demo-seed.json --skip-setup
```

Requirements and gotchas:

- Projects must be **company-managed**. Team-managed projects do not expose
  `priority`, `components`, or `fixVersions`, which makes most of the UI's filter
  axes empty.
- `issue/createmeta` translates issue type names into the account's display
  language and ignores `Accept-Language`. The tool reads
  `project/{key}/statuses` instead, which is not localized. Any code you add
  that matches type or status names must do the same.
- Jira assigns `created` at insert time and offers no way to backdate it, so
  seeded issues all share roughly one creation time. Realistic time spread is
  applied by `gadak snapshot`, not in Jira.
- Deleting issues needs the "Delete Issues" permission, which the default
  company-managed permission scheme does not grant. Plan seeding runs so you do
  not need to undo them.

## Verifying no internal strings leaked

```bash
grep -rn "your-company\|your-site\.atlassian\|internal\.example" web/ tools/ docs/
```

Constitution Article 7: no site URL, project key, custom field id, status name,
team label, or person may appear outside example values.

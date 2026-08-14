# 0006 — Confluence connector: same spine, mirror-only, three rounds

Status: accepted
Date: 2026-08-05

## Context

The roadmap's "second source" is due: everything before it has shipped, and the
`items` spine was designed for this moment (`items.kind` reserves `page`,
`comments`/`attachments`/`changelog` hang off `item_id`, `store.Batch` is the
source-neutral input contract). What does *not* exist is a connector interface —
`internal/sync` is a concrete Jira package — and every read query joins
`issues`, so a mirrored page is invisible to every current API.

## Decision

**Mirror-only first, read path second, UI third.** Three rounds, each shippable:

1. **R1 — mirror.** `internal/confluence` HTTP client + a Confluence sync run
   that writes pages into the existing spine: `sources` row `confluence`,
   `items.kind = "page"`, a new `pages` projection table (v9 migration),
   comments into the existing `comments` table, FTS via the existing
   `writeFTS` path. Verified with `gadak sql` and direct FTS queries.
2. **R2 — read path.** Kind-aware search (`items` left-joined to its
   projection), a pages read API, ETag combining both sources' versions
   (the ponytail comment in `server.go` comes due).
3. **R3 — UI.** Generic document detail view (reusing `AdfContent`,
   `CommentList`, `AttachmentGallery`), unified search results.

Shape decisions:

- **No Go interface yet.** Two concrete connectors that share the store's data
  contract (`store.Batch`) are cheaper to maintain than an interface designed
  from one example. Extract an interface when a third source forces it.
  (`docs/ARCHITECTURE.md` claimed one exists; corrected — the boundary is the
  data contract, which is real and enforced.)
- **Same credential.** Confluence Cloud accepts the same email + API token
  Basic auth on the same site; only the REST base differs (`/wiki/rest/api`,
  `/wiki/api/v2`). Config grows a `confluence` section (`spaces []string`,
  empty = every space the account can see), nothing else.
- **ADF bodies.** Pages are fetched with `body.atlas_doc_format`, so the
  existing ADF flattener feeds FTS and the existing web ADF renderer will
  render pages in R3. No HTML-to-markdown conversion layer.
- **Current version only.** No version-history replay: the mirror answers
  "what does the team know", not "how did this page evolve". The page's
  version number is stored for delta detection; history can come later if
  demand shows up.
- **Incremental via CQL.** `lastModified >= <watermark>` with the same
  overlap-and-max-watermark pattern the Jira sync uses. A separate
  comments-only pass covers pages whose comments changed without a body edit —
  comments do not bump the page version, so a version check alone misses them.
- **Keys.** A page's `key` is its numeric page ID (unique per site,
  disjoint in practice from Jira's `ABC-123` shape). Personal-state tables
  (`watches`, `favorites`) key globally; the numeric/prefixed-key split keeps
  the namespaces from colliding without a schema change.

## Why not

- **Version-history replay** (fetch every historical version): an order of
  magnitude more API calls on first sync for a question nobody has asked the
  mirror yet.
- **A `documents` table parallel to the whole spine**: the spine already fits;
  a parallel structure would fork FTS, comments, and attachments handling.
- **Interface-first refactor of `internal/sync`**: churn in the most
  battle-tested package of the codebase for zero user-visible change.

## Addendum (2026-08-15)

Empty `confluence.spaces` means every **global** space. Personal spaces are
mirrored only when named (`internal/config/config.go`;
`docs/CONFIGURATION.md`). The accepted body above still says
"every space the account can see" — that wording is superseded by this rule,
not rewritten in place.

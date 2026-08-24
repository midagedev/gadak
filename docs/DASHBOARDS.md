# Dashboards (agent-authored walls)

A dashboard is one HTML document plus a set of named queries, saved like a
saved view and rendered on a full-tab screen. You author it from the shell —
gadak has no dashboard editor — and `gadak dashboards open` focuses a running
web tab on it. The working example is
[`examples/dashboards/triage.html`](../examples/dashboards/triage.html);
this document is the contract that example follows.

```
gadak dashboards save triage --html examples/dashboards/triage.html \
  --datasource by_status=sql:select\ status_category,\ count(*)\ as\ n\ from\ issues_full\ group\ by\ 1\ order\ by\ 1 \
  --datasource mine=jql:assignee\ =\ currentUser()\ AND\ resolution\ is\ EMPTY
gadak dashboards open triage
```

Same name on save = update (upsert by name, id stable). `list`, `show`, `rm`
complete the lifecycle. Rows live in `local.db` — they survive a mirror wipe
and export/import with the workspace, and the mirror itself stays what it
always was: a disposable cache of the origin.

## The frame contract

Your HTML runs in an iframe with `sandbox="allow-scripts"` and nothing else —
no `allow-same-origin`, so the document's origin is *opaque*: it cannot read
gadak's cookies, storage, or DOM. The response carries a CSP that closes the
network to everything except inline script/style, `data:` images, and the
vendored chart library:

```
default-src 'none';
script-src 'unsafe-inline' http://127.0.0.1:7877/api/v1/dashboards/vendor/;
style-src  'unsafe-inline' http://127.0.0.1:7877/api/v1/dashboards/vendor/;
img-src data:
```

(The vendor source names the actual host the dashboard was served from; the
frame's origin is opaque, so CSP `'self'` matches nothing there.)

A dashboard that declares cached libraries (below) gets exactly one more
script source — `…/api/v1/dashboards/libs/` joined to script-src, never
style-src — and a dashboard that declares nothing keeps this policy byte for
byte. There is no variant of this header that names an external host.

Your document **never runs queries and never fetches**. There is no
`fetch()`, no `XMLHttpRequest`, no external script, no font, no image URL —
the CSP refuses all of them, before any socket is opened (measured at the
TCP layer; the e2e pins it with a loopback sink receiving zero connections).
One observability trap: devtools' network panel — and any CDP listener —
still lists a *canceled* request entry for a refused subresource, because
the browser creates the request object and then discards it. The entry is
not evidence that bytes left the machine. Queries are *registered* on the
saved row and executed by the serving page:

- **Datasource registration** happens at save time (`--datasource
  name=sql:…` or `name=jql:…`). Inline SQL inside the HTML is banned — not by
  policy but by mechanics: the document has no way to execute or send it.
- The serving page runs each datasource itself
  (`GET /api/v1/dashboards/{id}/data/{name}/`) and **pushes results in** as
  `postMessage`:

```js
window.addEventListener('message', function (ev) {
  var d = ev.data;
  if (!d || d.type !== 'data' || typeof d.name !== 'string') return;
  // d.name         — the datasource name you registered
  // d.columns      — ["status_category", "n"]  (lookup by name, never position)
  // d.rows         — [["new", 12], ["inprogress", 3], …]
  // d.truncated    — true when the row ceiling was hit
  // d.warning      — present when the query answered with a caveat, or failed
});
```

- **Backwards, you get exactly one verb.** `parent.postMessage({type:
  'refresh'}, '*')` asks the serving page to re-run every datasource and
  re-push. The host throttles this (2s floor, bursts coalesce). There is no
  `open`, no `navigate`, no URL-shaped verb, and no payload channel — unknown
  message types are logged and dropped.

### When data arrives

| trigger | what happens |
|---|---|
| open / frame load | every datasource runs, results push as they land |
| mirror delta poll (15s) | when the mirror moved, every datasource re-runs and re-pushes (≤2s) |
| your `refresh` verb | same re-run, throttled to once per 2s |
| `gadak dashboards save` on the open dashboard | the whole frame is replaced with the new document (p95 ≤1s), then the first push |

### Ceilings

One datasource answers at most **10,000 rows** and **2 MiB** of row data;
beyond that the result is truncated with `truncated: true`. The saved body
itself is capped at 8 MiB. A failed datasource does not blank the wall — it
pushes `columns: [], rows: []` with `warning: 'datasource failed: <code>'`,
so a broken card reads as a broken card, not a missing one.

## Writing queries

SQL datasources run against the mirror's `issues_full` view with **arbitrary
SQL and a read-only connection**. Read-only is an integrity boundary, not a
restriction on expressiveness: anything a SELECT can ask, you can ask — CTEs,
window functions, `json_each` over labels. What you cannot do is mutate:
writes are refused at the connection, and even if they were not, the mirror
is a cache — the next sync would roll your edit back. Writes go through the
origin (Jira) or not at all.

Query the computed columns, never display names — `status = 'In Progress'`
is silently 0 rows on a Korean account:

| instead of | use |
|---|---|
| `status = 'In Progress'` | `status_category = 'inprogress'` |
| `priority = 'High'` | `priority_rank` (0..4, lower = more urgent) |
| `issuetype = 'Bug'` | `issue_type_id` |

A display-name query still answers — with zero rows and a `warning`, so the
trap is visible on the wall. JQL datasources run through the same engine the
web UI uses; `assignee = currentUser()` and `resolution is EMPTY` behave as
in Jira. Full column reference: [`AGENTS.md`](../AGENTS.md) and
[`RECIPES.md`](RECIPES.md).

## Chart library (vendored: uPlot)

Don't reach for a CDN — outbound script hosts are not allowed, by policy and
by CSP. uPlot ships pinned inside gadak and is served from the same origin:

```html
<link rel="stylesheet" href="/api/v1/dashboards/vendor/uPlot.min.css">
<script src="/api/v1/dashboards/vendor/uPlot.iife.min.js"></script>
```

Use the **leading slash**. The document's base URL is
`/api/v1/dashboards/<id>/render/`; a relative `src` would misresolve. The
whitelist is fixed — `uPlot.iife.min.js`, `uPlot.min.css` — anything else on
that path is a 404.

uPlot loads as a classic script (the `uPlot` global). The example dashboard
(`examples/dashboards/triage.html`) charts with uPlot this way — the example
*is* the norm. Colors are explicit on every element: the frame inherits
nothing from the app shell, so set your own palette and test it against your
own background, not the host's.

## Other libraries (the lib cache)

Anything beyond uPlot — three for 3D, D3, date-fns — is **not** embedded.
Embedding it would ship ~750 KB to every user for a dashboard most never
write, and CSP would still never name an external host. Instead the user
downloads it once, on purpose, and from then on dashboards load it from
gadak's own disk:

```
gadak dashboards lib add https://cdn.jsdelivr.net/npm/three@0.149.0/build/three.min.js
```

That is the only outbound request in this feature — one `GET`, user-invoked,
to the exact URL typed. https only (plain http is refused unless the host is
localhost or an IP literal); at most 3 redirects, every hop re-checked
against the same rule; bodies over 50 MiB refused. The command prints the
evidence — url, sha384, size, and the cache path — so you can compare the
hash against an independent source before trusting the file:

```
curl -s <url> | openssl dgst -sha384 -r   # must equal the printed sha384
```

No expected hash is printed here on purpose: the procedure is the contract,
and a hash pasted into this document would rot with every upstream release.

Declare the lib when saving (the id is what `lib add` printed; repeat
`--lib` for several, order = load order, at most 8):

```
gadak dashboards save model --html model.html --lib <id>
```

Render injects `<script src="/api/v1/dashboards/libs/<id>" defer>` into the
document head for each declared lib, in order, and widens script-src with
the local libs path — nothing else. The document itself has no script tag
for the library and never names a CDN.

**One file per lib.** The cache stores one self-contained file per id;
`three@0.149.0/build/three.min.js` above is the single-file classic (UMD)
build on purpose. Modern three ships split ES-module builds
(`three.module.min.js` importing `./three.core.min.js` relatively) — a
relative import cannot resolve inside a hash-named one-file cache, so pin a
single-file build or bundle the module graph yourself before `lib add`.

The cache is disposable, like the mirror: `<profile>/dashboards/libs/` can
be deleted wholesale and repopulated with `lib add`. Re-adding the same url
with unchanged bytes is a no-op; if upstream changed, the add is refused
until `--replace` (the old id keeps serving until you re-save). `lib list`
shows the cache, `lib rm <id>` drops an entry. Before serving cached bytes,
gadak re-hashes them against the pin recorded at add time — a file modified
afterward fails that check (HTTP 500) instead of executing, and the injected
tag marks the failure on the document element
(`data-gadak-lib-error="<id>"`), which a defensive dashboard can show as a
broken card rather than a silent gap.

## Residual channels (read this before trusting the wall)

The sandbox plus CSP close script, style, image, font, frame, fetch, and
XHR traffic to every origin but the same-origin vendor and lib paths. Two
channels remain, and calling them "blocked" would be the one fatal flaw of
this document:

1. **Self-navigation URL loading.** A link or script inside the document can
   navigate the iframe *itself* to an external URL (`<a href>`,
   `location.href = …`, a form). Navigation is not a CSP-governed fetch, so
   the browser will request that URL — one outbound request, and the frame's
   contents are replaced by what comes back. Top navigation stays blocked
   (no `allow-top-navigation`): the gadak page around the frame cannot be
   moved. The opened document still cannot read anything of gadak's — it
   lands in the same opaque origin with the same empty hands.
2. **DNS prefetching.** `<link rel="dns-prefetch">` / `rel="preconnect"`
   hints are not governed by CSP; the browser may resolve the named host.
   That reveals a hostname can exist, nothing more.

Both channels can emit *a request naming a host you wrote into the
document*. Neither can carry data out: the frame holds no credentials, no
mirror rows beyond what was pushed into it, and no channel back except the
`refresh` verb. An authored dashboard is as trustworthy as its author — the
boundary is what a broken one can *take*, not what it can *say*.

## Why this shape

`gadak` is loopback single-user; a dashboard is written by the same person
who runs the serve. The threat model is not "untrusted third party crafts
HTML" — it is "an agent generates HTML on my behalf and I want the blast
radius of a mistake to be a broken wall, not an exfiltrated session." Every
choice above — opaque origin, query execution outside the frame, the
one-verb whitelist, vendor-and-libs-from-same-origin, libraries hashed at
download and re-hashed at serve — shrinks what a generated document can do
without shrinking what it can show.

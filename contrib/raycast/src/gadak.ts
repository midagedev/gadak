import { execFile } from "node:child_process";
import fs from "node:fs";

/**
 * Raycast's Node runtime does not inherit the user's shell PATH (it is
 * roughly `/usr/bin:/bin:/usr/sbin:/sbin`). A documented brew or Gadak.app
 * install therefore never shows up via PATH lookup alone.
 */
export const GADAK_CANDIDATES = [
  "/opt/homebrew/bin/gadak",
  "/usr/local/bin/gadak",
  "/Applications/Gadak.app/Contents/Resources/bin/gadak",
] as const;

export const INSTALL_COMMAND = "brew install midagedev/tap/gadak";
export const INSTALL_GUIDE_URL = "https://github.com/midagedev/gadak#install";

let cachedPath: string | undefined;
let cachedPref: string | undefined;

function present(p: string): boolean {
  try {
    fs.accessSync(p, fs.constants.X_OK);
    return true;
  } catch {
    return false;
  }
}

/** First existing path wins. Re-resolves if the cached path disappears. */
export function resolveGadakBinary(pref?: string): string | null {
  const trimmed = pref?.trim() ?? "";
  if (cachedPath && cachedPref === trimmed && present(cachedPath)) {
    return cachedPath;
  }
  cachedPath = undefined;
  cachedPref = trimmed;

  const ordered: string[] = [];
  if (trimmed) ordered.push(trimmed);
  for (const p of GADAK_CANDIDATES) {
    if (!ordered.includes(p)) ordered.push(p);
  }
  for (const p of ordered) {
    if (present(p)) {
      cachedPath = p;
      return p;
    }
  }
  return null;
}

let cachedProfilesBin: string | undefined;
let cachedProfilesStdout: string | undefined;
let profilesInflight: Promise<string> | null = null;

export function forgetResolvedGadak(): void {
  cachedPath = undefined;
  cachedProfilesBin = undefined;
  cachedProfilesStdout = undefined;
  profilesInflight = null;
}

/** Empty profile omits `/w/` (docs/DESKTOP.md: gadak://view?issue=KEY). */
function workspaceSegment(profile: string): string {
  return profile ? `/w/${encodeURIComponent(profile)}` : "";
}

export function deepLink(key: string, profile: string): string {
  // resolveView in desktop/deeplink.go: action `view`; an empty profile pref
  // must omit the /w/ segment (docs/DESKTOP.md: gadak://view?issue=KEY).
  return `gadak://view${workspaceSegment(profile)}?issue=${encodeURIComponent(key)}`;
}

export function docLink(key: string, profile: string): string {
  // Same grammar, document screen: docs/DESKTOP.md `doc=KEY`.
  return `gadak://view${workspaceSegment(profile)}?doc=${encodeURIComponent(key)}`;
}

/**
 * View deep link from a `gadak views --json` hash. The hash is already a
 * query string (`as=…&sc=…`); concatenating it must not encodeURIComponent
 * the whole thing (that would double-encode the axes). Empty hash is no
 * link — same as internal/deeplink.Compose for ActionView.
 */
export function viewLink(hash: string, profile: string): string {
  const q = hash.startsWith("?") ? hash.slice(1) : hash;
  if (!q) return "";
  return `gadak://view${workspaceSegment(profile)}?${q}`;
}

/**
 * Person panel: `person=` is account id when known, else email
 * (web/src/stores/person.svelte.ts select / #load; App.svelte bindParam).
 */
export function personLink(identity: string, profile: string): string {
  if (!identity) return "";
  return `gadak://view${workspaceSegment(profile)}?person=${encodeURIComponent(identity)}`;
}

/** Tooltip for a view whose unsupported list is non-empty (cmd/gadak/views.go). */
export function viewPartialTooltip(unsupported: string[]): string {
  const skipped = unsupported
    .map((s) => s.trim())
    .filter(Boolean)
    .join("; ");
  return skipped
    ? `Applies less than its JQL — skipped ${skipped}`
    : "Applies less than its JQL";
}

/**
 * Assemble `https://<host>/browse/<KEY>` from a profiles `site_host` and an
 * issue key. Returns null when the host cannot be normalized (empty, a
 * non-http(s) scheme, unparseable, or not a hostname). The key is always
 * `encodeURIComponent`'d so path characters cannot change the URL path.
 * Matches the web `jiraBrowseUrl` path shape and CLI `gadak open` host/key
 * join; always https, host-only — `site_host` is documented as host-only
 * (`cmd/gadak/profiles.go` siteHostOnly) but this still strips a scheme or
 * path if one is mixed in.
 */
export function jiraBrowseUrl(
  siteHost: string,
  issueKey: string,
): string | null {
  const key = issueKey.trim();
  if (!key) return null;

  const raw = siteHost.trim();
  if (!raw) return null;
  // Protocol-relative input is not a host and is not http(s).
  if (raw.startsWith("//")) return null;

  const schemeMatch = raw.match(/^([a-zA-Z][a-zA-Z0-9+.-]*):/);
  if (schemeMatch) {
    const scheme = schemeMatch[1].toLowerCase();
    if (scheme !== "http" && scheme !== "https") return null;
  }

  let host = "";
  try {
    const parsed = new URL(schemeMatch ? raw : `https://${raw}`);
    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
      return null;
    }
    host = parsed.hostname;
  } catch {
    return null;
  }

  host = host.replace(/\.+$/, "").toLowerCase();
  if (!isSafeHostname(host)) return null;

  return `https://${host}/browse/${encodeURIComponent(key)}`;
}

/** DNS-shaped hostname (or IPv4). Single labels and empty hosts are rejected. */
function isSafeHostname(host: string): boolean {
  return /^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)+$/.test(
    host,
  );
}

/**
 * Pick `site_host` from `gadak profiles --json` stdout. Returns "" (unknown)
 * for non-JSON, a missing/empty `profiles` array, a missing row, or an
 * old CLI whose rows have no `site_host` string. A named profile preference
 * is matched exactly and does not fall back to the active row (wrong-site
 * open). An empty preference uses the `active: true` row.
 */
export function siteHostFromProfiles(
  stdout: string,
  profilePref: string,
): string {
  const trimmed = stdout.trim();
  if (!trimmed) return "";
  let doc: unknown;
  try {
    doc = JSON.parse(trimmed);
  } catch {
    return "";
  }
  if (!doc || typeof doc !== "object") return "";
  const profiles = (doc as { profiles?: unknown }).profiles;
  if (!Array.isArray(profiles) || profiles.length === 0) return "";

  const rows = profiles.filter(
    (p): p is Record<string, unknown> => !!p && typeof p === "object",
  );
  const pref = profilePref.trim();
  const picked = pref
    ? rows.find((p) => p.name === pref)
    : rows.find((p) => p.active === true);
  if (!picked) return "";
  const host = picked.site_host;
  return typeof host === "string" ? host.trim() : "";
}

/** One `gadak profiles --json` per binary; pick the host in memory after that. */
export function resolveSiteHost(
  bin: string,
  profilePref: string,
): Promise<string> {
  return loadProfilesStdout(bin).then((stdout) =>
    siteHostFromProfiles(stdout, profilePref),
  );
}

function loadProfilesStdout(bin: string): Promise<string> {
  if (cachedProfilesBin === bin && cachedProfilesStdout !== undefined) {
    return Promise.resolve(cachedProfilesStdout);
  }
  if (profilesInflight && cachedProfilesBin === bin) {
    return profilesInflight;
  }
  cachedProfilesBin = bin;
  const requested = bin;
  profilesInflight = new Promise((resolve) => {
    execFile(
      bin,
      ["profiles", "--json"],
      { maxBuffer: 1024 * 1024 },
      (err, stdout) => {
        const text = err ? "" : String(stdout ?? "");
        if (cachedProfilesBin === requested) {
          cachedProfilesStdout = text;
          profilesInflight = null;
        }
        resolve(text);
      },
    );
  });
  return profilesInflight;
}

export type Issue = {
  issue_key: string;
  summary: string;
  status: string | null;
  status_category: string | null;
  assignee: string | null;
};

export type Match = { field: string; snippet: string };

export type Page = {
  key: string;
  title: string;
  space_key: string;
  author: string | null;
  updated_at: string;
  excerpt: string;
};

export type SearchOk = {
  issues: Issue[];
  pages: Page[];
  matches: Record<string, Match>;
  ms: number;
};

export type SearchFail = {
  stderr: string;
  message: string;
  code?: string | number;
};

export function isSearchFail(e: unknown): e is SearchFail {
  return typeof e === "object" && e !== null && "stderr" in e && "message" in e;
}

function firstNonEmptyLine(text: string): string {
  for (const line of text.split(/\r?\n/)) {
    const t = line.trim();
    if (t) return t;
  }
  return "";
}

/** Title for a failed gadak CLI spawn. Uses stderr when it names the problem. */
export function gadakErrorTitle(fail: SearchFail, fallback: string): string {
  const line = firstNonEmptyLine(fail.stderr);
  const lower = line.toLowerCase();
  // cmd/gadak/sql.go, mcp.go, warnIfStale: "no mirror" / never-synced.
  if (lower.includes("no mirror") || lower.includes("never finished a sync")) {
    return "no mirror yet — run `gadak init && gadak sync`";
  }
  if (line) return line;
  if (fail.code === "ENOENT") return "gadak is not installed";
  return firstNonEmptyLine(fail.message) || fallback;
}

/** Title for a failed `gadak search`. Uses stderr when it names the problem. */
export function searchErrorTitle(fail: SearchFail): string {
  return gadakErrorTitle(fail, "gadak search failed");
}

export function searchErrorDetail(fail: SearchFail): string {
  return firstNonEmptyLine(fail.stderr) || firstNonEmptyLine(fail.message);
}

export function searchErrorFull(fail: SearchFail): string {
  const body = fail.stderr.trim();
  return body || fail.message;
}

/** One row of the empty-query home: something you looked at recently. */
export type RecentVisit = {
  kind: "issue" | "page";
  key: string;
  title: string;
  status: string | null;
  viewed_at: string;
};

/** One row of the empty-query home: something that moved recently. */
export type RecentUpdate = {
  key: string;
  summary: string;
  status: string | null;
  assignee: string | null;
  updated_at: string;
};

export type RecentOk = { viewed: RecentVisit[]; updated: RecentUpdate[] };

/** `gadak sql --json` emits one JSON object per line. */
function runSQL<T>(bin: string, profile: string, query: string): Promise<T[]> {
  const args: string[] = [];
  if (profile) {
    args.push("--profile", profile);
  }
  args.push("sql", "--json", query);
  return new Promise((resolve, reject) => {
    execFile(
      bin,
      args,
      { maxBuffer: 8 * 1024 * 1024 },
      (err, stdout, stderr) => {
        if (err) {
          reject({
            stderr: String(stderr || ""),
            message: err.message,
            code: (err as NodeJS.ErrnoException).code,
          } satisfies SearchFail);
          return;
        }
        const rows: T[] = [];
        for (const line of stdout.split(/\r?\n/)) {
          const t = line.trim();
          if (!t) continue;
          try {
            rows.push(JSON.parse(t) as T);
          } catch {
            // a stray non-JSON line is a warning, not data
          }
        }
        resolve(rows);
      },
    );
  });
}

/** The empty-query home: recently viewed (local.db visits), recently updated.
 *  Both queries read the mirror only; failures degrade to empty sections. */
export async function runRecent(
  bin: string,
  profile: string,
): Promise<RecentOk> {
  const viewedQ = `
    select v.kind, v.key, max(v.viewed_at) as viewed_at,
           coalesce(i.summary, it.title) as title, i.status as status
    from local.visits v
    left join issues_full i on v.kind='issue' and i.key = v.key
    left join items it on v.kind='page' and it.kind='page' and it.key = v.key
    group by v.kind, v.key order by viewed_at desc limit 8`;
  const updatedQ = `
    select key, summary, status, assignee, updated_at
    from issues_full order by updated_at desc limit 8`;
  const [viewed, updated] = await Promise.all([
    runSQL<RecentVisit>(bin, profile, viewedQ).catch(() => [] as RecentVisit[]),
    runSQL<RecentUpdate>(bin, profile, updatedQ).catch(
      () => [] as RecentUpdate[],
    ),
  ]);
  return { viewed: viewed.filter((v) => v.title), updated };
}

export type ListedView = {
  kind: string;
  id: string;
  name: string;
  jql?: string;
  hash?: string;
  favourite?: boolean;
  owner?: string;
  applied?: string[];
  unsupported?: string[];
};

export function runViews(bin: string, profile: string): Promise<ListedView[]> {
  const args: string[] = [];
  if (profile) {
    args.push("--profile", profile);
  }
  args.push("views", "--json");
  return new Promise((resolve, reject) => {
    execFile(
      bin,
      args,
      { maxBuffer: 8 * 1024 * 1024 },
      (err, stdout, stderr) => {
        if (err) {
          reject({
            stderr: String(stderr || ""),
            message: err.message,
            code: (err as NodeJS.ErrnoException).code,
          } satisfies SearchFail);
          return;
        }
        try {
          const p = JSON.parse(stdout) as { views?: ListedView[] };
          resolve(Array.isArray(p.views) ? p.views : []);
        } catch {
          reject({
            stderr: String(stderr || stdout || ""),
            message: "gadak views --json returned a body that is not JSON",
          } satisfies SearchFail);
        }
      },
    );
  });
}

export type PersonRow = {
  name?: string | null;
  email?: string | null;
  account_id?: string | null;
};

export type Person = {
  name: string;
  identity: string;
  email: string;
};

/**
 * Identity for `person=` is account id when the row has one, else email
 * (web/src/stores/person.svelte.ts: held by account id, otherwise email;
 * #load looks up memberOf then memberOfAccountId). Rows with neither are
 * dropped — a display name is not a lookup key.
 */
export function collectPeople(rows: PersonRow[]): Person[] {
  const byId = new Map<string, Person>();
  const byEmail = new Map<string, Person>();
  const seen: Person[] = [];

  const remember = (p: Person, accountId: string, emailKey: string) => {
    if (accountId) byId.set(accountId, p);
    if (emailKey) byEmail.set(emailKey, p);
  };

  for (const row of rows) {
    const accountId = (row.account_id ?? "").trim();
    const email = (row.email ?? "").trim();
    const identity = accountId || email;
    if (!identity) continue;
    const name = (row.name ?? "").trim();
    const emailKey = email.toLowerCase();

    let p =
      (accountId ? byId.get(accountId) : undefined) ??
      (emailKey ? byEmail.get(emailKey) : undefined);
    if (!p) {
      p = { name: name || identity, identity, email };
      seen.push(p);
      remember(p, accountId, emailKey);
      continue;
    }
    if (accountId && p.identity !== accountId) {
      p.identity = accountId;
    }
    if (name && (p.name === p.identity || !p.name)) {
      p.name = name;
    }
    if (email && !p.email) {
      p.email = email;
    }
    remember(p, accountId, emailKey);
  }

  return seen.sort(
    (a, b) =>
      a.name.localeCompare(b.name, undefined, { sensitivity: "base" }) ||
      a.identity.localeCompare(b.identity),
  );
}

const PEOPLE_SQL = `
select name, email, account_id from (
  select assignee as name, assignee_email as email, assignee_id as account_id from issues
  union
  select reporter as name, reporter_email as email, reporter_id as account_id from issues
)
where coalesce(account_id, '') != '' or coalesce(email, '') != ''`;

export function runPeople(bin: string, profile: string): Promise<Person[]> {
  return runSQL<PersonRow>(bin, profile, PEOPLE_SQL).then(collectPeople);
}

export function runSearch(
  bin: string,
  profile: string,
  q: string,
): Promise<SearchOk> {
  const args: string[] = [];
  if (profile) {
    args.push("--profile", profile);
  }
  args.push("search", "--json", "--limit", "20", q);
  return new Promise((resolve, reject) => {
    const t0 = performance.now();
    execFile(
      bin,
      args,
      { maxBuffer: 32 * 1024 * 1024 },
      (err, stdout, stderr) => {
        if (err) {
          const code = (err as NodeJS.ErrnoException).code;
          reject({
            stderr: String(stderr || ""),
            message: err.message,
            code,
          } satisfies SearchFail);
          return;
        }
        try {
          const p = JSON.parse(stdout) as {
            issues?: Issue[];
            pages?: Page[];
            matches?: Record<string, Match>;
          };
          resolve({
            issues: p.issues ?? [],
            pages: p.pages ?? [],
            matches: p.matches ?? {},
            ms: performance.now() - t0,
          });
        } catch {
          reject({
            stderr: String(stderr || stdout || ""),
            message: "gadak search --json returned a body that is not JSON",
          } satisfies SearchFail);
        }
      },
    );
  });
}

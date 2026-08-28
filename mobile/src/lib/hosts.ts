// Known-host roster (GDK-1097 B1) — the first multi-host layer. Rows are
// plain localStorage metadata (label/endpoint/timestamps — nothing
// secret); tokens stay in the Keychain, one slot per host (secure.ts
// host-keyed slots). This module has no token field to leak: redaction by
// structure, not by filtering.
//
// Storage follows store.svelte.ts's guarded-cache pattern: every read and
// write is try/catch-quiet — the roster is a convenience layer, never a
// requirement. A doc whose schema this build does not speak reads as an
// empty roster and is never overwritten, so a future version's data
// survives this one.

export interface KnownHost {
  id: string
  label: string
  endpoint: string
  createdAt: string
  lastUsedAt: string
  pairingRevision: number
}

const HOSTS_KEY = 'gadak.hosts.v1'
const ACTIVE_KEY = 'gadak.hosts.active'
const SCHEMA = 1

/**
 * Roster id shape: 'local' (dev proxy adoption, endpoint '') or
 * 'paired:' + first 8 hex of the endpoint digest. Mirrored as a
 * validation regex in secure.ts and in Rust (mobile/src-tauri/src/lib.rs
 * valid_host_id) — slot strings are composed from these ids, so all three
 * must accept exactly the same set.
 */
const HOST_ID_RE = /^local$|^paired:[0-9a-f]{8}$/

export function isValidHostId(id: string): boolean {
  return HOST_ID_RE.test(id)
}

interface HostsDoc {
  schema: number
  hosts: KnownHost[]
}

function emptyDoc(): HostsDoc {
  return { schema: SCHEMA, hosts: [] }
}

function isKnownHost(value: unknown): value is KnownHost {
  if (typeof value !== 'object' || value === null) return false
  const h = value as Record<string, unknown>
  return (
    typeof h.id === 'string' &&
    typeof h.label === 'string' &&
    typeof h.endpoint === 'string' &&
    typeof h.createdAt === 'string' &&
    typeof h.lastUsedAt === 'string' &&
    typeof h.pairingRevision === 'number'
  )
}

function readDoc(): HostsDoc {
  try {
    const raw = localStorage.getItem(HOSTS_KEY)
    if (!raw) return emptyDoc()
    const parsed: unknown = JSON.parse(raw)
    if (typeof parsed !== 'object' || parsed === null) return emptyDoc()
    const doc = parsed as Record<string, unknown>
    if (doc.schema !== SCHEMA || !Array.isArray(doc.hosts)) return emptyDoc()
    return { schema: SCHEMA, hosts: doc.hosts.filter(isKnownHost) }
  } catch {
    return emptyDoc()
  }
}

function writeDoc(doc: HostsDoc): void {
  try {
    const raw = localStorage.getItem(HOSTS_KEY)
    if (raw) {
      const parsed: unknown = JSON.parse(raw)
      // A doc this build does not speak is left byte-identical — an
      // older build must not flatten a newer schema's roster.
      if (typeof parsed !== 'object' || parsed === null) return
      if ((parsed as Record<string, unknown>).schema !== SCHEMA) return
    }
    localStorage.setItem(HOSTS_KEY, JSON.stringify(doc))
  } catch {
    // Over-quota or unavailable: the roster is a convenience, never a requirement.
  }
}

/**
 * Stable id for an endpoint — re-pairing the same endpoint reuses the
 * same id, so the same Keychain slots. Normalization is deliberately
 * shallow: trim, then lowercase the URL exactly as it stands. No scheme
 * or host parsing and no default-port stripping, so the id can never
 * disagree with the endpoint string the user saw on the PairGate.
 */
export async function hostIdForEndpoint(endpoint: string): Promise<string> {
  const ep = endpoint.trim().toLowerCase()
  if (ep === '') return 'local'
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(ep))
  const hex = Array.from(new Uint8Array(digest), (b) => b.toString(16).padStart(2, '0')).join('')
  return `paired:${hex.slice(0, 8)}`
}

export function listHosts(): KnownHost[] {
  return readDoc().hosts
}

/** True once any roster doc exists — the migration's idempotence gate. */
export function hasHostsDoc(): boolean {
  try {
    return localStorage.getItem(HOSTS_KEY) !== null
  } catch {
    return false
  }
}

/**
 * Records a successful pairing against an endpoint. Re-pairing an
 * already-known endpoint bumps pairingRevision and refreshes
 * label/lastUsedAt instead of adding a row. Returns the host even when
 * persistence is unavailable — the token still lands in that host's
 * Keychain slot; the roster row is metadata.
 */
export async function upsertHostFromPairing(input: {
  endpoint: string
  label: string
}): Promise<KnownHost> {
  const id = await hostIdForEndpoint(input.endpoint)
  const now = new Date().toISOString()
  const doc = readDoc()
  const existing = doc.hosts.find((h) => h.id === id)
  if (existing) {
    existing.label = input.label
    existing.lastUsedAt = now
    existing.pairingRevision += 1
    writeDoc(doc)
    return existing
  }
  const host: KnownHost = {
    id,
    label: input.label,
    endpoint: input.endpoint.trim(),
    createdAt: now,
    lastUsedAt: now,
    pairingRevision: 1,
  }
  doc.hosts.push(host)
  writeDoc(doc)
  return host
}

/** Marks a host used (boot of an active session). Quiet no-op for unknown ids. */
export function touchHost(id: string): void {
  const doc = readDoc()
  const host = doc.hosts.find((h) => h.id === id)
  if (!host) return
  host.lastUsedAt = new Date().toISOString()
  writeDoc(doc)
}

/**
 * Drops a host row. If it was the active host, the active pointer goes
 * with it — removal owns the reset, because an active id with no row
 * would point boot at an empty slot.
 */
export function removeHost(id: string): void {
  const doc = readDoc()
  const hosts = doc.hosts.filter((h) => h.id !== id)
  if (hosts.length === doc.hosts.length) return
  writeDoc({ schema: SCHEMA, hosts })
  if (getActiveHostId() === id) setActiveHostId(null)
}

/** Active host id, or null when unset or not a well-formed id. */
export function getActiveHostId(): string | null {
  try {
    const raw = localStorage.getItem(ACTIVE_KEY)
    return raw !== null && isValidHostId(raw) ? raw : null
  } catch {
    return null
  }
}

/**
 * Points boot's token read at a host's slots. Null clears. A malformed id
 * is ignored, not stored — the injection guard that throws lives where
 * slot strings are composed (secure.ts), this side stays quiet-cache.
 */
export function setActiveHostId(id: string | null): void {
  try {
    if (id === null) localStorage.removeItem(ACTIVE_KEY)
    else if (isValidHostId(id)) localStorage.setItem(ACTIVE_KEY, id)
  } catch {
    /* convenience, never a requirement */
  }
}

/*
 * The single owner of "where this phone may dial" (GDK-1048).
 *
 * It restates the `http:default` allow list of
 * src-tauri/capabilities/default.json — including its PORT semantics, which
 * is the half a host-only check gets wrong. tauri-plugin-http matches that
 * list with URLPattern, where a pattern without a port admits the scheme's
 * default port only: `https://*.ts.net` was 443-only, so a phone paired
 * with a `tailscale serve` on 8443 passed every guard here and then had
 * each fetch refused before it left the app — reported as "cannot reach the
 * server" with zero requests in the serve log.
 *
 * Both transports consume this one predicate: the socket path via
 * assertAllowedShellEndpoint (lib/terminal/transport.ts), the fetch path
 * via the pre-dial check in request() (lib/api.ts). The two lists cannot
 * drift silently: dial-scope.test.ts reads the capability file itself and
 * asserts the verdict tables agree.
 *
 * Semantics mirror URLPattern over the entries below (measured against
 * Node 24's URLPattern, GDK-1048): `*.ts.net` matches any depth of
 * subdomain (the wildcard spans dots) but NOT the bare apex; an omitted
 * pattern port admits the default port only; `:*` admits any port,
 * default included; hostnames compare case-insensitively (the URL parser
 * lowercases them). `[::1]` is not in the list, so it is out of scope —
 * the plugin refuses it too, and the point of this module is to predict
 * exactly that refusal before the dial, not after.
 */

interface DialRule {
  /** Compared against `new URL(...).protocol` — exact, lowercase. */
  scheme: string
  /** A literal hostname, or `*.<suffix>` where the wildcard spans dots. */
  host: string
  /**
   * Port the pattern admits: `'*'` = any (default included). An omitted
   * port — the GDK-1048 trap — would mean "default port only"; no entry
   * here may carry it, and the parity test holds the line.
   */
  port: '*'
}

// Mirrors http:default in src-tauri/capabilities/default.json, entry for
// entry, so the diff against that file is eyeballable.
const ALLOW: DialRule[] = [
  { scheme: 'https:', host: '*.ts.net', port: '*' },
  { scheme: 'http:', host: '127.0.0.1', port: '*' },
  { scheme: 'http:', host: 'localhost', port: '*' },
]

/**
 * True when `endpoint` sits inside the `http:default` capability scope —
 * i.e. when a fetch to it would not be refused by the URL allowlist
 * itself. A correctness guard, not a security boundary (see
 * assertAllowedShellEndpoint); unparseable input is simply out of scope.
 */
export function inDialScope(endpoint: string): boolean {
  let u: URL
  try {
    u = new URL(endpoint)
  } catch {
    return false
  }
  const host = u.hostname.toLowerCase()
  return ALLOW.some((rule) => {
    if (u.protocol !== rule.scheme) return false
    if (rule.host.startsWith('*.')) {
      if (!host.endsWith(rule.host.slice(1))) return false
    } else if (host !== rule.host) {
      return false
    }
    return rule.port === '*'
  })
}

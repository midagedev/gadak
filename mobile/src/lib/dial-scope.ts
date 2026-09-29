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
 * Since GDK-2009 the restatement carries the PLATFORM axis too. Loopback
 * rides dev-loopback.json, whose `platforms` array excludes iOS — so the
 * corpus a target enforces is platform-dependent, and a predicate that
 * ignored the axis was a third copy saying a different thing than the two
 * native ones: on iOS it passed 127.0.0.1, the ACL refused it, and the
 * refusal surfaced as ApiError('network'). The shape now mirrors Rust's
 * endpoint_in_scope(endpoint, allow_loopback) in src-tauri/src/shell.rs —
 * one boolean axis, loopback gated on it — so the copies read side by side.
 *
 * Both transports consume this one predicate: the socket path via
 * assertAllowedShellEndpoint (lib/terminal/transport.ts), the fetch path
 * via the pre-dial check in request() (lib/api.ts). The two lists cannot
 * drift silently: dial-scope.test.ts reads the capability files themselves
 * and asserts the verdict tables agree — per platform, since the axis.
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

import { hasTauri } from './runtime'

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
// entry, so the diff against that file is eyeballable. That file has no
// `platforms` key: every target carries these.
const ALLOW: DialRule[] = [{ scheme: 'https:', host: '*.ts.net', port: '*' }]

// The loopback half mirrors dev-loopback.json — same file-for-file
// eyeballability — and is gated on the platform axis exactly where that
// file is: its `platforms` array, which excludes iOS (GDK-1581).
const ALLOW_LOOPBACK: DialRule[] = [
  { scheme: 'http:', host: '127.0.0.1', port: '*' },
  { scheme: 'http:', host: 'localhost', port: '*' },
]

/**
 * True when `endpoint` sits inside the `http:default` capability scope the
 * calling target enforces — i.e. when a fetch to it would not be refused by
 * the URL allowlist itself. `allowLoopback` is the platform arm and the
 * Rust mirror's exact shape: pass `!cfg!(target_os = "ios")` there, the
 * verdict of dev-loopback.json's `platforms` here (loopbackAllowedHere()).
 * A correctness guard, not a security boundary (see
 * assertAllowedShellEndpoint); unparseable input is simply out of scope.
 */
export function inDialScope(endpoint: string, allowLoopback: boolean): boolean {
  let u: URL
  try {
    u = new URL(endpoint)
  } catch {
    return false
  }
  const host = u.hostname.toLowerCase()
  const admits = (rule: DialRule): boolean => {
    if (u.protocol !== rule.scheme) return false
    if (rule.host.startsWith('*.')) {
      if (!host.endsWith(rule.host.slice(1))) return false
    } else if (host !== rule.host) {
      return false
    }
    return rule.port === '*'
  }
  return ALLOW.some(admits) || (allowLoopback && ALLOW_LOOPBACK.some(admits))
}

/*
 * WKWebView UA tokens for the two answers loopbackAllowedHere() gives
 * inside the packaged webview. iPadOS Safari's desktop-mode UA says
 * "Macintosh", but that is a hosted page, not this webview — hosted/dev
 * never reach the judge (their callers skip the scope check first), and
 * the app's own WKWebView reports iPhone/iPad. The non-iOS set is the UA
 * spelling of dev-loopback.json's `platforms` (macOS→Macintosh,
 * windows→"Windows NT", linux→Linux, android→Android); dial-scope.test.ts
 * pins that array, so a platforms edit and this token set move together
 * or the corpus gate goes red.
 */
const IOS_UA = /iPhone|iPad|iPod/
const NON_IOS_UA = /Android|Macintosh|Windows NT|Linux|CrOS/

/**
 * The platform arm at runtime — what Rust spells `!cfg!(target_os = "ios")`
 * at compile time. The webview cannot compile per target and cannot read
 * the capability files (they are baked into the native binary), and the
 * repo has no os plugin to ask (grep: no @tauri-apps/plugin-os in
 * package.json or Cargo.toml, no other iOS judge in mobile/src), so the user
 * agent is the one synchronous source — read at call time, like runtime.ts
 * reads the window: there is no initialization window to get wrong.
 *
 * Failure direction: inside the packaged webview the default is NARROW —
 * loopback is admitted only when the UA positively names a non-iOS OS, so
 * an unreadable or renamed UA refuses loopback, never widens it. Outside
 * the webview (dev's vite proxy, the hosted /m/ page, node tests) there is
 * no native ACL to predict at all, no production caller consults this
 * judge — their scope checks are skipped before it — and the honest answer
 * is the corpus union; the direct test callers below assert that shape.
 */
export function loopbackAllowedHere(): boolean {
  if (!hasTauri()) return true
  const ua = typeof navigator === 'undefined' ? '' : navigator.userAgent
  if (IOS_UA.test(ua)) return false
  return NON_IOS_UA.test(ua)
}

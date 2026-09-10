// The shell dial boundary (GDK-897).
//
// Until this module, the packaged shell socket rode tauri-plugin-websocket:
// a process-wide `websocket:default` grant with no URL allowlist, so the
// JS-side guards in transport.ts (inDialScope + assertPairedWsUrl) were
// correctness guards, not a boundary — anything running in the webview
// could call the plugin directly and dial any host on the internet. The
// plugin is gone now; the socket is dialled HERE, from commands whose
// only dial input is a session id: the URL is built from the pairing this
// process stored at pair time (shell_pair_set), validated against the
// same scope capabilities/default.json enforces for http, and given its
// Bearer by reading the terminal token out of secure storage — the JS
// side never names a URL and never sees the header.
//
// The wire the webview sees is unchanged on purpose: events on `shell-ws`
// carry the plugin's message shapes ({type:"Binary",data:[…]} /
// {type:"Text",data:"…"} / {type:"Close"}), so transport.ts's parsing —
// and its unit tests — did not move. The seam is the plugin's listener
// API; only the thing behind it changed.
use std::sync::Mutex;

use futures_util::{SinkExt, StreamExt};
use serde::{Deserialize, Serialize};
use tauri::{AppHandle, Emitter, Url};
use tauri_plugin_secure_storage::SecureStorageExt;
use tokio::sync::mpsc;
use tokio_tungstenite::connect_async;
use tokio_tungstenite::tungstenite::Message;
use tokio_tungstenite::tungstenite::client::IntoClientRequest;
use tokio_tungstenite::tungstenite::http::HeaderValue;

use crate::{token_options, token_slot};

const EVENT: &str = "shell-ws";
const OUT_OF_SCOPE: &str = "shell endpoint is outside the app dialling scope";
const NO_PAIRING: &str = "no terminal pairing is armed";
const NOT_OPEN: &str = "shell socket is not open";
const BAD_SESSION: &str = "invalid shell session id";

/// What the shell may dial, one function for both transports (this is the
/// Rust mirror of src/lib/dial-scope.ts, which itself restates the
/// `http:default` allow list of capabilities/default.json — see that file
/// for the port-semantics history). `allow_loopback` is the platform arm:
/// loopback rides dev-loopback.json, whose `platforms` list excludes iOS,
/// so callers pass `!cfg!(target_os = "ios")` and the App Store binary
/// refuses 127.0.0.1 the same day the capability file does.
///
/// Like the JS predicate this is exact about the near misses: `*.ts.net`
/// means any depth of subdomain but NOT the bare apex, and the suffix trick
/// (`ts.net.evil.example`) fails because the dot is part of the match.
fn endpoint_in_scope(endpoint: &str, allow_loopback: bool) -> bool {
    let Ok(u) = Url::parse(endpoint) else {
        return false;
    };
    let Some(host) = u.host_str() else {
        return false;
    };
    let host = host.to_ascii_lowercase();
    match u.scheme() {
        // Any port, default included — the `:*` in the capability entry.
        "https" => host.ends_with(".ts.net"),
        "http" => allow_loopback && (host == "127.0.0.1" || host == "localhost"),
        _ => false,
    }
}

/// The serve's session ids are hex (internal/term manager.go newID), but
/// the id is about to become a URL path segment, so the bar is "path-safe
/// unreserved", not "looks hex" — anything that could splice a second
/// segment (`/`, `?`, `#`), a percent escape, or control bytes is refused
/// here rather than trusted to the builder below.
fn valid_session_id(id: &str) -> bool {
    !id.is_empty() && id.bytes().all(|b| b.is_ascii_alphanumeric() || matches!(b, b'-' | b'.' | b'_' | b'~'))
}

/// `<endpoint>/api/v1/terminal/sessions/<id>/ws/` with http→ws, https→wss —
/// the same URL src/lib/terminal/transport.ts shellWsUrl builds for the dev
/// branch. Host, port and scheme come from the stored pairing; the only
/// caller-supplied part is the charset-validated id, so the URL cannot
/// leave the scope the pairing was admitted in.
fn shell_ws_url(endpoint: &str, session_id: &str) -> Result<String, String> {
    let base = Url::parse(endpoint).map_err(|_| OUT_OF_SCOPE.to_string())?;
    let scheme = match base.scheme() {
        "https" => "wss",
        "http" => "ws",
        _ => return Err(OUT_OF_SCOPE.to_string()),
    };
    let authority = match base.port() {
        Some(p) => format!("{}:{}", base.host_str().unwrap_or_default(), p),
        None => base.host_str().unwrap_or_default().to_string(),
    };
    Ok(format!("{scheme}://{authority}/api/v1/terminal/sessions/{session_id}/ws/"))
}

/// The pairing this process dials for — RAM only, on purpose: it is armed
/// from the same persisted meta + token the Shell tab itself needs
/// (loadTerminal at boot, pairTerminal on scan), so the native half can
/// never point somewhere the phone's own pairing state does not.
struct ShellPairing {
    endpoint: String,
    /// Roster host id keying the terminal token's secure-storage slot.
    host: Option<String>,
}

enum Outgoing {
    Bytes(Vec<u8>),
    Text(String),
}

static PAIRING: Mutex<Option<ShellPairing>> = Mutex::new(None);
static SENDER: Mutex<Option<mpsc::Sender<Outgoing>>> = Mutex::new(None);

/// Events to the webview, in the plugin's message shapes so transport.ts
/// parses them exactly as it parsed the plugin (its `data` on Close was
/// the plugin's close frame; the transport reads only `type` there).
#[derive(Clone, Serialize)]
#[serde(tag = "type", rename_all = "PascalCase")]
enum ShellEvent {
    Binary { data: Vec<u8> },
    Text { data: String },
    Close,
}

/// JS-visible outgoing message: PTY bytes as a number array, control JSON
/// (resize) as a string — the shapes NativeWs.send already produced. pub
/// because the command's generated handler names the type.
#[derive(Deserialize)]
#[serde(untagged)]
pub enum ShellOutgoing {
    Bytes(Vec<u8>),
    Text(String),
}

/// Arms the pairing the shell commands dial. Called by the store on the
/// two roads that arm the Shell tab (pairTerminal, and loadTerminal's
/// stored read at boot / host switch) — an endpoint outside the dial
/// scope is refused here, before any socket exists, and re-arming closes
/// any socket a previous pairing had open.
#[tauri::command]
pub async fn shell_pair_set(endpoint: String, host: Option<String>) -> Result<(), String> {
    if !endpoint_in_scope(&endpoint, !cfg!(target_os = "ios")) {
        return Err(OUT_OF_SCOPE.into());
    }
    close_socket();
    *PAIRING.lock().unwrap() = Some(ShellPairing { endpoint, host });
    Ok(())
}

/// Dials the paired endpoint's terminal route. The session id is the only
/// dial input the webview ever supplies; everything else — URL, scope
/// verdict, Bearer — is decided here from the pairing + secure storage.
#[tauri::command]
pub async fn shell_ws_connect(app: AppHandle, session_id: String) -> Result<(), String> {
    if !valid_session_id(&session_id) {
        return Err(BAD_SESSION.into());
    }
    let pairing = PAIRING
        .lock()
        .unwrap()
        .as_ref()
        .map(|p| (p.endpoint.clone(), p.host.clone()))
        .ok_or_else(|| NO_PAIRING.to_string())?;
    if !endpoint_in_scope(&pairing.0, !cfg!(target_os = "ios")) {
        return Err(OUT_OF_SCOPE.into());
    }
    let url = shell_ws_url(&pairing.0, &session_id)?;

    // The Bearer is read, not passed: the token's only JS-visible door is
    // token_get, and the header must never ride an invoke argument.
    // None is a real state — the serve's local rule admits loopback with
    // no token at all (terminal.go terminalLocal), and dev-loopback is
    // where that combination lives.
    let token = match token_slot(Some("terminal"), pairing.1.as_deref()) {
        Ok(key) => app
            .secure_storage()
            .get_item(app.clone(), token_options(&key))
            .map(|r| r.data)
            .map_err(|e| e.to_string())?,
        Err(_) => None,
    };

    let mut request = url
        .as_str()
        .into_client_request()
        .map_err(|e| e.to_string())?;
    if let Some(t) = token {
        let value = HeaderValue::from_str(&format!("Bearer {t}"))
            .map_err(|_| "shell auth header is not valid".to_string())?;
        request.headers_mut().insert("Authorization", value);
    }

    // One socket at a time — the pane attaches one session, and a new
    // connect replaces whatever came before it.
    close_socket();

    let (ws, _resp) = connect_async(request).await.map_err(|e| e.to_string())?;
    let (tx, mut rx) = mpsc::channel::<Outgoing>(64);
    *SENDER.lock().unwrap() = Some(tx.clone());

    let task_app = app.clone();
    tauri::async_runtime::spawn(async move {
        let (mut sink, mut stream) = ws.split();
        loop {
            tokio::select! {
                outgoing = rx.recv() => match outgoing {
                    // Every sender dropped (close) — a clean shutdown.
                    None => break,
                    Some(Outgoing::Bytes(b)) => {
                        if sink.send(Message::Binary(b.into())).await.is_err() {
                            break;
                        }
                    }
                    Some(Outgoing::Text(t)) => {
                        if sink.send(Message::Text(t.into())).await.is_err() {
                            break;
                        }
                    }
                },
                frame = stream.next() => match frame {
                    None | Some(Err(_)) => break,
                    Some(Ok(Message::Binary(b))) => {
                        let _ = task_app.emit(EVENT, ShellEvent::Binary { data: b.to_vec() });
                    }
                    Some(Ok(Message::Text(t))) => {
                        let _ = task_app.emit(EVENT, ShellEvent::Text { data: t.to_string() });
                    }
                    // Tungstenite queues the Pong itself; it flushes on the
                    // next write, and a shell is never long without one.
                    Some(Ok(Message::Ping(_))) | Some(Ok(Message::Pong(_))) => {}
                    Some(Ok(Message::Close(_))) => break,
                    Some(Ok(Message::Frame(_))) => {}
                },
            }
        }
        let _ = sink.close().await;
        let _ = task_app.emit(EVENT, ShellEvent::Close);
        // Retire the registry only if it is still ours — a newer connect
        // has already replaced it, and its Close was the stale socket's.
        let mut slot = SENDER.lock().unwrap();
        if slot.as_ref().is_some_and(|current| current.same_channel(&tx)) {
            *slot = None;
        }
    });
    Ok(())
}

/// Sends PTY bytes (number array) or a control frame (string) on the open
/// socket. Not-open is an error the JS adapter swallows, matching the
/// plugin's no-op send on a dead socket.
#[tauri::command]
pub async fn shell_ws_send(message: ShellOutgoing) -> Result<(), String> {
    let outgoing = match message {
        ShellOutgoing::Bytes(b) => Outgoing::Bytes(b),
        ShellOutgoing::Text(t) => Outgoing::Text(t),
    };
    // Clone the sender out of the lock before awaiting — a MutexGuard held
    // across .await makes the command's future !Send and tauri refuses it.
    let tx = SENDER
        .lock()
        .unwrap()
        .as_ref()
        .ok_or_else(|| NOT_OPEN.to_string())?
        .clone();
    tx.send(outgoing).await.map_err(|_| NOT_OPEN.to_string())
}

/// Closes the open socket, if any. The spawned task sees every sender
/// dropped, closes the stream, and emits the Close event the pane's
/// reconnect path already knows.
#[tauri::command]
pub async fn shell_ws_close() -> Result<(), String> {
    close_socket();
    Ok(())
}

fn close_socket() {
    *SENDER.lock().unwrap() = None;
}

#[cfg(test)]
mod tests {
    use super::*;

    // The JS predicate's own table (src/lib/dial-scope.test.ts), mirrored
    // verdict for verdict: the two transports must not drift, and the
    // near misses are the ones a host-only check gets wrong.
    #[test]
    fn ts_net_https_any_port_is_in_scope() {
        assert!(endpoint_in_scope("https://home.example.ts.net", false));
        assert!(endpoint_in_scope("https://home.example.ts.net:8443", false));
        assert!(endpoint_in_scope("https://deep.home.example.ts.net:443", false));
        assert!(endpoint_in_scope("https://HOME.EXAMPLE.TS.NET", false));
    }

    #[test]
    fn the_ts_net_near_misses_are_refused() {
        // Bare apex, suffix trick, and look-alikes — all outside.
        assert!(!endpoint_in_scope("https://ts.net", false));
        assert!(!endpoint_in_scope("https://ts.net.evil.example", false));
        assert!(!endpoint_in_scope("https://notts.net.example", false));
        assert!(!endpoint_in_scope("https://example.com", false));
        // Plaintext to a tailnet name is http, and http is loopback-only.
        assert!(!endpoint_in_scope("http://home.example.ts.net", false));
        // A ws:// URL is not an endpoint — the transport's own rule.
        assert!(!endpoint_in_scope("ws://home.example.ts.net", false));
        assert!(!endpoint_in_scope("file:///etc/passwd", false));
        assert!(!endpoint_in_scope("not a url", false));
        assert!(!endpoint_in_scope("", false));
    }

    // Loopback is the platform arm: dev-loopback.json excludes iOS, so the
    // same phone admits 127.0.0.1 in development and refuses it in the App
    // Store binary. Both arms are tested on every host — the flag is an
    // argument precisely so the iOS arm is testable off-device.
    #[test]
    fn loopback_http_is_the_non_ios_arm() {
        for ep in ["http://127.0.0.1:7899", "http://localhost:5180", "http://127.0.0.1"] {
            assert!(endpoint_in_scope(ep, true), "{ep} must be in scope off-iOS");
            assert!(!endpoint_in_scope(ep, false), "{ep} must be refused on iOS");
        }
        // https loopback is still no: the rule is scheme+host together.
        assert!(!endpoint_in_scope("https://127.0.0.1:7899", true));
        // IPv6 loopback is not in the capability list either.
        assert!(!endpoint_in_scope("http://[::1]:7899", true));
    }

    #[test]
    fn ws_url_maps_scheme_and_keeps_host_and_port() {
        assert_eq!(
            shell_ws_url("https://home.example.ts.net", "abc123").unwrap(),
            "wss://home.example.ts.net/api/v1/terminal/sessions/abc123/ws/",
        );
        assert_eq!(
            shell_ws_url("http://127.0.0.1:7899/", "abc123").unwrap(),
            "ws://127.0.0.1:7899/api/v1/terminal/sessions/abc123/ws/",
        );
        // Only http(s) endpoints map — anything else is out of scope, and
        // the builder refuses before a URL is ever formed.
        assert!(shell_ws_url("ftp://home.example.ts.net", "abc123").is_err());
    }

    #[test]
    fn session_ids_are_path_safe_or_refused() {
        assert!(valid_session_id("0123456789abcdef0123456789abcdef"));
        assert!(valid_session_id("sess-1"));
        for bad in [
            "", "a/b", "../x", "a?b", "a#b", "a%2f", "a b", "a\tb",
        ] {
            assert!(!valid_session_id(bad), "{bad:?} must not reach a URL");
        }
    }
}

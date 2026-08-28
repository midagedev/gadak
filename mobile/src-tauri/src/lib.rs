// gadak mobile — native skeleton (GDK-800 A-native).
//
// Transport: tauri-plugin-http gives the webview a native HTTP client
// (reqwest — no Origin header, no CORS preflight). The serve gate answers
// webview fetch with forbidden_origin and serves no CORS headers, so native
// HTTP is the only packaged-app path (docs/decisions/0003). Its URL scope
// lives in capabilities/default.json. tauri-plugin-websocket is the same
// split for PTY bytes: a webview WebSocket cannot set Authorization and
// its origin is not the serve's. The plugin has no URL allowlist, and
// src/lib/terminal/transport.ts only restates one in JS — a correctness
// guard, not a boundary, because the grant is process-wide (GDK-897).
//
// Secure storage: tauri-plugin-secure-storage (community — iOS Keychain /
// Android Keystore / desktop keyring). The app does not use the plugin's
// JS API: the token_* commands below are the only door to the token, so
// swapping the backend (e.g. for an official keychain plugin when one
// ships) means editing this file alone.
//
// barcode-scanner and notification are registered for their later UI
// chunks (A-pair QR scan, A-feed local notifications); nothing calls them
// yet. The camera usage string rides in Info.ios.plist.
use tauri_plugin_secure_storage::{OptionsRequest, SecureStorageExt};

/// Secure-store entry for the serve-scope pairing token. Frozen: renaming
/// it silently unpairs every already-paired phone. endpoint/label are not
/// secrets and stay in localStorage (mobile/src/lib/store.svelte.ts
/// META_KEY).
const TOKEN_KEY_SERVE: &str = "pairing-token";
/// Second slot: a Bearer whose pairing scope is exactly `terminal`.
const TOKEN_KEY_TERMINAL: &str = "pairing-token-terminal";

/// Resolves the Keychain key for a token kind. Missing kind is serve so
/// existing `invoke('token_get')` call sites keep working. Unknown kind
/// is an error, not a silent default.
fn token_key(kind: Option<&str>) -> Result<&'static str, String> {
    match kind {
        None | Some("serve") => Ok(TOKEN_KEY_SERVE),
        Some("terminal") => Ok(TOKEN_KEY_TERMINAL),
        Some(_) => Err("unknown token kind".into()),
    }
}

/// Roster host ids (mobile/src/lib/hosts.ts): the dev-proxy 'local', or
/// 'paired:' + 8 lowercase hex. Lockstep with the TS-side regex in
/// secure.ts — same posture as token_key: anything else is an error, so a
/// crafted id can never splice arbitrary text into a Keychain key.
fn valid_host_id(host: &str) -> bool {
    if host == "local" {
        return true;
    }
    match host.strip_prefix("paired:") {
        Some(hex) => {
            hex.len() == 8
                && hex.chars().all(|c| c.is_ascii_digit() || ('a'..='f').contains(&c))
        }
        None => false,
    }
}

/// Resolves the Keychain key, optionally host-keyed (GDK-1097 B1): no
/// host → the frozen legacy key (the address every pre-B1 phone's pairing
/// lives at); a valid roster host id → "<key>@<hostId>". An invalid host
/// id is an error, never a silent fallback to the legacy slot.
fn token_slot(kind: Option<&str>, host: Option<&str>) -> Result<String, String> {
    let base = token_key(kind)?;
    match host {
        None => Ok(base.to_string()),
        Some(h) if valid_host_id(h) => Ok(format!("{base}@{h}")),
        Some(_) => Err("unknown host id".into()),
    }
}

/// Built through serde so this compiles regardless of the plugin's field
/// visibility; keys are the camelCase wire names of its OptionsRequest.
fn token_options(key: &str) -> OptionsRequest {
    serde_json::from_value(serde_json::json!({
        "prefixedKey": key,
        "sync": false,
        // 1 = whenUnlockedThisDeviceOnly (the plugin's KeychainAccess enum):
        // readable only while unlocked, never synced through iCloud
        // Keychain. Desktop (OS keyring) ignores this field.
        "keychainAccess": 1,
    }))
    .expect("static token options must deserialize")
}

/// Reads a pairing token; None when that slot was never written.
#[tauri::command]
async fn token_get(
    app: tauri::AppHandle,
    kind: Option<String>,
    host: Option<String>,
) -> Result<Option<String>, String> {
    let key = token_slot(kind.as_deref(), host.as_deref())?;
    app.secure_storage()
        .get_item(app.clone(), token_options(&key))
        .map(|r| r.data)
        .map_err(|e| e.to_string())
}

/// Stores a pairing token (Keychain grade on device).
#[tauri::command]
async fn token_set(
    app: tauri::AppHandle,
    token: String,
    kind: Option<String>,
    host: Option<String>,
) -> Result<(), String> {
    let key = token_slot(kind.as_deref(), host.as_deref())?;
    let opts = serde_json::from_value(serde_json::json!({
        "prefixedKey": key,
        "sync": false,
        "keychainAccess": 1,
        "data": token,
    }))
    .map_err(|e| e.to_string())?;
    app.secure_storage()
        .set_item(app.clone(), opts)
        .map(|_| ())
        .map_err(|e| e.to_string())
}

/// Deletes a pairing token (unpair).
#[tauri::command]
async fn token_del(
    app: tauri::AppHandle,
    kind: Option<String>,
    host: Option<String>,
) -> Result<(), String> {
    let key = token_slot(kind.as_deref(), host.as_deref())?;
    app.secure_storage()
        .remove_item(app.clone(), token_options(&key))
        .map(|_| ())
        .map_err(|e| e.to_string())
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let builder = tauri::Builder::default()
        .plugin(tauri_plugin_http::init())
        .plugin(tauri_plugin_websocket::init())
        .plugin(tauri_plugin_secure_storage::init())
        .plugin(tauri_plugin_notification::init())
        .invoke_handler(tauri::generate_handler![token_get, token_set, token_del]);

    // Mobile-only crate (its lib is #![cfg(mobile)] — an empty shell on
    // desktop), so the registration itself is mobile-gated.
    #[cfg(mobile)]
    let builder = builder.plugin(tauri_plugin_barcode_scanner::init());

    builder
        .setup(|_app| {
            // iOS: wry leaves the WKWebView's scroll view on .automatic
            // inset adjustment, so UIKit shrinks the layout viewport by the
            // safe areas while the page pays env(safe-area-inset-*) again —
            // measured on the simulator: innerHeight 778 vs screen 874 with
            // env top 62 / bottom 34 reported at the same time. The page is
            // the single owner of the insets (mobile/src/app.css .safe-top/
            // .safe-bottom), so the shell must go full-bleed.
            #[cfg(target_os = "ios")]
            {
                use tauri::Manager;
                if let Some(webview) = _app.get_webview_window("main") {
                    webview.with_webview(|pv| unsafe {
                        use objc2::runtime::AnyObject;
                        let wk = pv.inner() as *mut AnyObject;
                        let scroll: *mut AnyObject = objc2::msg_send![&*wk, scrollView];
                        // 2 = UIScrollViewContentInsetAdjustmentNever
                        let _: () = objc2::msg_send![&*scroll, setContentInsetAdjustmentBehavior: 2isize];
                    })?;
                }
            }
            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}

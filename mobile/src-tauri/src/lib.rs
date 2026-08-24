// gadak mobile — native skeleton (GDK-800 A-native).
//
// Transport: tauri-plugin-http gives the webview a native HTTP client
// (reqwest — no Origin header, no CORS preflight). The serve gate answers
// webview fetch with forbidden_origin and serves no CORS headers, so native
// HTTP is the only packaged-app path (docs/decisions/0003). Its URL scope
// lives in capabilities/default.json.
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

/// Secure-store entry for the pairing token. endpoint/label are not
/// secrets and stay in localStorage (mobile/src/lib/settings.ts).
const TOKEN_KEY: &str = "pairing-token";

/// Built through serde so this compiles regardless of the plugin's field
/// visibility; keys are the camelCase wire names of its OptionsRequest.
fn token_options() -> OptionsRequest {
    serde_json::from_value(serde_json::json!({
        "prefixedKey": TOKEN_KEY,
        "sync": false,
        // 1 = whenUnlockedThisDeviceOnly (the plugin's KeychainAccess enum):
        // readable only while unlocked, never synced through iCloud
        // Keychain. Desktop (OS keyring) ignores this field.
        "keychainAccess": 1,
    }))
    .expect("static token options must deserialize")
}

/// Reads the pairing token; None when never paired.
#[tauri::command]
async fn token_get(app: tauri::AppHandle) -> Result<Option<String>, String> {
    app.secure_storage()
        .get_item(app.clone(), token_options())
        .map(|r| r.data)
        .map_err(|e| e.to_string())
}

/// Stores the pairing token (Keychain grade on device).
#[tauri::command]
async fn token_set(app: tauri::AppHandle, token: String) -> Result<(), String> {
    let opts = serde_json::from_value(serde_json::json!({
        "prefixedKey": TOKEN_KEY,
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

/// Deletes the pairing token (unpair).
#[tauri::command]
async fn token_del(app: tauri::AppHandle) -> Result<(), String> {
    app.secure_storage()
        .remove_item(app.clone(), token_options())
        .map(|_| ())
        .map_err(|e| e.to_string())
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let builder = tauri::Builder::default()
        .plugin(tauri_plugin_http::init())
        .plugin(tauri_plugin_secure_storage::init())
        .plugin(tauri_plugin_notification::init())
        .invoke_handler(tauri::generate_handler![token_get, token_set, token_del]);

    // Mobile-only crate (its lib is #![cfg(mobile)] — an empty shell on
    // desktop), so the registration itself is mobile-gated.
    #[cfg(mobile)]
    let builder = builder.plugin(tauri_plugin_barcode_scanner::init());

    builder
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}

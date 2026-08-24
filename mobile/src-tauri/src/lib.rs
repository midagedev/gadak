// gadak mobile — desktop/dev entry. This is the stock create-tauri-app v2
// builder: no custom commands, no plugins. The queue/pair screens are pure
// webview against the vite bundle; the only network destination is the
// configured serve endpoint (see mobile/src/lib/api.ts). When a native
// capability lands (Keychain, QR camera, local notifications — see the
// plugin gap map in the round report), its init call is added here and the
// deviation from the template gets a comment explaining why.
#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}

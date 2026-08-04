/// <reference types="svelte" />
/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Set to "1" only for the zero-install hosted demo build. */
  readonly VITE_HOSTED_DEMO?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}

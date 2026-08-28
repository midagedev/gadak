/*
 * Settings dialog tab ids. Owned here so lib/integrations.ts can type
 * DESKTOP_ONLY_SETTINGS_TABS without importing the .svelte module (the
 * unit vitest project has no svelte plugin).
 */

export const SETTINGS_TABS = [
  'sync',
  'sources',
  'features',
  'groups',
  'members',
  'fields',
  'workspaces',
  'integrations',
  'devices',
  'about',
] as const

export type SettingsTab = (typeof SETTINGS_TABS)[number]

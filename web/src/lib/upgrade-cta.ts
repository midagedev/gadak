/*
 * Single owner of "what does a person do to upgrade this install" (GDK-216).
 *
 * Banner, notes dialog, and Settings consume this. Adding a package path
 * (AUR, Scoop) means adding a row here — not another os === branch in a
 * surface. The app never downloads or installs; a command, when present,
 * is something the person copies and runs.
 *
 * linux / windows are intentionally absent: AUR is unpublished (GDK-115),
 * Scoop was out of scope (GDK-209). An invented command would be a lie.
 * Empty os is static export / hosted demo / an older server.
 */

export type UpgradeCta = {
  /** Shell command to copy, or null when this OS has no package path. */
  command: string | null
}

const COMMAND_BY_OS: Record<string, string> = {
  darwin: 'brew upgrade --cask gadak',
}

export function upgradeCta(os: string): UpgradeCta {
  return { command: COMMAND_BY_OS[os] ?? null }
}

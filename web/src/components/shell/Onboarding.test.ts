/*
 * GDK-71: desktop first-run (and replace-token) opens the official token URL
 * in the in-app browse pane and focuses the paste field. Serve/hosted keep
 * target="_blank". No component-mount harness — vitest is environment:'node'
 * and importing .svelte fails (FeaturesTab.test.ts). The click path is also
 * driven in e2e/onboarding.spec.ts against pretendDesktop + mockBrowseRoutes.
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'

const HERE = dirname(fileURLToPath(import.meta.url))
const ONBOARDING = join(HERE, 'Onboarding.svelte')
const JIRA_KEY = join(HERE, '../write/JiraKeySettings.svelte')
const DESKTOP_LINKS = join(HERE, '../../lib/desktop-links.ts')

const OTHER_TAB = "{ inApp: true, kind: 'other', key: null }"

describe('GDK-71 desktop token page in the browse pane', () => {
  const onboarding = readFileSync(ONBOARDING, 'utf8')
  const replaceToken = readFileSync(JIRA_KEY, 'utf8')
  const links = readFileSync(DESKTOP_LINKS, 'utf8')

  test('openInAppBrowser is the existing pane-open entry, not a new POST path', () => {
    expect(links).toMatch(/export async function openInAppBrowser/)
    expect(links).toMatch(/fetch\('\/desktop\/browse'/)
    expect(links).toMatch(/browse\.adopt\(/)
  })

  test('onboarding: desktop click opens the pane and focuses the token input', () => {
    expect(onboarding).toMatch(/surface\(\) === 'desktop'/)
    expect(onboarding).toMatch(/openInAppBrowser\(TOKEN_URL, \{ inApp: true, kind: 'other', key: null \}/)
    expect(onboarding).toMatch(/tokenEl\?\.focus\(\)/)
    expect(onboarding).toMatch(/event\.preventDefault\(\)/)
    expect(onboarding).toMatch(/t\('browse\.openExternal'\)/)
    // Serve/hosted still use the same anchor; the handler no-ops off desktop.
    expect(onboarding).toMatch(/href=\{TOKEN_URL\}/)
    expect(onboarding).toMatch(/target="_blank"/)
  })

  test('replace-token stays on the system browser — the dialog hides the pane', () => {
    // JiraKeySettings is a dialog, and the browse stack hides the native
    // webview while any dialog is up (browse-stack.ts: nativeVisible =
    // paneOpen && !dialogOpen — the GDK-76/77 stacking contract). An in-app
    // open from inside that dialog paints nothing the user can see, so the
    // token link there must keep target="_blank". Onboarding is not a
    // dialog; only that surface intercepts (GDK-71).
    expect(replaceToken).not.toMatch(/openInAppBrowser/)
    expect(replaceToken).toMatch(/href=\{API_TOKEN_URL\}/)
    expect(replaceToken).toMatch(/target="_blank"/)
  })

  test('kind other matches the existing GitHub in-app tab classification', () => {
    expect(links).toContain(OTHER_TAB)
    expect(onboarding).toContain(OTHER_TAB)
  })
})

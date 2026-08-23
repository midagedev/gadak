import { expect, test } from 'vitest'
import { hardcodedE2EHosts } from './helpers'

/*
 * GDK-672: e2e/*.spec.ts pinning 127.0.0.1:7877 serializes every worktree onto
 * one port. apiURL() / e2eServePort() in helpers.ts is the owner.
 *
 * demo/ hosted/ perf/ are not scanned: they are other suites
 * (playwright.config testIgnore), same split as no-bare-timeout.unit.ts.
 */

test('e2e/*.spec.ts does not hardcode 127.0.0.1:7877', () => {
  const hits = hardcodedE2EHosts()
  expect(
    hits,
    `hardcoded 127.0.0.1:7877 — use apiURL() from e2e/helpers.ts:\n${hits.join('\n')}`,
  ).toEqual([])
})

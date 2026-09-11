import { expect, test } from 'vitest'
import { builtinServePort, e2eServePort, linearServePort } from './helpers'

/*
 * GDK-1789: the built-in spec's port is owned, never derived. The derivation
 * (suite port + 1) is what let a parallel round on the neighbouring port
 * silently take it — the spec's serve failed to bind and its healthz poll
 * adopted the impostor. This pins the two halves of the fix that are pure
 * functions of the environment: an explicit pin is honoured (and cannot name
 * the suite's own ports), and the default is a free ephemeral port the suite
 * is not sitting on. The identity half of the fix — the poll refusing a
 * healthy impostor — is the live spec's job (built-in-attachments.spec.ts
 * pollServeIdentity), proven by its FAIL-first repro.
 */

const stash: Record<string, string | undefined> = {}
const PINNED = ['GADAK_E2E_BUILTIN_PORT', 'GADAK_E2E_PORT', 'GADAK_E2E_LINEAR_PORT']

test.beforeEach(() => {
  for (const name of PINNED) {
    stash[name] = process.env[name]
    delete process.env[name]
  }
})

test.afterEach(() => {
  for (const name of PINNED) {
    if (stash[name] === undefined) delete process.env[name]
    else process.env[name] = stash[name]
  }
})

test('GADAK_E2E_BUILTIN_PORT pins the port and refuses the suite’s other two', async () => {
  process.env.GADAK_E2E_BUILTIN_PORT = '8099'
  expect(await builtinServePort()).toBe('8099')

  process.env.GADAK_E2E_BUILTIN_PORT = e2eServePort()
  await expect(builtinServePort()).rejects.toThrow(/must differ from the suite's other ports/)

  process.env.GADAK_E2E_BUILTIN_PORT = linearServePort()
  await expect(builtinServePort()).rejects.toThrow(/must differ from the suite's other ports/)

  process.env.GADAK_E2E_BUILTIN_PORT = 'not-a-port'
  await expect(builtinServePort()).rejects.toThrow(/GADAK_E2E_BUILTIN_PORT must be an integer/)

  process.env.GADAK_E2E_BUILTIN_PORT = '70000'
  await expect(builtinServePort()).rejects.toThrow(/out of range/)
})

test('the default is a free ephemeral port the suite itself is not on', async () => {
  // Several grabs: each binds 127.0.0.1:0 and closes, so consecutive calls
  // may repeat — what must never happen is answering one of the suite's
  // occupied ports. That is the retry loop's whole contract; forcing the
  // kernel to actually offer a taken port is not possible from outside, so
  // the loop itself is pinned by the grabs never landing on one.
  for (let i = 0; i < 8; i++) {
    const port = await builtinServePort()
    expect(port).toMatch(/^[1-9][0-9]*$/)
    expect(Number(port)).toBeLessThanOrEqual(65535)
    expect([e2eServePort(), linearServePort()], `grab ${i} landed on a suite port`).not.toContain(port)
  }
})

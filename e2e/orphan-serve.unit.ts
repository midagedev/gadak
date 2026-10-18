import { spawn } from 'node:child_process'
import { describe, expect, test } from 'vitest'
// @ts-expect-error plain ESM shared with port-held.mjs, no declaration file
import { classifyListener, describePid, reapAbandonedServe } from './orphan-serve.mjs'

type Row = { pid: number; ppid: number; command: string }
const verdict = (r: Row): boolean => classifyListener(r).abandoned

const ROOT_SERVE =
  '/Users/x/repo/gadak-wt/e2e/.tmp/gadak-7911 serve --addr 127.0.0.1:7911 --static dist/app --no-open --no-sync'
const PHONE_API = '/var/folders/ab/T/gadak-mobile-api-7899 demo --addr 127.0.0.1:7899'
const PHONE_UI =
  'node /Users/x/repo/gadak/mobile/node_modules/.bin/vite preview --config /Users/x/repo/gadak/mobile/vite.config.ts --outDir /var/folders/T/gadak-mobile-gate-5182 --port 5182 --strictPort --host 127.0.0.1'

describe('classifyListener (GDK-1951)', () => {
  test('ppid 1 + root gate serve is abandoned', () => {
    expect(verdict({ pid: 91452, ppid: 1, command: ROOT_SERVE })).toBe(true)
  })
  test('ppid 1 + phone gate demo / vite preview is abandoned', () => {
    expect(verdict({ pid: 2, ppid: 1, command: PHONE_API })).toBe(true)
    expect(verdict({ pid: 3, ppid: 1, command: PHONE_UI })).toBe(true)
  })
  test('a living parent is never abandoned', () => {
    expect(verdict({ pid: 4, ppid: 5120, command: ROOT_SERVE })).toBe(false)
    expect(verdict({ pid: 4, ppid: 5120, command: PHONE_API })).toBe(false)
  })
  test("a developer's gadak serve from /usr/local/bin is not touched", () => {
    expect(verdict({ pid: 6, ppid: 1, command: '/usr/local/bin/gadak serve --addr 127.0.0.1:7911' })).toBe(false)
    expect(verdict({ pid: 6, ppid: 1, command: '/usr/local/bin/gadak demo' })).toBe(false)
  })
  test('a command that merely contains "gadak" is not touched', () => {
    expect(verdict({ pid: 7, ppid: 1, command: 'vim /Users/x/repo/gadak/e2e/.tmp/gadak-7911 serve' })).toBe(false)
    expect(verdict({ pid: 7, ppid: 1, command: 'tail -f gadak-mobile-api-7899 demo.log' })).toBe(false)
    expect(verdict({ pid: 7, ppid: 1, command: 'node /other/vite preview --config /other/vite.config.ts' })).toBe(false)
  })
})

describe('reapAbandonedServe', () => {
  const run = (rows: Record<number, Row & { etime: string }>) => {
    const killed: number[] = []
    const line = reapAbandonedServe(7911, {
      listenerPids: () => Object.keys(rows).map(Number),
      describePid: (pid: number) => rows[pid] ?? null,
      terminate: (pid: number) => void killed.push(pid),
    })
    return { line, killed }
  }
  test('kills only the abandoned listener and names it', () => {
    const { line, killed } = run({ 91452: { pid: 91452, ppid: 1, etime: '02-03:14:08', command: ROOT_SERVE } })
    expect(killed).toEqual([91452])
    expect(line).toBe(`reaped abandoned gate serve pid 91452 on :7911 (up 02-03:14:08, parent gone): ${ROOT_SERVE}`)
  })
  test('leaves a foreign listener alone', () => {
    const { line, killed } = run({ 9: { pid: 9, ppid: 1, etime: '01:00', command: '/usr/local/bin/gadak serve' } })
    expect(killed).toEqual([])
    expect(line).toBeNull()
  })
})

describe('real orphan', () => {
  test('describePid reads a live child, and it is not abandoned (parent alive)', async () => {
    const child = spawn('sleep', ['30'], { stdio: 'ignore' })
    try {
      const row = describePid(child.pid)
      expect(row.ppid).toBe(process.pid)
      expect(verdict(row)).toBe(false)
    } finally {
      child.kill('SIGKILL')
    }
  })
})

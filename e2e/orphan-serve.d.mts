// Types for e2e/orphan-serve.mjs (GDK-1951). The module stays plain ESM so
// e2e/port-held.mjs can import it with no build step; the TypeScript callers
// (e2e/helpers.ts, mobile/e2e/serve.ts) read these declarations.
export interface ListenerRow {
  pid: number
  ppid: number
  etime?: string
  command: string
}
export function classifyListener(row: ListenerRow | null | undefined): { abandoned: boolean; reason: string }
export function listenerPids(port: number | string): number[]
export function describePid(pid: number): ListenerRow | null
export function reapAbandonedServe(
  port: number,
  deps?: {
    listenerPids?: (port: number) => number[]
    describePid?: (pid: number) => ListenerRow | null
    terminate?: (pid: number) => string
  },
): string | null

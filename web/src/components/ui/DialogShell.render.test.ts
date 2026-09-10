/*
 * GDK-1328 pilot: a mounted-component test, the minimal viable form.
 *
 * Everything else in this directory reads .svelte source and pattern-matches
 * it (DialogShell.test.ts's own header says why: the default unit project has
 * no svelte plugin). That proves the source contains a class string; it does
 * not prove the component renders it. This pilot mounts one component —
 * DialogShell, whose chrome contract GDK-316 established — with the two
 * zero-dependency pieces the repo already ships:
 *
 *   - `render` from 'svelte/server' (SSR, no DOM, no jsdom/happy-dom);
 *   - `createRawSnippet` from 'svelte' for children/footer props.
 *
 * Assertions run over the rendered HTML string. That is still string
 * matching, but on output — the template logic, i18n call and conditional
 * footer all executed first, which source regex never does. Costs kept out
 * on purpose (see the round report for the full cost side): no DOM means no
 * event simulation (onclick/Esc/trap stay in e2e), no reactivity (props are
 * render-once), and HTML-string queries stay deliberately coarse.
 *
 * Runs under the pages-store project (svelte plugin required to import
 * .svelte). Server render ignores `use:trap` and event handlers — the noop
 * trap below stands in for the real one.
 */
import { createRawSnippet } from 'svelte'
import { render } from 'svelte/server'
import { describe, expect, test } from 'vitest'
import DialogShell from './DialogShell.svelte'
import { en } from '../../lib/i18n/en'

const text = (s: string) =>
  createRawSnippet(() => ({
    render: () => s,
  }))

const noop = () => {}

const CLOSE_ESC = en['common.closeEsc']

function mount(props: Record<string, unknown> = {}): string {
  return render(DialogShell, {
    props: {
      onclose: noop,
      trap: noop,
      ariaLabel: 'Fixture dialog',
      children: text('<p data-fixture-body>body</p>'),
      ...props,
    },
  }).html
}

/** count matches of a global regex on the rendered html */
function count(html: string, re: RegExp): number {
  return [...html.matchAll(re.global ? re : new RegExp(re.source, re.flags + 'g'))].length
}

describe('GDK-316 chrome, rendered (GDK-1328 mount pilot)', () => {
  test('header X is named closeEsc through the i18n call, on both axes', () => {
    const html = mount()
    expect(html).toContain(`aria-label="${CLOSE_ESC}"`)
    expect(html).toContain(`title="${CLOSE_ESC}"`)
    // exactly one dismiss control — a second phrasing is a second vocabulary
    expect(count(html, /aria-label="/g)).toBe(1 + 1) // the X + the panel's aria-label
  })

  test('panel is role=dialog, aria-modal, labelled, in the shared chrome', () => {
    const html = mount()
    expect(html).toContain('role="dialog"')
    expect(html).toContain('aria-modal="true"')
    expect(html).toContain('aria-label="Fixture dialog"')
    // the shared visual contract — measured where the browser reads it
    expect(html).toContain('rounded-lg border border-border-strong bg-bg-panel shadow-overlay')
    expect(html).toContain('fixed inset-0')
  })

  test('a footer snippet gets exactly one data-dialog-footer container', () => {
    const html = mount({ footer: text('<button data-fixture-primary>save</button>') })
    expect(count(html, /data-dialog-footer/g)).toBe(1)
    expect(html).toContain('data-fixture-primary')
  })

  test('no footer snippet, no footer container — content-only dialogs have none', () => {
    const html = mount()
    expect(count(html, /data-dialog-footer/g)).toBe(0)
  })

  test('title renders as the h2; children land inside the panel', () => {
    const html = mount({ title: 'Fixture title' })
    expect(html).toContain('<h2')
    expect(html).toContain('Fixture title')
    expect(html).toContain('data-fixture-body')
  })
})

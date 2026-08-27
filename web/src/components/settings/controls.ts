/*
 * Control classes shared by the settings dialog and its tabs.
 *
 * They live here rather than in each tab because a settings control that looks
 * different from the settings control next to it is the one defect this screen
 * cannot afford: it is the screen people trust to describe the mirror.
 */

// Width is owned by the call site (GDK-1052): the bare bases carry no w-*,
// and INPUT/SELECT add w-full only for the sites that want to fill their
// parent. Composing `{INPUT} w-24` loses to that w-full by cascade order —
// class order in the attribute decides nothing — so a width override must
// compose from the bare base: class="{INPUT_BARE} w-24 flex-none".
// controls.test.ts scans the tabs for violations of this rule.
export const INPUT_BARE =
  'h-control rounded-md border border-border-strong bg-bg-base px-2 text-body text-text-primary outline-none focus:border-accent'
export const INPUT = `${INPUT_BARE} w-full`
// Spelled out rather than `${INPUT} pr-7`: px-2 and pr-7 resolve against each
// other by cascade order, not by class order, so the chevron's clearance
// would depend on how Tailwind happens to emit the two utilities.
export const SELECT_BARE =
  'h-control appearance-none rounded-md border border-border-strong bg-bg-base pl-2 pr-7 text-body text-text-primary outline-none focus:border-accent'
export const SELECT = `${SELECT_BARE} w-full`
export const SELECT_CHEVRON =
  'pointer-events-none absolute right-2 top-1/2 -translate-y-1/2 rotate-90 text-text-muted'
export const ADD_BTN =
  'inline-flex h-control-sm items-center self-start rounded-md border border-border-strong px-2 text-micro text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary'
export const DEL_BTN =
  'flex w-6 flex-none items-center justify-center text-text-muted transition-colors hover:text-status-reopen'
export const COPY_BTN =
  'inline-flex h-control-sm items-center rounded border border-border-strong px-1.5 text-micro text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary'

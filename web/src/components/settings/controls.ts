/*
 * Control classes shared by the settings dialog and its tabs.
 *
 * They live here rather than in each tab because a settings control that looks
 * different from the settings control next to it is the one defect this screen
 * cannot afford: it is the screen people trust to describe the mirror.
 */

export const INPUT =
  'h-control w-full rounded-md border border-border-strong bg-bg-base px-2 text-[12px] text-text-primary outline-none focus:border-accent'
// Spelled out rather than `${INPUT} pr-7`: px-2 and pr-7 resolve against each
// other by cascade order, not by class order, so the chevron's clearance
// would depend on how Tailwind happens to emit the two utilities.
export const SELECT =
  'h-control w-full appearance-none rounded-md border border-border-strong bg-bg-base pl-2 pr-7 text-[12px] text-text-primary outline-none focus:border-accent'
export const SELECT_CHEVRON =
  'pointer-events-none absolute right-2 top-1/2 -translate-y-1/2 rotate-90 text-text-muted'
export const ADD_BTN =
  'inline-flex h-control-sm items-center self-start rounded-md border border-border-strong px-2 text-micro text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary'
export const DEL_BTN =
  'flex w-6 flex-none items-center justify-center text-text-muted transition-colors hover:text-status-reopen'
export const COPY_BTN =
  'inline-flex h-control-sm items-center rounded border border-border-strong px-1.5 text-micro text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary'

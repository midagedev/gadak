// GDK-1988: the field is a buffer while Hangul is being assembled.
//
// Measured on iOS 18.7 Safari at /m/ (document-level probe, typing 한글):
//
//   8619ms keydown     key="ㅎ" code=Unidentified isComposing=false
//   8628ms beforeinput type=insertText data="ㅎ" isComposing=false value=""
//   8631ms INPUT      type=insertText data="ㅎ" isComposing=false value="ㅎ"
//   8632ms WS-SEND    [227,133,142]
//   9230ms ㅏ / 9764ms ㄴ / 10664ms ㄱ / 10947ms ㅡ / 11132ms ㄹ — same shape
//
// Not one composition event in the whole run, and `isComposing` false on the
// very first 자모 of an empty field. The gate in ime.ts is correct and never
// sees a composition to gate: every 자모 commits standalone and the PTY gets
// six of them. Two known causes produce exactly this trace and we cannot tell
// them apart from it (the field was cleared after every keystroke, which is
// where the second keystroke's evidence went): WebKit may refuse to drive the
// IME on a 1px fully-transparent textarea (izagood/harkroom#795), or the iOS
// Korean keyboard may never fire composition events at all and assemble by
// rewriting the field's own text (NousResearch/hermes-agent#50067 — the same
// 자모 spill on iPad, diagnosed as xterm clearing the textarea and resetting
// WebKit's composition context).
//
// This module is the half that is right either way: stop clearing, and let
// the tail that can still change stay in the field where iOS can rewrite it.
// 한 + ㅏ → 하나, so "send everything but the last syllable" is not safe —
// a syllable already sent can still change. The boundary WebKit does give is
// a word: a non-Hangul character, Enter, Tab, a key-bar key. Holding the
// whole Hangul run until one of those arrives removes the rewrite problem
// instead of solving it. The screen pairs this with a visible strip, because
// nothing reaches the PTY while a run is held and the user would otherwise
// be typing blind (naru-remote specs/009-live-type-through: assemble locally,
// deliver one committed unit at a boundary, mirror what is pending).
//
// Pure on purpose — a Korean sequence is a unit test here, not a device tap.

/** Hangul jamo and syllables, the code points an IME can still rewrite. */
function isHangulChar(cp: number): boolean {
  return (
    (cp >= 0x1100 && cp <= 0x11ff) || // Jamo
    (cp >= 0x3130 && cp <= 0x318f) || // Compatibility Jamo
    (cp >= 0xa960 && cp <= 0xa97f) || // Jamo Extended-A
    (cp >= 0xac00 && cp <= 0xd7a3) || // Syllables
    (cp >= 0xd7b0 && cp <= 0xd7ff) // Jamo Extended-B
  )
}

/** True when every character is Hangul and there is at least one. */
export function isHangulText(text: string): boolean {
  if (text === '') return false
  for (const ch of text) {
    if (!isHangulChar(ch.codePointAt(0) ?? 0)) return false
  }
  return true
}

/**
 * What to do with the field after one non-composing `input` event.
 *
 * `hold` keeps the field exactly as the browser left it; `send` is the text
 * to hand the PTY, and it is the WHOLE field value, never the event's `data`.
 * Sending `data` would drop the run: a field holding 한글 that receives a
 * space fires `input` with `data=" "` and a value of `한글 `.
 */
export type HoldOutcome = { hold: true; send: '' } | { hold: false; send: string }

const HOLD: HoldOutcome = { hold: true, send: '' }

export function decideInput(data: string, value: string): HoldOutcome {
  // An edit with nothing inserted is the field being rewritten under us —
  // iOS replacing 한 with 하, or a Backspace the screen let through so the
  // textarea could take a 자모 off the run. Neither is a boundary.
  if (data === '') return HOLD
  if (isHangulText(data)) return HOLD
  return { hold: false, send: value }
}

/** Enter, Tab, a key-bar key, leaving the session: the run goes out whole. */
export function decideBoundary(value: string): HoldOutcome {
  if (value === '') return HOLD
  return { hold: false, send: value }
}

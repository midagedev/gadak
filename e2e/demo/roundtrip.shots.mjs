/**
 * Shot list for the 0.20 round-trip cut (GDK-1253), read by camera.mjs.
 *
 * Every time here is a mark name plus an offset, every target a mark that
 * carries a rect (roundtrip.spec.ts `markAt`). A reshoot moves the marks and
 * this file does not change. The choreography, in one line: the whole window
 * at rest, a slow push toward the roster while the four shells sit there, a
 * punch-in on the card and the shell it opens, back out, and the same from
 * the palette.
 */
export default {
  take: 'scratch/roundtrip/take-light.webm',
  proof: 'e2e/.tmp/roundtrip/proof-light.jsonl',
  out: 'scratch/roundtrip/roundtrip.mp4',
  frame: { w: 1920, h: 1296, fps: 30 },
  bg: '#0e0d0c',
  // The capture is 1× CSS px (camera.mjs header); 1.45 is as far as its text
  // survives being enlarged — checked on the sheet, not assumed.
  maxZoom: 1.45,

  // Running order. X is unbroken from the list through the board toggle into
  // the chaos hold; Y and Z are the two recoveries. Same bounds as the 0.19
  // cut-roundtrip.sh (0.19), which this file replaced.
  segments: [
    { name: 'open', in: ['list_hold', -0.2], out: ['a_enter', -0.4] },
    { name: 'card', in: ['a_enter', -0.4], out: ['a_alive', 0] },
    { name: 'palette', in: ['b_enter', 0], out: ['end_frame', 1.4] },
  ],

  // The camera arrives at `to` at `at`, having moved for `dur` seconds.
  camera: [
    // the list and the board toggle: the whole window, still
    { at: ['list_hold', 0], to: 'full' },
    // chaos: a slow creep toward the roster and the board above it — four
    // issue keys, four shells — so the hold is not a freeze frame
    { at: ['chaos', 1.7], dur: 1.7, ease: 'inout', to: ['board', 'roster'], zoom: 1.1, fit: false },
    // recovery A: punch to the card and the roster tab beneath it as the
    // pointer arrives on the card; the glyph is revealed inside this frame
    // 1.45 (the cap): at 1.33 the right edge fell mid-title through the Done
    // column (blind verdict, 2026-09-03); at 1.45 the frame is exactly the roster
    // plus New and In progress, and Done is off frame whole rather than sliced
    { at: ['a_enter', 0.9], dur: 0.9, ease: 'out', to: ['a_enter', 'a_tab'], pad: 56, zoom: 1.45, fit: false },
    // the shell answers: ease back to see the whole dock reading
    { at: ['a_replay', 1.2], dur: 1.2, ease: 'inout', to: 'full' },
    // recovery B: punch to the palette row as it appears
    { at: ['b_row', 0.5], dur: 0.7, ease: 'out', to: 'b_row', pad: 80 },
    // and out again as the second shell comes back
    { at: ['b_replay', 1.2], dur: 1.2, ease: 'inout', to: 'full' },
  ],

  endcard: { secs: 2.8, png: 'scratch/roundtrip/endcard.png', dissolve: 0.5 },
  // the frame the clip is judged by: recovery A standing — card above, its
  // shell with a finished reading beneath
  poster: ['a_replay', 3.0],
}

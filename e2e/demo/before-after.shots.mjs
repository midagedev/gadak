/**
 * Shot list for the 0.20 "read" cut — the same screens one release apart,
 * read by camera.mjs. Two takes of before-after.spec.ts: `before` against a
 * serve of the previous release, `after` against this one. Each beat opens on
 * the previous release and a wipe sweeps the new one in from the left; the
 * tags in the corners say which is which.
 */
export default {
  takes: {
    before: { take: 'scratch/before-after/take-before.webm', proof: 'e2e/.tmp/ba/proof-before.jsonl', lead: 0 },
    after: { take: 'scratch/before-after/take-after.webm', proof: 'e2e/.tmp/ba/proof-after.jsonl', lead: 0 },
  },
  out: 'scratch/before-after/read.mp4',
  frame: { w: 1920, h: 1296, fps: 30 },
  bg: '#0e0d0c',
  maxZoom: 1.45,

  segments: [
    // 1. the issue body — punched in on the panel; 0.19 fills the frame, then
    //    0.20 wipes in over it under the same camera
    {
      name: 'issue', take: 'after', in: ['detail_hold', 0], out: ['wide_click', 0],
      // rtl: the panel sits on the right, so the reveal starts on the body itself
      wipe: { from: 'before', at: ['detail_hold', 0.3], start: 0.7, dur: 1.4, dir: 'rtl', labels: ['0.19', '0.20'] },
      camera: [{ at: ['detail_hold', 0], to: 'detail_hold', zoom: 1.45, fit: false }],
    },
    // 2. the wide-reading toggle — the prose takes the column; the camera
    //    eases out so the whole window is seen doing it
    {
      name: 'wide', take: 'after', in: ['wide_click', 0], out: ['wide_hold', 2.6],
      camera: [
        { at: ['wide_click', 0], to: 'detail_hold', zoom: 1.45, fit: false },
        { at: ['wide_hold', 0.7], dur: 1.0, ease: 'inout', to: 'full' },
      ],
    },
    // 3. a wiki page, same wipe
    {
      name: 'page', take: 'after', in: ['page_hold', 0], out: ['page_hold', 2.8],
      wipe: { from: 'before', at: ['page_hold', 0.3], start: 0.5, dur: 1.4, labels: ['0.20', '0.19'] },
    },
    // 4. the terminal dock — a side pane becomes a band with a roster
    {
      name: 'dock', take: 'after', in: ['dock_hold', 0], out: ['dock_hold', 3.0],
      wipe: { from: 'before', at: ['dock_hold', 0.3], start: 0.5, dur: 1.4, labels: ['0.20', '0.19'] },
    },
  ],

  endcard: { secs: 2.8, png: 'scratch/before-after/endcard.png', dissolve: 0.5 },
  poster: { take: 'after', at: ['wide_hold', 1.0] },
}

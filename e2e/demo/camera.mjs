#!/usr/bin/env node
/**
 * camera.mjs — the camera layer over a marked take (GDK-1381).
 *
 *   node e2e/demo/camera.mjs e2e/demo/roundtrip.shots.mjs [--sheet] [--dry] [--lead S]
 *
 * A take is one continuous Playwright recording plus its proof marks
 * (`{mark, epoch_ms, note, rect?}` per line — see roundtrip.spec.ts). Until
 * now the cut was segments of that recording, full frame. This adds a camera:
 * the shot list says, in mark names, where the camera should be looking and
 * when it should arrive, and this script renders the frames.
 *
 * Why a camera at all. release-video.md measured the failure: a 1440×900
 * capture put body text at ~1.1% of frame height, unreadable on a phone. The
 * fix at capture time was a smaller viewport; the fix per beat is a punch-in
 * on the rect the beat is about — the card, the roster, the palette row. The
 * marks already know where those are (they carry `rect` in CSS px), so the
 * shot list never contains a coordinate.
 *
 * Why not ffmpeg alone. zoompan is frame-stepped and jerky at fractional
 * zoom, crop/scale expressions cannot express an eased move without a
 * generator, and this machine's ffmpeg has no libfreetype (no drawtext). And
 * why not Remotion: a new dependency (lockfile-platform gate, licence) for
 * easing that is ten lines. Playwright is already here; a canvas is the
 * compositor.
 *
 * Motion blur is the camera's, not the content's. Each output frame averages
 * K sub-samples of the camera transform across a 180° shutter; when the
 * camera is still, K is 1 and the frame is the source pixel, crisp — a blurry
 * hold would spend the legibility the zoom just bought. Sub-samples that
 * cross a source-frame boundary read the neighbouring source frame, so a fast
 * pan does not strobe.
 *
 * Source pixels are the ceiling. Playwright's screencast records CSS px at 1×
 * whatever deviceScaleFactor says (measured 2026-09-03: DPR 2 with a 2× video
 * size padded the 1× frame with grey), so every zoom is an upscale of the
 * capture. `maxZoom` in the shot list is that budget; the runbook's rule
 * ("cropping into a capture only enlarges soft pixels") is why it is small.
 */
import { chromium } from '@playwright/test'
import { execFileSync, spawnSync } from 'node:child_process'
import { existsSync, mkdirSync, readFileSync, readdirSync, rmSync, statSync, writeFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { pathToFileURL } from 'node:url'

const ROOT = resolve(dirname(new URL(import.meta.url).pathname), '../..')
const args = process.argv.slice(2)
const shotsPath = args.find((a) => !a.startsWith('--'))
if (!shotsPath) {
  console.error('camera: usage: node e2e/demo/camera.mjs <shots.mjs> [--sheet] [--dry] [--lead S]')
  process.exit(64)
}
const flag = (n) => args.includes(n)
const opt = (n) => {
  const i = args.indexOf(n)
  return i >= 0 ? args[i + 1] : undefined
}

const shots = (await import(pathToFileURL(resolve(shotsPath)).href)).default
// One take (`take`/`proof`) or several (`takes: {name: {take, proof, lead?}}`).
// Segments and camera keys name the take they read from; a wipe reads two.
const takeDefs = shots.takes || { main: { take: shots.take, proof: shots.proof, lead: shots.lead } }
const DEFAULT_TAKE = Object.keys(takeDefs)[0]
const out = resolve(ROOT, shots.out)
const { w: W, h: H, fps: FPS } = shots.frame
const work = resolve(ROOT, shots.work || `${dirname(shots.out)}/.camera`)
for (const [name, d] of Object.entries(takeDefs)) {
  d.take = resolve(ROOT, d.take)
  d.proof = resolve(ROOT, d.proof)
  for (const f of [d.take, d.proof]) {
    if (!existsSync(f)) {
      console.error(`camera: take ${name}: missing ${f}`)
      process.exit(1)
    }
  }
}

// ── The takes ──────────────────────────────────────────────────────────────
// Every take: size, duration, marks (video seconds = epoch − start + lead).
// The lead is the gap between the spec's wall clock and the recorder's; when
// the shot list does not pin it, rt-marks.py finds it from the take's luma
// cliffs (pane open / ⌘K backdrop), and a take with neither witness is 0.
let takeW = 0, takeH = 0
for (const [name, d] of Object.entries(takeDefs)) {
  const probe = execFileSync('ffprobe', [
    '-v', 'error', '-select_streams', 'v:0',
    '-show_entries', 'stream=width,height', '-show_entries', 'format=duration',
    '-of', 'csv=p=0', d.take,
  ]).toString().trim().split(/\s+/)
  const [w, h] = probe[0].split(',').map(Number)
  d.w = w; d.h = h; d.dur = Number(probe[1])
  if (!takeW) { takeW = w; takeH = h }
  if (w !== takeW || h !== takeH) { console.error(`camera: take ${name} is ${w}×${h}, others ${takeW}×${takeH}`); process.exit(1) }
  d.marks = new Map()
  d.viewport = null
  for (const line of readFileSync(d.proof, 'utf8').split('\n')) {
    if (!line.trim()) continue
    const rec = JSON.parse(line)
    if (!d.marks.has(rec.mark)) d.marks.set(rec.mark, rec)
    if (rec.viewport) d.viewport = rec.viewport
  }
  if (!d.marks.has('start')) { console.error(`camera: take ${name}: proof has no start mark`); process.exit(1) }
  d.t0 = d.marks.get('start').epoch_ms
  d.cssScale = d.viewport ? w / d.viewport.w : 1
  if (opt('--lead') !== undefined) d.lead = Number(opt('--lead'))
  else if (d.lead === undefined) {
    try { d.lead = Number(execFileSync('python3', [resolve(ROOT, 'e2e/demo/rt-marks.py'), 'lead', d.take, d.proof], { stdio: ['ignore', 'pipe', 'ignore'] }).toString()) }
    catch { d.lead = 0 }
  }
}
if (Math.abs(takeW / takeH - W / H) > 0.01) {
  console.error(`camera: takes ${takeW}×${takeH} and frame ${W}×${H} differ in aspect`)
  process.exit(1)
}
const takeOf = (name) => {
  const d = takeDefs[name || DEFAULT_TAKE]
  if (!d) throw new Error(`camera: no take named '${name}'`)
  return d
}
/** [mark, off] → seconds in `take`. A mark may be qualified: 'before:detail_hold'. */
const at = ([name, off = 0], takeName) => {
  let tn = takeName
  if (name.includes(':')) [tn, name] = name.split(':')
  const d = takeOf(tn)
  const m = d.marks.get(name)
  if (!m) throw new Error(`camera: take ${tn || DEFAULT_TAKE} has no '${name}' mark`)
  return (m.epoch_ms - d.t0) / 1000 + d.lead + off
}
const rectOf = (name, takeName) => {
  let tn = takeName
  if (name.includes(':')) [tn, name] = name.split(':')
  const d = takeOf(tn)
  const m = d.marks.get(name)
  if (!m) throw new Error(`camera: take ${tn || DEFAULT_TAKE} has no '${name}' mark`)
  if (!m.rect) throw new Error(`camera: mark '${name}' carries no rect — mark it with markAt()`)
  return m.rect
}
const cssScale = takeOf(DEFAULT_TAKE).cssScale

// ── Views ──────────────────────────────────────────────────────────────────
// A view is where the camera looks: centre (take px) and zoom (1 = the whole
// take fills the frame). A target becomes a view by fitting its padded rect.
const FULL = { cx: takeW / 2, cy: takeH / 2, z: 1 }
const maxZoom = shots.maxZoom ?? 1.5
function clampView(v) {
  const z = Math.min(Math.max(v.z, 1), maxZoom)
  const hw = takeW / (2 * z), hh = takeH / (2 * z)
  return {
    cx: Math.min(Math.max(v.cx, hw), takeW - hw),
    cy: Math.min(Math.max(v.cy, hh), takeH - hh),
    z,
  }
}
function union(rects) {
  const x0 = Math.min(...rects.map((r) => r.x)), y0 = Math.min(...rects.map((r) => r.y))
  const x1 = Math.max(...rects.map((r) => r.x + r.w)), y1 = Math.max(...rects.map((r) => r.y + r.h))
  return { x: x0, y: y0, w: x1 - x0, h: y1 - y0 }
}
function viewFor(key, takeName) {
  if (key.to === 'full' || key.to === undefined) return FULL
  const targets = Array.isArray(key.to) ? key.to : [key.to]
  const rects = targets.map((t) => (typeof t === 'string' ? rectOf(t, takeName) : t))
  const r = union(rects)
  const pad = key.pad ?? 32
  const s = cssScale
  const fit = Math.min(takeW / ((r.w + 2 * pad) * s), takeH / ((r.h + 2 * pad) * s))
  // `zoom` caps the fit; `fit: false` makes it the zoom outright (a creep
  // toward a region that is most of the frame, where fitting would be ~1.0)
  const z = key.zoom === undefined ? fit : key.fit === false ? key.zoom : Math.min(key.zoom, fit)
  return clampView({ cx: (r.x + r.w / 2) * s, cy: (r.y + r.h / 2) * s, z })
}

// Keyframes: the camera ARRIVES at `to` at `at`, having moved for `dur`
// seconds. Gesture marks fire as the gesture starts, so "arrive a beat after
// the mark" is the natural way to write a punch-in on what the gesture
// reveals. Between arrivals the view holds. Keys live on the shot list (one
// take) or on each segment (several takes); times are that take's seconds.
const buildKeys = (list, takeName) => (list || [])
  .map((k) => ({ t: at(k.at, takeName), dur: k.dur ?? 0, ease: k.ease ?? 'inout', view: viewFor(k, takeName) }))
  .sort((a, b) => a.t - b.t)
const globalKeys = buildKeys(shots.camera, DEFAULT_TAKE)
const EASE = {
  linear: (x) => x,
  inout: (x) => (x < 0.5 ? 4 * x * x * x : 1 - Math.pow(-2 * x + 2, 3) / 2),
  out: (x) => 1 - Math.pow(1 - x, 4),
}
function viewAt(t, keys) {
  if (!keys.length) return FULL
  let prev = keys[0]
  for (const k of keys) {
    if (t >= k.t) { prev = k; continue }
    // between prev (held) and k (arriving)
    const start = k.t - k.dur
    if (t <= start) return prev.view
    const x = EASE[k.ease]((t - start) / k.dur)
    const a = prev.view, b = k.view
    // zoom interpolates in log space — a linear zoom reads as accelerating
    const z = Math.exp(Math.log(a.z) + (Math.log(b.z) - Math.log(a.z)) * x)
    return clampView({ cx: a.cx + (b.cx - a.cx) * x, cy: a.cy + (b.cy - a.cy) * x, z })
  }
  return prev.view
}

// ── Timeline: segments of take time, concatenated ──────────────────────────
// A segment reads one take. It may open with a WIPE: a frozen frame of another
// take (`wipe.from` at `wipe.at`) fills the frame, and from `wipe.start` for
// `wipe.dur` seconds an edge sweeps left→right revealing this take under it,
// both sides under the same camera. `wipe.labels` = [left, right] version tags.
const segments = shots.segments.map((s) => {
  const take = s.take || DEFAULT_TAKE
  const seg = {
    name: s.name || '', take, in: at(s.in, take), out: at(s.out, take),
    keys: s.camera ? buildKeys(s.camera, take) : globalKeys,
    wipe: null,
  }
  if (s.wipe) {
    const from = s.wipe.from
    seg.wipe = {
      from, fromT: at(s.wipe.at, from), start: s.wipe.start ?? 0.5, dur: s.wipe.dur ?? 1.2,
      ease: s.wipe.ease ?? 'inout', labels: s.wipe.labels || null, edge: s.wipe.edge ?? 28,
      // 'ltr' reveals the new take from the left edge; 'rtl' from the right — pick
      // the side the beat's subject sits on, so the reveal starts on it
      dir: s.wipe.dir ?? 'ltr',
    }
  }
  return seg
})
for (const s of segments) {
  if (!(s.out > s.in)) {
    console.error(`camera: segment ${s.name} is empty (${s.in.toFixed(2)} → ${s.out.toFixed(2)}) — marks moved`)
    process.exit(1)
  }
  const d = takeOf(s.take)
  if (s.out > d.dur) console.warn(`camera: segment ${s.name} runs past take ${s.take} (${s.out.toFixed(2)} > ${d.dur.toFixed(2)})`)
}
/** output frame index → {seg, t (take seconds), s (segment-local seconds)} */
const frames = []
for (const seg of segments) {
  const n = Math.round((seg.out - seg.in) * FPS)
  for (let i = 0; i < n; i++) frames.push({ seg, t: seg.in + i / FPS, s: i / FPS })
}
const liveFrames = frames.length
const endcard = shots.endcard
const endFrames = endcard ? Math.round(endcard.secs * FPS) : 0
const dissolve = endcard ? Math.round((endcard.dissolve ?? 0.5) * FPS) : 0
/** take time → output time, for the poster (first segment of that take that holds it) */
function outTime(t, takeName = DEFAULT_TAKE) {
  let acc = 0
  for (const s of segments) {
    if (s.take === takeName && t >= s.in && t <= s.out) return acc + (t - s.in)
    acc += s.out - s.in
  }
  return null
}

for (const [name, d] of Object.entries(takeDefs)) console.log(`camera: take ${name.padEnd(8)} lead ${d.lead.toFixed(2)}s · ${d.w}×${d.h} ${d.dur.toFixed(1)}s · css×${d.cssScale.toFixed(3)}`)
for (const s of segments) {
  console.log(`camera: seg ${s.name.padEnd(10)} [${s.take}] ${s.in.toFixed(2).padStart(7)} → ${s.out.toFixed(2).padStart(7)}  (${(s.out - s.in).toFixed(2)}s)${s.wipe ? `  wipe ${s.wipe.from}@${s.wipe.fromT.toFixed(2)} +${s.wipe.start}s/${s.wipe.dur}s` : ''}`)
  if (s.keys !== globalKeys) for (const k of s.keys) console.log(`camera:   key ${k.t.toFixed(2).padStart(7)}  z=${k.view.z.toFixed(2)} c=(${k.view.cx.toFixed(0)},${k.view.cy.toFixed(0)}) dur=${k.dur}`)
}
if (segments.some((s) => s.keys === globalKeys)) for (const k of globalKeys) console.log(`camera: key ${k.t.toFixed(2).padStart(7)}  z=${k.view.z.toFixed(2)} c=(${k.view.cx.toFixed(0)},${k.view.cy.toFixed(0)}) dur=${k.dur}`)
console.log(`camera: ${liveFrames} live frames + ${endFrames} end card = ${((liveFrames + endFrames) / FPS).toFixed(1)}s`)
if (flag('--dry')) process.exit(0)

// ── Source frames ──────────────────────────────────────────────────────────
// Extracted once per take at the output fps. `fps=` (a filter), not `-r`: the
// screencast is variable-frame-rate and -r on input misplaces frames.
for (const [name, d] of Object.entries(takeDefs)) {
  d.srcDir = resolve(work, `src-${name}`)
  const stamp = resolve(d.srcDir, '.stamp')
  const st = statSync(d.take)
  const want = `${d.take}:${FPS}:${st.size}:${st.mtimeMs}` // a retake overwrites the same path
  if (!existsSync(stamp) || readFileSync(stamp, 'utf8') !== want) {
    rmSync(d.srcDir, { recursive: true, force: true })
    mkdirSync(d.srcDir, { recursive: true })
    console.log(`camera: extracting source frames of ${name}…`)
    execFileSync('ffmpeg', ['-v', 'error', '-y', '-i', d.take, '-vf', `fps=${FPS}`, '-q:v', '2', resolve(d.srcDir, '%06d.jpg')])
    writeFileSync(stamp, want)
  }
  d.srcCount = readdirSync(d.srcDir).filter((f) => f.endsWith('.jpg')).length
}
const srcIndex = (t, d) => Math.min(Math.max(Math.floor(t * FPS) + 1, 1), d.srcCount)

// ── Render ─────────────────────────────────────────────────────────────────
const outDir = resolve(work, 'out')
rmSync(outDir, { recursive: true, force: true })
mkdirSync(outDir, { recursive: true })

const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: W, height: H }, deviceScaleFactor: 1 })
// Frames are served to the page from one fake origin: a file: image on a
// canvas taints it (toDataURL throws SecurityError — measured), and an http
// origin the route answers from disk does not.
const ORIGIN = 'http://camera.local'
await page.route(`${ORIGIN}/**`, (route) => {
  const path = new URL(route.request().url()).pathname
  if (path === '/') return route.fulfill({ contentType: 'text/html', body: `<canvas id="c" width="${W}" height="${H}"></canvas>` })
  if (path.startsWith('/src/')) {
    const [, , tn, file] = path.split('/')
    return route.fulfill({ contentType: 'image/jpeg', body: readFileSync(resolve(takeOf(tn).srcDir, file)) })
  }
  if (path === '/end.png' && endcard) return route.fulfill({ contentType: 'image/png', body: readFileSync(resolve(ROOT, endcard.png)) })
  return route.fulfill({ status: 404, body: '' })
})
await page.goto(`${ORIGIN}/`)
await page.evaluate(({ srcDir, endPng }) => {
  const c = document.getElementById('c')
  const ctx = c.getContext('2d', { alpha: false })
  ctx.imageSmoothingEnabled = true
  ctx.imageSmoothingQuality = 'high'
  const cache = new Map()
  const order = []
  window.__frame = async (take, idx) => {
    const k = `${take}/${idx}`
    if (cache.has(k)) return cache.get(k)
    const img = new Image()
    img.src = `${srcDir}/${take}/${String(idx).padStart(6, '0')}.jpg`
    await img.decode()
    cache.set(k, img)
    order.push(k)
    while (order.length > 48) cache.delete(order.shift())
    return img
  }
  window.__end = null
  if (endPng) {
    const img = new Image()
    img.src = endPng
    window.__end = img.decode().then(() => img)
  }
  /**
   * samples: [{take, idx, cx, cy, z, wipe?}] — averaged with additive blending
   * over black, each at 1/K. K = 1 draws the plain frame. A sample's `wipe`
   * = {take, idx, x (0..1 edge), edge (px soft band), labels} draws the OTHER
   * take under the same camera to the RIGHT of the edge — before on the right,
   * after on the left, the reveal sweeping left→right.
   * label: stamped top-left for the contact sheet (canvas text needs no
   * freetype — this ffmpeg has none).
   * endMix: 0..1 — how much of the end card is over the frame.
   */
  const drawTake = async (take, idx, s) => {
    const img = await window.__frame(take, idx)
    const sc = (c.width / img.naturalWidth) * s.z
    ctx.setTransform(sc, 0, 0, sc, c.width / 2 - s.cx * sc, c.height / 2 - s.cy * sc)
    ctx.drawImage(img, 0, 0)
    ctx.setTransform(1, 0, 0, 1, 0, 0)
  }
  const tag = (text, x, y, align) => {
    const h = Math.round(c.height / 34)
    ctx.font = `600 ${h}px ui-sans-serif, -apple-system, "SF Pro Text", "Helvetica Neue", sans-serif`
    const tw = ctx.measureText(text).width
    const padX = Math.round(h * 0.7), padY = Math.round(h * 0.38)
    const bw = tw + padX * 2, bh = h + padY * 2
    const bx = align === 'right' ? x - bw : x
    ctx.fillStyle = 'rgba(14,13,12,.82)'
    ctx.beginPath(); ctx.roundRect(bx, y, bw, bh, Math.round(h / 3)); ctx.fill()
    ctx.fillStyle = '#f4efe4'
    ctx.textBaseline = 'middle'
    ctx.textAlign = 'left'
    ctx.fillText(text, bx + padX, y + bh / 2 + 1)
  }
  window.__render = async (samples, label, endMix, bg) => {
    const W = c.width, H = c.height
    ctx.globalCompositeOperation = 'source-over'
    ctx.globalAlpha = 1
    ctx.fillStyle = bg || '#000'
    ctx.fillRect(0, 0, W, H)
    const K = samples.length
    if (K > 1) {
      // additive over black at 1/K each = the mean of the K samples
      ctx.fillStyle = '#000'
      ctx.fillRect(0, 0, W, H)
      ctx.globalCompositeOperation = 'lighter'
      ctx.globalAlpha = 1 / K
    }
    for (const s of samples) {
      if (!s.wipe) { await drawTake(s.take, s.idx, s); continue }
      // wipe: "before" to the right of the edge, "after" to the left — each
      // pixel drawn exactly once per sample, or the additive average of K
      // samples would sum both takes and wash the reveal out (measured: a
      // near-white left half on the sheet)
      const rtl = s.wipe.dir === 'rtl'
      const ex = rtl ? W * (1 - s.wipe.x) : s.wipe.x * W   // the seam's x
      // the new take occupies [0,ex) for ltr and [ex,W) for rtl; the old take the rest
      const newRect = rtl ? [ex, 0, W - ex, H] : [0, 0, ex, H]
      const oldRect = rtl ? [0, 0, ex, H] : [ex, 0, W - ex, H]
      if (oldRect[2] > 0) {
        ctx.save(); ctx.beginPath(); ctx.rect(...oldRect); ctx.clip()
        await drawTake(s.wipe.take, s.wipe.idx, s)
        ctx.restore()
      }
      if (newRect[2] > 0) {
        ctx.save(); ctx.beginPath(); ctx.rect(...newRect); ctx.clip()
        await drawTake(s.take, s.idx, s)
        ctx.restore()
      }
    }
    ctx.setTransform(1, 0, 0, 1, 0, 0)
    ctx.globalCompositeOperation = 'source-over'
    ctx.globalAlpha = 1
    const w0 = samples[0].wipe
    if (w0 && w0.x > 0 && w0.x < 1) {
      // the seam: a hairline of paper with a soft shadow falling onto the old take
      const mid = samples[Math.floor(K / 2)].wipe
      const rtl = mid.dir === 'rtl'
      const ex = Math.round(rtl ? W * (1 - mid.x) : mid.x * W)
      const g = rtl ? ctx.createLinearGradient(ex, 0, ex - w0.edge, 0) : ctx.createLinearGradient(ex, 0, ex + w0.edge, 0)
      g.addColorStop(0, 'rgba(0,0,0,.28)'); g.addColorStop(1, 'rgba(0,0,0,0)')
      ctx.fillStyle = g; ctx.fillRect(rtl ? ex - w0.edge : ex, 0, w0.edge, H)
      ctx.fillStyle = '#f4efe4'; ctx.fillRect(ex - 1, 0, 2, H)
    }
    if (w0 && w0.labels) {
      // labels = [left, right]; each shows while its side still holds some frame
      const pad = Math.round(H / 40)
      const rtl = w0.dir === 'rtl'
      const leftVisible = rtl ? w0.x < 0.98 : w0.x > 0.02
      const rightVisible = rtl ? w0.x > 0.02 : w0.x < 0.98
      if (leftVisible) tag(w0.labels[0], pad, pad, 'left')
      if (rightVisible) tag(w0.labels[1], W - pad, pad, 'right')
    }
    if (endMix > 0 && window.__end) {
      const img = await window.__end
      ctx.globalAlpha = endMix
      ctx.drawImage(img, 0, 0, W, H)
      ctx.globalAlpha = 1
    }
    if (label) {
      ctx.font = `bold ${Math.round(H / 28)}px ui-monospace, Menlo, monospace`
      const pad = Math.round(H / 60)
      const tw = ctx.measureText(label).width
      ctx.fillStyle = 'rgba(0,0,0,.75)'
      ctx.fillRect(pad, pad, tw + pad * 2, Math.round(H / 28) + pad * 2)
      ctx.fillStyle = '#ffd866'
      ctx.textBaseline = 'top'
      ctx.fillText(label, pad * 2, pad * 2)
    }
    return c.toDataURL('image/png')
  }
}, {
  srcDir: `${ORIGIN}/src`,
  endPng: endcard ? `${ORIGIN}/end.png` : null,
})

// Shutter: 180° — the blur spans half a frame interval. K grows with how far
// the frame centre travels during that window (or how far a wipe edge moves);
// still camera → K = 1.
const SHUTTER = 0.5 / FPS
function wipeX(seg, s) {
  const w = seg.wipe
  if (!w) return null
  if (s <= w.start) return 0
  if (s >= w.start + w.dur) return 1
  return EASE[w.ease]((s - w.start) / w.dur)
}
function samplesAt(seg, t, s) {
  const d = takeOf(seg.take)
  const a = viewAt(t - SHUTTER / 2, seg.keys), b = viewAt(t + SHUTTER / 2, seg.keys)
  const sc = W / takeW
  const dc = Math.hypot((a.cx - b.cx) * sc * a.z, (a.cy - b.cy) * sc * a.z)
  const dz = Math.abs(a.z - b.z) * sc * Math.hypot(takeW, takeH) / 2
  let disp = dc + dz
  const xa = wipeX(seg, s - SHUTTER / 2), xb = wipeX(seg, s + SHUTTER / 2)
  if (xa !== null) disp += Math.abs(xb - xa) * W
  const K = disp < 0.75 ? 1 : Math.min(12, Math.max(3, Math.ceil(disp / 1.5)))
  const samples = []
  for (let i = 0; i < K; i++) {
    const dt = K === 1 ? 0 : -SHUTTER / 2 + (SHUTTER * (i + 0.5)) / K
    const v = viewAt(t + dt, seg.keys)
    const smp = { take: seg.take, idx: srcIndex(t + dt, d), cx: v.cx, cy: v.cy, z: v.z }
    if (seg.wipe) {
      const fd = takeOf(seg.wipe.from)
      smp.wipe = { take: seg.wipe.from, idx: srcIndex(seg.wipe.fromT, fd), x: wipeX(seg, s + dt), edge: seg.wipe.edge, labels: seg.wipe.labels, dir: seg.wipe.dir }
    }
    samples.push(smp)
  }
  return samples
}
const save = (i, dataUrl) => writeFileSync(resolve(outDir, `${String(i).padStart(6, '0')}.png`), Buffer.from(dataUrl.slice(dataUrl.indexOf(',') + 1), 'base64'))

if (flag('--sheet')) {
  // One frame per mark and per camera arrival (per segment), stamped, tiled:
  // the check that the lead is right and the camera lands on what the beat
  // is about.
  const stampsAt = []
  for (const seg of segments) {
    const d = takeOf(seg.take)
    for (const [name, m] of d.marks) {
      const t = (m.epoch_ms - d.t0) / 1000 + d.lead
      if (t >= seg.in && t <= seg.out) stampsAt.push({ seg, t, label: `${seg.take}:${name} @${t.toFixed(2)}` })
    }
    for (const k of seg.keys) if (k.t >= seg.in && k.t <= seg.out) stampsAt.push({ seg, t: k.t, label: `↳ cam z${k.view.z.toFixed(2)} @${k.t.toFixed(2)}` })
    if (seg.wipe) stampsAt.push({ seg, t: seg.in + seg.wipe.start + seg.wipe.dur / 2, label: `↔ wipe ${seg.name} mid` })
  }
  stampsAt.sort((a, b) => (segments.indexOf(a.seg) - segments.indexOf(b.seg)) || a.t - b.t)
  let i = 0
  for (const st of stampsAt) save(i++, await page.evaluate(({ samples, label, bg }) => window.__render(samples, label, 0, bg), { samples: samplesAt(st.seg, st.t, st.t - st.seg.in), label: st.label, bg: shots.bg }))
  const cols = Math.min(4, i), rows = Math.ceil(i / cols)
  const sheet = out.replace(/\.mp4$/, '-sheet.png')
  execFileSync('ffmpeg', ['-v', 'error', '-y', '-framerate', '1', '-i', resolve(outDir, '%06d.png'), '-vf', `scale=${Math.round(W / 3)}:-1,tile=${cols}x${rows}`, '-frames:v', '1', sheet])
  await browser.close()
  console.log(`camera: sheet at ${sheet} (${i} frames)`)
  process.exit(0)
}

console.log(`camera: rendering ${liveFrames + endFrames} frames → ${outDir}`)
const started = Date.now()
let blurred = 0
for (let i = 0; i < liveFrames; i++) {
  const f = frames[i]
  const samples = samplesAt(f.seg, f.t, f.s)
  if (samples.length > 1) blurred++
  // the last `dissolve` live frames cross-fade into the end card
  const mix = endcard && i >= liveFrames - dissolve ? (i - (liveFrames - dissolve) + 1) / (dissolve + 1) : 0
  save(i, await page.evaluate(({ samples, mix, bg }) => window.__render(samples, '', mix, bg), { samples, mix, bg: shots.bg }))
  if (i % 150 === 0) process.stdout.write(`camera: ${i}/${liveFrames} (${((Date.now() - started) / 1000).toFixed(0)}s)\r`)
}
{
  const f = frames[liveFrames - 1]
  const last = samplesAt(f.seg, f.t, f.s)
  for (let i = 0; i < endFrames; i++) save(liveFrames + i, await page.evaluate(({ samples, bg }) => window.__render(samples, '', 1, bg), { samples: last, bg: shots.bg }))
}
await browser.close()
console.log(`\ncamera: rendered in ${((Date.now() - started) / 1000).toFixed(0)}s · ${blurred} frames carried motion blur`)

mkdirSync(dirname(out), { recursive: true })
const enc = spawnSync('ffmpeg', [
  '-v', 'error', '-y', '-framerate', String(FPS), '-i', resolve(outDir, '%06d.png'),
  '-c:v', 'libx264', '-pix_fmt', 'yuv420p', '-crf', '18', '-preset', 'slow', '-movflags', '+faststart', out,
], { stdio: 'inherit' })
if (enc.status !== 0) process.exit(enc.status ?? 1)

if (shots.poster) {
  const pt = shots.poster.take || DEFAULT_TAKE
  const tp = outTime(at(shots.poster.at || shots.poster, pt), pt)
  if (tp === null) console.warn('camera: poster mark is outside every segment')
  else {
    const poster = out.replace(/\.mp4$/, '-poster.png')
    execFileSync('ffmpeg', ['-v', 'error', '-y', '-ss', tp.toFixed(2), '-i', out, '-frames:v', '1', poster])
    console.log(`camera: poster at ${poster} (t=${tp.toFixed(2)}s)`)
  }
}
console.log(execFileSync('ffprobe', ['-v', 'error', '-show_entries', 'format=duration,size', '-of', 'default=nw=1', out]).toString().trim())
console.log(`camera: ${out}`)

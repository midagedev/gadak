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
const take = resolve(ROOT, shots.take)
const proofPath = resolve(ROOT, shots.proof)
const out = resolve(ROOT, shots.out)
const { w: W, h: H, fps: FPS } = shots.frame
const work = resolve(ROOT, shots.work || `${dirname(shots.out)}/.camera`)
for (const f of [take, proofPath]) {
  if (!existsSync(f)) {
    console.error(`camera: missing ${f}`)
    process.exit(1)
  }
}

// ── The take ───────────────────────────────────────────────────────────────
const probe = execFileSync('ffprobe', [
  '-v', 'error', '-select_streams', 'v:0',
  '-show_entries', 'stream=width,height', '-show_entries', 'format=duration',
  '-of', 'csv=p=0', take,
]).toString().trim().split(/\s+/)
const [takeW, takeH] = probe[0].split(',').map(Number)
const takeDur = Number(probe[1])
if (Math.abs(takeW / takeH - W / H) > 0.01) {
  console.error(`camera: take ${takeW}×${takeH} and frame ${W}×${H} differ in aspect`)
  process.exit(1)
}

// ── Marks ──────────────────────────────────────────────────────────────────
// Video seconds for each mark: (epoch − start) + lead. The lead is the gap
// between the spec's wall clock and the recorder's, found by rt-marks.py from
// the pane-open luma cliff (it moves between takes; a hardcoded value ended a
// film before the card landed, twice).
const lead = opt('--lead') !== undefined
  ? Number(opt('--lead'))
  : Number(execFileSync('python3', [resolve(ROOT, 'e2e/demo/rt-marks.py'), 'lead', take, proofPath]).toString())
const marks = new Map()
let viewport = null
for (const line of readFileSync(proofPath, 'utf8').split('\n')) {
  if (!line.trim()) continue
  const rec = JSON.parse(line)
  if (!marks.has(rec.mark)) marks.set(rec.mark, rec)
  if (rec.viewport) viewport = rec.viewport
}
if (!marks.has('start')) {
  console.error('camera: proof has no start mark')
  process.exit(1)
}
const t0 = marks.get('start').epoch_ms
// CSS px → take px. The recorder writes the viewport at 1× (see header), so
// this is 1 unless the config asked for a scaled video.
const cssScale = viewport ? takeW / viewport.w : 1
const at = ([name, off = 0]) => {
  const m = marks.get(name)
  if (!m) throw new Error(`camera: the take has no '${name}' mark`)
  return (m.epoch_ms - t0) / 1000 + lead + off
}
const rectOf = (name) => {
  const m = marks.get(name)
  if (!m) throw new Error(`camera: the take has no '${name}' mark`)
  if (!m.rect) throw new Error(`camera: mark '${name}' carries no rect — mark it with markAt()`)
  return m.rect
}

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
function viewFor(key) {
  if (key.to === 'full' || key.to === undefined) return FULL
  const targets = Array.isArray(key.to) ? key.to : [key.to]
  const rects = targets.map((t) => (typeof t === 'string' ? rectOf(t) : t))
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
// reveals. Between arrivals the view holds.
const keys = shots.camera
  .map((k) => ({ t: at(k.at), dur: k.dur ?? 0, ease: k.ease ?? 'inout', view: viewFor(k) }))
  .sort((a, b) => a.t - b.t)
const EASE = {
  linear: (x) => x,
  inout: (x) => (x < 0.5 ? 4 * x * x * x : 1 - Math.pow(-2 * x + 2, 3) / 2),
  out: (x) => 1 - Math.pow(1 - x, 4),
}
function viewAt(t) {
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
const segments = shots.segments.map((s) => ({ in: at(s.in), out: at(s.out), name: s.name || '' }))
for (const s of segments) {
  if (!(s.out > s.in)) {
    console.error(`camera: segment ${s.name} is empty (${s.in.toFixed(2)} → ${s.out.toFixed(2)}) — marks moved`)
    process.exit(1)
  }
  if (s.out > takeDur) console.warn(`camera: segment ${s.name} runs past the take (${s.out.toFixed(2)} > ${takeDur.toFixed(2)})`)
}
/** output frame index → take time */
const frames = []
for (const s of segments) {
  const n = Math.round((s.out - s.in) * FPS)
  for (let i = 0; i < n; i++) frames.push(s.in + i / FPS)
}
const liveFrames = frames.length
const endcard = shots.endcard
const endFrames = endcard ? Math.round(endcard.secs * FPS) : 0
const dissolve = endcard ? Math.round((endcard.dissolve ?? 0.5) * FPS) : 0
/** take time → output time, for the poster */
function outTime(t) {
  let acc = 0
  for (const s of segments) {
    if (t >= s.in && t <= s.out) return acc + (t - s.in)
    acc += s.out - s.in
  }
  return null
}

console.log(`camera: lead ${lead.toFixed(2)}s · take ${takeW}×${takeH} ${takeDur.toFixed(1)}s · css×${cssScale.toFixed(3)}`)
for (const s of segments) console.log(`camera: seg ${s.name.padEnd(10)} ${s.in.toFixed(2).padStart(7)} → ${s.out.toFixed(2).padStart(7)}  (${(s.out - s.in).toFixed(2)}s)`)
for (const k of keys) console.log(`camera: key ${k.t.toFixed(2).padStart(7)}  z=${k.view.z.toFixed(2)} c=(${k.view.cx.toFixed(0)},${k.view.cy.toFixed(0)}) dur=${k.dur}`)
console.log(`camera: ${liveFrames} live frames + ${endFrames} end card = ${((liveFrames + endFrames) / FPS).toFixed(1)}s`)
if (flag('--dry')) process.exit(0)

// ── Source frames ──────────────────────────────────────────────────────────
// Extracted once at the output fps. `fps=` (a filter), not `-r`: the
// screencast is variable-frame-rate and -r on input misplaces frames.
const srcDir = resolve(work, 'src')
const stamp = resolve(srcDir, '.stamp')
const st = statSync(take)
const want = `${take}:${FPS}:${st.size}:${st.mtimeMs}` // a retake overwrites the same path
if (!existsSync(stamp) || readFileSync(stamp, 'utf8') !== want) {
  rmSync(srcDir, { recursive: true, force: true })
  mkdirSync(srcDir, { recursive: true })
  console.log('camera: extracting source frames…')
  execFileSync('ffmpeg', ['-v', 'error', '-y', '-i', take, '-vf', `fps=${FPS}`, '-q:v', '2', resolve(srcDir, '%06d.jpg')])
  writeFileSync(stamp, want)
}
const srcCount = readdirSync(srcDir).filter((f) => f.endsWith('.jpg')).length
const srcIndex = (t) => Math.min(Math.max(Math.floor(t * FPS) + 1, 1), srcCount)

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
  if (path.startsWith('/src/')) return route.fulfill({ contentType: 'image/jpeg', body: readFileSync(resolve(srcDir, path.slice(5))) })
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
  window.__frame = async (idx) => {
    if (cache.has(idx)) return cache.get(idx)
    const img = new Image()
    img.src = `${srcDir}/${String(idx).padStart(6, '0')}.jpg`
    await img.decode()
    cache.set(idx, img)
    order.push(idx)
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
   * samples: [{idx, cx, cy, z}] — averaged with additive blending over black,
   * each at 1/K. K = 1 draws the plain frame.
   * label: stamped top-left for the contact sheet (canvas text needs no
   * freetype — this ffmpeg has none).
   * endMix: 0..1 — how much of the end card is over the frame.
   */
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
      const img = await window.__frame(s.idx)
      const sc = (W / img.naturalWidth) * s.z
      ctx.setTransform(sc, 0, 0, sc, W / 2 - s.cx * sc, H / 2 - s.cy * sc)
      ctx.drawImage(img, 0, 0)
    }
    ctx.setTransform(1, 0, 0, 1, 0, 0)
    ctx.globalCompositeOperation = 'source-over'
    ctx.globalAlpha = 1
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
// the frame centre travels during that window; still camera → K = 1.
const SHUTTER = 0.5 / FPS
function samplesAt(t) {
  const a = viewAt(t - SHUTTER / 2), b = viewAt(t + SHUTTER / 2)
  const sc = W / takeW
  // displacement of the frame centre and of a corner, in output px
  const dc = Math.hypot((a.cx - b.cx) * sc * a.z, (a.cy - b.cy) * sc * a.z)
  const dz = Math.abs(a.z - b.z) * sc * Math.hypot(takeW, takeH) / 2
  const disp = dc + dz
  const K = disp < 0.75 ? 1 : Math.min(12, Math.max(3, Math.ceil(disp / 1.5)))
  const samples = []
  for (let i = 0; i < K; i++) {
    const ts = K === 1 ? t : t - SHUTTER / 2 + (SHUTTER * (i + 0.5)) / K
    const v = viewAt(ts)
    samples.push({ idx: srcIndex(ts), cx: v.cx, cy: v.cy, z: v.z })
  }
  return samples
}
const save = (i, dataUrl) => writeFileSync(resolve(outDir, `${String(i).padStart(6, '0')}.png`), Buffer.from(dataUrl.slice(dataUrl.indexOf(',') + 1), 'base64'))

if (flag('--sheet')) {
  // One frame per mark and per camera arrival, stamped, tiled: the check that
  // the lead is right and the camera lands on what the beat is about.
  const stampsAt = []
  for (const [name, m] of marks) {
    const t = (m.epoch_ms - t0) / 1000 + lead
    if (outTime(t) !== null) stampsAt.push({ t, label: `${name} @${t.toFixed(2)}` })
  }
  for (const k of keys) if (outTime(k.t) !== null) stampsAt.push({ t: k.t, label: `↳ cam z${k.view.z.toFixed(2)} @${k.t.toFixed(2)}` })
  stampsAt.sort((a, b) => a.t - b.t)
  let i = 0
  for (const s of stampsAt) save(i++, await page.evaluate(({ samples, label, bg }) => window.__render(samples, label, 0, bg), { samples: samplesAt(s.t), label: s.label, bg: shots.bg }))
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
  const t = frames[i]
  const samples = samplesAt(t)
  if (samples.length > 1) blurred++
  // the last `dissolve` live frames cross-fade into the end card
  const mix = endcard && i >= liveFrames - dissolve ? (i - (liveFrames - dissolve) + 1) / (dissolve + 1) : 0
  save(i, await page.evaluate(({ samples, mix, bg }) => window.__render(samples, '', mix, bg), { samples, mix, bg: shots.bg }))
  if (i % 150 === 0) process.stdout.write(`camera: ${i}/${liveFrames} (${((Date.now() - started) / 1000).toFixed(0)}s)\r`)
}
for (let i = 0; i < endFrames; i++) {
  const last = samplesAt(frames[liveFrames - 1])
  save(liveFrames + i, await page.evaluate(({ samples, bg }) => window.__render(samples, '', 1, bg), { samples: last, bg: shots.bg }))
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
  const tp = outTime(at(shots.poster))
  if (tp === null) console.warn('camera: poster mark is outside every segment')
  else {
    const poster = out.replace(/\.mp4$/, '-poster.png')
    execFileSync('ffmpeg', ['-v', 'error', '-y', '-ss', tp.toFixed(2), '-i', out, '-frames:v', '1', poster])
    console.log(`camera: poster at ${poster} (t=${tp.toFixed(2)}s)`)
  }
}
console.log(execFileSync('ffprobe', ['-v', 'error', '-show_entries', 'format=duration,size', '-of', 'default=nw=1', out]).toString().trim())
console.log(`camera: ${out}`)

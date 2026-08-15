# Hosted demo build

`build.mjs` writes the zero-install demo into `dist/hosted/`:

1. Vite with `VITE_HOSTED_DEMO=1` and base `/gadak/`
2. A static first frame injected into `dist/hosted/index.html` (`first-frame.html`)
3. `gadak export-static` over `examples/demo.db`

The frame is not in `web/index.html`. `gadak serve` and the desktop app keep the existing boot shell.

## Preview the first frame (one command)

```bash
./tools/hosted-demo/preview.sh
```

Then open http://127.0.0.1:4173/gadak/ . Use a 390×844 window (or an iPhone UA) to see the in-app-browser scale from `width=1100`.

If `dist/hosted/index.html` is missing, the script runs `node tools/hosted-demo/build.mjs` first.

## Readability gate (not in CI)

```bash
node tools/hosted-demo/build.mjs
npx playwright test --config e2e/hosted/playwright.config.ts e2e/hosted/first-frame.spec.ts
```

`first-frame.spec.ts` loads the built demo at 390×844 and checks: the frame is in the HTML before `bootstrap.json` arrives; claim and brew visual font-size ≥ 14 CSS px after the 1100→390 scale; `web-demo.mp4` is not requested until the poster is tapped.

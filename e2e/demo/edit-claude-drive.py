#!/usr/bin/env python3
"""Build static-cut inputs from a claude-drive web timeline.

Jump-cut keep-windows (the previous 60–90 s editor) are gone: they dropped
the HTML-authoring wait and left a 68 s clip that was 81 % frozen. Waits
are now compressed by e2e/demo/static-cut.py; this file only names the
1.0× protect windows and the tail trim.

Reads:
  <dir>/web-timeline.jsonl
  <dir>/vhs-show-epoch
  <dir>/raw.mp4            (term-crop MAD → typing / TUI-land windows)

Writes:
  <dir>/edit-protect.json   (static-cut --protect-json)
  <dir>/edit-segments.json  (human-readable copy)
"""
from __future__ import annotations

import importlib.util
import json
import os
import sys

DIR = sys.argv[1] if len(sys.argv) > 1 else "e2e/.tmp/claude-drive"
DURATION = float(sys.argv[2]) if len(sys.argv) > 2 else 0.0

TIMELINE = os.path.join(DIR, "web-timeline.jsonl")
EPOCH = os.path.join(DIR, "vhs-show-epoch")
RAW = os.path.join(DIR, "raw.mp4")
OUT_PROTECT = os.path.join(DIR, "edit-protect.json")
OUT_SEGS = os.path.join(DIR, "edit-segments.json")

VISUAL = {"accent_changed", "label_changed", "dashboard_open", "dashboard_chart_visible"}
PAD = 1.2
END_HOLD = 1.5
# `$ claude` is fully typed by ~0.8s (tape Sleep 500ms + 6 glyphs × 45ms).
# 1.2s leaves ~0.4s to read it. Spec range for this beat is 0.8–1.5s.
OPENING_HOLD = 1.2
# Same beat length after the TUI paints, so `$ claude` → empty TUI is 1×
# (static-cut folds 1–2 frame paint spikes into the boot hold otherwise).
TUI_HOLD = 1.2
# Measurement of the prompt-typing window. Glyph MAD sits under static-cut's
# 0.50 threshold (VHS 8 fps duplicated onto 25 fps → most frames MAD=0;
# 0.4s term-crop mean is ~0.04 while typing and ~0.001 on the boot freeze).
HEAD_PROBE_S = 24.0
HEAD_FPS = 25.0
ROLL_N = 10
GLYPH_ROLL = 0.015
TYPE_SUSTAIN = 1.0
TYPE_QUIET = 0.6
TYPE_PAD = 0.8
PAINT_MAD = 1.0
# Only if measurement fails — the old blanket 0–18s protect, which also
# kept the 7s TUI boot at 1×.
FALLBACK_HEAD = 18.0
# Camera work after the wall opens (scroll + hold). trim_end must include this.
DASH_POST = 8.0
THRESHOLD = 0.50


def load_epoch() -> float:
    if not os.path.isfile(EPOCH):
        return 0.0
    return float(open(EPOCH).read().strip() or "0")


def _load_static_cut():
    path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "static-cut.py")
    spec = importlib.util.spec_from_file_location("static_cut", path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"edit-claude-drive: cannot load {path}")
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def term_crop_vf(width: int, height: int) -> str:
    """Terminal pane. Numbers match e2e/demo/export-claude-drive.sh FLAGSHIP_*."""
    if width == 1080 and height == 1350:
        return "1080:520:0:48"
    if width == 1880 and height == 720:
        return "720:688:0:32"
    return f"{width}:{height}:0:0"


def rolling_mean(xs: list[float], n: int) -> list[float]:
    out = [0.0] * len(xs)
    s = 0.0
    for i, x in enumerate(xs):
        s += x
        if i >= n:
            s -= xs[i - n]
        k = n if i >= n - 1 else i + 1
        out[i] = s / k
    return out


def measure_head_windows(duration: float) -> tuple[list[dict], dict]:
    """Protect opening + TUI-land + prompt typing; leave the boot freeze to static-cut.

    Returns (windows, probe dict). Typing start/end are measured from term-crop
    MAD, not tape constants — takes drift by a few hundred ms.
    """
    opening = {"start": 0.0, "end": min(OPENING_HOLD, duration)}
    meta = {
        "opening_hold": OPENING_HOLD,
        "tui_paint": None,
        "tui_window": None,
        "type_start": None,
        "type_end": None,
        "fallback": False,
    }
    if not os.path.isfile(RAW):
        print("edit-claude-drive: raw.mp4 missing; fallback 0–18s protect", file=sys.stderr)
        meta["fallback"] = True
        return [{"start": 0.0, "end": min(FALLBACK_HEAD, duration)}], meta

    sc = _load_static_cut()
    info = sc.ffprobe_stream(RAW)
    w, h = int(info["width"]), int(info["height"])
    crop = term_crop_vf(w, h)
    cw, ch = (int(x) for x in crop.split(":")[:2])
    probe_s = min(HEAD_PROBE_S, duration)
    mads = sc.frame_mads(RAW, HEAD_FPS, cw, ch, seconds=probe_s, vf_extra=f"crop={crop}")
    if len(mads) < int(OPENING_HOLD * HEAD_FPS) + 2:
        print("edit-claude-drive: too few head frames; fallback 0–18s protect", file=sys.stderr)
        meta["fallback"] = True
        return [{"start": 0.0, "end": min(FALLBACK_HEAD, duration)}], meta

    roll = rolling_mean(mads, ROLL_N)
    i0 = int(round(OPENING_HOLD * HEAD_FPS))
    n = len(mads)

    tui = None
    for i in range(i0, n):
        if mads[i] >= PAINT_MAD:
            tui = i / HEAD_FPS
            break
    meta["tui_paint"] = tui

    type_start = None
    run = 0
    run_start = None
    for i in range(i0, n):
        if roll[i] > GLYPH_ROLL:
            if run_start is None:
                run_start = i
            run += 1
            if run / HEAD_FPS >= TYPE_SUSTAIN:
                type_start = run_start / HEAD_FPS
                break
        else:
            run = 0
            run_start = None
    meta["type_start"] = type_start

    type_end = None
    if type_start is not None:
        quiet = 0
        last_hi = int(round(type_start * HEAD_FPS))
        for i in range(int(round(type_start * HEAD_FPS)), n):
            if roll[i] > GLYPH_ROLL:
                quiet = 0
                last_hi = i
            else:
                quiet += 1
                if quiet / HEAD_FPS >= TYPE_QUIET:
                    break
        type_end = min(duration, last_hi / HEAD_FPS + TYPE_PAD)
        meta["type_end"] = type_end

    if type_start is None or type_end is None or type_end <= type_start:
        print(
            "edit-claude-drive: typing window not found; fallback 0–18s protect",
            file=sys.stderr,
        )
        meta["fallback"] = True
        return [{"start": 0.0, "end": min(FALLBACK_HEAD, duration)}], meta

    windows = [opening]
    if tui is not None and tui + 0.05 < type_start:
        t1 = min(tui + TUI_HOLD, type_start)
        if t1 > tui:
            tw = {"start": tui, "end": t1}
            windows.append(tw)
            meta["tui_window"] = tw
    windows.append({"start": type_start, "end": type_end})
    print(
        "edit-claude-drive: "
        f"opening=[0,{OPENING_HOLD:.3f}] tui={tui} typing=[{type_start:.3f},{type_end:.3f}] "
        f"crop={crop}",
        file=sys.stderr,
    )
    return windows, meta


def load_events(vhs0: float) -> list[dict]:
    out = []
    if not os.path.isfile(TIMELINE):
        return out
    for line in open(TIMELINE):
        line = line.strip()
        if not line:
            continue
        ev = json.loads(line)
        t = ev.get("t")
        if not isinstance(t, (int, float)):
            continue
        ev["raw"] = (t / 1000.0) - vhs0
        out.append(ev)
    return out


def main() -> None:
    vhs0 = load_epoch()
    events = load_events(vhs0)
    if DURATION <= 1:
        print("edit-claude-drive: duration missing", file=sys.stderr)
        sys.exit(1)

    visual = [e for e in events if e.get("label") in VISUAL and 0 <= e["raw"] <= DURATION]
    protect_times = [
        e["raw"]
        for e in visual
        if e.get("label") in {"accent_changed", "label_changed", "dashboard_open"}
    ]
    head_windows, head_meta = measure_head_windows(DURATION)
    windows = list(head_windows)
    dash = [e["raw"] for e in visual if e.get("label") == "dashboard_open"]
    chart = [e["raw"] for e in visual if e.get("label") == "dashboard_chart_visible"]
    if dash:
        t0 = min(dash)
        t1 = max(chart) if chart else t0
        dash_end = min(DURATION, t1 + DASH_POST)
        windows.append({"start": max(0.0, t0 - PAD), "end": dash_end})
        last = dash_end
    else:
        last = max((e["raw"] for e in visual), default=0.0)
    head_floor = max((w["end"] for w in head_windows), default=0.0)
    trim_end = min(DURATION, max(last + 0.25, head_floor + 1.0))

    doc = {
        "vhs_show_epoch": vhs0,
        "duration": DURATION,
        "threshold": THRESHOLD,
        "pad": PAD,
        "end_hold": END_HOLD,
        "trim_end": trim_end,
        "protect": [{"t": t, "label": "event"} for t in protect_times],
        "protect_windows": windows,
        "visual_events": [{"label": e.get("label"), "raw": e.get("raw")} for e in visual],
        "head": head_meta,
    }
    # static-cut wants protect[].t and protect_windows[].start/end
    open(OUT_PROTECT, "w").write(json.dumps(doc, indent=2) + "\n")
    open(OUT_SEGS, "w").write(json.dumps(doc, indent=2) + "\n")
    print(
        json.dumps(
            {k: doc[k] for k in ("trim_end", "protect_windows", "visual_events", "head")},
            indent=2,
        )
    )


if __name__ == "__main__":
    main()

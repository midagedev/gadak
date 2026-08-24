#!/usr/bin/env python3
"""Cut a synced claude-drive raw hstack down to 60–90 s.

Keeps command→web pairs at 1x (from the Playwright timeline) and the
opening prompt. Long token-streaming gaps are cut, not ramped — ramping
those is what kills the causality the clip is for.

Reads:
  e2e/.tmp/claude-drive/web-timeline.jsonl
  e2e/.tmp/claude-drive/vhs-show-epoch
  e2e/.tmp/claude-drive/raw.mp4   (ffprobe duration via argv)

Writes:
  e2e/.tmp/claude-drive/edit-filter.txt
  e2e/.tmp/claude-drive/edit-segments.json
"""
from __future__ import annotations

import json
import os
import sys

DIR = sys.argv[1] if len(sys.argv) > 1 else "e2e/.tmp/claude-drive"
DURATION = float(sys.argv[2]) if len(sys.argv) > 2 else 0.0

TIMELINE = os.path.join(DIR, "web-timeline.jsonl")
EPOCH = os.path.join(DIR, "vhs-show-epoch")
OUT_FILTER = os.path.join(DIR, "edit-filter.txt")
OUT_SEGS = os.path.join(DIR, "edit-segments.json")

VISUAL = {"accent_changed", "label_changed", "dashboard_open"}
TARGET_MIN = 64.0
TARGET_MAX = 88.0


def load_epoch() -> float:
    if not os.path.isfile(EPOCH):
        return 0.0
    return float(open(EPOCH).read().strip() or "0")


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


def merge(windows: list[tuple[float, float]]) -> list[tuple[float, float]]:
    if not windows:
        return []
    windows = sorted((max(0.0, a), b) for a, b in windows if b > a)
    merged = [windows[0]]
    for a, b in windows[1:]:
        pa, pb = merged[-1]
        if a <= pb + 0.15:
            merged[-1] = (pa, max(pb, b))
        else:
            merged.append((a, b))
    return merged


def total(windows: list[tuple[float, float]]) -> float:
    return sum(b - a for a, b in windows)


def clamp_to_duration(windows: list[tuple[float, float]], dur: float) -> list[tuple[float, float]]:
    out = []
    for a, b in windows:
        a = max(0.0, min(a, dur))
        b = max(0.0, min(b, dur))
        if b - a >= 0.2:
            out.append((a, b))
    return out


def build(dur: float, events: list[dict]) -> list[tuple[float, float]]:
    # Colour beats stay tight; dashboard_open keeps the save+open pair plus a hold.
    start_hold = 20.0
    dash_pre, dash_post = 8.0, 16.0
    col_pre, col_post = 5.0, 4.0

    def windows_with(start: float, cpre: float, cpost: float, dpre: float, dpost: float) -> list[tuple[float, float]]:
        w = [(0.0, min(start, dur))]
        for ev in events:
            if ev.get("label") not in VISUAL:
                continue
            t = ev["raw"]
            if t < 0 or t > dur:
                continue
            if ev.get("label") == "dashboard_open":
                w.append((t - dpre, t + dpost))
            else:
                w.append((t - cpre, t + cpost))
        return merge(clamp_to_duration(w, dur))

    w = windows_with(start_hold, col_pre, col_post, dash_pre, dash_post)
    n = total(w)

    if n < TARGET_MIN:
        start_hold = min(22.0, dur)
        dash_post = 20.0
        w = windows_with(start_hold, col_pre, col_post, dash_pre, dash_post)
        n = total(w)

    while n > TARGET_MAX and dash_post > 8.0:
        dash_post = max(8.0, dash_post - 1.0)
        start_hold = max(16.0, start_hold - 1.0)
        w = windows_with(start_hold, col_pre, col_post, dash_pre, dash_post)
        n = total(w)

    return w


def filter_complex(windows: list[tuple[float, float]]) -> str:
    if len(windows) == 1:
        a, b = windows[0]
        return f"[0:v]trim=start={a:.3f}:end={b:.3f},setpts=PTS-STARTPTS[out]"
    parts = []
    labels = []
    for i, (a, b) in enumerate(windows):
        lab = f"v{i}"
        labels.append(f"[{lab}]")
        parts.append(f"[0:v]trim=start={a:.3f}:end={b:.3f},setpts=PTS-STARTPTS[{lab}]")
    parts.append("".join(labels) + f"concat=n={len(windows)}:v=1:a=0[out]")
    return ";".join(parts)


def main() -> None:
    vhs0 = load_epoch()
    events = load_events(vhs0)
    if DURATION <= 1:
        print("edit-claude-drive: duration missing", file=sys.stderr)
        sys.exit(1)
    windows = build(DURATION, events)
    if not windows:
        windows = [(0.0, min(TARGET_MAX, DURATION))]
    filt = filter_complex(windows)
    open(OUT_FILTER, "w").write(filt + "\n")
    doc = {
        "vhs_show_epoch": vhs0,
        "duration": DURATION,
        "kept_s": total(windows),
        "windows": [{"start": a, "end": b} for a, b in windows],
        "visual_events": [
            {"label": e.get("label"), "raw": e.get("raw")}
            for e in events
            if e.get("label") in VISUAL
        ],
        "filter": filt,
    }
    open(OUT_SEGS, "w").write(json.dumps(doc, indent=2) + "\n")
    print(json.dumps({k: doc[k] for k in ("kept_s", "windows", "visual_events")}, indent=2))


if __name__ == "__main__":
    main()

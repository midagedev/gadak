#!/usr/bin/env python3
"""Compress static runs in an mp4 without dropping them (not mpdecimate).

Clip-agnostic: input mp4 + protect timestamps → output mp4 + decision log.

A frame is static when its mean-abs-diff against the previous downscaled
grey frame is below --threshold. Contiguous static runs ≥ --min-run seconds
are locally sped so the run lasts --max-static seconds (default 0.5).
Protected windows (event ± --pad, plus the end hold) stay 1.0×.

Local speedup, not frame sampling: a frozen hold sped 20× is the same
picture either way, and concat-of-trims keeps one decision per run instead
of a per-frame keep/drop flicker on the threshold boundary. mpdecimate is
banned — it left 12.8 s of an unreadable 68 s flagship.

Threshold is measured, not guessed: --probe prints MAD percentiles and a
candidate table. Cursor blink and a "Synced 3m ago" tick must land as
static; a colour change or dashboard open must not.
"""
from __future__ import annotations

import argparse
import json
import math
import os
import subprocess
import sys
from typing import Iterable


SCALE_W = 160


def run(cmd: list[str], **kw) -> subprocess.CompletedProcess:
    return subprocess.run(cmd, check=True, **kw)


def ffprobe_stream(path: str) -> dict:
    out = run(
        [
            "ffprobe",
            "-v",
            "error",
            "-select_streams",
            "v:0",
            "-show_entries",
            "stream=width,height,r_frame_rate,avg_frame_rate,nb_frames",
            "-show_entries",
            "format=duration",
            "-of",
            "json",
            path,
        ],
        capture_output=True,
        text=True,
    ).stdout
    doc = json.loads(out)
    stream = (doc.get("streams") or [{}])[0]
    fmt = doc.get("format") or {}
    dur = float(fmt.get("duration") or 0)
    num, den = 25, 1
    rate = stream.get("avg_frame_rate") or stream.get("r_frame_rate") or "25/1"
    if "/" in rate and not rate.startswith("0/"):
        a, b = rate.split("/", 1)
        if float(b) != 0:
            num, den = int(a), int(b)
    fps = num / den if den else 25.0
    # Concat artifacts sometimes report tbr=200. Content fps for these clips is ≤25.
    if fps > 60:
        fps = 25.0
    return {
        "width": int(stream.get("width") or 0),
        "height": int(stream.get("height") or 0),
        "duration": dur,
        "fps": fps,
    }


def scale_h(src_h: int, src_w: int) -> int:
    if src_w <= 0:
        return 60
    h = max(2, int(round(src_h * SCALE_W / src_w)))
    return h + (h % 2)


def iter_gray_frames(
    path: str,
    fps: float,
    src_w: int,
    src_h: int,
    seconds: float | None = None,
    vf_extra: str = "",
) -> Iterable[bytes]:
    h = scale_h(src_h, src_w)
    nbytes = SCALE_W * h
    vf = f"fps={fps:.6f}"
    if vf_extra:
        vf = vf_extra + "," + vf
    vf += f",scale={SCALE_W}:{h}:flags=bilinear,format=gray"
    cmd = ["ffmpeg", "-v", "error"]
    if seconds is not None and seconds > 0:
        cmd.extend(["-t", f"{seconds:.6f}"])
    cmd.extend(
        [
            "-i",
            path,
            "-an",
            "-vf",
            vf,
            "-f",
            "rawvideo",
            "-pix_fmt",
            "gray",
            "pipe:1",
        ]
    )
    proc = subprocess.Popen(
        cmd,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    assert proc.stdout is not None
    try:
        while True:
            buf = proc.stdout.read(nbytes)
            if not buf or len(buf) < nbytes:
                break
            yield buf
    finally:
        proc.stdout.close()
        proc.wait()


def mad(a: bytes, b: bytes) -> float:
    n = len(a)
    if n == 0 or n != len(b):
        return 0.0
    s = 0
    # 4-byte stride is enough: blink is a few pixels even after 160-wide scale.
    step = 1 if n < 8000 else 1
    for i in range(0, n, step):
        s += abs(a[i] - b[i])
    return s / ((n + step - 1) // step)


def frame_mads(
    path: str,
    fps: float,
    src_w: int,
    src_h: int,
    seconds: float | None = None,
    vf_extra: str = "",
) -> list[float]:
    """MAD[i] compares frame i to i-1. MAD[0] = 0."""
    prev: bytes | None = None
    out: list[float] = []
    for fr in iter_gray_frames(path, fps, src_w, src_h, seconds=seconds, vf_extra=vf_extra):
        if prev is None:
            out.append(0.0)
        else:
            out.append(mad(prev, fr))
        prev = fr
    return out


def percentile(xs: list[float], p: float) -> float:
    if not xs:
        return 0.0
    ys = sorted(xs)
    if p <= 0:
        return ys[0]
    if p >= 100:
        return ys[-1]
    k = (len(ys) - 1) * (p / 100.0)
    lo = int(math.floor(k))
    hi = int(math.ceil(k))
    if lo == hi:
        return ys[lo]
    t = k - lo
    return ys[lo] * (1 - t) + ys[hi] * t


def merge_windows(windows: Iterable[tuple[float, float]]) -> list[tuple[float, float]]:
    windows = sorted((max(0.0, a), b) for a, b in windows if b > a)
    if not windows:
        return []
    merged = [windows[0]]
    for a, b in windows[1:]:
        pa, pb = merged[-1]
        if a <= pb:
            merged[-1] = (pa, max(pb, b))
        else:
            merged.append((a, b))
    return merged


def in_windows(t: float, windows: list[tuple[float, float]]) -> bool:
    for a, b in windows:
        if a <= t < b:
            return True
        if t < a:
            return False
    return False


def classify(mads: list[float], threshold: float) -> list[bool]:
    """True = static. Frame 0 is static (nothing to compare)."""
    return [m <= threshold for m in mads]


def close_islands(
    static: list[bool],
    mads: list[float],
    fps: float,
    max_island: float,
    motion_mad: float,
) -> list[bool]:
    """Close short, low-MAD dynamic islands (blink / relative-time ticks).

    A colour-change or dashboard-open island has MAD ≫ motion_mad (p99 was
    4.8 on the 68 s flagship; blink hold p95 was 0.012). Those stay 1× even
    when they only last 2–3 frames.
    """
    if max_island <= 0 or fps <= 0:
        return static
    n = len(static)
    max_n = max(1, int(round(max_island * fps)))
    out = list(static)
    i = 0
    while i < n:
        if out[i]:
            i += 1
            continue
        j = i
        while j < n and not out[j]:
            j += 1
        peak = max(mads[i:j]) if j > i else 0.0
        nfr = j - i
        # 1–2 frames are never a command→web pair (protect windows cover those).
        # Fold them so a single encoder spike cannot split a hold into jump cuts.
        fold = i > 0 and j < n and out[i - 1] and out[j] and (
            nfr <= 2 or (nfr <= max_n and peak < motion_mad)
        )
        if fold:
            for k in range(i, j):
                out[k] = True
        i = j
    return out


def segments(
    n: int,
    fps: float,
    static: list[bool],
    protect: list[tuple[float, float]],
    min_run: float,
    max_static: float,
    trim_end: float | None,
) -> list[dict]:
    """Walk frames; emit trim segments with speed."""
    if n == 0 or fps <= 0:
        return []
    dur = n / fps
    end = dur if trim_end is None else min(dur, max(0.0, trim_end))
    n_end = min(n, max(1, int(round(end * fps))))

    segs: list[dict] = []

    def emit(i0: int, i1: int, kind: str) -> None:
        if i1 <= i0:
            return
        a = i0 / fps
        b = i1 / fps
        orig = b - a
        if orig < 1e-4:
            return
        if kind == "static" and orig >= min_run:
            new = min(max_static, orig)
            speed = orig / new if new > 0 else 1.0
            segs.append(
                {
                    "start": a,
                    "end": b,
                    "orig": orig,
                    "new": new,
                    "speed": speed,
                    "kind": "static",
                }
            )
        else:
            segs.append(
                {
                    "start": a,
                    "end": b,
                    "orig": orig,
                    "new": orig,
                    "speed": 1.0,
                    "kind": kind if kind != "static" else "short-static",
                }
            )

    i = 0
    while i < n_end:
        t = i / fps
        if in_windows(t, protect) or not static[i]:
            j = i + 1
            while j < n_end:
                tj = j / fps
                if in_windows(tj, protect) or not static[j]:
                    j += 1
                    continue
                break
            emit(i, j, "1x")
            i = j
        else:
            j = i + 1
            while j < n_end:
                tj = j / fps
                if in_windows(tj, protect) or not static[j]:
                    break
                j += 1
            emit(i, j, "static")
            i = j

    # Merge adjacent 1x / short-static (both speed 1).
    merged: list[dict] = []
    for s in segs:
        if merged and abs(merged[-1]["speed"] - s["speed"]) < 1e-9 and merged[-1]["speed"] == 1.0:
            merged[-1]["end"] = s["end"]
            merged[-1]["orig"] += s["orig"]
            merged[-1]["new"] += s["new"]
            if merged[-1]["kind"] != s["kind"]:
                merged[-1]["kind"] = "1x"
        else:
            merged.append(dict(s))
    return merged


def filter_complex(segs: list[dict]) -> str:
    if not segs:
        return "[0:v]null[out]"
    if len(segs) == 1 and segs[0]["speed"] == 1.0:
        a, b = segs[0]["start"], segs[0]["end"]
        return f"[0:v]trim=start={a:.4f}:end={b:.4f},setpts=PTS-STARTPTS[out]"
    parts = []
    labels = []
    for i, s in enumerate(segs):
        lab = f"v{i}"
        labels.append(f"[{lab}]")
        a, b, sp = s["start"], s["end"], s["speed"]
        if abs(sp - 1.0) < 1e-9:
            parts.append(f"[0:v]trim=start={a:.4f}:end={b:.4f},setpts=PTS-STARTPTS[{lab}]")
        else:
            parts.append(
                f"[0:v]trim=start={a:.4f}:end={b:.4f},setpts=(PTS-STARTPTS)/{sp:.6f}[{lab}]"
            )
    parts.append("".join(labels) + f"concat=n={len(segs)}:v=1:a=0[out]")
    return ";".join(parts)


def decision_lines(segs: list[dict]) -> list[str]:
    lines = []
    for s in segs:
        lines.append(
            f"[{s['start']:.3f},{s['end']:.3f}] {s['orig']:.3f}s → {s['new']:.3f}s  "
            f"{s['speed']:.2f}x  {s['kind']}"
        )
    return lines


def load_protect(args: argparse.Namespace) -> tuple[list[float], list[tuple[float, float]]]:
    times: list[float] = []
    extra: list[tuple[float, float]] = []
    for t in args.protect or []:
        times.append(float(t))
    for w in args.protect_window or []:
        a, b = w.split(",", 1)
        extra.append((float(a), float(b)))
    if args.protect_json:
        doc = json.load(open(args.protect_json))
        for ev in doc.get("protect") or doc.get("visual_events") or []:
            t = ev.get("t", ev.get("raw"))
            if isinstance(t, (int, float)):
                times.append(float(t))
        for w in doc.get("protect_windows") or []:
            extra.append((float(w["start"]), float(w["end"])))
        if args.trim_end is None and doc.get("trim_end") is not None:
            args.trim_end = float(doc["trim_end"])
        if args.threshold is None and doc.get("threshold") is not None:
            args.threshold = float(doc["threshold"])
    return times, extra


def protect_windows(
    times: list[float],
    extra: list[tuple[float, float]],
    pad: float,
    end_hold: float,
    duration: float,
    trim_end: float | None,
) -> list[tuple[float, float]]:
    w = extra[:]
    last = 0.0
    for t in times:
        w.append((t - pad, t + pad))
        last = max(last, t)
    if times:
        hold_end = last + max(end_hold, pad)
        w.append((last, hold_end))
    end = duration if trim_end is None else min(duration, trim_end)
    return merge_windows((max(0.0, a), min(end, b)) for a, b in w)


def estimate(segs: list[dict]) -> float:
    return sum(s["new"] for s in segs)


def probe_table(mads: list[float], fps: float, duration: float, candidates: list[float],
                protect: list[tuple[float, float]], min_run: float, max_static: float,
                trim_end: float | None, hold_lo: float, hold_hi: float,
                ignore_island: float, motion_mad: float) -> list[dict]:
    rows = []
    n = len(mads)
    hold = [mads[i] for i in range(n) if hold_lo <= i / fps < hold_hi]
    for t in candidates:
        st = close_islands(classify(mads, t), mads, fps, ignore_island, motion_mad)
        segs = segments(n, fps, st, protect, min_run, max_static, trim_end)
        n_static = sum(1 for x in st if x)
        n_runs = sum(1 for s in segs if s["kind"] == "static")
        hold_static = sum(1 for i in range(n) if hold_lo <= i / fps < hold_hi and st[i])
        hold_n = max(1, sum(1 for i in range(n) if hold_lo <= i / fps < hold_hi))
        rows.append(
            {
                "threshold": t,
                "pct_static": 100.0 * n_static / max(1, n),
                "n_static_runs": n_runs,
                "out_s": estimate(segs),
                "hold_pct_static": 100.0 * hold_static / hold_n,
                "hold_mad_p95": percentile(hold, 95) if hold else 0.0,
            }
        )
    return rows


def encode(inp: str, outp: str, filt: str, fps: float) -> None:
    # fps filter after concat so sped segments do not inherit a 200 tbr.
    full = filt.replace("[out]", "[cut]") + f";[cut]fps={fps:.6f},format=yuv420p[out]"
    run(
        [
            "ffmpeg",
            "-y",
            "-i",
            inp,
            "-filter_complex",
            full,
            "-map",
            "[out]",
            "-an",
            "-c:v",
            "libx264",
            "-pix_fmt",
            "yuv420p",
            "-preset",
            "medium",
            "-crf",
            "21",
            "-movflags",
            "+faststart",
            outp,
        ]
    )


def parse_args(argv: list[str]) -> argparse.Namespace:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--input", "-i", required=True)
    p.add_argument("--output", "-o")
    p.add_argument("--protect", action="append", default=[], help="timestamp seconds (repeatable)")
    p.add_argument("--protect-window", action="append", default=[], help="start,end seconds")
    p.add_argument("--protect-json", help="timeline JSON with protect[] / visual_events[]")
    p.add_argument("--pad", type=float, default=1.2)
    p.add_argument("--min-run", type=float, default=0.6)
    p.add_argument("--max-static", type=float, default=0.5)
    p.add_argument("--end-hold", type=float, default=1.5)
    p.add_argument(
        "--threshold",
        type=float,
        default=0.50,
        help="MAD below this is static (measured 2026-08-24 on claude-drive raw: "
        "hold p95=0.004, p99=0.49, colour-change max=80; 0.15–1.20 are equivalent)",
    )
    p.add_argument(
        "--ignore-island",
        type=float,
        default=0.08,
        help="low-MAD dynamic islands ≤ this many seconds become static (blink)",
    )
    p.add_argument(
        "--motion-mad",
        type=float,
        default=1.0,
        help="do not close an island whose peak MAD reaches this (real motion)",
    )
    p.add_argument("--trim-end", type=float, default=None)
    p.add_argument("--fps", type=float, default=None)
    p.add_argument("--log", help="write JSON decision log")
    p.add_argument("--probe", action="store_true", help="print MAD table; do not encode")
    p.add_argument(
        "--hold-range",
        default="",
        help="probe: start,end seconds of a known-static hold (blink / relative time)",
    )
    return p.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv or sys.argv[1:])
    meta = ffprobe_stream(args.input)
    fps = args.fps or meta["fps"] or 25.0
    duration = meta["duration"]
    print(
        f"static-cut: input {args.input} {meta['width']}x{meta['height']} "
        f"{duration:.3f}s fps={fps:.3f}",
        file=sys.stderr,
    )
    mads = frame_mads(args.input, fps, meta["width"], meta["height"])
    if not mads:
        print("static-cut: no frames decoded", file=sys.stderr)
        return 1
    nz = [m for m in mads if m > 0]
    print(
        f"static-cut: frames={len(mads)} mad p50={percentile(nz,50):.4f} "
        f"p90={percentile(nz,90):.4f} p95={percentile(nz,95):.4f} "
        f"p99={percentile(nz,99):.4f} max={max(mads):.4f}",
        file=sys.stderr,
    )

    times, extra = load_protect(args)
    protect = protect_windows(
        times, extra, args.pad, args.end_hold, duration, args.trim_end
    )

    hold_lo, hold_hi = 0.0, 0.0
    if args.hold_range and "," in args.hold_range:
        a, b = args.hold_range.split(",", 1)
        hold_lo, hold_hi = float(a), float(b)
    elif duration > 8:
        hold_lo, hold_hi = max(0.0, duration - 8.0), duration - 1.5

    # Candidates span blink-sized MAD up through small UI ticks.
    candidates = [0.15, 0.30, 0.50, 0.80, 1.20, 2.00, 3.50, 6.00]
    table = probe_table(
        mads, fps, duration, candidates, protect,
        args.min_run, args.max_static, args.trim_end, hold_lo, hold_hi,
        args.ignore_island,
        args.motion_mad,
    )
    print("threshold  pct_static  runs≥0.6  out_s  hold_pct_static  hold_mad_p95", file=sys.stderr)
    for row in table:
        print(
            f"{row['threshold']:9.2f}  {row['pct_static']:10.1f}  {row['n_static_runs']:8d}  "
            f"{row['out_s']:5.1f}  {row['hold_pct_static']:15.1f}  {row['hold_mad_p95']:13.4f}",
            file=sys.stderr,
        )

    print(f"static-cut: threshold={args.threshold}", file=sys.stderr)

    static = close_islands(
        classify(mads, args.threshold), mads, fps, args.ignore_island, args.motion_mad
    )
    segs = segments(
        len(mads), fps, static, protect, args.min_run, args.max_static, args.trim_end
    )
    lines = decision_lines(segs)
    out_s = estimate(segs)
    n_prot = sum(1 for s in segs if s["kind"] == "1x" and any(
        not (s["end"] <= a or s["start"] >= b) for a, b in protect
    ))
    doc = {
        "input": args.input,
        "threshold": args.threshold,
        "fps": fps,
        "duration": duration,
        "pad": args.pad,
        "min_run": args.min_run,
        "max_static": args.max_static,
        "ignore_island": args.ignore_island,
        "motion_mad": args.motion_mad,
        "end_hold": args.end_hold,
        "protect_windows": [{"start": a, "end": b} for a, b in protect],
        "protect_times": times,
        "trim_end": args.trim_end,
        "out_s": out_s,
        "segments": segs,
        "log": lines,
        "probe": table,
        "mad": {
            "p50": percentile(nz, 50),
            "p90": percentile(nz, 90),
            "p95": percentile(nz, 95),
            "p99": percentile(nz, 99),
            "max": max(mads),
        },
        "method": "local-speedup",
        "protected_1x_evidence": [
            s for s in segs if abs(s["speed"] - 1.0) < 1e-9
        ],
    }
    log_txt = "\n".join(lines) + "\n"
    print(log_txt, end="")
    print(f"static-cut: out_s={out_s:.3f} segments={len(segs)} protect={protect}", file=sys.stderr)

    if args.log:
        os.makedirs(os.path.dirname(os.path.abspath(args.log)) or ".", exist_ok=True)
        open(args.log, "w").write(json.dumps(doc, indent=2) + "\n")
        txt = os.path.splitext(args.log)[0] + ".txt"
        open(txt, "w").write(log_txt)

    if args.probe:
        return 0
    if not args.output:
        print("static-cut: --output required unless --probe", file=sys.stderr)
        return 2
    filt = filter_complex(segs)
    doc["filter"] = filt
    if args.log:
        open(args.log, "w").write(json.dumps(doc, indent=2) + "\n")
    encode(args.input, args.output, filt, fps)
    print(f"static-cut: wrote {args.output}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except subprocess.CalledProcessError as e:
        print(f"static-cut: command failed: {e}", file=sys.stderr)
        sys.exit(e.returncode or 1)

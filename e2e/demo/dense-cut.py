#!/usr/bin/env python3
"""Measure where a take is dead air and print an ffmpeg filter that
time-lapses exactly those stretches.

A live-agent take is long where the model is working: the board does not
move, nobody is typing, and the transcript grows a line a second while the
spinner turns. Those stretches are what a viewer skips — the payoffs either
side of them are what the clip exists for. This finds them by measuring the
frames rather than guessing a beat table (a live model does not repeat its
pacing), and hands export-terminal.sh a filter that keeps the head of each
such stretch at 1x and compresses the rest.

    dense-cut.py VIDEO --trim-head S [--board X:Y:W:H] [--input X:Y:W:H]
                 [--work X:Y:W:H] [--hold S] [--keep S] [--rest S]
                 [--work-min S] [--work-keep S] [--work-rest S] [--fps N]

Prints two lines on stdout — the filter_complex string, then the expected
output length in seconds — and the segment plan on stderr.

Detection is all ffmpeg: frames sampled at --fps in grey, a three-frame
temporal median (a one-frame flash — the TUI clearing a line before it
redraws it — is not a change), the difference to the frame before, a
threshold at --amp grey levels, and signalstats' mean over the result,
which times the region's area is the count of pixels that moved. A frame
is a change in a region when more than --pixels of them did. --amp 40 is
tuned against the roster's live dot and the "Synced" dot, which breathe at
a few grey levels and which mpdecimate's block sums counted as motion (it
kept a frame every 0.6s through a 20s think).

Three regions, each a crop this script is told about because only the
config that framed the take knows where they are:

  --board  the list / dashboard column. A change here is a payoff, and
           every payoff ships at 1x.
  --input  the row the person types into. Typing is content — the Korean
           prompt is the clip's second claim — and ships at 1x.
  --work   the transcript above the input row, minus the spinner band. A
           run of changes that are *only* here is the agent working.

Two rules make the cut:

  * a still stretch — no change anywhere for longer than --hold — keeps its
    first --keep seconds at 1x and compresses the rest to --rest seconds.
  * a working stretch — a run of --work-only changes at least --work-min
    long — keeps its first --work-keep seconds at 1x (the prompt echo, the
    first line of the answer) and compresses the rest to --work-rest.

Stretches under those floors ship whole, so a scripted take whose beats are
deliberate reading holds, run with no --work region, passes through unchanged.
The closing hold is never compressed: the last frame is what the clip ends
on, and the viewer is meant to sit in it.
"""
import argparse
import re
import subprocess
import sys


def probe(video):
    out = subprocess.run(
        [
            "ffprobe", "-v", "error", "-select_streams", "v:0",
            "-show_entries", "stream=width,height:format=duration",
            "-of", "csv=p=0",
            video,
        ],
        check=True, capture_output=True, text=True,
    ).stdout.split()
    w, h = (int(x) for x in out[0].split(",")[:2])
    dur = float(out[-1].split(",")[-1])
    return w, h, dur


def rect(spec):
    x, y, w, h = (int(v) for v in spec.split(":"))
    return x, y, w, h


def change_times(video, trim, fps, region, amp, pixels):
    """Sampled timestamps (from trim) at which `region` differs from the frame before."""
    x, y, w, h = region
    chain = [
        f"crop={w}:{h}:{x}:{y}",
        f"fps={fps}",
        "format=gray",
        "tmedian=radius=1",
        "tblend=all_mode=difference",
        f"lutyuv=y='if(gt(val,{amp}),255,0)'",
        "signalstats",
        "metadata=print:key=lavfi.signalstats.YAVG:file=-",
    ]
    out = subprocess.run(
        [
            "ffmpeg", "-nostdin", "-v", "error", "-ss", str(trim), "-i", video,
            "-vf", ",".join(chain), "-an", "-f", "null", "-",
        ],
        check=True, capture_output=True, text=True,
    ).stdout
    times = []
    pts = None
    for line in out.splitlines():
        m = re.search(r"pts_time:([0-9.]+)", line)
        if m:
            pts = float(m.group(1))
            continue
        m = re.search(r"YAVG=([0-9.]+)", line)
        if m and pts is not None:
            if float(m.group(1)) * w * h / 255.0 > pixels:
                times.append(pts)
    return times


def stretches(events, usable, a):
    """Yield (start, end, kind) for every still and working stretch."""
    # Still: gaps between any two changes.
    anyt = sorted({t for t, _ in events}) or [0.0]
    for t0, t1 in zip([0.0] + anyt, anyt + [usable]):
        if t1 - t0 > a.hold:
            yield t0, t1, "still"
    if not a.work:
        return
    # Working: maximal runs of work-only changes with no board/input change
    # inside and no gap over --hold (a gap that long is the still rule's).
    run = []
    for t, kind in events + [(usable, "end")]:
        if kind == "work" and (not run or t - run[-1] <= a.hold):
            run.append(t)
            continue
        if run and run[-1] - run[0] >= a.work_min:
            yield run[0], run[-1], "work"
        run = [t] if kind == "work" else []


def plan(events, usable, a):
    """Return [(start, end, speed)] covering [0, usable]."""
    fast = []
    for s, e, kind in stretches(events, usable, a):
        if e >= usable:
            # The tail is the closing hold — the wall, the finished list —
            # and the one still stretch a viewer is meant to sit in.
            continue
        keep, rest = (a.keep, a.rest) if kind == "still" else (a.work_keep, a.work_rest)
        f0 = s + keep
        if e - f0 <= rest * 1.5:
            continue
        fast.append((f0, e, (e - f0) / rest, kind))
    fast.sort()
    segs = []
    cursor = 0.0
    for f0, e, speed, kind in fast:
        if f0 < cursor:  # overlapping rules: the earlier one owns the frames
            continue
        if f0 > cursor:
            segs.append((cursor, f0, 1.0, ""))
        segs.append((f0, e, speed, kind))
        cursor = e
    if cursor < usable:
        segs.append((cursor, usable, 1.0, ""))
    return segs


def filter_for(segs):
    n = len(segs)
    parts = [f"[0:v]fps=30,format=yuv420p,split={n}" + "".join(f"[i{k}]" for k in range(n)) + ";"]
    for k, (s, e, speed, _) in enumerate(segs):
        trim = f"trim={s:.3f}:{e:.3f}" if k < n - 1 else f"trim=start={s:.3f}"
        pts = "PTS-STARTPTS" if speed == 1.0 else f"(PTS-STARTPTS)/{speed:.4f}"
        parts.append(f"[i{k}]{trim},setpts={pts}[s{k}];")
    parts.append("".join(f"[s{k}]" for k in range(n)) + f"concat=n={n}:v=1:a=0,fps=30[v]")
    return "".join(parts)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("video")
    ap.add_argument("--trim-head", type=float, default=0.0)
    ap.add_argument("--board", type=rect, help="X:Y:W:H of the list/dashboard column")
    ap.add_argument("--input", type=rect, help="X:Y:W:H of the typing row")
    ap.add_argument("--work", type=rect, help="X:Y:W:H of the transcript, spinner band excluded")
    ap.add_argument("--hold", type=float, default=2.4, help="a gap longer than this is a still stretch")
    ap.add_argument("--keep", type=float, default=1.2, help="seconds of a still stretch kept at 1x")
    ap.add_argument("--rest", type=float, default=0.5, help="what the rest of a still stretch becomes")
    ap.add_argument("--work-min", type=float, default=4.0, help="shortest run that counts as working")
    ap.add_argument("--work-keep", type=float, default=2.0, help="seconds of a working stretch kept at 1x")
    ap.add_argument("--work-rest", type=float, default=2.5, help="what the rest of a working stretch becomes")
    ap.add_argument("--fps", type=int, default=10)
    ap.add_argument("--amp", type=int, default=40, help="grey levels a pixel must move to count")
    ap.add_argument("--pixels", type=int, default=80, help="changed pixels that make a change frame")
    a = ap.parse_args()

    w, h, dur = probe(a.video)
    usable = dur - a.trim_head
    regions = {"board": a.board, "input": a.input, "work": a.work}
    if not any(regions.values()):
        regions = {"work" if a.work else "any": (0, 0, w, h)}
    events = []
    for kind, r in regions.items():
        if r:
            events += [(t, kind) for t in change_times(a.video, a.trim_head, a.fps, r, a.amp, a.pixels)]
    events.sort()

    segs = plan(events, usable, a)
    out_len = sum((e - s) / sp for s, e, sp, _ in segs)
    for s, e, sp, kind in segs:
        tag = "1x   " if sp == 1.0 else f"{sp:4.1f}x"
        print(f"  {s:6.1f} → {e:6.1f}  {tag}  ({e - s:5.1f}s → {(e - s) / sp:4.1f}s) {kind}", file=sys.stderr)
    print(f"dense-cut: {len(events)} change frames, {usable:.1f}s → {out_len:.1f}s", file=sys.stderr)
    print(filter_for(segs))
    print(f"{out_len:.1f}")


if __name__ == "__main__":
    main()

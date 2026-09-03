"""Mark arithmetic for the round-trip cut (GDK-1159).

Two jobs, both about one problem: the spec's wall clock and the recorder's
clock do not start together, and the gap MOVES between takes (measured -0.3s,
+0.5s, +0.02s on three consecutive takes). A cut list written in raw seconds
was a second early once and ended the film before the card landed.

    python3 rt-marks.py lead <take.webm> <proof.jsonl>
        The gap, in seconds, found rather than guessed: the terminal pane
        opening drops the average luma of the pane's column off a cliff
        (38 -> 28 on a dark take), and that cliff IS the `pane_open` mark.

    python3 rt-marks.py at <proof.jsonl> <mark> <offset> <lead>
        A mark's position in video seconds, plus an offset.

Lives in its own file because a heredoc nested inside `$( )` inside bash is a
parse error waiting to happen, and it was.
"""

import json
import statistics
import subprocess
import sys


RECORDS = {}


def load(proof):
    marks = {}
    with open(proof) as f:
        for line in f:
            line = line.strip()
            if line:
                rec = json.loads(line)
                marks.setdefault(rec["mark"], rec["epoch_ms"])
                RECORDS.setdefault(rec["mark"], rec)
    if "start" not in marks:
        sys.exit(f"rt-marks: no 'start' mark in {proof}")
    return marks


def pane_crop(take):
    """The pane's region of the take as an ffmpeg crop, from the pane_open
    mark's rect (CSS px → take px via the start mark's viewport). The 0.19
    literal below encoded a side pane; 0.20 made the dock a bottom band and the
    literal would have measured paper. With the rect, the geometry is the
    spec's, not this file's."""
    rec = RECORDS.get("pane_open") or {}
    rect = rec.get("rect")
    if not rect:
        return "520:600:272:120"
    vp = (RECORDS.get("start") or {}).get("viewport")
    scale = 1.0
    if vp:
        w = subprocess.run(
            ["ffprobe", "-v", "error", "-select_streams", "v:0",
             "-show_entries", "stream=width", "-of", "csv=p=0", take],
            capture_output=True, text=True).stdout.strip()
        if w:
            scale = int(w) / vp["w"]
    # inset a little so the pane's own border and the roster's chrome do not
    # dilute the cliff
    x = int((rect["x"] + 8) * scale)
    y = int((rect["y"] + 8) * scale)
    w = int((rect["w"] - 16) * scale)
    h = int((rect["h"] - 16) * scale)
    return f"{w}:{h}:{x}:{y}"


def seconds(marks, name):
    if name not in marks:
        sys.exit(f"rt-marks: the take has no '{name}' mark")
    return (marks[name] - marks["start"]) / 1000.0


def luma_series(take, upto):
    """(pts_seconds, average_luma) over the pane's column, frame by frame."""
    out = subprocess.run(
        ["ffmpeg", "-v", "error", "-t", f"{upto:.1f}", "-i", take,
         "-vf", f"crop={pane_crop(take)},signalstats,"
                "metadata=print:key=lavfi.signalstats.YAVG:file=-",
         "-f", "null", "-"],
        capture_output=True, text=True).stdout
    series, pts = [], None
    for line in out.splitlines():
        line = line.strip()
        if line.startswith("frame:"):
            pts = int(line.split("pts:")[1].split()[0])
        elif "YAVG=" in line and pts is not None:
            series.append((pts / 1000.0, float(line.split("YAVG=")[1])))
    return series


def full_luma_series(take, start, length):
    """(pts_seconds, average_luma) over the whole frame from `start`."""
    out = subprocess.run(
        ["ffmpeg", "-v", "error", "-ss", f"{start:.2f}", "-t", f"{length:.1f}",
         "-i", take, "-vf", "signalstats,"
         "metadata=print:key=lavfi.signalstats.YAVG:file=-",
         "-f", "null", "-"],
        capture_output=True, text=True).stdout
    series, pts = [], None
    for line in out.splitlines():
        line = line.strip()
        if line.startswith("frame:"):
            pts = int(line.split("pts:")[1].split()[0])
        elif "YAVG=" in line and pts is not None:
            series.append((start + pts / 1000.0, float(line.split("YAVG=")[1])))
    return series


def main():
    cmd = sys.argv[1]
    if cmd == "at":
        proof, name, off, lead = sys.argv[2], sys.argv[3], float(sys.argv[4]), float(sys.argv[5])
        print(f"{seconds(load(proof), name) + lead + off:.2f}")
        return

    if cmd != "lead":
        sys.exit(f"rt-marks: unknown command {cmd!r}")

    take, proof = sys.argv[2], sys.argv[3]
    pane_open = seconds(load(proof), "pane_open")
    series = luma_series(take, pane_open + 4)
    if len(series) < 30:
        sys.exit("rt-marks: could not read luma from the take")
    # Ignore the first second. The recorder's opening frames are the page
    # before it paints — the very first sample reads 24.9 against a settled
    # 36.6 — and a search that starts at t=0 locks onto that instead of the
    # pane, which put the whole cut 3.6 seconds early.
    settled = [(t, v) for t, v in series if t >= 1.0]
    if len(settled) < 20:
        sys.exit("rt-marks: take too short to calibrate")
    base = statistics.median(v for _, v in settled[:20])
    drop = next((t for t, v in settled if v < base - 4.0), None)
    if drop is None:
        # No cliff: the pane opens in the first second, so the "before"
        # baseline is already dark. Second witness: the ⌘K palette at b_enter
        # dims the whole frame with its backdrop — a global luma drop, late in
        # the take, and the lead is one number for the whole take.
        marks = load(proof)
        if "b_enter" in marks:
            b = seconds(marks, "b_enter")
            series = full_luma_series(take, max(b - 2.0, 0.0), 5.0)
            if len(series) >= 30:
                base = statistics.median(v for _, v in series[:15])
                hit = next((t for t, v in series if v < base - 2.5), None)
                if hit is not None:
                    print(f"{hit - b:.2f}")
                    return
        # Neither witness: the two clocks start within a frame or two of each
        # other in that case, so 0 is closer than any guess and the sheet is
        # the check either way.
        print("0.00")
        return
    print(f"{drop - pane_open:.2f}")


main()

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


def load(proof):
    marks = {}
    with open(proof) as f:
        for line in f:
            line = line.strip()
            if line:
                rec = json.loads(line)
                marks.setdefault(rec["mark"], rec["epoch_ms"])
    if "start" not in marks:
        sys.exit(f"rt-marks: no 'start' mark in {proof}")
    return marks


def seconds(marks, name):
    if name not in marks:
        sys.exit(f"rt-marks: the take has no '{name}' mark")
    return (marks[name] - marks["start"]) / 1000.0


def luma_series(take, upto):
    """(pts_seconds, average_luma) over the pane's column, frame by frame."""
    out = subprocess.run(
        ["ffmpeg", "-v", "error", "-t", f"{upto:.1f}", "-i", take,
         "-vf", "crop=520:600:272:120,signalstats,"
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
        sys.exit("rt-marks: no pane-open luma cliff — pass GADAK_RT_LEAD by hand")
    print(f"{drop - pane_open:.2f}")


main()

# The release video gate

Every minor version ships one short video, made from that release's
changelog. The rule has a back side, and the back side is the point:

> **If there is nothing to film, the release is not finished.**

So this is not a marketing checklist run after the tag. It is a gate on
**scope**, run when the minor's contents are being decided — the sibling of
[`release-audit.md`](release-audit.md), which runs at the other end.

## Why this runbook exists

v0.19 got the judgment the expensive way. Four issues built the shooting rig
(GDK-1115 through GDK-1118), a two-camera 26-second hero was shot, and only
then did anyone ask what it was arguing. The maker's verdict: *"I can't tell
what it's even about, and it builds no anticipation."*

Scoring the 29 issues closed for that minor found **zero** that would carry a
video: 24 of them were bugs and UI polish. v0.19 was, up to that point, a
release that *fixed* things rather than one where something *appeared*.

The lesson is the ordering. The question below is cheap when asked before the
work and expensive when asked after it.

## The five gates

Answerable yes/no **before** anything is built.

| | Gate | Disqualifying signal |
|---|---|---|
| **G1** | Does it say itself in one sentence — **without the product's name**? | If the name is needed, it is a feature description, not a claim |
| **G2** | Is the difference **in the pixels**? | "It no longer breaks / is safer / doesn't lose things" — **absence cannot be filmed** |
| **G3** | Does the drama happen **inside the frame**? | Waiting, background work, being away from the desk |
| **G4** | Is it readable in **five seconds with no prior knowledge**? | A concept has to be explained first |
| **G5** | Does it create **envy**? | "That'd be handy" is not enough — it must be a pain they have today |

### G2 is the one that gets missed

Both of v0.19's failures were the same disease.

The hero's climax was *"you walk away, come back, and the terminal scrollback
is still there, the issue is Done."* That is a **continuity** claim: the
viewer cannot feel the absence of a failure they never expected. Relief does
not travel.

Then, having diagnosed exactly that, the replacement sentence drafted for the
next attempt was *"there is no copy-paste between the terminal and the
tracker."* Also an absence. Three separate reviewers caught it independently.

**A claim of absence describes a feature. A claim of existence can be
photographed.** The repair is usually a rewrite, not a reshoot:

- ✗ "There is no copy-paste between the terminal and the tracker."
- ✓ "The command that just failed files itself as a ticket."

### G3: the drama has to be reachable by a camera

The hero's most interesting minute — an agent working while nobody watched —
happened by design where the camera was not. A film whose argument lives
off-frame does not improve with a better take.

## Scoring

Score every candidate feature in the minor, and score the doors not taken
too — the comparison is most of the value. One table, one line of reasoning
per verdict:

| Feature | G1 | G2 | G3 | G4 | G5 | One line |
|---|:-:|:-:|:-:|:-:|:-:|---|

A verdict without a reason is not a verdict. If a gate is borderline, say
which way you called it and why; that sentence is what a later reader needs.

**If nothing in the minor passes, that is the finding.** Do not lower the
gates and do not shoot anyway. Take the scoring to the backlog and ask which
open item would pass — that answer is the roadmap steer the gate exists to
produce.

## Marking the backlog

Issues that pass carry the **`filmable`** label, applied when the issue is
opened, with the G1 sentence in the description.

The alarm is a count: **a minor whose scope contains zero `filmable` issues
is under-scoped**, and it rings while there is still time to act on it.

```bash
gadak --profile oss sql "select key, priority_rank, summary from issues_full
  where json_extract(labels,'\$') like '%filmable%'
    and json_extract(labels,'\$') like '%release-0.20%'
  order by priority_rank"
```

## Shooting is a separate craft

Legibility, aspect ratio and pacing are not gates — a feature does not fail
for being hard to frame. But one measurement from the v0.19 post-mortem
belongs here, because it explains a film that passed every gate on paper and
was still unreadable:

The desk footage was a 1440×900 viewport scaled into a 1080-high frame. At
1:1, a row of body text stood about 10 px tall in a 900 px frame — **roughly
1.1% of frame height**, against the 5–8% that subtitle practice treats as
readable. On a phone, where this kind of video is actually watched, nothing
in it could be read at all.

The fix is at capture time, not in post: **shoot a viewport small enough that
the interface fills the frame.** Cropping into a wide capture afterwards only
enlarges soft pixels.

Two more things that survived the same post-mortem:

- **Deterministic beats live.** The failed shoot needed a live model, a
  simulator, and a 45-second away-wait, so every retake cost eight minutes.
  A take driven by scripts and Playwright can be retimed frame by frame.
- **Say the name.** 26 seconds went by without the word *gadak* appearing at
  a readable size, and without a URL. Reach spent on an unnamed product is
  reach thrown away.

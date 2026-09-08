---
title: "gadak 0.21: what happened while you were away"
date: 2026-09-08
description: "A tracker stores state. The person coming back needs the change. 0.21 computes it from the history the mirror already holds — nothing new collected, nothing leaving the machine, and every number printed with its definition."
lang: en
---

Monday. You open the tracker. The list looks the way it always looks: forty
issues, the same columns, the same colours. Something moved over the
weekend — a reviewer commented, a blocker closed, somebody reassigned two of
yours — and the list does not say which. The notifications that would have
said it arrived on Friday evening, and you archived them on Friday evening,
because that is what one does with notifications on a Friday evening.

The tracker knew. Every one of those events is in its history. It simply had
no sentence for the question you were asking, which was not "what is the
state of the project" but "what changed since I last looked". A tracker
stores state. A person coming back needs the change.

This essay is about that gap, and about the release that closes part of it.
The [0.18 essay](/essays/jql-has-no-group-by/) argued about *where* the data
should live: behind an API, or in a file on your machine. This one is about
*when* — what a mirror can tell you at the moment you return, because it has
been keeping the history all along.

## The mirror already knows

gadak keeps a local SQLite mirror of your tracker — Jira, Linear, or its own
built-in one. The mirror carries the changelog of every issue, every comment,
and — in a second file beside it — the record of which issues *you* opened,
and when. None of that is new. It has been there since the first sync.

What 0.21 adds is not a collector. It is the set of questions we finally ask
that data, and the rule that every answer arrives with the definition of how
it was computed printed beside it. Nothing in this release reaches out to a
network, and nothing new leaves the machine. Everything below is a read.

## Three questions on return

**What changed.** The list now opens with one dim line: *since last session
3 h ago · 7 issues changed · 2 of them assigned here*. The boundary is not midnight and not "since your last
login". It is your own previous *session* — a run of reads with no gap
longer than thirty minutes between them. Open an issue and the detail panel
does the same at a smaller scale: *since last opened 3 days ago — 2 status
changes, 1 new comment*. The line is latched when the page loads and never
increments while you work. That is deliberate. A counter that ticks up is a
notification; a line you read once on arrival is awareness. We wanted the
second one.

**How long has this sat.** Trackers answer age with a fixed number: seventy-
two hours, and then the row goes amber. Seventy-two hours is nobody's
number. It is a default someone typed into a settings file. 0.21 learns the
threshold from your own workspace instead: the 85th percentile of cycle time
over the issues your team finished in the last ninety days, never reopened.
If your team closes most things in two days, an issue in progress for four is
already unusual and the mark says so. If you close things in three weeks, the
mark stays quiet for three weeks. The chip that shows an issue's duration
tells you the team's p85 on hover, and `list`, `ready` and `next` carry
`age_days` in the CLI so an agent sees the same number a person does.

**Did we get faster.** `gadak retro` prints one table, ISO weeks across the
top: sessions, time to first write after arriving, how old the work in
progress is (p85 and the single oldest), how many issues closed and how long
they took (median and p85), and *mismatch* — more on that in a moment. Under
the table, every row's definition, every time. `--open` puts the issues
behind any cell on the app, so a number is never a dead end. The table is
weekly because a week is the smallest span over which any of this means
anything, and because the retrospective is the one meeting where a team is
already asking these questions from memory.

## The row only a mirror can produce

The last row of that table is *mismatch*: comments claiming the work is
finished, on issues whose status says it is not. "Merged, closing." "Fixed in
the release branch." "완료했습니다." The comment is the truth as the person
saw it; the status is the truth as the tracker recorded it; and they
disagree.

Jira cannot make this row. Comment text and status live in different
endpoints, and JQL has no join. The mirror has both in one file, so the
question is one query. When the panel detects a done-sounding comment under
an open status, it offers a single quiet button — *Move to done* — with the
fact that earned it on hover: *the latest comment says done; the status is
still In Review*. Dismiss it once and it stays dismissed for that issue.

We shipped the first version of this heuristic wrong. It matched substrings,
so "abandoned" contained "done", "unresolved" contained "resolved", and the
Korean 미완료 (*not* done) contained 완료 (done). The signal pointed
backwards, at exactly the comments saying the work was not finished. The fix
is a guard, not a parser: English words must stand on their own, a CJK done
word must not carry a negation prefix or suffix, quoted text and code fences
are stripped, questions are rejected, and only comments newer than the last
status change count. A test table of nineteen rows failed against the old
rule before it passed against the new one. The retro row says *heuristic* in
its own definition, and it means it.

## What we refused to build

Every one of these signals was one step away from a feature we did not want.

No score. The retro shows *closed this week* and *cycle time*; it does not
rank people, and there is no per-person column. Feedback that reads as a
judgement of the person changes behaviour less reliably than feedback about
the task, and often for the worse — that finding is old and robust, and it is
the reason the subject of every sentence gadak prints is the issue, never
the reader.

No notification. Nothing in 0.21 interrupts. The session line, the resume
card, the age mark and the *Move to done* button all wait where you will
look next, and say nothing when there is nothing to say. On a quiet morning
the session line does not appear at all.

No fixed SLA as the rule. The stale threshold is your workspace's own
distribution whenever there is enough of one. Until eleven issues have
finished in the last ninety days the mark falls back to seventy-two hours,
and the setting says so in as many words — the fixed number is the fallback,
not the definition.

No outbound. The retro, the session line and the learned threshold are all
computed from the two files on your disk. No aggregate is sent anywhere, and
there is no account to send it to.

## The agent's line

One more change belongs in this essay, though it is a single line of text.
When a coding agent writes to Jira Cloud or Linear through gadak — a comment,
a transition, a new issue — that write now ends with `— via gadak · Name`.

The reason is dull and important. On those origins an agent writes with a
person's credential, and the tracker cannot tell the two apart. Before 0.21,
a comment the agent left looked exactly like one you typed. That is fine
until it is wrong, and then nobody knows whose mistake it was. A team that is
going to let agents into the tracker needs, first, to be able to see them
there. The line is the smallest thing that does that, and `actor.trailer =
false` removes it if your team has decided otherwise.

## Where this loses

The honesty rule from the last essay applies here too. These are the places
the numbers are weaker than they look.

*Every number on this page comes from one workspace: mine.* gadak's own
backlog runs on gadak. That is a real workspace with real weeks in it, and it
is one data point written by the person who built the instrument. The
success condition I set for 0.21 was not a feature list but a sentence from
someone who is not me, with numbers from their own mirror, saying the return
got cheaper. That sentence does not exist yet. When it does, it will be
quoted, not paraphrased; until then, this release is a claim about what the
mirror can compute, not about what it does for you.

*The mismatch row is a heuristic.* Word boundaries and negation guards get
it to a precision where a button you can dismiss once is a fair trade. They
do not get it to a number you should put in a report about a person. The
definition under the row says so.

*Linear has no changelog.* Its API gives an issue's started and completed
timestamps, so the flow columns fill from those, and only when the issue next
moves. An existing Linear mirror shows dashes until it does.

*Thirty minutes is a guess.* The session boundary is a parameter with a
default, bounded between five minutes and a day, and printed under the table
so you know which value produced the row.

## What the two essays add up to

The 0.18 essay made one argument: put the data in a file and questions the
API cannot ask become one query. This one makes the second: once the history
is in that file, the moment you return can be answered from it — what
changed, how long this has sat, whether the week went well — without
collecting anything new and without a number that is not defined beside
itself.

Neither argument needs an agent to be true. Both get sharper with one in the
room, because an agent that reads the same mirror sees the same session line,
the same age, the same p85, and answers in the same numbers.

## Try it

```bash
gadak retro                    # the weekly table, definitions under it
gadak retro --open closed --week 1
gadak list                     # open issues, age_days beside the priority
```

The [live demo](/demo/) runs the same UI over a 534-issue fixture in your
browser, session line included. The reasoning behind the signals — which
research each one leans on, and what we chose not to do with it — is in
[THEORY.md](https://github.com/midagedev/gadak/blob/main/docs/project/THEORY.md).
The row definitions are in
[DERIVE.md](https://github.com/midagedev/gadak/blob/main/docs/DERIVE.md), as
SQL you can run against your own mirror.

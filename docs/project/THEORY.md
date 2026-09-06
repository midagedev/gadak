# Theory: a mirror that gets work done

Why gadak's next features are the ones they are, and why some obvious ones are
not. `UX_PRINCIPLES.md` says how a screen should behave; this document says
why the work behind the screen goes better. Specs for rounds that add a
signal, a card, or a view quote the tenet and the grammar rule they serve.

Established 2026-09-06 from a literature review (not a user study — see
*Limits*). Every claim below names its source; where a source could not be
found the claim was dropped. Korean discussion of the same material lives in
the owner's notes; this file is the English original.

## The one-sentence version

Making cognition easier means keeping the state you would otherwise hold in
your head in the mirror, and handing it back the instant you return.

Five bodies of research say this in five vocabularies. Working memory is
small (individual cognition). The more work is in progress, the later all of
it finishes (flow). Teams move on what they can see of each other's state
(coordination). Agents must work where a person can see them (human–agent).
Screens should show change before state (representation). gadak already put
a mirror in exactly that place; what remains is the signal the mirror holds
but does not yet show.

## Six tenets

Every proposal is judged against these. A feature that advances none of them
is not gadak's next value.

**T1 — State belongs outside the head.** People offload what they can trust
to an external store and then genuinely remember it less (Risko & Gilbert
2016); the strategy is rational only while the store is trustworthy. A stale
or wrong mirror forces double bookkeeping. Freshness chips and honest sync
errors are therefore a cognitive contract, not decoration. (Hollan, Hutchins
& Kirsh 2000; Kirsh 1995; Scaife & Rogers 1996.)

**T2 — The cost of returning is the cost of the work.** Interruptions cannot
be prevented; only resumption can be designed. In 10,000 recorded
programming sessions only 10% resumed editing within a minute of an
interruption; most navigated first to rebuild context (Parnin & Rugaber
2011). Attention residue from an unfinished task degrades the next one (Leroy
2009). The tool's first job is to answer "where was I, and what changed
since" on arrival. This is the cognitive basis of `UX_PRINCIPLES.md` §6.

**T3 — Change is the information.** A list shows state; a delta shows what
the reader actually needs. Passive awareness of others' activity lowers
coordination cost without the interruption cost of notification (Dourish &
Bellotti 1992). A notification is an interruption; awareness is what is
visible when you come back.

**T4 — Age is the risk.** Little's law: WIP = throughput × cycle time.
Adding in-progress work lengthens every item's cycle time. Of the flow
metrics, *work item age* is the only one that speaks about work still in
progress (Vacanti 2015); an in-progress issue whose *age* has passed the 85th
percentile *cycle time of the team's finished work* is not going to be late —
it already is. Both sides of that comparison matter: age is measured on work
in flight, the percentile on work that finished (the aging chart's reading,
ActionableAgile). DeGrandis's five time thieves (too much WIP, unknown
dependencies, unplanned work, conflicting priorities, neglected work) all
leave traces in Jira data.

**T5 — The ticket is a boundary object.** A developer reads an issue as a
task, a manager as progress, an agent as a node in a state machine. Objects
that several communities read differently yet coordinate through work only
when they carry structure all sides can read (Star & Griesemer 1989). Hence
`status_category` and `links` outrank prose *as keys* — not as the truer
record — and a ticket whose comments say "done" while its status does not is a
*candidate* defect signal, guarded and never counted as a fact (Aranda &
Venolia 2009 found tracker records erroneous or misleading in seven of ten
reconstructed cases). Issue trackers are communication hubs before they are
databases (Bertram et al. 2010).

**T6 — The agent is a teammate.** Automation that takes work away leaves the
person least able to intervene exactly when it fails (Bainbridge 1983);
trust must be calibrated to what the automation actually did (Lee & See
2004); how much the machine decides on its own is a choice on a scale, not a
convention (Parasuraman, Sheridan & Wickens 2000). Agents in gadak read the
same mirror and write through the same origin as the person — a structural
precondition already met. Two things are still missing: a **preview** of the
write an agent is about to make, and one place where a person sees what it
changed. Four risks name themselves: complacency under load, loss of the
out-of-the-loop reader's awareness, WIP inflation by claiming (T4), and
attribution laundering — on a Jira or Linear origin the agent writes under a
person's credential and the origin cannot tell the two apart.

## Two stances

Most Jira users take two stances a day: **contributor** (my issues) and
**steward** (the team's issues). The research treats them as different
problems.

The contributor's task is execution: one issue's context in working memory,
cost paid in resumption and next actions. Nearly everything under T1–T2 is
about this stance. Unit: the issue. Question: "what do I do now, and what am
I waiting on?"

The steward's task is supervision — human-factors' *supervisory control*
(Sheridan 1992): watch several semi-autonomous workers, intervene on
exceptions. Unit: the distribution (WIP per person, the age tail, the
blocking graph, the priority spread). People are poor at scanning lists for
rare events (Mackworth 1948); give a steward a list and they miss it, give
them the exception and they see it.

The stances hurt each other. A steward's cheapest way to learn state is to
ask, and asking interrupts the contributor. Daily stand-ups drift from
coordination into status reporting to the manager (Stray, Sjøberg & Dybå
2016). Shared-workspace awareness exists to break this loop: a steward view
that reads derived signals (age, silence, blocks) instead of demanding
reports is a device that protects the contributor's flow, not a convenience.

Steward views slide into surveillance. Three design rules follow: aggregate
by *work* (issues, queues, epics), with people as a secondary axis only;
never rank people; and show nothing Jira does not already show that person —
the mirror creates no new data.

When agents join the team the steward stance generalizes without change:
what did it do, where did it stop, where is my decision needed. An "agent
activity" view is a column of the steward view, not a separate feature.

## Coaching grammar

The owner's condition: help the user, but never overtly — it has to flow.
The literature says this is not taste but evidence.

Feedback interventions improve performance on average, yet more than a third
of 607 measured effects *decreased* it; the effect shrinks as attention moves
from the task toward the self (Kluger & DeNisi 1996). Directive feedback
provokes reactance (Brehm 1966) and undermines intrinsic motivation, where
informational feedback supports it (Ryan & Deci 2000). The Office Assistant
descended from a design that intervened only when expected utility exceeded
cost (Horvitz et al. 1998); the product dropped the cost side and became the
canonical interrupting advisor. Guidance that helps novices burdens experts
(Kalyuga et al. 2003); scaffolding is built to be removed (Wood, Bruner &
Ross 1976).

Influence without speech is possible and measured: arrangement — defaults,
order, salience — outperforms advice (Thaler & Sunstein 2008), and disclosing
a nudge does not weaken it (Loewenstein et al. 2015); behaviour follows fast,
automatic processes more than motivated ones (Adams et al. 2015); a small
friction at the moment of a bad habit prompts reflection without blocking
(Cox et al. 2016); information should rest in the periphery and move to the
centre only when it earns it (Weiser & Brown 1996; Pousman & Stasko 2006);
deferring interruptions to task breakpoints sharply lowers their cost (Iqbal
& Bailey 2008); visible small progress is the strongest motivator (Amabile &
Kramer 2011) and scores, streaks and rankings are its degenerate form
(Forsgren et al. 2021).

The rules, each with the compliant form first and the violation second:

- **G1 Speak about the work, never about the person.** "In progress for 12
  days" / ~~"You have too much in progress"~~. The subject is always an
  issue, a queue, an epic.
- **G2 Inform, don't instruct.** State the fact; place the action beside it.
  A "Move to Done" button next to "this comment says done" / ~~"Please update
  the status"~~. Verbs live on buttons, not in sentences.
- **G3 Speak only at boundaries.** Session start, opening an issue, just
  before a transition, choosing a priority. Never while editing, reading, or
  inside the terminal.
- **G4 Arrangement is the coaching.** Default sort by age and priority, a
  first screen that is "my work" — before adding a sentence, ask whether
  order, density or weight can do the same job.
- **G5 Quiet until there is a reason.** Every signal has a peripheral form
  (dim meta) and a central form (weight, chip) with a numeric promotion
  condition. Colour is the last resort; red is not used.
- **G6 Friction, never a block.** Starting a fourth in-progress issue shows
  the count. No confirm dialog, no warning icon. If the user proceeds, the
  user is right.
- **G7 Say why it is shown; remember when it is dismissed.** Every signal has
  a one-line basis on hover ("p85 cycle time 9d; this issue 12d"). Dismissal
  is remembered per signal and per scope and never re-asked. Transparency
  costs nothing (Loewenstein et al. 2015).
- **G8 Withdraw as the user learns.** Per signal kind and scope, count how
  often it was shown and how often the user handled it unprompted — meaning
  the condition the signal names stopped being true in the same session *and*
  the signal's own affordance was not clicked. After three consecutive, the
  signal drops from its central form to its peripheral one; it is never
  deleted, the layout never moves, and it is not re-promoted within fourteen
  days. Coaching succeeds by disappearing (Carroll & Carrithers 1984).
- **G9 Show progress, never a score.** Issues closed this week and a shorter
  age tail live where the user goes on purpose (the team-flow view). No
  push, no streaks, no rankings, no "well done".
- **G10 If there is a conversation, speak there.** The agent is the natural
  channel: when the user asks "what next", the skill has the answer carry
  age, blockers, and who is waiting. Asked-for is an answer, not an
  interruption. The skill file is where coaching content lives.

Success is when the user does not feel coached and does feel that things
somehow stay in order. There is no coaching-intensity setting: if the grammar
is right, none is needed (`UX_PRINCIPLES.md` §4).

## Seven moments

Applied to gadak's screens, coaching reduces to seven boundaries. Outside
this table gadak says nothing.

| Boundary | Fact shown | Adjacent action | Promotion to centre | Rules |
|---|---|---|---|---|
| Session start | Session strip: what changed since my previous *session* of reads | Open the changed items as a list | Only when something changed; otherwise no strip. One utterance per session — the count is latched at load and never increments, which is what keeps it awareness and not notification | G3 G1 |
| Opening an issue | One meta line: age · last change · blockers, and the resume card — what changed since my previous visit *to this issue* | Transition · comment (existing UI) | Always dim; past the p85 age, weight and amber on the glyph and digits only — no box, no red (the GDK-1336 decision) | G1 G5 |
| Just before moving to in-progress | Small "in progress: n" count | none | Only when n ≥ the user's recent median + 1 | G6 G2 |
| Choosing a priority | Distribution mini-bar inside the picker | none | Always, quietly | G4 G2 |
| Writing a done-word comment | Inline "Move to Done" | Transition | Done vocabulary detected on an unfinished issue **after the guards**: word boundaries for the English words, a negation check for the CJK ones (미/未/불/非 before, 되지 않 after), quotes and code fences stripped, questions rejected, and the comment newer than `status_changed_at`. One dismissal silences it for that issue. The affordance costs one dismissal when wrong, so ~75–85% precision is enough here — the same match is **not** good enough to be counted for a steward | G2 G7 |
| Opening the team-flow view | Age tail · neglected · delegation ledger · closed this week | Per issue | None — the user came here | G9 G3 |
| Asking the agent | Age, blockers, who is waiting, inside the answer | The user decides | Only when asked; `skills/gadak/SKILL.md` fixes the answer shape | G10 |

## What the mirror already holds

The theory asks for data gadak collected before it had the theory:

- Local mirror with no network in the read path — foraging cost near zero
  (T1, T2; `UX_PRINCIPLES.md` §1–§2).
- `gadak sql` and `docs/RECIPES.md` — combining information fragments in one
  query (Fritz & Murphy 2010).
- `gadak recents` and viewed-first navigation — the first resumption cue
  (T2; §6).
- The local watch feed and unread chip — awareness, not notification (T3;
  `decisions/0011`).
- `reopen_count`, `reopen_reason`, `epic_key` — derived signals Jira cannot
  answer (T4, T5).
- `gadak ready` — the blocking graph read through the origin's link
  catalogue (T5).
- `status_changed_at` and `changelog` — the raw material of age and cycle
  time (T4).
- Skill and MCP surfaces that read the same mirror and write through the
  same origin (T5, T6).
- Keying on `status_category`, never on display names — the stable structure
  a boundary object needs (T5).

## What is missing, by cost

Candidates are ordered by whether the data already exists and whether SQL
alone can finish them. Registration and sequencing live in the GDK backlog;
this list is the reasoning, not the queue.

*Recipes only, data present*: a flow recipe pack (aging WIP, cycle-time
percentiles, weekly throughput, WIP trend); priority-entropy diagnosis;
three waiting lists (mine / what I wait on / what waits on me); neglected
in-progress detection; a delegation ledger (issues I reported, assigned to
others, by silence).

*Small UI*: the resume card; the issue meta line; a "my work / team flow"
stance switch; the in-progress count before a transition; the inline
done-word transition.

*Design first*: agent-activity review (needs a marker on gadak-mediated
writes); issue hygiene lint; an aging-WIP chart.

*Cheapest of all*: the agent's answer — `gadak next` output and the skill
file carrying age, blockers and who is waiting.

## What the theory argues against

Non-goals define the product as much as goals do.

- Expanding notifications — interruption cost is measured; awareness
  replaces it.
- Automatic importance ranking — information-theoretic "surprisal" of issues
  correlated only weakly with importance (Caddy et al. 2024); cut by
  user-defined relations instead.
- An everything dashboard — a dashboard that shows everything shows nothing
  (Few 2006). One card, one question.
- Gamification and productivity scores — single metrics distort behaviour
  (Forsgren et al. 2021).
- Replacing Jira workflows — the mirror does not change the origin; showing
  to nudge is where gadak stops.

## Predictions the theory makes

A theory that cannot be wrong is not worth building on. All four measures
come from the mirror and `local.db`; there is no telemetry.

1. **Resume time** — session start to first meaningful write — falls after
   the resume card ships. If it does not, T2's implementation is wrong.
2. **Items over the line** — the count, or share, of in-progress issues whose
   age exceeds the finished-work p85 — falls once the flow pack and neglected
   detection are visible. (Not the p85 *of in-progress age*: that number is
   dominated by whichever few items are oldest, and it moves when an old item
   finishes for reasons unrelated to visibility.) Visibility alone changes
   behaviour (T4).
3. **Priority entropy** widens after the diagnosis ships, or that team
   already uses priority as an order.
4. **Structure/prose mismatch** — done-word comments on unfinished issues —
   falls after the inline transition. If not, T5 does not match that team's
   practice.

`gadak retro` is the instrument: weekly, per workspace, definitions printed
with the numbers, checked once by hand SQL before anyone is asked to trust
it.

## Limits

This is a literature review, not a user study or an experiment. Interruption
and resumption findings come largely from laboratories and single-organisation
observation; their transfer is an assumption. Empirical work on Jira
specifically is thin and mostly on public open-source datasets (Montgomery,
Lüders & Maalej 2022), and what exists says the record is *machine-stable*,
not complete: every bug history Aranda & Venolia rebuilt omitted something,
and the rationale usually left no trace at all. Literature on agents as
tracker users was thin when this file was written and no longer is — three
vendors shipped agent identities into trackers during 2025–26, and 2026
studies measure agentic pull requests at scale — so T6 rests on practice as
well as on automation research. That is why the predictions above exist.

## Sources

Adams, Costa, Jung & Choudhury 2015, *Mindless computing*, UbiComp ·
Amabile & Kramer 2011, *The Progress Principle* ·
Amershi et al. 2019, *Guidelines for Human-AI Interaction*, CHI ·
Anderson 2010, *Kanban* ·
Aranda & Venolia 2009, *The secret life of bugs*, ICSE ·
Bainbridge 1983, *Ironies of automation*, Automatica 19(6) ·
Baysal, Holmes & Godfrey 2014, *No issue left behind*, FSE ·
Bertram, Voida, Greenberg & Walker 2010, *Communication, collaboration, and bugs*, CSCW ·
Brehm 1966, *A Theory of Psychological Reactance* ·
Caddy, Treude, Wagner & Barr 2024, *The role of surprisal in issue trackers*, EMSE ·
Carroll & Carrithers 1984, *Training wheels in a user interface*, CACM 27(8) ·
Clark & Brennan 1991, *Grounding in communication* ·
Cowan 2001, *The magical number 4*, BBS 24 ·
Cox, Gould, Cecchinato, Iacovides & Renfree 2016, *Design frictions for mindful interactions*, CHI EA ·
DeGrandis 2017, *Making Work Visible* ·
Dourish & Bellotti 1992, *Awareness and coordination in shared workspaces*, CSCW ·
Endsley 1995, *Toward a theory of situation awareness*, Human Factors 37(1) ·
Few 2006, *Information Dashboard Design* ·
Forsgren et al. 2021, *The SPACE of developer productivity*, ACM Queue ·
Fritz & Murphy 2010, *Using information fragments to answer the questions developers ask*, ICSE ·
Hollan, Hutchins & Kirsh 2000, *Distributed cognition*, TOCHI 7(2) ·
Horvitz 1999, *Principles of mixed-initiative user interfaces*, CHI ·
Horvitz, Breese, Heckerman, Hovel & Rommelse 1998, *The Lumière project*, UAI ·
Iqbal & Bailey 2008, *Effects of intelligent notification management*, CHI ·
Kalyuga, Ayres, Chandler & Sweller 2003, *The expertise reversal effect*, Educational Psychologist 38(1) ·
Kirsh 1995, *The intelligent use of space*, Artificial Intelligence 73 ·
Kluger & DeNisi 1996, *The effects of feedback interventions on performance*, Psychological Bulletin 119(2) ·
Lee & See 2004, *Trust in automation*, Human Factors 46(1) ·
Leroy 2009, *Why is it so hard to do my work?*, OBHDP 109(2) ·
Little 1961, *A proof for the queuing formula L = λW*, Operations Research 9(3) ·
Loewenstein, Bryce, Hagmann & Rajpal 2015, *Warning: you are about to be nudged*, Behavioral Science & Policy 1(1) ·
Mackworth 1948, *The breakdown of vigilance during prolonged visual search*, QJEP 1 ·
Montgomery, Lüders & Maalej 2022, *An alternative issue tracking dataset of public Jira repositories*, MSR ·
Noda, Storey, Forsgren & Greiler 2023, *DevEx: what actually drives productivity*, ACM Queue 21(2) ·
Parnin & Rugaber 2011, *Resumption strategies for interrupted programming tasks*, Software Quality Journal 19 ·
Parasuraman, Sheridan & Wickens 2000, *A model for types and levels of human interaction with automation*, IEEE SMC-A 30(3) ·
Pirolli & Card 1999, *Information foraging*, Psychological Review 106(4) ·
Pousman & Stasko 2006, *A taxonomy of ambient information systems*, AVI ·
Reinertsen 2009, *The Principles of Product Development Flow* ·
Risko & Gilbert 2016, *Cognitive offloading*, TiCS 20(9) ·
Ryan & Deci 2000, *Self-determination theory*, American Psychologist 55(1) ·
Scaife & Rogers 1996, *External cognition*, IJHCS 45 ·
Sheridan 1992, *Telerobotics, Automation, and Human Supervisory Control* ·
Star & Griesemer 1989, *Institutional ecology, 'translations' and boundary objects*, Social Studies of Science 19(3) ·
Stray, Sjøberg & Dybå 2016, *The daily stand-up meeting: a grounded theory study*, JSS 114 ·
Sweller 1988, *Cognitive load during problem solving*, Cognitive Science 12 ·
Thaler & Sunstein 2008, *Nudge* ·
Vacanti 2015, *Actionable Agile Metrics for Predictability* ·
Weiser & Brown 1996, *The coming age of calm technology* ·
Wood, Bruner & Ross 1976, *The role of tutoring in problem solving*, JCPP 17

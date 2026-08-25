# Maintenance

How this project is run, written down before there is a queue to run it
against. If you are deciding whether to depend on gadak, this page is the
honest part.

## Who maintains it

One person, in evenings and weekends, alongside a full-time job. There is no
company behind it, no support contract, and no second maintainer to escalate
to. That is not a temporary state waiting to be fixed — it is the shape of the
project, and everything below follows from it.

The mitigation is not a promise to keep up. It is that **the mirror is
disposable**: gadak writes one SQLite file, and if the project stalls, you
delete `~/.gadak` and have lost nothing. Your Atlassian site was the source of
truth the whole time. Depend on gadak the way you depend on a useful script,
not the way you depend on a database.

## Releases

**One release window per week.** `main` moves whenever there is something worth
committing, but tags are cut in a single weekly window, and the release notes
are that week's work as one entry.

Three releases went out on one day during the first month. That pace was
possible only because nobody was on the receiving end: every tag a user sees is
an upgrade they have to decide about, and a version number is a poor place to
put the satisfaction of having finished something. Security fixes are the one
thing that leaves outside the window.

**The version stays 0.x for a while.** Not out of modesty — 0.x is the honest
label for software whose schema moved fifteen times in its first month, and it
is the signal that lets both sides act sensibly.

## Issues

Open one. That part is genuinely welcome — for a long stretch this project had
no outside users at all, and a report is worth more than a star.

What makes a report actionable, in order:

1. **`gadak doctor` output.** It redacts keys, names, URLs and tokens, and it
   answers most of the questions a reply would otherwise have to ask. Almost
   every real bug so far has come down to *someone else's Jira is not shaped
   like mine* — localized status names, team-managed vs company-managed
   projects, a custom-field catalogue an order of magnitude larger.
2. What you expected, and what happened instead.
3. Whether it reproduces against the bundled demo (`gadak demo`). If it does,
   it is likely fixable this week. If it only happens on your site, expect a
   few rounds of questions, because there is no telemetry — deliberately — and
   your instance cannot be inspected from here.

**Stale issues get closed.** If a thread goes quiet for 60 days while waiting
on information, it closes. That is bookkeeping, not a verdict: reopen it with
the missing piece and it picks up where it stopped.

## Pull requests

Small, focused fixes with a test: yes, gladly.

**Connectors are different.** `CONTRIBUTING.md` says a third source connector
is among the most useful things to contribute, and that is true of the value.
It is also the largest thing a stranger can hand over, and after the merge it
belongs to whoever is still here. So a connector PR needs one more thing than
correctness: **a maintenance commitment — that you will answer issues about
your connector for at least six months.** Without it the answer is no, and the
refusal is about arithmetic rather than the quality of the code.

## Shipped in 0.16

- **Windows desktop zip** — unsigned portable pack
  (`Gadak-<version>-windows-x64.zip` / `windows-arm64`) on the GitHub Release.
  Details: [`docs/INSTALL.md`](INSTALL.md#desktop-app-windows).

## Not planned

Beyond the list in [`docs/ROADMAP.md`](project/ROADMAP.md):

- **Linux desktop shell** — 0.16 ships the Windows zip above; Linux stays
  `gadak serve` in a browser tab (or `install-service`). A native Linux window
  is a pack-script-and-WebKitGTK problem for a maintainer whose macOS build
  has not yet carried real users. The pack script exists
  (`desktop/build-linux.sh`); it is not a release asset.
- **Per-site custom field handling.** Fields are mapped by configuration, not
  by special cases in the code for one company's setup.
- **New UI locales.** Two is what one maintainer can keep honest.
- **Webhooks or real-time sync.** The polling loop is understandable and
  restartable; a push path is neither, and it needs an endpoint this
  architecture deliberately does not have.
- **A response-time promise of any kind.** See `SECURITY.md`.

Anything on this list can move if evidence shows up. Bring the evidence — a
list of people who asked, an instance that proves the need — and it gets
reconsidered in the open.

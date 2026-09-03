# 0012 — Markdown is the editing source; ADF is the wire

Status: accepted 2026-09-03 (GDK-1383; slices GDK-1384, GDK-1385, GDK-1386)

## Context

A body reaches gadak in one of two shapes. Jira, Confluence and the Built-in
tracker hold Atlassian Document Format; Linear holds markdown. The mirror
keeps whichever the origin gave it — `description_adf` / `body_adf` beside
`body_text` — and the web renders ADF through one hand-written
`renderAdf`. Every write surface was a `<textarea>` whose text went through
`jira.Doc`: one paragraph per line, a fenced block as `codeBlock`, `@Name`
as a mention, nothing else. Linear's writer flattened that ADF back to text.

Measured on 2026-09-03: the bodies agents write with `-m` are markdown —
the GDK backlog is full of `**`, `##` and `- ` — and every one of them was
stored as literal characters in a paragraph, then shown as such. Linear
markdown had no renderer at all. The migrated GDK backlog rendered as 844
single-paragraph walls (GDK-1382). Editing a body Jira's editor had
formatted was refused, because the textarea could only give back text.

The question was how to give a person and an agent an editor that is
WYSIWYG or close to it, on all three origins, without gadak becoming the
keeper of a fourth body format.

## Decision

**Markdown is the one editing source. ADF is the wire.**

- Every write path — `create -m`, `edit -m`, `comment`, `page create/edit/
  comment`, the web description editor and comment composer, the create
  dialog — takes markdown and sends ADF built by `adf.FromMarkdown`.
  `jira.Doc` is that converter plus the mention and media passes it always
  had. Jira and the Built-in tracker store the ADF as sent; Linear's writer
  turns it back into markdown with `adf.Markdown`.
- Line semantics are CommonMark's with one rule made explicit: a blank
  line separates paragraphs, a single newline is a `hardBreak`. This is what
  an agent typing into `-m` means, and it is what turns a migrated
  one-text-node body back into its blocks.
- The subset is paragraph, heading 1–6, hardBreak, rule, bullet and ordered
  lists (nested), blockquote, codeBlock with language, tables with
  single-paragraph cells, and the marks strong, em, code, strike, link.
  Raw HTML is text. `md → ADF → md` and `ADF → md → ADF` are the identity
  on the subset; golden tests pin both.
- Reading back for an edit: `adf.Markdown` returns a simple document's
  text as typed — its `**` is the author's markdown — and serializes a rich
  one with escaping, so a Jira-authored literal `*` stays literal.
- `FormatLoss` narrows to what has no markdown: panel, media, status, date,
  inlineCard, mention, expand, extension; textColor, underline, subsup. The
  refusal and its flags (`--force-plain`, `--force`) keep their names; only
  the trigger changes.
- Display (GDK-1385): the detail and comment DTOs are the single owner of
  the rendered document. A simple ADF and a Linear markdown body are
  re-read as markdown on the way out — derived in the response, never
  written to the mirror, so the Linear "ADF column stays empty" contract
  stands. The editor takes `description_md` and `format_loss` from the same
  response, and the web's copied `isSimpleAdf` tables go away.
- Preview is Write / Preview, GitHub-style, rendering ADF the server built
  from the draft. No TypeScript parser, no second implementation.

## Consequences

- No ProseMirror / Tiptap. It would be the first editor dependency, it has
  no ADF transformer we could own, and the phone would need a second one.
  A structural editor stays possible later *on top of* this: the wire is
  already ADF.
- `-m` help and the skill text say markdown. A user who typed a literal
  `*text*` before gets emphasis now; that is the trade, and it is the same
  trade GitHub, Linear and every chat tool made.
- The Built-in tracker needed no change for the editor. The one change it
  needed was for migration: a fixture slot that stores ADF verbatim
  (GDK-1382).
- goldmark (`github.com/yuin/goldmark`) is the first third-party import in
  the root module. `desktop/go.mod` follows it.

## Rejected

- **A structural ADF editor now.** See above; it is a 0.21+ question and it
  does not remove the need for markdown on Linear.
- **Markdown → ADF at sync time for Linear.** Would store a document the
  origin never had; `internal/sync/linear_test.go` forbids exactly that.
- **A TypeScript markdown parser for live preview.** Two parsers drift; the
  loopback round trip costs nothing a person can feel.

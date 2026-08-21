# Gadak

Search the local gadak mirror of Jira and Confluence from Raycast, open a
saved view or Jira filter, look up a person, and land in the Gadak app.

## Search Jira & Confluence

Each keystroke runs `gadak search --json --limit 20` against the on-disk
mirror. Results come in two sections: issues, then documents (Confluence
pages). Enter opens `gadak://view?issue=<KEY>` for an issue and
`gadak://view?doc=<ID>` for a document. Issue rows also have **Open in Jira**,
which opens `https://<site_host>/browse/<KEY>` in the browser when
`gadak profiles --json` supplies a `site_host` for the selected profile.
The **Default Action** preference can swap those two on Enter (`⌘↵` is the
other). When a profile preference is set, the gadak links gain a
`/w/<profile>` segment.

## Views

One `gadak views --json` load. Each row is a Jira filter (`kind: jira`) or a
saved view (`kind: saved`). Enter opens `gadak://view?<hash>` — `hash` is the
view's query string and is appended as-is (not re-encoded). A row whose
`unsupported` list is non-empty shows a warning accessory: gadak applied less
than that JQL, so the list can differ from Jira. The secondary action copies
the JQL. Typing filters the loaded list.

## People

One `gadak sql --json` load of distinct assignees and reporters. Enter opens
`gadak://view?person=<identity>`. Identity is the Jira account id when the
mirror has one (`assignee_id` / `reporter_id`), otherwise the email
(`assignee_email` / `reporter_email`). Rows with neither are omitted — a
display name is not a lookup key, so a site that hides emails still lists
anyone who has an account id. Typing filters the loaded list; it does not
re-run SQL.

## How this differs from the API-based Jira extensions

Extensions like Jira Search talk to the Atlassian REST API, so each
keystroke is a network round-trip and needs Atlassian credentials stored in
Raycast. This extension searches a **local SQLite mirror** maintained by
the [Gadak app](https://github.com/midagedev/gadak) instead:

- results are instant (~20 ms) and work **offline**
- one search covers **Jira issues and Confluence pages** together
- no Atlassian credentials in Raycast — the mirror on disk is the source
- Enter lands in the Gadak app by default (full detail, comments, transitions);
  issue rows can also open the Jira page in the browser

The trade-off is the requirement: it only makes sense if you use Gadak,
and results are as fresh as the mirror's last sync (about a minute behind
Jira in normal use). If you want to query Jira's API directly from Raycast
without the app, the existing Jira extensions are the right tool.

## Zero-config

On a machine that installed gadak the documented way, this extension needs
no preferences. It resolves the binary in this order (first existing path
wins):

1. the **gadak binary** preference, if set
2. `/opt/homebrew/bin/gadak` (Homebrew, Apple silicon)
3. `/usr/local/bin/gadak` (Homebrew, Intel)
4. `/Applications/Gadak.app/Contents/Resources/bin/gadak` (the app bundle)

Raycast's Node process does not inherit the user's shell `PATH`, so a `gadak`
that only works in Terminal is not found that way.

## Requirements

- gadak installed — the [macOS app](https://github.com/midagedev/gadak#install)
  or `brew install midagedev/tap/gadak`
- a synced mirror (`gadak init && gadak sync`)

Without a binary, the command offers that install command and the install
guide. None of the commands create a mirror.

## Preferences

| Preference          | Default | What it does                                                                                                                  |
| ------------------- | ------- | ----------------------------------------------------------------------------------------------------------------------------- |
| Gadak Binary        | empty   | Absolute path. Empty uses the discovery list above.                                                                           |
| Profile             | empty   | Passed as `gadak --profile`. Empty uses gadak's default profile and a deeplink with no `/w/` segment.                         |
| Show search latency | off     | Search command only. When on, the results header includes the search time in milliseconds.                                    |
| Default Action      | Gadak   | Search command only. Which action Enter performs on an issue: open it in the Gadak app, or open its Jira page in the browser. |

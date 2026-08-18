# 0009 — CJK mid-compound search is app-layer bigrams, not a different tokenizer

Status: accepted
Date: 2026-08-19

## Decision

`items_fts` keeps `tokenize='unicode61 remove_diacritics 2'`. A fourth
column, `cjk_bigram`, carries overlapping 2-grams of CJK runs only. Queries
whose terms are CJK and at least two runes long are rewritten into those
bigrams; a one-rune CJK term and every Latin term keep today's prefix
rewrite. English is not n-grammed.

Rejected: switching `items_fts` to `trigram`; a second `items_bigram` table;
injecting bigrams into the three scored columns; CJK unigrams; `LIKE` as the
search path; a custom tokenizer.

The reasons are measured, not argued — the tables below are the record, so
that this is not re-litigated from intuition. Two of them overturn things
this repository previously wrote down:

- `specs/000-product/data-model.md` said to revisit with `trigram`. Trigram
  **cannot answer the Korean case at all**: a 2-character Korean query produces no
  trigram tokens, so `MATCH '결제'` returns **0 rows and no error** — the
  worst failure shape available. It also costs 5.4–5.9× the FTS bytes and
  5.8× the reindex time, and it wrecks English precision (`ency` pulls
  currency, latency, consistency, dependency, urgency: precision 0.342).
- `trigram` *can* take `remove_diacritics` on SQLite 3.51.0, contrary to what this repository assumed. It does not save the 2-character case, so it changes
  nothing.

A custom tokenizer stays closed for the same reason as before:
`modernc.org/sqlite` exposes no registration API and `CGO_ENABLED=0` is the
build contract.

## Consequences

- Schema v22. The FTS shape has one owner (`itemsFTSCreate`,
  `internal/store/schema.go`) and `repairItemsFTS` already rebuilds when the
  stored CREATE differs, so the migration is the DDL change plus the version
  bump. Measured rebuild: 13.5 ms at 605 items, and the closest 20k number is
  338 ms — a one-time Open() hitch, not a migration a user waits on.
- The rebuild INSERT, `writeFTS`, the snapshot builder and
  `scripts/scrub-demo-db.py` must all fill the fourth column. Any one of them
  writing three columns leaves mid-match silently empty rather than broken,
  which is why they are named here.
- `docs/RECIPES.md` currently promises that word middles miss, with `결제` /
  `간편결제` as the example. That sentence becomes half false and must be
  split: CJK 2-char hits mid-compound; English middles still miss
  (`ency` ≠ `idempotency`, on purpose — precision).
- `gadak sql` users and agents get this without a rewrite layer: A-col is the
  only shape measured where a stranger who opens the file and types
  `items_fts MATCH '결제'` gets mid-compound hits. That is the disposable-
  mirror rule doing work — the file has to be useful without gadak running.
- Cost accepted: precision drops on short CJK fragments (`용자` 0.33, `파이`
  0.20) because a bigram index cannot tell a fragment from a word. One-rune
  CJK keeps today's token-start behaviour rather than becoming a scan.

Implementation is GDK-259 and has not landed; this document is the design and
the measurements it rests on.

## Provenance

Measured 2026-08-18 in a throwaway lab module outside the work tree
(`modernc.org/sqlite v1.56.0`, the version in `go.mod`; cross-checked with
`sqlite3` 3.51.0). Corpora: a 48-document Korean fixture, the 605-item
`examples/demo.db` (copied with `.backup`, never opened for write), and a
20 000-row corpus shaped like `seedSearchScale`. No production file was
modified in that round, and the lab tree was not kept.

---
## 0. What the tree actually does

| Claim as it stood | Re-measured |
|---|---|
| `items_fts` is `tokenize='unicode61 remove_diacritics 2'` (`internal/store/schema.go:16-21`) | Yes. Canonical DDL is `itemsFTSCreate`. `examples/demo.db` snapshot **drops** `contentless_delete=1` (portable / GDK-112). |
| `unicode61` is word-level; `결제*` misses `간편결제`; only suffix `*` | Yes. FAIL-first: `Search("결제")` = `[KO-PAY-2]` (결제내역) and not `간편결제`. |
| demo `MATCH 'idempot*'` = 14, `'ency*'` = 0 | **Yes, exact.** |
| demo trigram `'결제'` = 0 | Yes, on a rebuilt copy. Failure mode: **silent 0 rows, not an error.** `fts5vocab` emits only 3-rune grams (`간편결`, `편결제`); query `결제` produces no tokens. |
| Built-in tokenizers = ascii / unicode61 / porter / trigram; no mecab/icu | Yes (`sqlite3` 3.51.0 and the same four succeed / two fail). |
| Custom tokenizer path closed | Not re-tested; no counter-evidence. `modernc.org/sqlite` + `CGO_ENABLED=0` is unchanged. |
| `trigram` cannot take `remove_diacritics` | **False on this SQLite.** `tokenize='trigram remove_diacritics 2'` created cleanly; `café` MATCH `cafe` = 1. |
| MATCH assembled in `search_key.go` / `queries.go` | **False.** `search_key.go` is key lookup only. `queries.go` is Jira saved-filters. MATCH string is `ftsPrefixQuery` at `internal/store/read.go:841-857`, executed at `read.go:757` and `read.go:885`. |
| `docs/RECIPES.md:66` says middles miss | Yes: “Word *middles* still miss — searching `결제` will not hit `간편결제`.” |
| demo.db is probably English | Yes. 534 issues + 71 pages = 605 items. Hangul titles = **0**. Hangul bodies = **2** (wiki pages, space-separated words, no `간편결제`). Korean numbers below are from the 48-doc lab fixture. |
| demo.db schema version | `PRAGMA user_version` = **15** (not 21). Current binary applies 21 migrations (`schema.go:6`). |
| `search_scale_fixture_test.go` scale | `searchScaleN = 20000` (`search_scale_fixture_test.go:16`). |

---

## 1. Measurement table (measured only)

Corpora: **ko** = 48 lab docs; **demo** = 605 items from `sqlite3 examples/demo.db ".backup …"`; **scale** = 20k rows shaped like `seedSearchScale` plus injected compounds (every 200th row).

Engine for timings/sizes: `modernc.org/sqlite v1.56.0` (same as `go.mod`). CLI checks: `/usr/bin/sqlite3` 3.51.0.

### 1a. Size (file bytes + `dbstat` FTS bytes)

| Corpus | Method | File bytes | FTS `dbstat` bytes | vs U61 FTS |
|---|---|---:|---:|---|
| demo | U61 (today) | 667 648 | 266 240 | 1.00× |
| demo | **A-col** (CJK bigram 4th column) | 671 744 | 270 336 | **1.015×** (+4 096) |
| demo | A-table (U61 + `items_bigram`) | 692 224 | 290 816 | 1.09× |
| demo | **B trigram** | 1 830 912 | **1 429 504** | **5.37×** |
| demo | C LIKE (no FTS) | 401 408 | 0 | n/a |
| scale 20k | U61 | 19 181 568 | 4 055 040 | 1.00× |
| scale 20k | A-table | 19 181 568 | 4 349 952 | 1.07× FTS |
| scale 20k | B trigram | 40 464 384 | **23 748 608** | **5.86×** |
| scale 20k | C LIKE | 14 073 856 | 0 | n/a |
| ko 48 | A-col / A-inject / U61 | 57 344 | 28 672 | page-granularity tie |

demo snapshot live FTS `dbstat` = 266 240 — matches the U61 rebuild.

### 1b. Reindex wall (3-run median, Go harness)

| Corpus | U61 | A-col | A-table | B trigram |
|---|---:|---:|---:|---:|
| ko 48 | 2.499 ms | 2.770 ms | 4.064 ms | 2.402 ms |
| demo 605 | **9.818 ms** | **13.505 ms** | 15.278 ms | 30.669 ms |
| scale 20k | **193.680 ms** | not timed in this harness | **337.734 ms** | **1120.998 ms** |

Scale A-col was not a named Go reindex target. A Python row-loop one-off (~2.6 s) is **not** comparable (default journal, no batch pragmas) and is not used below. Closest Go number for “U61 + extra CJK index at 20k” is A-table 338 ms.

Per-item at demo size: ~16–22 µs. A 20k Open() rebuild is well under a second for A; ~1.1 s for trigram.

### 1c. Recall / precision (human-intent gold) + query latency

**ko fixture, headline queries** (median of 10, ms):

| Query | Gold n | U61-prefix hits / rec / prec / ms | A-col hits / rec / prec / ms | B trigram hits / rec / prec / ms | C LIKE hits / rec / prec / ms |
|---|---:|---|---|---|---|
| `결제` (mid) | 8 | 6 / 0.50 / 0.67 / 0.045 | **10 / 1.00 / 0.80 / 0.037** | **0 / 0 / 0 / 0.017** | 10 / 1.00 / 0.80 / 0.059 |
| `ency` (mid) | 3 | 0 / 0 / 0 / 0.036 | 0 / 0 / 0 / 0.113 | 10 / 1.00 / **0.30** / 0.047 | 10 / 1.00 / **0.30** / 0.059 |
| `idempot*` (prefix) | 4 | 4 / 1 / 1 / 0.040 | 4 / 1 / 1 / 0.035 | 4 / 1 / 1 / 0.131 | 4 / 1 / 1 / 0.050 |
| `결제*` | 4 | 6 / 0.75 / 0.50 / 0.041 | 10 / 1 / 0.40 / 0.041 | **0 / 0 / 0 / 0.018** | 10 / 1 / 0.40 / 0.061 |
| `결` (1-char) | 10 | 7 / 0.60 / 0.86 / 0.044 | **0 / 0 / 0 / 0.031** (rewrite=`"결"`) | 0 / 0 / 0 / 0.016 | 11 / 1 / 0.91 / 0.058 |
| `결제 실패` | 2 | 2 / 1 / 1 / 0.058 | 3 / 1 / 0.67 / 0.048 | 0 | 3 / 1 / 0.67 / 0.094 |
| `auth` | 4 | 2 / 0.25 / 0.50 / 0.035 | 2 / 0.25 / 0.50 / 0.084 | 5 / 1 / 0.80 / 0.089 | 5 / 1 / 0.80 / 0.052 |
| `파이` (FP bait) | 1 | 3 / 1 / 0.33 / 0.035 | 5 / 1 / **0.20** / 0.035 | 0 | 5 / 1 / 0.20 / 0.061 |
| `용자` (fragment) | 1 | 1 / 1 / 1 / 0.034 | 3 / 1 / **0.33** / 0.032 | 0 | 3 / 1 / 0.33 / 0.059 |
| `편결` (fragment) | 5 (substr) | 0 | 5 / 1 / 1 (all 간편결제) | 0 | 5 |
| `간편결제` | 5 | 5 / 1 / 1 / 0.039 | 5 / 1 / 1 / 0.063 | 5 / 1 / 1 / 0.046 | 5 |
| `배포파이프라인` | 2 | 2 / 1 / 1 | 2 / 1 / 1 | 2 / 1 / 1 | 2 |
| `결재` | 1 | 1 / 1 / 1 | 1 / 1 / 1 | **0** (2-char) | 1 |
| `zzznomatchtokenzzz` | 0 | 0 | 0 | 0 | 0 |

A-table on `결제` = A-col (10 / 1.00 / 0.80) with UNION median 0.071 ms (slightly slower). A-inject = A-col. A-uni is the only A variant that recalls `결` (11 / 1.00 / 0.91).

**demo.db (English):**

| Query | U61-prefix | A-col / A-table | B trigram | C LIKE |
|---|---|---|---|---|
| `ency` | **0** / 0.035 ms | **0** / 0.033 ms | **38** rec 0.929 prec **0.342** / 0.094 ms | 38 / 0.342 / **2.863 ms** |
| `idempot*` | **14 / 1 / 1** / 0.044 | 14 / 1 / 1 / 0.046 | 14 / 1 / 1 / 0.148 | 14 / 2.323 ms |
| `idempotency` | 11 / 1 / 1 / 0.041 | 11 | 11 / 0.241 | 11 / 2.089 |
| `auth` (human gold 33) | 35 / rec 0.939 prec 0.886 / 0.061 | same 35 | 37 / 1.00 / 0.892 / 0.087 | 37 / 2.412 |
| `webhook AND retry` | 16 / 1 / 1 / 0.056 | 16 | 16 / 0.348 | 16 / 2.146 |
| `retry` | 35 / 1 / 1 / 0.053 | 35 | 35 / 0.111 | 35 / 2.381 |
| `p` (1-char) | 431 / 0.625 ms | 431 / 0.751 | **0** | 605 / 2.132 |
| zero token | 0 / 0.037 | 0 | 0 / 0.039 | 0 / **1.864** |

`ency` gold = docs containing `idempotency`/`idempotent` (14). Trigram/LIKE miss `idempotent` (no `ency` letters) and pick up currency/latency/consistency/dependency/frequency/urgency/transparency (25 FP).

**scale 20k latency (median / max ms):**

| Query | U61-prefix | A-table | B trigram | C LIKE |
|---|---:|---:|---:|---:|
| `결제` | 0.179 / 1.425 (100 hits, miss 100 mid) | **0.426 / 1.307** (200/200) | 0.017 (0 hits) | **55.074 / 68.655** |
| `ency` | 0.095 (0) | 0.116 (0) | 0.788 (200) | **59.505 / 62.748** |
| `idempot*` | 0.174 | 0.201 | 0.661 | 41.759 |
| `retry` (20k hits) | 13.740 / 21.170 | 21.016 / 27.079 | 19.536 / 24.616 | 48.582 / 52.872 |
| `p` (20k hits) | 48.362 / 50.086 | 56.547 / 57.854 | 0.019 (0 hits) | 27.553 |
| zero token | 0.125 | 0.138 | 0.128 | **33.882 / 39.454** |

LIKE is 30–60 ms even on a 0-hit at 20k. A/B stay sub-ms on selective queries.

### 1d. Plain `sqlite3` / `items_fts MATCH` (no gadak)

Measured with `/usr/bin/sqlite3` on the lab copies:

| DB | SQL | n |
|---|---|---:|
| ko_u61 | `items_fts MATCH '결제'` | 3 |
| ko_u61 | `items_fts MATCH '"결제"*'` | 6 |
| ko_trigram | `items_fts MATCH '결제'` | **0** (no error) |
| ko_trigram | `items_fts MATCH '간편결제'` | 5 |
| ko_trigram | `items_fts MATCH 'ency'` | 10 |
| **ko_bigram_col** | **`items_fts MATCH '결제'`** | **10** |
| ko_bigram_col | `items_fts MATCH 'ency'` | 0 |
| ko_bigram_table | `items_fts MATCH '결제'` | 3 (unchanged) |
| ko_bigram_table | `items_bigram MATCH '결제'` | 10 |
| ko_like | `LIKE '%결제%'` | 10 |
| demo_u61 | `MATCH 'idempot*'` | 14 |
| demo_u61 | `MATCH 'ency*'` | 0 |
| demo_trigram | `MATCH 'ency'` | 38 |

So: **A-col is the only FTS shape where a stranger opening the file and typing `items_fts MATCH '결제'` gets mid-compound hits.** A-table requires knowing `items_bigram`. B requires 3+ runes. C is not MATCH.

### 1e. Doc / MATCH teaching

| Method | `items_fts MATCH 'term'` still the taught query? | Becomes false? |
|---|---|---|
| A-col / A-inject | Yes, and `'결제'` starts working | RECIPES.md:66 (“middles still miss”) becomes **false for CJK 2-char**. English middles still miss. |
| A-table | `items_fts MATCH '결제'` stays 3 | RECIPES.md:66 **stays true** on `items_fts`. Mid-match is a second table the docs do not name. |
| B | Syntax works; 1–2 char terms silently empty | “MATCH 'term'” is **misleading** for 2-char Korean. `remove_diacritics` can be kept, contrary to the earlier assumption. |
| C | MATCH unchanged | Mid-match only via `LIKE`/`instr` (RECIPES already shows a comment `LIKE` recipe). |

---

## 2. Query set — hits / false positives

Human gold is `RelevantQueries` on the 48-doc fixture (`data/ko_docs.json`). Mechanical substring gold is used for `편결` and scale.

### 2a. `결제` (want 간편결제 / 결제내역 / 정기결제 / comment hits)

U61-prefix (6): KO-PAY-1, KO-PAY-2, KO-PAY-4, KO-PAY-6, KO-CMT-1, KO-FRAG-1.

KO-PAY-1 is **not** a mid-compound hit here — the comment is `재현: 결제 버튼 두 번 클릭.` The FAIL-first seed has no such comment; `Search("결제")` then returns only KO-PAY-2.

**Missed by U61, caught by A/C:** KO-PAY-3 (`정기결제`), KO-PAY-5 / KO-USE-2 / KO-MIX-1 (`간편결제` only).

A/C extra vs human gold (2): KO-PAY-6 (`결제가 아닙니다`), KO-FRAG-1 (`결제 아님`) — they contain the word 결제 used to deny payment. Mechanical substring precision for `결제` is 10/10.

B: **0**.

### 2b. `ency` (want idempotency, not currency)

ko: A/U61 = 0. B/C = 10 = {KO-IDEM-1, KO-IDEM-2, KO-MIX-1} ∪ {KO-CUR-1/2, KO-LAT-1/2, KO-CON-1, KO-DEPEN-1, KO-URG-1}. prec=0.300.

demo: A/U61 = 0. B/C = 38, gold 14, tp 13, fp 25, prec=0.342.

A-en3 **with** query rewritten to `"enc" "ncy"`: same 10 as B on ko (see `out/a_en3_rewrite.txt`). The harness A-en3 row still used `"ency"*` and stayed at 0 — that row must not be read as “English n-grams don’t work.”

### 2c. Prefix regression

`idempot*` / `idempotency` / `webhook AND retry` / `retry`: A matches U61 exactly on ko and demo (14 / 11 / 16 / 35). B matches those too, slower.

`결제*`: B = **0** (2-char + `*`). A/C = 10 (broader than today’s 6).

### 2d. Short queries

`결`: U61-prefix = token-start (결제…, 결재…, 결과…). A-col rewrite `"결"` = **0** (no unigram in the index). A-table UNION still has the U61 half (7). A-uni / LIKE = 11. B = 0.

`p` on demo: U61/A = 431; B = 0; LIKE = 605 (mid-token `p` too).

### 2e. `auth`

U61-prefix `"auth"*`: authentication / authorize / **authors** (KO-AUTH-EN-3 FP). Misses `oauth` (does not start with auth).

B/LIKE: hits oauth + unauthorized + authors. rec 1.00, prec 0.80 on ko; demo 37 vs gold 33.

### 2f. FP baits

| Query | U61-prefix | A-col | Meaning |
|---|---|---|---|
| `파이` | 3 (파이 차트, 파이프라인 token-start, 파이프라인 in body) | 5 (+ 배포파이프라인 mid) | pie vs pipeline |
| `용자` | 1 (title `용자`) | 3 (+ 사용자인증 ×2) | fragment |
| `편결` | 0 | 5 (every 간편결제) | fragment |

---

## 3. Recommendation

**Ship candidate A, sub-choice: extra column `cjk_bigram` on `items_fts`, CJK overlapping bigrams only, unicode61 tokenizer unchanged. Do not n-gram English. Do not emit CJK unigrams by default.**

Call this **A-col hybrid**.

Why this and not A-table / A-inject / B / C is in §4. Weaknesses of the pick (same honesty):

- Fragment queries (`편결`, `용자`, `파이`) light up compounds. That is the cost of mid-match. Cap it; do not add unigrams on top.
- English infix (`ency` → idempotency) stays **broken**. Adding English 3-grams drops demo `ency` precision to 0.342 (25 FP). Do not “finish” English in the same change.
- 1-char `결`: rewrite must keep `"결"*` (token-start), **not** exact `"결"`. Measured A-col with exact `"결"` = 0. A-table UNION already does the right thing via the U61 half.
- `ftsColumnPrefixHit` (`read.go:1037`) still treats mid-token as a non-hit. A bigram-only MATCH will take the “omit rather than guess” branch (`read.go:970`) unless field attribution also tests CJK substring. Snippets can go missing on those rows.
- `items_fts MATCH '결제'` in raw SQL starts returning more rows. Agents who treated 0 as “no such word” will see hits. RECIPES.md:66 must be rewritten in the same change.
- `scripts/scrub-demo-db.py` portable rebuild must fill the 4th column or the hosted snapshot silently loses CJK mid-match.

### 3a. Implementation order (`file:line`)

1. **`internal/store/tok` or a few funcs next to `ftsPrefixQuery`** — CJK-run split + overlapping 2-grams. Search first; nothing like this exists (grep `bigram` / `CJKBigrams` in tree = 0). Copy guards from this lab’s `tok/tok.go`.
2. **`internal/store/schema.go:16-21`** — `itemsFTSCreate` adds `, cjk_bigram` after `comments_text`. Keep `tokenize='unicode61 remove_diacritics 2'` and `contentless_delete=1`. Do **not** edit `schemaV1` (append-only).
3. **`internal/store/schema.go:6`** — append `schemaV22`. Body can be a comment/`SELECT 1`; the live rebuild is `repairItemsFTS` comparing `itemsFTSCreate`. Still bump `user_version` so `sync_state.schema_version` moves (`store.go:156-162`).
4. **`internal/store/write.go:653-659`** — `writeFTS` INSERT four values. 4th = space-joined CJK bigrams of title, body, comments.
5. **`internal/store/fts_repair.go:66-73`** — rebuild INSERT must compute the same 4th column (SQL cannot emit bigrams; walk rows in Go, or a small SQL-side helper if you can keep it identical to writeFTS).
6. **`internal/snapshot/build.go:419-425` and `:613-619`** — same 4-tuple as writeFTS (two clones).
7. **`internal/store/read.go:841-857` `ftsPrefixQuery`** — for each bare field: if all CJK and `len(runes)>=2`, emit quoted bigrams AND (`"간편" "편결" "결제"`); if 1 CJK rune, keep `"결"*`; else keep today’s `"term"*`. FTS punctuation / AND/OR/NOT/NEAR still pass through.
8. **`internal/store/read.go:708-716` `ftsRankSQL`** — four weights. Today `20, 2, 1`. Pass a 4th (start at `2.0`; do not invent without a relevance fixture run). `bm25` with 3 weights on 4 columns is undefined enough to fail a review.
9. **`internal/store/read.go:1037-1077`** — CJK query tokens: substring (or bigram) counts as a column hit so snippets survive. Keep English mid-token **not** a hit (`TestFTSColumnPrefixHit` / Aperture).
10. **`internal/store/store_test.go:38`** — `items_fts` columns add `cjk_bigram`.
11. **`scripts/scrub-demo-db.py:77-91`** — portable CREATE + INSERT must include `cjk_bigram` (still no `contentless_delete`). Extend the MATCH probe set with `결제` once demo grows a compound, or keep probes on the English set and add a lab-style Korean probe in Go tests.
12. **Docs to change with the implementation:** `docs/RECIPES.md:66`, `specs/000-product/data-model.md:374-391`, `internal/mcp/tools.go:61-62`. Optionally `AGENTS.md:107-110`, `skills/gadak/SKILL.md:142-145`, `docs/DERIVE.md:241-244`.
13. **Promote the FAIL-first overlay test** into `internal/store` (new file). Do not commit the lab overlay.

`export-static` (`cmd/gadak/export_static.go`) freezes HTTP JSON, not FTS. No change unless the hosted sqlite-wasm path runs MATCH; then it rides the snapshot + scrub DDL.

### 3b. Recurrence gate (FAIL-first — already red on this tree)

Lab tests (not in `internal/store`):

Standalone (same DDL + `ftsPrefixQuery` copy):

```
=== RUN   TestMidMatch_KoreanCompound
    midmatch_test.go:30: MATCH "\"결제\"*" keys=[KO-PAY-2]: want KO-PAY-1 (간편결제). prefix-only hits 결제내역 (KO-PAY-2) and misses the mid-compound.
--- FAIL: TestMidMatch_KoreanCompound (0.01s)
=== RUN   TestMidMatch_EnglishInfix
    midmatch_test.go:39: MATCH "\"ency\"*" hit 0 rows; want idempotency (KO-IDEM-1).
--- FAIL: TestMidMatch_EnglishInfix (0.00s)
=== RUN   TestPrefixRegression_Idempot
--- PASS: TestPrefixRegression_Idempot (0.00s)
```

`store.Search` via `go test -overlay lab/failfirst/overlay.json` (tree not written):

```
=== RUN   TestGDK259_MidMatch_KoreanCompound
    gdk259_midmatch_test.go:23: Search("결제") keys=[KO-PAY-2]: want KO-PAY-1 (title 간편결제 실패). unicode61+ftsPrefixQuery cannot match a mid-compound.
--- FAIL: TestGDK259_MidMatch_KoreanCompound (0.02s)
=== RUN   TestGDK259_MidMatch_EnglishInfix
    gdk259_midmatch_test.go:35: Search("ency") keys=[]: want KO-IDEM-1 (idempotency). rewrite is "\"ency\"*" (token prefix only).
--- FAIL: TestGDK259_MidMatch_EnglishInfix (0.02s)
=== RUN   TestGDK259_PrefixRegression_Idempot
--- PASS: TestGDK259_PrefixRegression_Idempot (0.01s)
```

**Promote this shape, with one product decision on English:**

| Case | Seed | Assert after A-col |
|---|---|---|
| Korean mid | title `간편결제 실패`, **no** standalone `결제` token | `Search("결제")` contains that key |
| Prefix still | title `결제내역 …` | `Search("결제")` still contains it |
| Prefix English | title `idempotency key handling…` | `Search("idempot")` contains it |
| English infix **unchanged** | same row | `Search("ency")` is **empty** (A-col must not grow this) |

`TestMidMatch_EnglishInfix` as written (want a hit) is the B/C/A-en3 contract. **Do not promote that assert as a must-pass for A-col** or you will force English n-grams and the 0.34 precision drop. Keep it as a skipped/documented “not this change” test, or flip it to `want 0` as a precision lock.

FAIL-first for the Korean assert is already true on current `store.Search`.

### 3c. Precision ceiling (what to watch)

Freeze the 48-doc KO fixture and demo.db copies as the gate corpus.

| Sensor | A-col measured | Fail the PR if |
|---|---|---|
| ko `결제` human prec | 0.800 (2 deny-docs) | prec < 0.75 or hits > 12 on this fixture |
| ko `파이` human prec | 0.200 (4 pipeline FPs) | hits > 5 on this fixture |
| ko `용자` | 3 hits / prec 0.333 | hits > 3 |
| ko `편결` | 5 = all 간편결제 | hits > 5 |
| demo `ency` | **0** | hits ≠ 0 |
| demo `idempot*` | **14** | hits ≠ 14 |
| demo `webhook AND retry` | 16 | hits ≠ 16 |
| demo `auth*` | 35 | hits outside 35±0 (prefix set must not grow) |

Do not add CJK unigrams unless a later ticket accepts `결` → 결과/결재/결제 all lighting up (A-uni: 11 hits).

### 3d. Query rewrite owner

**One place: `ftsPrefixQuery` (`read.go:841`), called from `searchAll` (`read.go:757`).** That is the gadak search / REST / MCP search path.

`gadak sql` and `gadak_query` will **not** rewrite. Under A-col that is acceptable: `MATCH '결제'` already hits the bigram column. Under A-table it would not — another reason not to pick A-table if agents keep using `items_fts MATCH`.

---

## 4. Rejected (with numbers)

### B — `tokenize='trigram'`

- `결제` / `결제*` / `결` / `결재` = **0, no error**. The case that started this is 2-char Korean, so this fails it outright.
- Index: demo FTS **5.37×** (266 240 → 1 429 504); scale **5.86×** (4.1 MB → 23.7 MB).
- Reindex 20k: **1121 ms** vs U61 194 ms.
- `ency` on demo: 38 hits, prec **0.342**.
- `p` / 1-char: 0. Breaks the documented prefix path that `search_scale` exists to keep cheap.
- `remove_diacritics` *can* be added on 3.51.0 — the earlier assumption was wrong, but it does not save 2-char Korean.
- `data-model.md:391` “Revisit with `trigram`” is the wrong revisit.

### C — `LIKE` / `instr` as the search path

- Recall/precision on Korean mid = A-col (same substring).
- demo: 2.1–2.9 ms (605 rows) — fine for `gadak sql` recipes (already in RECIPES.md:84).
- **20k: 34–69 ms median**, including **33.9 ms for a guaranteed 0-hit**. Not a keystroke path; not a replacement for FTS. `search_scale` already treated 20k as the size that opened GDK-166.
- No index, so every query is a full scan of title+body+comments.
- Same English FP profile as trigram (`ency` prec 0.342).
- Use: documented SQL escape hatch only, not `store.Search`.

### A-table (`items_bigram` beside `items_fts`)

- Same Korean recall as A-col when gadak UNIONs.
- `items_fts MATCH '결제'` stays 3 — RECIPES.md:66 stays true and agents who only read MCP still miss.
- Dual write in `writeFTS`, `fts_repair`, snapshot ×2.
- Extra ~24 KB FTS on demo; UNION `p` at 20k: 56.5 ms vs 48.4 ms.
- Pick this only to keep MATCH-on-`items_fts` bit-identical. The disposable-mirror rule (“someone else opens the file”) then requires teaching `items_bigram`.

### A-inject (bigrams concatenated into title/body/comments)

- Same MATCH-'결제'=10 as A-col, no extra column.
- Pollutes the three BM25 columns (weights 20/2/1 were measured 2026-08-17 on clean tokens, `read.go:694-712`).
- DDL unchanged → `repairItemsFTS` will **not** rebuild; a v22 backfill is mandatory.
- Reject as default; A-col keeps scoring columns intact.

### A-uni (CJK unigrams)

- Fixes `결` (rec 1.00) and makes every 1-rune query a scan of a huge posting list. ko `결` = 11 docs including 결과/결재.
- Do not ship unless product wants 1-char mid.

### A-en3 (English 3-grams)

- `ency` prec 0.300 / 0.342. Rejected for the same reason as B on English.

### Custom tokenizer

- Closed (`modernc.org/sqlite` + `CGO_ENABLED=0`). Not attempted.

---

## 5. Migration cost (measured)

Schema owner: `var migrations` `schema.go:6` (v1…**v21**). Applied in `store.go:124-163`. `PRAGMA user_version` is the level; `sync_state.schema_version` is updated in the same tx (`store.go:156-162`). Released migrations are never edited (`schema.go:3-5`).

FTS shape owner: `itemsFTSCreate` (`schema.go:16-21`), compared on every `Open` by `repairItemsFTS` (`fts_repair.go:32-48`). A different CREATE **already rebuilds** from `items`+`comments` (`fts_repair.go:51-80`). That is the migration for A-col: change `itemsFTSCreate`, bump v22 so the documented version moves, let Open rebuild.

Measured rebuild:

| Mirror size | A-col / A-table | Notes |
|---|---|---|
| 48 | 2.8 / 4.1 ms | lab |
| **605 (demo)** | **13.5 / 15.3 ms** | stand-in for a small real mirror |
| **20 000** | A-table **338 ms**; U61 194 ms; trigram 1121 ms | GDK-166 size |

A user `gadak.db` of a few thousand issues is a tens-of-milliseconds Open hitch, once.

`fts_repair` still holds after A-col **if** the rebuild INSERT fills `cjk_bigram` the same way as `writeFTS`. If it only copies three columns, mid-match is empty until the next per-row write.

Snapshot (`internal/snapshot/build.go:419-425`, `:613-619`): same 4-tuple or snapshot issues lose CJK mid-match.

`export-static`: no FTS in the JSON freeze. Hosted sqlite-wasm reads the scrubbed demo.db — **scrub must emit the 4th column** (`scripts/scrub-demo-db.py:77-91`).

---

## 6. Statements that become false (or stay true)

| Location | Today | After A-col | Edit |
|---|---|---|---|
| `docs/RECIPES.md:66` | “Word *middles* still miss — searching `결제` will not hit `간편결제`.” | False for CJK 2-char | Replace with: CJK 2-char is a bigram (`결제` hits `간편결제`); English middles still miss (`ency` ≠ `idempotency`). |
| `internal/mcp/tools.go:61-62` | `items_fts MATCH 'term'` over titles/bodies/comments | Still valid; `term` that is a CJK bigram hits more | Add one clause: CJK mid-compound matches; do not key examples on 1–2 Latin letters. |
| `internal/mcp/tools.go:90-93` | `MATCH 'billing'` | Still true (Latin token) | No change required. |
| `specs/000-product/data-model.md:374-378` | 3-column CREATE | 4th column | Update DDL. |
| `data-model.md:387-391` | unicode61 because client substring exists; “Revisit with `trigram`” | Client substring still true for **titles** (`web/src/stores/filters.svelte.ts:640-656` `includes`). Trigram revisit is **measured-wrong**. | Drop the trigram sentence. Say FTS CJK mid-match is app-layer bigrams in `cjk_bigram`. |
| `AGENTS.md:107-110`, `skills/gadak/SKILL.md:142-145` | `MATCH 'webhook AND retry'` | Still true | Optional one-line CJK note. |
| `docs/DERIVE.md:241-244` | `MATCH 'idempotency AND retry'` | Still true | Optional. |
| `internal/store/search_scale_fixture_test.go:156-158` | “Mid-token must not count” for **field attribution** / `"p"` vs Aperture | Keep for English | Do not flip this to substring for Latin. |

A-table would leave RECIPES.md:66 **true** and the MCP example incomplete.

---


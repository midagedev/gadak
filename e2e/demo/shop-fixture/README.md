# shop — the repo the filmed Claude sessions investigate

Fabricated, not a real product. Four files, one per issue the take claims:

| file | issue | the bug that is actually in the code |
| --- | --- | --- |
| `src/checkout.ts` | STD-1 checkout retries the same card twice | `newRequestId()` mints a fresh id per attempt, so the gateway sees two unrelated charges |
| `src/search.ts` | STD-9 search highlight leaks into the sidebar | `clearSearch()` resets `lastQuery` but never clears the sidebar's `data-mark` |
| `src/comment-box.ts` | STD-14 comment box eats the first keystroke | `box.focus()` runs before the element is laid out |
| `src/invoice-export.ts` | STD-4 invoice export drops the tax column | `EXPORT_COLUMNS` is missing `tax`; `PREVIEW_COLUMNS` has it |

The bugs are real so the investigation is real: a live Claude reads these files
and finds them. Copy the tree to the PTY's cwd (`$HOME` of the throwaway agent
drive) before the take — at `$HOME` rather than a subdirectory, because only
`$HOME` is trusted in `.claude.json` and an untrusted folder opens a dialog
that swallows the prompt.

// Package attachaudit names one thing gadak cannot fix and must not hide:
// an attachment whose bytes were cut off when they were uploaded.
//
// A built-in origin (issuetap) capped stored attachment bytes at 8 MiB
// before gadak GDK-1614 lifted it, and the cap truncated silently — the row
// kept the filename and the mime, and the blob held the first 8 MiB. Those
// bytes are gone: the original file was never stored anywhere else, so
// there is nothing to recover from. What is left is to stop presenting the
// remainder as a whole file.
//
// The signal is a size of exactly 8 MiB, which is a suspicion and not a
// verdict: a file can genuinely be 8388608 bytes long. Every string here
// says "may be" for that reason. The single owner of both the predicate and
// the wording lives in this package so `gadak doctor` and `gadak issue`
// cannot describe the same row differently.
package attachaudit

import "strconv"

// TruncatedSize is the old built-in upload cap, in bytes. An attachment of
// exactly this size is the suspicion; anything else is not.
const TruncatedSize int64 = 8 << 20 // 8388608

// Suspect reports whether size is the truncation signal.
func Suspect(size int64) bool { return size == TruncatedSize }

// Mark is the tag on one attachment line. Short, and hedged: the size alone
// cannot tell a cut file from a file that is really this long.
const Mark = "(exactly 8 MiB — may be truncated)"

// Summary is the doctor line for n suspect attachments in one workspace. It
// is only true of a built-in-origin workspace: a Jira or Linear workspace
// never went through that upload cap, so the caller gates on the origin
// type, not on the count.
func Summary(n int) string {
	files := "attachments"
	if n == 1 {
		files = "attachment"
	}
	return plural(n, files) + " of exactly 8 MiB — the size the built-in origin's old upload cap produced, so the bytes may have been cut short on the way in, and what is missing cannot be recovered; re-attach the originals if you still have them. Which ones: " + sampleQuery
}

func plural(n int, noun string) string {
	return strconv.Itoa(n) + " " + noun
}

// sampleQuery names the suspect attachments. doctor prints it rather than the
// names themselves: the document is meant to be paste-safe, and issue keys
// and filenames are the user's material.
const sampleQuery = `gadak sql "SELECT i.key, a.filename FROM attachments a JOIN items i ON i.id = a.item_id WHERE a.size = 8388608"`

package migrate

// Streaming emission of the seed document (GDK-1618).
//
// The seed used to be assembled the obvious way: InlineAttachments filled
// every attachment's bytes into the document as base64, then
// json.MarshalIndent produced a second full copy of the whole thing. Both
// halves scaled with the archive — measured on a real Jira mirror, 19,076
// attachments totalling 183 GiB, 22% of them over 8 MiB — so a workspace
// large enough to be worth migrating was the one that could not be.
//
// WriteDoc is the single owner of "what the seed file looks like" now.
// Attachment bytes are fetched at the moment the writer reaches them and
// base64-encode straight into the output stream, so peak memory is one
// copy buffer, not the sum of the archive. Everything the old pair did —
// the printable-text slot, the 404 and size-cap accounting, the metadata
// row a failed download still leaves behind — happens here, per attachment,
// in the same order.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// StreamFetch opens one attachment's bytes by content id. Implemented by
// the caller over the source origin client (Jira's and issuetap's
// /attachment/content/{id} are the same shape). size is the response's
// declared length, -1 when the origin does not say; body is nil unless
// status is 200 and err is nil, and the caller of StreamFetch closes it. A
// missing file answers 404 with err nil.
type StreamFetch func(ctx context.Context, contentID string) (status int, size int64, body io.ReadCloser, err error)

// inlineTextMax caps what the readable `text` slot may hold. Choosing that
// slot means deciding on the bytes before the key is written, which means
// buffering them; past this size the file goes out as base64 instead —
// same bytes, restored identically by the loader, one streamed pass.
const inlineTextMax = 1 << 20

// WriteDoc writes doc to w as JSON — the seed issuetap loads one-shot.
//
// JSON, not YAML, even though the seed file keeps issuetap's legacy name:
// yaml.v3's emitter produces block scalars its own parser rejects on real
// Jira bodies (leading-space/blank-line combinations — measured on the
// first full GDK export, "did not find expected key"). JSON escaping has no
// such class, and JSON is valid YAML, so issuetap's yaml.Unmarshal reads it
// unchanged.
//
// fetch nil emits attachment metadata with no bytes (--skip-attachments).
// A download that fails, 404s or exceeds the size cap is counted in st and
// leaves the metadata row: a partial archive that says what is missing
// beats no archive.
func WriteDoc(ctx context.Context, w io.Writer, doc *Doc, fetch StreamFetch, st *Stats) error {
	bw := bufio.NewWriterSize(w, 64<<10)
	if err := writeDoc(ctx, bw, doc, fetch, st); err != nil {
		return err
	}
	return bw.Flush()
}

// docHead and docTail are the document's non-issue halves. Doc itself is
// never marshalled whole any more — the issues array is written one issue
// at a time — so the two structs carry the same keys in the same order.
// TestWriteDocKeysMatchMarshal fails if a new Doc field lands in neither.
type docHead struct {
	Users      []User      `json:"users,omitempty"`
	Projects   []Project   `json:"projects,omitempty"`
	Statuses   []Status    `json:"statuses,omitempty"`
	Priorities []Priority  `json:"priorities,omitempty"`
	IssueTypes []IssueType `json:"issueTypes,omitempty"`
}

type docTail struct {
	Spaces []Space `json:"spaces,omitempty"`
	Pages  []Page  `json:"pages,omitempty"`
}

// issueMeta is Issue without the attachments array (written separately) —
// a defined type, so it inherits every json tag and cannot drift from it.
type issueMeta Issue

// attachMeta is Attachment without its content slots.
type attachMeta Attachment

func writeDoc(ctx context.Context, w *bufio.Writer, doc *Doc, fetch StreamFetch, st *Stats) error {
	if _, err := w.WriteString("{"); err != nil {
		return err
	}
	first := true
	if err := writeFields(w, &first, docHead{
		Users: doc.Users, Projects: doc.Projects, Statuses: doc.Statuses,
		Priorities: doc.Priorities, IssueTypes: doc.IssueTypes,
	}); err != nil {
		return err
	}
	if len(doc.Issues) > 0 {
		if err := comma(w, &first); err != nil {
			return err
		}
		if _, err := w.WriteString(`"issues":[`); err != nil {
			return err
		}
		for i := range doc.Issues {
			if i > 0 {
				if _, err := w.WriteString(","); err != nil {
					return err
				}
			}
			if err := writeIssue(ctx, w, &doc.Issues[i], fetch, st); err != nil {
				return err
			}
		}
		if _, err := w.WriteString("]"); err != nil {
			return err
		}
	}
	if err := writeFields(w, &first, docTail{Spaces: doc.Spaces, Pages: doc.Pages}); err != nil {
		return err
	}
	_, err := w.WriteString("}")
	return err
}

func writeIssue(ctx context.Context, w *bufio.Writer, is *Issue, fetch StreamFetch, st *Stats) error {
	if _, err := w.WriteString("{"); err != nil {
		return err
	}
	meta := issueMeta(*is)
	meta.Attachments = nil
	first := true
	if err := writeFields(w, &first, meta); err != nil {
		return err
	}
	if len(is.Attachments) > 0 {
		if err := comma(w, &first); err != nil {
			return err
		}
		if _, err := w.WriteString(`"attachments":[`); err != nil {
			return err
		}
		for i := range is.Attachments {
			if i > 0 {
				if _, err := w.WriteString(","); err != nil {
					return err
				}
			}
			if err := writeAttachment(ctx, w, is.Attachments[i], is.Key, fetch, st); err != nil {
				return err
			}
		}
		if _, err := w.WriteString("]"); err != nil {
			return err
		}
	}
	_, err := w.WriteString("}")
	return err
}

// writeAttachment emits one attachment object, downloading its bytes on the
// spot when there is a fetch. The content key is decided before any byte is
// written, because JSON has no way to take one back: a text/* file small
// enough for the readable slot is read into memory first (at most
// inlineTextMax), everything else base64-encodes straight through.
func writeAttachment(ctx context.Context, w *bufio.Writer, a Attachment, issueKey string, fetch StreamFetch, st *Stats) error {
	meta := attachMeta(a)
	meta.Text, meta.DataBase64 = "", ""

	if fetch == nil {
		return writeObject(w, meta, nil)
	}
	switch {
	case a.SourceURL != "":
		st.AttachSkipURL++
		return writeObject(w, meta, nil)
	case a.Size > maxAttachmentBytes:
		st.AttachTooLarge++
		return writeObject(w, meta, nil)
	}

	status, size, body, err := fetch(ctx, a.ContentID)
	if body != nil {
		defer body.Close()
	}
	switch {
	case err != nil:
		st.AttachErrors = append(st.AttachErrors,
			fmt.Sprintf("%s (%s): %v", a.Filename, issueKey, err))
		return writeObject(w, meta, nil)
	case status == 404:
		st.AttachMissing++
		return writeObject(w, meta, nil)
	case status != 200:
		st.AttachErrors = append(st.AttachErrors,
			fmt.Sprintf("%s (%s): origin status %d", a.Filename, issueKey, status))
		return writeObject(w, meta, nil)
	case size > maxAttachmentBytes:
		st.AttachTooLarge++
		return writeObject(w, meta, nil)
	case body == nil:
		st.AttachErrors = append(st.AttachErrors,
			fmt.Sprintf("%s (%s): origin answered 200 with no body", a.Filename, issueKey))
		return writeObject(w, meta, nil)
	}

	// An origin that does not declare a length cannot be streamed under the
	// cap — an over-cap file would only be discovered with half its base64
	// already in the file, and a truncated attachment written as a whole one
	// is the silent loss GDK-1617 fixed on the way in. Read it bounded
	// instead: one file at a time, never the archive.
	if size < 0 {
		buf, err := readCapped(body, maxAttachmentBytes)
		if err != nil {
			st.AttachErrors = append(st.AttachErrors,
				fmt.Sprintf("%s (%s): %v", a.Filename, issueKey, err))
			return writeObject(w, meta, nil)
		}
		if buf == nil {
			st.AttachTooLarge++
			return writeObject(w, meta, nil)
		}
		if isPrintableText(a.MimeType, buf) {
			meta.Text = string(buf)
			st.AttachInlined++
			st.AttachBytes += int64(len(buf))
			return writeObject(w, meta, nil)
		}
		st.AttachInlined++
		st.AttachBytes += int64(len(buf))
		return writeObject(w, meta, bytes.NewReader(buf))
	}

	// text/* small enough for the readable slot: the key depends on the
	// bytes, so read them (bounded by inlineTextMax) before choosing.
	var head []byte
	if strings.HasPrefix(a.MimeType, "text/") && size <= inlineTextMax {
		head = make([]byte, size)
		if _, err := io.ReadFull(body, head); err != nil {
			st.AttachErrors = append(st.AttachErrors,
				fmt.Sprintf("%s (%s): %v", a.Filename, issueKey, err))
			return writeObject(w, meta, nil)
		}
		if isPrintableText(a.MimeType, head) {
			meta.Text = string(head)
			st.AttachInlined++
			st.AttachBytes += size
			return writeObject(w, meta, nil)
		}
	}

	// head is either nil or the whole file (it was read in full above and
	// only the printable test rejected it), so there is nothing left to join.
	src := io.Reader(io.LimitReader(body, size))
	if head != nil {
		src = bytes.NewReader(head)
	}
	n, err := writeObjectCounting(w, meta, src)
	switch {
	case err != nil:
		return err
	case n != size:
		st.AttachErrors = append(st.AttachErrors,
			fmt.Sprintf("%s (%s): origin declared %d bytes and sent %d", a.Filename, issueKey, size, n))
	}
	st.AttachInlined++
	st.AttachBytes += n
	return nil
}

// readCapped reads at most cap bytes. A body longer than that returns nil
// with a nil error — the caller counts it as over the cap.
func readCapped(r io.Reader, cap int64) ([]byte, error) {
	buf, err := io.ReadAll(io.LimitReader(r, cap+1))
	if err != nil {
		return nil, err
	}
	if int64(len(buf)) > cap {
		return nil, nil
	}
	return buf, nil
}

// writeObject emits one attachment object: its metadata keys, then the
// base64 content read from src (nil for a metadata-only row).
func writeObject(w *bufio.Writer, meta attachMeta, src io.Reader) error {
	_, err := writeObjectCounting(w, meta, src)
	return err
}

// writeObjectCounting is writeObject and reports how many source bytes the
// base64 slot consumed.
func writeObjectCounting(w *bufio.Writer, meta attachMeta, src io.Reader) (int64, error) {
	if _, err := w.WriteString("{"); err != nil {
		return 0, err
	}
	first := true
	if err := writeFields(w, &first, meta); err != nil {
		return 0, err
	}
	var n int64
	if src != nil {
		if err := comma(w, &first); err != nil {
			return 0, err
		}
		if _, err := w.WriteString(`"dataBase64":"`); err != nil {
			return 0, err
		}
		enc := base64.NewEncoder(base64.StdEncoding, w)
		copied, err := io.Copy(enc, src)
		if err != nil {
			_ = enc.Close()
			return copied, err
		}
		if err := enc.Close(); err != nil {
			return copied, err
		}
		if _, err := w.WriteString(`"`); err != nil {
			return copied, err
		}
		n = copied
	}
	if _, err := w.WriteString("}"); err != nil {
		return n, err
	}
	return n, nil
}

// writeFields marshals v — which must be a struct — and writes its members
// without the surrounding braces, so the caller owns the object. omitempty
// is honoured by the marshaller, so an all-empty struct writes nothing.
func writeFields(w *bufio.Writer, first *bool, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	inner := b[1 : len(b)-1]
	if len(inner) == 0 {
		return nil
	}
	if err := comma(w, first); err != nil {
		return err
	}
	_, err = w.Write(inner)
	return err
}

func comma(w *bufio.Writer, first *bool) error {
	if *first {
		*first = false
		return nil
	}
	_, err := w.WriteString(",")
	return err
}

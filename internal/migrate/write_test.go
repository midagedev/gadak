package migrate

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// fullDoc is a document with every Doc field populated — the fixture the
// key-parity test needs, since a field left empty is a field omitempty
// would hide.
func fullDoc() *Doc {
	return &Doc{
		Users:      []User{{AccountID: "u1", DisplayName: "One"}},
		Projects:   []Project{{Key: "STD", Name: "Standard"}},
		Statuses:   []Status{{ID: "1", Name: "To Do", Category: "new"}},
		Priorities: []Priority{{ID: "3", Name: "Medium"}},
		IssueTypes: []IssueType{{ID: "10", Name: "Task"}},
		Issues: []Issue{{
			Key: "STD-1", Summary: "one", Description: "body",
			DescriptionADF: `{"type":"doc"}`, Project: "STD", Type: "10",
			Status: "1", Priority: "3", Assignee: "u1", Reporter: "u1",
			Parent: "STD-9", Labels: []string{"a"}, Components: []string{"c"},
			FixVersions: []string{"v1"}, Duedate: "2026-01-01", Resolution: "r",
			Created: "2026-01-01T00:00:00.000+0000", Updated: "2026-01-02T00:00:00.000+0000",
			Comments:    []Comment{{Author: "u1", Body: "hi", Created: "2026-01-01T00:00:00.000+0000"}},
			Attachments: []Attachment{{Filename: "a.txt", MimeType: "text/plain", Author: "u1"}},
			Links:       []Link{{Type: "Blocks", Outward: "STD-2"}},
			History: []History{{At: "2026-01-01T00:00:00.000+0000", Author: "u1",
				Items: []HistoryItem{{Field: "status", From: "1", To: "2"}}}},
		}},
		Spaces: []Space{{Key: "SPC", Name: "Space"}},
		Pages: []Page{{ID: "p1", Title: "Page", Space: "SPC", Version: 2,
			When: "2026-01-01T00:00:00.000+0000", Author: "u1", Body: "text",
			BodyADF: `{"type":"doc"}`, Labels: []string{"l"}, Parent: "p0",
			Comments: []PageComment{{Author: "u1", Body: "c"}}}},
	}
}

// TestWriteDocKeysMatchMarshal is the structural guard on the hand-written
// emitter: WriteDoc walks the document in three pieces (docHead, the issue
// array, docTail) instead of marshalling Doc whole, so a new Doc field
// could be emitted by json.Marshal and silently dropped by the seed writer.
// With no fetch the two must agree exactly, key for key, at every level.
func TestWriteDocKeysMatchMarshal(t *testing.T) {
	doc := fullDoc()
	var buf bytes.Buffer
	if err := WriteDoc(context.Background(), &buf, doc, nil, &Stats{}); err != nil {
		t.Fatalf("WriteDoc: %v", err)
	}
	var got, want any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("WriteDoc output is not JSON: %v\n%s", err, buf.String())
	}
	ref, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(ref, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("streamed document differs from json.Marshal\n got: %s\nwant: %s", buf.String(), ref)
	}
}

// TestWriteDocAttachments is the content pass, inherited from the
// InlineAttachments test it replaces (GDK-1618): the readable slot, the
// base64 slot, and every reason a row keeps its metadata and no bytes.
func TestWriteDocAttachments(t *testing.T) {
	bin := []byte{0x00, 0xFF, 0x10}
	long := strings.Repeat("x", inlineTextMax+1)
	doc := &Doc{Issues: []Issue{{Key: "T-1", Attachments: []Attachment{
		{Filename: "a.txt", MimeType: "text/plain", ContentID: "1"},
		{Filename: "b.bin", MimeType: "application/octet-stream", ContentID: "2"},
		{Filename: "gone.txt", MimeType: "text/plain", ContentID: "3"},
		{Filename: "linear.png", MimeType: "image/png", ContentID: "4", SourceURL: "https://uploads.linear.app/x"},
		{Filename: "huge.bin", ContentID: "5", Size: maxAttachmentBytes + 1},
		{Filename: "nolen.txt", MimeType: "text/plain", ContentID: "6"},
		{Filename: "long.txt", MimeType: "text/plain", ContentID: "7"},
	}}}}
	st := &Stats{}
	fetch := func(_ context.Context, id string) (int, int64, io.ReadCloser, error) {
		body := func(b []byte) (int, int64, io.ReadCloser, error) {
			return 200, int64(len(b)), io.NopCloser(bytes.NewReader(b)), nil
		}
		switch id {
		case "1":
			return body([]byte("hello\n"))
		case "2":
			return body(bin)
		case "3":
			return 404, 0, nil, nil
		case "6":
			// No Content-Length: the bounded read, not the stream.
			return 200, -1, io.NopCloser(strings.NewReader("no length\n")), nil
		case "7":
			return body([]byte(long))
		}
		t.Fatalf("unexpected fetch %q", id)
		return 0, 0, nil, nil
	}
	var buf bytes.Buffer
	if err := WriteDoc(context.Background(), &buf, doc, fetch, st); err != nil {
		t.Fatalf("WriteDoc: %v", err)
	}

	var out struct {
		Issues []struct {
			Attachments []Attachment `json:"attachments"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, buf.String())
	}
	atts := out.Issues[0].Attachments
	if len(atts) != 7 {
		t.Fatalf("every attachment keeps its row, got %d", len(atts))
	}
	if atts[0].Text != "hello\n" || atts[0].DataBase64 != "" {
		t.Fatalf("text inline: %+v", atts[0])
	}
	if atts[1].DataBase64 != base64.StdEncoding.EncodeToString(bin) || atts[1].Text != "" {
		t.Fatalf("binary inline: %+v", atts[1])
	}
	for _, i := range []int{2, 3, 4} {
		if atts[i].Text != "" || atts[i].DataBase64 != "" {
			t.Fatalf("attachment %d keeps metadata only: %+v", i, atts[i])
		}
	}
	if atts[5].Text != "no length\n" {
		t.Fatalf("unknown-length text: %+v", atts[5])
	}
	// Past the readable-slot cap the same bytes go out as base64 — the key
	// has to be chosen before any byte is written, and the loader restores
	// either slot identically.
	if atts[6].Text != "" || atts[6].DataBase64 != base64.StdEncoding.EncodeToString([]byte(long)) {
		t.Fatalf("over-cap text goes out as base64, got text=%d b64=%d", len(atts[6].Text), len(atts[6].DataBase64))
	}
	if st.AttachInlined != 4 || st.AttachMissing != 1 || st.AttachSkipURL != 1 || st.AttachTooLarge != 1 {
		t.Fatalf("stats %+v", st)
	}
	if want := int64(len("hello\n") + len(bin) + len("no length\n") + len(long)); st.AttachBytes != want {
		t.Fatalf("AttachBytes = %d, want %d", st.AttachBytes, want)
	}
}

// repeating is an attachment body that costs nothing to produce — the alloc
// contract below must measure the writer, not its test fixture.
type repeating struct{ left int64 }

func (r *repeating) Read(p []byte) (int, error) {
	if r.left <= 0 {
		return 0, io.EOF
	}
	n := int64(len(p))
	if n > r.left {
		n = r.left
	}
	for i := int64(0); i < n; i++ {
		p[i] = byte(i)
	}
	r.left -= n
	return int(n), nil
}

func (r *repeating) Close() error { return nil }

// TestWriteDocAllocsDoNotScaleWithArchive is GDK-1618's numeric contract.
//
// The old shape held every attachment's bytes as a base64 string inside the
// document and then let json.MarshalIndent copy the whole thing, so total
// allocation tracked the archive: the measured real mirror is 19,076
// attachments and 183 GiB. The baseline below is literally that shape,
// built from the same exported types, so the assertion has a live FAIL-first
// half — it is not a remembered number.
func TestWriteDocAllocsDoNotScaleWithArchive(t *testing.T) {
	const (
		n = 8
		m = 1 << 20 // bytes per attachment
	)
	doc := &Doc{Issues: []Issue{{Key: "T-1"}}}
	for i := 0; i < n; i++ {
		doc.Issues[0].Attachments = append(doc.Issues[0].Attachments, Attachment{
			Filename:  fmt.Sprintf("f%d.bin", i),
			MimeType:  "application/octet-stream",
			ContentID: fmt.Sprintf("%d", i),
			Size:      m,
		})
	}
	fetch := func(_ context.Context, _ string) (int, int64, io.ReadCloser, error) {
		return 200, m, &repeating{left: m}, nil
	}

	streamed := allocatedBytes(t, func() {
		st := &Stats{}
		if err := WriteDoc(context.Background(), io.Discard, doc, fetch, st); err != nil {
			t.Fatalf("WriteDoc: %v", err)
		}
		if st.AttachInlined != n {
			t.Fatalf("inlined %d, want %d", st.AttachInlined, n)
		}
	})

	inlined := allocatedBytes(t, func() {
		// The pre-GDK-1618 shape: every body in memory as base64, then one
		// more copy of the whole document from the marshaller.
		d := &Doc{Issues: []Issue{{Key: "T-1", Attachments: append([]Attachment(nil), doc.Issues[0].Attachments...)}}}
		for i := range d.Issues[0].Attachments {
			body, err := io.ReadAll(&repeating{left: m})
			if err != nil {
				t.Fatal(err)
			}
			d.Issues[0].Attachments[i].DataBase64 = base64.StdEncoding.EncodeToString(body)
		}
		data, err := json.MarshalIndent(d, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if len(data) == 0 {
			t.Fatal("empty")
		}
	})

	t.Logf("n=%d m=%d bytes: streamed %d bytes allocated, inlined-in-document %d", n, m, streamed, inlined)
	// The archive is n*m bytes. Streaming must stay far under it — a
	// quarter is generous room for the copy buffers and the document's own
	// JSON, and still an order below the shape it replaces.
	if limit := int64(n) * m / 4; streamed > limit {
		t.Fatalf("streaming allocated %d bytes for a %d-byte archive (limit %d) — the bytes are piling up again", streamed, int64(n)*m, limit)
	}
	if streamed*4 > inlined {
		t.Fatalf("streaming allocated %d bytes, the in-document shape %d — no headroom, the writer is not streaming", streamed, inlined)
	}
}

// allocatedBytes reports how many bytes fn caused the heap to allocate in
// total (freed or not) — the axis that catches "every attachment passes
// through memory", which a live-heap reading at the end would miss.
func allocatedBytes(t *testing.T, fn func()) int64 {
	t.Helper()
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	fn()
	runtime.ReadMemStats(&after)
	return int64(after.TotalAlloc - before.TotalAlloc)
}

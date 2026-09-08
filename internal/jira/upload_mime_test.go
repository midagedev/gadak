package jira

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// GDK-1617: multipart.CreateFormFile hardcodes application/octet-stream,
// and an origin that keeps what it is told — gadak's own tracker does, and
// it holds the only copy — then stores every screenshot and every clip as a
// generic download. is_image and is_video are computed from the stored mime
// (internal/server/read.go), so the app showed a file row instead of a
// thumbnail and a player, permanently. Measured with `gadak attach` of a
// .png and an .mp4: both landed as application/octet-stream.
func TestUploadDeclaresTheContentTypeFromTheFilename(t *testing.T) {
	got := map[string]string{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Errorf("request content type: %v", err)
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			got[part.FileName()] = part.Header.Get("Content-Type")
			_, _ = io.Copy(io.Discard, part)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"70001"}]`))
	}))
	defer ts.Close()

	c := New(ts.URL, "you@example.com", "token")
	for _, name := range []string{"shot.png", "clip.mp4", "notes.txt", "blob.unknownext"} {
		if _, err := c.Upload(context.Background(), "STD-1", name, strings.NewReader("x")); err != nil {
			t.Fatalf("upload %s: %v", name, err)
		}
	}
	want := map[string]string{
		"shot.png":  "image/png",
		"clip.mp4":  "video/mp4",
		"notes.txt": "text/plain",
		// No known type is the honest generic answer, not a guess.
		"blob.unknownext": "application/octet-stream",
	}
	for name, wantCT := range want {
		base := strings.SplitN(got[name], ";", 2)[0]
		if strings.TrimSpace(base) != wantCT {
			t.Errorf("%s uploaded as %q, want %q", name, got[name], wantCT)
		}
	}
}

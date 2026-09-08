package main

import (
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/store"
)

// A 200 carrying the origin's own HTML page is not the attachment — writing
// it under the attachment's name is the silent-wrong-file shape GDK-1644
// measured against a Jira Server instance that does not serve the route.
func TestHTMLErrorPageIsRefusedWhenTheAttachmentIsNotHTML(t *testing.T) {
	att := store.DetailAttachment{ExternalID: "10001", Filename: "shot.png", MimeType: "image/png"}
	err := refuseErrorPage(att, "text/html;charset=UTF-8")
	if err == nil {
		t.Fatal("an HTML page was accepted as the bytes of a PNG")
	}
	for _, want := range []string{"10001", "shot.png", "image/png"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("refusal does not name %q: %v", want, err)
		}
	}
}

// An .html attachment is an ordinary thing to attach. Refusing those would
// trade a silent wrong file for a refused right one.
func TestAnHTMLAttachmentIsStillDownloadable(t *testing.T) {
	att := store.DetailAttachment{ExternalID: "7", Filename: "report.html", MimeType: "text/html"}
	if err := refuseErrorPage(att, "text/html;charset=UTF-8"); err != nil {
		t.Fatalf("a genuine HTML attachment was refused: %v", err)
	}
}

func TestNonHTMLResponsesAreUntouched(t *testing.T) {
	att := store.DetailAttachment{ExternalID: "3", Filename: "shot.png", MimeType: "image/png"}
	for _, ct := range []string{"image/png", "application/octet-stream", "", "not a media type"} {
		if err := refuseErrorPage(att, ct); err != nil {
			t.Fatalf("Content-Type %q refused: %v", ct, err)
		}
	}
}

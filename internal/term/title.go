package term

import (
	"strings"
	"unicode"
)

// The window title as a session subtitle (GDK-1389).
//
// A shell that sets its window title is already saying what it is doing —
// Claude Code rewrites it as its task changes, and Ubuntu's stock prompt
// puts `user@host: cwd` there. The scanner in bell.go had to learn where an
// OSC string ends anyway, to tell a title's 0x07 terminator from a real BEL
// (GDK-1163); this file is the payload it used to drop on the floor. There
// is one parser over the stream, not two: a second state machine would be a
// second opinion on where a string ends, and that question is exactly the
// one this package has already got wrong once.
//
// The title never touches NeedsAttention. That bit means "a person is
// wanted", and a prompt repainting its title is the case that made it stop
// meaning anything (attention_test.go, TestWindowTitleOSCDoesNotAskForAPerson).
// Capture is downstream of the bell decision and cannot reach it.

const (
	// maxTitleRunes bounds a stored title. A roster row is one line, and a
	// hand-given name is capped at 64 (maxNameRunes); a title is machine
	// prose — a verb plus a path — so it gets more room, but not prose
	// room. Beyond this it is a log line, not a subtitle, and the row
	// truncates it anyway.
	maxTitleRunes = 120
	// oscCaptureMax bounds the bytes held for an OSC string that has not
	// terminated yet. The payload is whatever the shell ran chose to
	// print: a program that writes ESC ] and then megabytes without a
	// terminator would otherwise hold all of it. Comfortably above any
	// real title even after multi-byte runes, and the scanner keeps
	// hunting the terminator after the buffer stops growing.
	oscCaptureMax = 4096
)

// capture appends one payload byte of the open OSC string, if one is open
// and the buffer has room.
func (s *bellScanner) capture(b byte) {
	if !s.capturing || len(s.osc) >= oscCaptureMax {
		return
	}
	s.osc = append(s.osc, b)
}

// endString closes the open string and, when it was an OSC 0 or OSC 2,
// leaves its title for takeTitle.
func (s *bellScanner) endString() {
	s.state = bellGround
	if s.capturing {
		if title, ok := windowTitle(s.osc); ok {
			s.title, s.titleSet = title, true
		}
	}
	s.capturing = false
	s.osc = s.osc[:0]
}

// takeTitle returns the window title this stream last set and clears it —
// take, not peek, so a session stores a title once per time the shell sends
// one rather than on every chunk of output that follows it.
func (s *bellScanner) takeTitle() (string, bool) {
	if !s.titleSet {
		return "", false
	}
	title := s.title
	s.title, s.titleSet = "", false
	return title, true
}

// windowTitle reads an OSC payload (everything between `ESC ]` and the
// terminator) and reports the window title it sets, if it sets one.
//
// OSC 0 sets icon name and window title, OSC 2 sets the window title. Every
// other Ps is a different instruction that happens to share the envelope —
// OSC 1 is the icon name alone, OSC 7 the working directory, OSC 8 a
// hyperlink, OSC 133 a prompt mark — and none of them is what a person
// reading the roster is being told. The Ps is matched exactly: `02` is not
// `0`, because a client that normalises numbers here would be guessing.
func windowTitle(payload []byte) (string, bool) {
	i := strings.IndexByte(string(payload), ';')
	if i < 0 {
		return "", false
	}
	switch string(payload[:i]) {
	case "0", "2":
	default:
		return "", false
	}
	title := sanitizeTitle(string(payload[i+1:]))
	return title, title != ""
}

// sanitizeTitle makes an untrusted string safe to put in a roster row.
//
// Every control character becomes a space, runs of spaces collapse, and the
// result is trimmed and capped. Control characters are the whole risk here:
// a CR or a newline would break the single line the row is, and the escape
// bytes are how a crafted title would try to repaint someone else's terminal
// when the value is echoed back through one (`gadak terminal list`). They
// become spaces rather than vanishing so a title does not silently join two
// words into one. Trimming last means a title that was only control bytes or
// only whitespace comes back empty — and an empty title is no subtitle at
// all, not an empty one.
func sanitizeTitle(s string) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		if r == 0x7f || unicode.IsControl(r) || unicode.IsSpace(r) || !unicode.IsPrint(r) {
			space = true
			continue
		}
		if space && b.Len() > 0 {
			b.WriteRune(' ')
		}
		space = false
		b.WriteRune(r)
	}
	out := b.String()
	if r := []rune(out); len(r) > maxTitleRunes {
		out = strings.TrimSpace(string(r[:maxTitleRunes]))
	}
	return out
}

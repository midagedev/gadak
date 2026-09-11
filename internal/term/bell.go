package term

// Telling a bell from a string terminator (GDK-1163, 2026-08-30).
//
// 0x07 means two different things on a terminal stream. On its own it is BEL:
// the shell, the agent or the TUI is asking for a person, which is the signal
// NeedsAttention exists for. Inside an OSC/DCS/SOS/PM/APC string it is ST —
// the byte that ends the string — and it says nothing at all.
//
// Counting every 0x07 as a bell made the bit permanently true on any ordinary
// Linux box. Ubuntu's stock /etc/skel/.bashrc puts the window title in the
// prompt for an xterm-ish TERM:
//
//	PS1="\[\e]0;\u@\h: \w\a\]$PS1"
//
// and sessions here start with TERM=xterm-256color, so *every prompt* carries
// an OSC 0 terminated by 0x07. Measured on the GitHub runner: the strip row
// went to "wants you" and stayed there through an attach, because the next
// prompt raised it again — the bit had stopped meaning anything, and the e2e
// that pins it failed there and nowhere else.
//
// The scanner below is the smallest parser that answers the question. It is
// not a terminal emulator: it tracks only whether the stream is currently
// inside a string-terminated sequence, which is exactly what decides how the
// next 0x07 reads. State lives across calls because a PTY read can end
// anywhere, including between the ESC and the ] that follows it.
type bellScanner struct {
	state bellState
	// osc holds the payload of the OSC string currently open, empty when
	// none is or when the open one is not an OSC (DCS/SOS/PM/APC carry no
	// title). Capped at oscCaptureMax — see title.go for why a shell that
	// opens a string and never closes it must not be able to grow it.
	osc       []byte
	capturing bool
	// title is the last window title this stream set, waiting for
	// takeTitle to collect it. titleSet is the "waiting" — an empty title
	// is not a title, so the bool cannot be derived from the string.
	title    string
	titleSet bool
}

type bellState uint8

const (
	// bellGround: ordinary output. 0x07 here is a bell.
	bellGround bellState = iota
	// bellEscape: an ESC was seen in ground; the next byte decides whether a
	// string is opening.
	bellEscape
	// bellString: inside OSC/DCS/SOS/PM/APC. 0x07 here is the terminator.
	bellString
	// bellStringEscape: an ESC inside a string; `ESC \` is the other spelling
	// of the terminator.
	bellStringEscape
)

const (
	escByte = 0x1b
	stByte  = '\\'
)

// opensString reports whether `b`, following an ESC, opens a sequence whose
// terminator is a string terminator: OSC (`]`), DCS (`P`), SOS (`X`), PM
// (`^`) or APC (`_`). Every other escape sequence — CSI included — ends on a
// byte of its own and cannot swallow a bell.
func opensString(b byte) bool {
	switch b {
	case ']', 'P', 'X', '^', '_':
		return true
	}
	return false
}

// scan advances over p and reports whether it contained at least one real
// BEL: a 0x07 in ground state, not one ending a string.
func (s *bellScanner) scan(p []byte) bool {
	rang := false
	for _, b := range p {
		switch s.state {
		case bellGround:
			switch {
			case b == bellByte:
				rang = true
			case b == escByte:
				s.state = bellEscape
			}
		case bellEscape:
			switch {
			case opensString(b):
				s.state = bellString
				// Only OSC carries a window title; the other string
				// openers are captured as "not a title" so a DCS
				// payload can never be mistaken for one.
				s.capturing = b == ']'
				s.osc = s.osc[:0]
			case b == escByte:
				// Two ESCs in a row: the second one is the one that counts.
			case b == bellByte:
				// `ESC BEL` opens nothing, so this 0x07 is still a bell.
				rang = true
				s.state = bellGround
			default:
				s.state = bellGround
			}
		case bellString:
			switch b {
			case bellByte:
				s.endString()
			case escByte:
				s.state = bellStringEscape
			default:
				s.capture(b)
			}
		case bellStringEscape:
			switch b {
			case stByte:
				s.endString()
			case escByte:
				// Still waiting for the byte after an ESC.
			default:
				// An ESC inside a string payload is malformed. Drop
				// it and its introducer rather than let either reach
				// the title; the terminator hunt continues.
				s.state = bellString
			}
		}
	}
	return rang
}

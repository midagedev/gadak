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
				s.state = bellGround
			case escByte:
				s.state = bellStringEscape
			}
		case bellStringEscape:
			switch b {
			case stByte:
				s.state = bellGround
			case escByte:
				// Still waiting for the byte after an ESC.
			default:
				s.state = bellString
			}
		}
	}
	return rang
}

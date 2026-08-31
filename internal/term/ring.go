package term

// ring is the fixed-size scrollback a session replays to a reattaching
// client. Circular on purpose: the alternative — append and trim the front
// — copies the whole buffer on every write, and this one is written once
// per PTY read.
type ring struct {
	buf    []byte
	w      int
	filled int
}

func newRing(size int) *ring {
	if size <= 0 {
		size = DefaultRingBytes
	}
	return &ring{buf: make([]byte, size)}
}

func (r *ring) write(p []byte) {
	if len(p) == 0 {
		return
	}
	n := len(r.buf)
	if len(p) >= n {
		copy(r.buf, p[len(p)-n:])
		r.w = 0
		r.filled = n
		return
	}
	c := copy(r.buf[r.w:], p)
	if c < len(p) {
		copy(r.buf, p[c:])
	}
	r.w = (r.w + len(p)) % n
	if r.filled += len(p); r.filled > n {
		r.filled = n
	}
}

// bytes returns a copy of the buffered output, oldest first.
func (r *ring) bytes() []byte {
	if r.filled == 0 {
		return nil
	}
	out := make([]byte, r.filled)
	if r.filled < len(r.buf) {
		copy(out, r.buf[:r.w])
		return out
	}
	n := copy(out, r.buf[r.w:])
	copy(out[n:], r.buf[:r.w])
	return out
}

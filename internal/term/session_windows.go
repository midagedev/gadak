//go:build windows

package term

import "fmt"

// Windows has no PTY here yet. ConPTY is a different shape from
// openpty/Setsid — a pseudo-console handle pair plus
// PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, with no process group to signal —
// and it is being spiked separately (GDK-861). An honest refusal beats a
// silent stub that opens something and then behaves unlike every other
// platform: the pane can say "not on Windows yet" and mean it.
//
// The type keeps the same method set as the unix ptyProc so the shared
// session core compiles unchanged; none of these methods can be reached,
// because startProc never returns a value.
type ptyProc struct{}

func startProc(opts Options) (*ptyProc, error) {
	_ = opts
	return nil, fmt.Errorf("%w: the terminal needs ConPTY on Windows (GDK-861)", ErrUnsupportedPlatform)
}

func (p *ptyProc) Read(b []byte) (int, error)  { return 0, ErrUnsupportedPlatform }
func (p *ptyProc) Write(b []byte) (int, error) { return 0, ErrUnsupportedPlatform }
func (p *ptyProc) pid() int                    { return 0 }
func (p *ptyProc) resize(cols, rows uint16) error {
	_, _ = cols, rows
	return ErrUnsupportedPlatform
}
func (p *ptyProc) winsize() (cols, rows uint16, err error) {
	return 0, 0, ErrUnsupportedPlatform
}
func (p *ptyProc) hangup() error      { return ErrUnsupportedPlatform }
func (p *ptyProc) kill() error        { return ErrUnsupportedPlatform }
func (p *ptyProc) wait() (int, error) { return -1, ErrUnsupportedPlatform }
func (p *ptyProc) closePTY() error    { return ErrUnsupportedPlatform }

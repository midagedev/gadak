package main

import (
	"errors"
	"fmt"
	"github.com/midagedev/gadak/internal/origin"
	"net"
	"strconv"
	"syscall"
	"time"
)

// portAction is the outcome when the preferred listen address is already taken.
type portAction int

const (
	// portActionOpenExisting: same-profile gadak already serves; open its URL, exit 0.
	portActionOpenExisting portAction = iota
	// portActionFallback: try successive ports (default addr only).
	portActionFallback
	// portActionFail: surface an error (explicit --addr, or fallback exhausted).
	portActionFail
)

// gadakProbe is origin.GadakProbe — the single probe implementation lives in
// internal/origin (GDK-423); this alias keeps the fallback table readable.
// It reports what a loopback GET to the progress endpoint learned about the
// process holding a port.
type gadakProbe = origin.GadakProbe

// portBusyDecision is the pure classification of a busy preferred address.
type portBusyDecision struct {
	Action      portAction
	ExistingURL string // open-existing: URL to print/open
	Occupant    string // fallback log: who held the preferred port
	ErrDetail   string // fail: extra text for the error (gadak identity, etc.)
}

// probeFunc classifies the process on 127.0.0.1:<port>. Injected in tests.
type probeFunc func(port string) gadakProbe

// listenFunc is net.Listen-shaped; injected so tests can stub without sockets.
type listenFunc func(network, address string) (net.Listener, error)

const (
	portFallbackMax = 20
	probeTimeout    = 700 * time.Millisecond
)

// decidePortBusy classifies a busy preferred address without touching sockets.
// addrPinned means the user passed --addr (no fallback; rule 3).
func decidePortBusy(addr string, addrPinned bool, currentProfile string, probe probeFunc) portBusyDecision {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return portBusyDecision{Action: portActionFail, ErrDetail: err.Error()}
	}
	p := gadakProbe{}
	if probe != nil {
		p = probe(port)
	}

	if addrPinned {
		d := portBusyDecision{Action: portActionFail}
		if p.IsGadak {
			d.ErrDetail = fmt.Sprintf("another gadak instance is using this address (profile %q)", p.Profile)
		}
		return d
	}

	// Default addr: same-profile gadak → hand off; otherwise fall back.
	if p.IsGadak && p.Profile == currentProfile {
		return portBusyDecision{
			Action:      portActionOpenExisting,
			ExistingURL: browseAddr(addr),
		}
	}
	return portBusyDecision{
		Action:   portActionFallback,
		Occupant: occupantLabel(p),
	}
}

func occupantLabel(p gadakProbe) string {
	if p.IsGadak {
		return fmt.Sprintf("gadak profile %q", p.Profile)
	}
	return "another process"
}

// probeGadakOnPort delegates to the single implementation in internal/origin
// (GDK-423): loopback only, no Origin header, X-Gadak required.
func probeGadakOnPort(port string, timeout time.Duration) gadakProbe {
	return origin.ProbeGadakOnPort(port, timeout)
}

// isAddrInUse reports whether err is EADDRINUSE (darwin/linux via errors.Is).
func isAddrInUse(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE)
}

// shiftPort returns host:port+delta for a host:port address.
func shiftPort(addr string, delta int) (string, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return "", err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", fmt.Errorf("port in %q: %w", addr, err)
	}
	return net.JoinHostPort(host, strconv.Itoa(port+delta)), nil
}

// bindListen tries to bind addr. On EADDRINUSE it applies the port-busy rules:
// same-profile gadak (unpinned) → existing URL, no listener; unpinned other →
// try port+1..+20; pinned → error (with gadak detail when known).
//
// listen defaults to net.Listen. Returns:
//   - (ln, boundAddr, "", nil) on success (bound may differ after fallback)
//   - (nil, "", existingURL, nil) when another same-profile gadak is already up
//   - (_, _, _, err) on hard failure
func bindListen(addr string, addrPinned bool, currentProfile string, probe probeFunc, listen listenFunc) (net.Listener, string, string, error) {
	ln, bound, existing, _, err := bindListenDetail(addr, addrPinned, currentProfile, probe, listen)
	return ln, bound, existing, err
}

// bindListenDetail is bindListen plus the occupant label when a fallback was used
// (for the one-line CLI log).
func bindListenDetail(addr string, addrPinned bool, currentProfile string, probe probeFunc, listen listenFunc) (net.Listener, string, string, string, error) {
	if listen == nil {
		listen = net.Listen
	}
	if probe == nil {
		probe = func(port string) gadakProbe { return probeGadakOnPort(port, probeTimeout) }
	}

	ln, err := listen("tcp", addr)
	if err == nil {
		return ln, addr, "", "", nil
	}
	if !isAddrInUse(err) {
		return nil, "", "", "", err
	}

	dec := decidePortBusy(addr, addrPinned, currentProfile, probe)
	switch dec.Action {
	case portActionOpenExisting:
		return nil, "", dec.ExistingURL, "", nil
	case portActionFail:
		if dec.ErrDetail != "" {
			return nil, "", "", "", fmt.Errorf("listen %s: %w (%s)", addr, err, dec.ErrDetail)
		}
		return nil, "", "", "", fmt.Errorf("listen %s: %w", addr, err)
	case portActionFallback:
		// continue below
	}

	for delta := 1; delta <= portFallbackMax; delta++ {
		try, shiftErr := shiftPort(addr, delta)
		if shiftErr != nil {
			return nil, "", "", "", shiftErr
		}
		ln, err = listen("tcp", try)
		if err == nil {
			return ln, try, "", dec.Occupant, nil
		}
		if !isAddrInUse(err) {
			return nil, "", "", "", err
		}
	}
	return nil, "", "", "", fmt.Errorf("listen %s: address already in use; no free port in +1..+%d", addr, portFallbackMax)
}

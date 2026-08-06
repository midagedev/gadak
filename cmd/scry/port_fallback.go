package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"syscall"
	"time"
)

// portAction is the outcome when the preferred listen address is already taken.
type portAction int

const (
	// portActionOpenExisting: same-profile scry already serves; open its URL, exit 0.
	portActionOpenExisting portAction = iota
	// portActionFallback: try successive ports (default addr only).
	portActionFallback
	// portActionFail: surface an error (explicit --addr, or fallback exhausted).
	portActionFail
)

// scryProbe is what a loopback GET to the progress endpoint learned about the
// process holding a port.
type scryProbe struct {
	IsScry  bool
	Profile string
}

// portBusyDecision is the pure classification of a busy preferred address.
type portBusyDecision struct {
	Action      portAction
	ExistingURL string // open-existing: URL to print/open
	Occupant    string // fallback log: who held the preferred port
	ErrDetail   string // fail: extra text for the error (scry identity, etc.)
}

// probeFunc classifies the process on 127.0.0.1:<port>. Injected in tests.
type probeFunc func(port string) scryProbe

// listenFunc is net.Listen-shaped; injected so tests can stub without sockets.
type listenFunc func(network, address string) (net.Listener, error)

const (
	portFallbackMax = 20
	probeTimeout    = 700 * time.Millisecond
	// probePath is always under the issues API base (see internal/server).
	probePath = "/api/v1/issues/sync/progress/"
)

// decidePortBusy classifies a busy preferred address without touching sockets.
// addrPinned means the user passed --addr (no fallback; rule 3).
func decidePortBusy(addr string, addrPinned bool, currentProfile string, probe probeFunc) portBusyDecision {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return portBusyDecision{Action: portActionFail, ErrDetail: err.Error()}
	}
	p := scryProbe{}
	if probe != nil {
		p = probe(port)
	}

	if addrPinned {
		d := portBusyDecision{Action: portActionFail}
		if p.IsScry {
			d.ErrDetail = fmt.Sprintf("another scry instance is using this address (profile %q)", p.Profile)
		}
		return d
	}

	// Default addr: same-profile scry → hand off; otherwise fall back.
	if p.IsScry && p.Profile == currentProfile {
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

func occupantLabel(p scryProbe) string {
	if p.IsScry {
		return fmt.Sprintf("scry profile %q", p.Profile)
	}
	return "another process"
}

// probeScryOnPort GETs the progress endpoint on 127.0.0.1 only (never remote).
// No Origin header — CLI probe, not a browser.
func probeScryOnPort(port string, timeout time.Duration) scryProbe {
	if timeout <= 0 {
		timeout = probeTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	url := "http://" + net.JoinHostPort("127.0.0.1", port) + probePath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return scryProbe{}
	}
	// Deliberately no Origin (and no custom User-Agent).
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return scryProbe{}
	}
	defer res.Body.Close()
	if res.Header.Get("X-Scry") == "" {
		return scryProbe{}
	}
	return scryProbe{
		IsScry:  true,
		Profile: res.Header.Get("X-Scry-Profile"),
	}
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
// same-profile scry (unpinned) → existing URL, no listener; unpinned other →
// try port+1..+20; pinned → error (with scry detail when known).
//
// listen defaults to net.Listen. Returns:
//   - (ln, boundAddr, "", nil) on success (bound may differ after fallback)
//   - (nil, "", existingURL, nil) when another same-profile scry is already up
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
		probe = func(port string) scryProbe { return probeScryOnPort(port, probeTimeout) }
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

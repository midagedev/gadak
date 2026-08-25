package origin

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
)

// ServeOriginPassthrough starts the loopback origin-only listener for one
// standalone workspace and advertises it: 127.0.0.1:0, exactly two routes
// (GET ProbePath, RESTPrefix/), everything else 404 — this is not a UI/API
// server (decision 0003). h must answer both routes with the X-Gadak
// identity headers (a server.Handler does). The caller must already hold
// the persist lock (GDK-343 lock-before-advertise); on any failure here the
// caller decides whether to release it. stop withdraws the advertise file
// first, then closes the listener.
func ServeOriginPassthrough(dir string, h http.Handler) (stop func(), err error) {
	if dir == "" || h == nil {
		return func() {}, errors.New("origin: passthrough: dir and handler are required")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return func() {}, fmt.Errorf("could not open origin passthrough listener: %w", err)
	}
	mux := http.NewServeMux()
	mux.Handle("GET "+ProbePath+"{$}", h)
	mux.Handle(RESTPrefix+"/", h)
	srv := &http.Server{Handler: mux}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("warning: origin passthrough listener: %v", err)
		}
	}()
	if err := WriteAdvertise(dir, ln.Addr().String()); err != nil {
		_ = srv.Close()
		return func() {}, fmt.Errorf("could not advertise origin owner: %w", err)
	}
	return func() {
		_ = RemoveAdvertise(dir)
		_ = srv.Close()
	}, nil
}

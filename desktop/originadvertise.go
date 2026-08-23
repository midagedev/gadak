package main

import (
	"net/http"

	"github.com/midagedev/gadak/internal/apprun"
	"github.com/midagedev/gadak/internal/config"
)

// startStandaloneOriginListener is the desktop test name for
// apprun.StartOriginPassthrough. Production run() calls the Runtime
// method so the boot sequence has one owner; tests keep this name.
func startStandaloneOriginListener(cfg *config.Config, api http.Handler) (func(), error) {
	return apprun.StartOriginPassthrough(cfg, api)
}

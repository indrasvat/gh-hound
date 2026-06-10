//go:build !unix

package main

import "os"

// resizeSignals is a no-op on platforms without SIGWINCH; the TUI
// keeps its launch-time geometry there.
func resizeSignals() (<-chan os.Signal, func()) {
	return nil, func() {}
}

//go:build unix

package main

import (
	"os"
	"os/signal"
	"syscall"
)

// resizeSignals delivers SIGWINCH so the TUI can re-read the terminal
// size; the stop func releases the signal registration.
func resizeSignals() (<-chan os.Signal, func()) {
	events := make(chan os.Signal, 1)
	signal.Notify(events, syscall.SIGWINCH)
	return events, func() { signal.Stop(events) }
}

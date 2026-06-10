//go:build unix

package main

import (
	"syscall"
	"testing"
	"time"
)

func TestResizeSignalsDeliverSIGWINCH(t *testing.T) {
	events, stop := resizeSignals()
	defer stop()
	if events == nil {
		t.Fatal("resizeSignals must return a live channel on unix")
	}
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGWINCH); err != nil {
		t.Fatal(err)
	}
	select {
	case <-events:
	case <-time.After(2 * time.Second):
		t.Fatal("SIGWINCH was not delivered to the resize channel")
	}
}

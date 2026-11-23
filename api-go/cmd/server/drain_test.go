package main

import (
	"os"
	"syscall"
	"testing"
	"time"
)

// The drain delay keeps the server up for SHUTDOWN_DRAIN_DELAY_SECS; 0 does
// not wait, and a second stop signal ends the wait early.
func TestWaitDrainDelay(t *testing.T) {
	quit := make(chan os.Signal, 1)

	start := time.Now()
	waitDrainDelay(0, quit)
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Errorf("delay 0 waited %v", d)
	}

	start = time.Now()
	waitDrainDelay(80*time.Millisecond, quit)
	if d := time.Since(start); d < 80*time.Millisecond {
		t.Errorf("delay 80ms returned after %v", d)
	}

	quit <- syscall.SIGTERM
	start = time.Now()
	waitDrainDelay(time.Minute, quit)
	if d := time.Since(start); d > time.Second {
		t.Errorf("second signal: waited %v", d)
	}
}

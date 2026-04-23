//go:build windows

package displayapp

import (
	"os"
	"testing"
)

func TestShouldStopNonBlocking(t *testing.T) {
	t.Run("empty channels", func(t *testing.T) {
		interrupt := make(chan os.Signal, 1)
		exit := make(chan struct{})

		if shouldStop(interrupt, exit) {
			t.Fatal("expected no stop when both channels are empty")
		}
	})

	t.Run("queued interrupt", func(t *testing.T) {
		interrupt := make(chan os.Signal, 1)
		exit := make(chan struct{})

		interrupt <- os.Interrupt
		if !shouldStop(interrupt, exit) {
			t.Fatal("expected stop when interrupt is queued")
		}
	})

	t.Run("closed exit", func(t *testing.T) {
		interrupt := make(chan os.Signal, 1)
		exit := make(chan struct{})

		close(exit)
		if !shouldStop(interrupt, exit) {
			t.Fatal("expected stop when exit channel is closed")
		}
	})
}

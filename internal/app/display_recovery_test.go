package app

import (
	"fmt"
	"io"
	"syscall"
	"testing"
)

func TestIsRecoverableDisplayWriteError(t *testing.T) {
	t.Run("errno", func(t *testing.T) {
		if !isRecoverableDisplayWriteError(syscall.Errno(995)) {
			t.Fatal("expected ERROR_OPERATION_ABORTED to be recoverable")
		}
	})

	t.Run("wrapped errno", func(t *testing.T) {
		err := fmt.Errorf("write display payload: %w", syscall.Errno(995))
		if !isRecoverableDisplayWriteError(err) {
			t.Fatal("expected wrapped ERROR_OPERATION_ABORTED to be recoverable")
		}
	})

	t.Run("message text", func(t *testing.T) {
		if isRecoverableDisplayWriteError(io.EOF) {
			t.Fatal("expected unrelated error to be non-recoverable")
		}

		err := fmt.Errorf("write display payload: The I/O operation has been aborted because of either a thread exit or an application request.")
		if !isRecoverableDisplayWriteError(err) {
			t.Fatal("expected aborted I/O message to be recoverable")
		}
	})
}

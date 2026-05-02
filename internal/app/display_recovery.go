package app

import (
	"errors"
	"strings"
	"syscall"
)

const windowsOperationAbortedErrno = syscall.Errno(995)

func isRecoverableDisplayWriteError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, windowsOperationAbortedErrno) {
		return true
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "operation has been aborted")
}

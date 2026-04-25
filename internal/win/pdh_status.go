package win

import (
	"errors"
	"fmt"
)

const (
	pdhStatusNoMachine     = 0x800007D0
	pdhStatusNoInstance    = 0x800007D1
	pdhStatusRetry         = 0x800007D4
	pdhStatusNoData        = 0x800007D5
	pdhStatusInvalidData   = 0xC0000BBA
	pdhStatusInvalidHandle = 0xC0000BBC
)

// PdhError carries the PDH operation name and raw status code.
type PdhError struct {
	Op     string
	Status uintptr
}

func (e PdhError) Error() string {
	return fmt.Sprintf("%s: 0x%08X", e.Op, e.Status)
}

func (e PdhError) Retryable() bool {
	return IsRetryablePdhStatus(e.Status)
}

// IsRetryablePdhStatus reports whether the caller should try the operation
// again later or rebuild the query.
func IsRetryablePdhStatus(status uintptr) bool {
	switch status {
	case pdhStatusNoMachine, pdhStatusNoInstance, pdhStatusRetry, pdhStatusNoData, pdhStatusInvalidData:
		return true
	default:
		return false
	}
}

func IsRetryablePdhError(err error) bool {
	var pdhErr PdhError
	if errors.As(err, &pdhErr) {
		return pdhErr.Retryable()
	}
	return false
}

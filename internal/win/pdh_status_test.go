package win

import "testing"

func TestIsRetryablePdhStatus(t *testing.T) {
	tests := []struct {
		name   string
		status uintptr
		want   bool
	}{
		{name: "invalid data", status: pdhStatusInvalidData, want: true},
		{name: "no machine", status: pdhStatusNoMachine, want: true},
		{name: "no instance", status: pdhStatusNoInstance, want: true},
		{name: "retry", status: pdhStatusRetry, want: true},
		{name: "no data", status: pdhStatusNoData, want: true},
		{name: "invalid handle", status: pdhStatusInvalidHandle, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsRetryablePdhStatus(tc.status); got != tc.want {
				t.Fatalf("IsRetryablePdhStatus(0x%08X) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

func TestIsRetryablePdhError(t *testing.T) {
	err := PdhError{Op: "PdhGetFormattedCounterArrayW(data)", Status: pdhStatusInvalidData}
	if !IsRetryablePdhError(err) {
		t.Fatal("expected invalid-data PDH error to be retryable")
	}

	err = PdhError{Op: "PdhGetFormattedCounterArrayW(data)", Status: pdhStatusInvalidHandle}
	if IsRetryablePdhError(err) {
		t.Fatal("expected invalid-handle PDH error to be non-retryable")
	}
}

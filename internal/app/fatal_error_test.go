package app

import "testing"

func TestFormatFatalFailure(t *testing.T) {
	t.Run("no error", func(t *testing.T) {
		got := formatFatalFailure(nil, nil)
		if got != fatalFailureTitle {
			t.Fatalf("formatFatalFailure() = %q, want %q", got, fatalFailureTitle)
		}
	})

	t.Run("error", func(t *testing.T) {
		got := formatFatalFailure(errString("display unplugged"), nil)
		want := "The program could not continue because:\ndisplay unplugged"
		if got != want {
			t.Fatalf("formatFatalFailure() = %q, want %q", got, want)
		}
	})

	t.Run("panic", func(t *testing.T) {
		got := formatFatalFailure(nil, "boom")
		want := "The program crashed because of an unexpected panic:\nboom"
		if got != want {
			t.Fatalf("formatFatalFailure() = %q, want %q", got, want)
		}
	})
}

type errString string

func (e errString) Error() string { return string(e) }

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

func TestDebugStartupDialog(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		prev := debugEnabled
		debugEnabled = false
		defer func() { debugEnabled = prev }()

		title, message, ok := debugStartupDialog()
		if ok {
			t.Fatal("expected debug startup dialog to be disabled")
		}
		if title != "" || message != "" {
			t.Fatalf("debugStartupDialog() = (%q, %q, %v), want empty values", title, message, ok)
		}
	})

	t.Run("enabled", func(t *testing.T) {
		prev := debugEnabled
		debugEnabled = true
		defer func() { debugEnabled = prev }()

		title, message, ok := debugStartupDialog()
		if !ok {
			t.Fatal("expected debug startup dialog to be enabled")
		}
		if title != fatalFailureTitle {
			t.Fatalf("title = %q, want %q", title, fatalFailureTitle)
		}
		if message != debugStartupDialogMessage {
			t.Fatalf("message = %q, want %q", message, debugStartupDialogMessage)
		}
	})
}

type errString string

func (e errString) Error() string { return string(e) }

package app

import (
	"fmt"
	"strings"
)

const fatalFailureTitle = "Turing Display error"
const debugStartupDialogMessage = "In debug mode now"

func formatFatalFailure(err error, recovered any) string {
	if recovered != nil {
		return formatRecoveredPanic(recovered)
	}
	if err == nil {
		return fatalFailureTitle
	}
	return formatErrorChain(err)
}

func formatErrorChain(err error) string {
	var b strings.Builder
	b.WriteString("The program could not continue because:\n")
	b.WriteString(strings.TrimSpace(err.Error()))
	return b.String()
}

func formatRecoveredPanic(recovered any) string {
	var panicText string
	switch v := recovered.(type) {
	case error:
		panicText = v.Error()
	default:
		panicText = fmt.Sprint(v)
	}
	panicText = strings.TrimSpace(panicText)
	if panicText == "" {
		panicText = "unknown panic"
	}
	return "The program crashed because of an unexpected panic:\n" + panicText
}

func debugStartupDialog() (title, message string, ok bool) {
	if !debugEnabled {
		return "", "", false
	}
	return fatalFailureTitle, debugStartupDialogMessage, true
}

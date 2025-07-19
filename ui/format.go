package ui

import (
	"fmt"
	"strings"
)

// FormatAssistantResponse formats AI response: code blocks in bright green, text in bright white.
func FormatAssistantResponse(resp string) string {
	var b strings.Builder
	lines := strings.Split(resp, "\n")
	insideCode := false
	for _, line := range lines {
		if strings.HasPrefix(line, "```") {
			insideCode = !insideCode
			continue
		}
		if insideCode {
			b.WriteString(BrightGreen(line))
		} else {
			b.WriteString(BrightWhite(line))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// ShowIndexStatus displays index-related status messages in UI
func ShowIndexStatus(message string) {
	fmt.Println(Info("🔍 " + message))
}

// ShowIndexSuccess displays successful index operations
func ShowIndexSuccess(message string) {
	fmt.Println(Success("🔍 " + message))
}

// ShowIndexError displays index-related errors
func ShowIndexError(message string) {
	fmt.Println(Error("🔍 " + message))
}

// ShowIndexWarning displays index-related warnings
func ShowIndexWarning(message string) {
	fmt.Println(Warning("🔍 " + message))
}

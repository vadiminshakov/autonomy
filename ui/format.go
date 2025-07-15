package ui

import (
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

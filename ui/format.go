package ui

import (
	"fmt"
	"strings"
)

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

func ShowIndexStatus(message string) {
	fmt.Println(BrightBlue(message))
}

func ShowIndexSuccess(message string) {
	fmt.Println(BrightGreen(message))
}

func ShowIndexError(message string) {
	fmt.Println(BrightRed(message))
}

func ShowIndexWarning(message string) {
	fmt.Println(BrightYellow(message))
}

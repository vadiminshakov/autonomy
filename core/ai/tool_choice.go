package ai

import (
	"strings"
)

type ToolChoiceMode int

const (
	// ToolChoiceModeAny forces tool usage
	ToolChoiceModeAny ToolChoiceMode = iota
	// ToolChoiceModeAuto allows model to decide
	ToolChoiceModeAuto
)

// DetermineToolChoiceMode analyzes user message and determines tool choice mode
func DetermineToolChoiceMode(userMessage string) ToolChoiceMode {
	// convert to lowercase for case-insensitive matching
	lowerMsg := strings.ToLower(userMessage)

	if isToolResultMessage(lowerMsg) {
		return ToolChoiceModeAny
	}

	msgType := analyzeMessageType(lowerMsg)

	switch msgType {
	case "action":
		// force tool usage for action requests
		return ToolChoiceModeAny
	case "question":
		// let model decide for pure questions
		return ToolChoiceModeAuto
	default:
		// when uncertain, prefer tool usage
		return ToolChoiceModeAny
	}
}

// isToolResultMessage checks if message is a tool result
func isToolResultMessage(msg string) bool {
	// Structural patterns that indicate tool results
	patterns := []string{
		`^result of \w+:`,
		`^error executing \w+:`,
		`^✅ done \w+`,
		`^❌ error in \w+`,
		`tools used:`,
		`files created:`,
		`files modified:`,
		`files read:`,
		`commands executed:`,
	}

	for _, pattern := range patterns {
		if strings.Contains(msg, pattern) {
			return true
		}
	}

	// check for common result indicators
	resultIndicators := []string{
		"result of ",
		"error executing ",
		"task state summary",
		"✅ done",
		"❌ error",
		"successfully",
		"failed to",
		"completed",
	}

	for _, indicator := range resultIndicators {
		if strings.Contains(msg, indicator) {
			return true
		}
	}

	return false
}

// analyzeMessageType determines if message is an action or question
func analyzeMessageType(msg string) string {
	// action keywords that indicate a tool should be used
	actionKeywords := []string{
		// english verbs
		"read", "write", "create", "update", "delete", "execute", "run", "check",
		"analyze", "search", "find", "look", "show", "display", "get", "fetch",
		"test", "build", "compile", "fix", "modify", "edit", "rename", "move",
		"copy", "list", "view", "examine", "inspect", "review", "implement",
		"add", "remove", "install", "configure", "setup", "deploy", "debug",
		// russian verbs
		"прочитай", "читай", "посмотри", "покажи", "найди", "проверь", "запусти",
		"создай", "удали", "измени", "исправь", "проанализируй", "выполни",
		"протестируй", "собери", "скомпилируй", "переименуй", "скопируй",
		"перемести", "отобрази", "изучи", "рассмотри", "реализуй", "добавь",
		"установи", "настрой", "отладь",
		// imperative patterns
		"let's", "давай", "please", "пожалуйста", "can you", "could you",
		"i need", "мне нужно", "необходимо", "требуется", "нужно",
		"make", "do", "perform", "сделай", "выполни",
	}

	// question keywords that typically don't need tools
	questionKeywords := []string{
		"what is", "what are", "what does", "what do",
		"why is", "why are", "why does", "why do",
		"how is", "how are", "how does", "how do",
		"when is", "when are", "when does", "when do",
		"where is", "where are", "where does", "where do",
		"who is", "who are", "which is", "which are",
		"explain", "describe", "tell me about",
		// russian question patterns
		"что такое", "что это", "почему", "как работает",
		"когда", "где", "кто", "какой", "какая", "какие",
		"объясни", "расскажи", "опиши",
	}

	// check for question patterns first (more specific)
	for _, keyword := range questionKeywords {
		if strings.Contains(msg, keyword) {
			// but override if it also contains strong action words
			for _, action := range []string{"implement", "create", "write", "fix", "реализуй", "создай", "напиши", "исправь"} {
				if strings.Contains(msg, action) {
					return "action"
				}
			}

			return "question"
		}
	}

	for _, keyword := range actionKeywords {
		if strings.Contains(msg, keyword) {
			return "action"
		}
	}

	return "uncertain"
} 
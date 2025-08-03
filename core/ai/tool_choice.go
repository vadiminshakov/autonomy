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

	// for tool result messages, always force tool usage to continue the workflow
	if isToolResultMessage(lowerMsg) {
		return ToolChoiceModeAny
	}

	// for force/guidance messages, require tools
	if strings.Contains(lowerMsg, "you must use a tool") ||
	   strings.Contains(lowerMsg, "execute it now") ||
	   strings.Contains(lowerMsg, "providing gentle reminder") {
		return ToolChoiceModeAny
	}

	// if it's a pure informational question without action verbs, allow auto
	// otherwise, force tool usage to ensure progress
	if isPureQuestion(lowerMsg) {
		return ToolChoiceModeAuto
	}

	// default: force tool usage to ensure the agent makes progress
	return ToolChoiceModeAny
}

// isToolResultMessage checks if message is a tool result
func isToolResultMessage(msg string) bool {
	// structural patterns that indicate tool results
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

// isPureQuestion checks if the message is a pure informational question
func isPureQuestion(msg string) bool {
	// only consider it a pure question if it starts with question words
	// and doesn't contain action verbs
	questionStarters := []string{
		"what is", "what are", "what does", "what's",
		"why is", "why are", "why does", "why",
		"how is", "how are", "how does", "how",
		"when is", "when are", "when does", "when",
		"where is", "where are", "where does", "where",
		"who is", "who are", "which is", "which",
		"что такое", "что это", "почему", "как",
		"когда", "где", "кто", "какой",
	}

	hasQuestionStarter := false
	for _, starter := range questionStarters {
		if strings.HasPrefix(msg, starter) {
			hasQuestionStarter = true
			break
		}
	}

	if !hasQuestionStarter {
		return false
	}

	// check if it contains action verbs that would require tools
	actionVerbs := []string{
		"create", "write", "modify", "fix", "implement", "add", "remove",
		"run", "execute", "test", "build", "deploy", "install", "configure",
		"создай", "напиши", "измени", "исправь", "запусти", "установи",
	}

	for _, verb := range actionVerbs {
		if strings.Contains(msg, verb) {
			return false
		}
	}

	return true
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
package tools

import (
	"context"
	"fmt"
	"os"
	"strings"
)

func init() {
	Register("fix_validation_errors", fixValidationErrors)
}

func fixValidationErrors(args map[string]interface{}) (string, error) {
	filePath, ok := args["file"].(string)
	if !ok {
		if filePath, ok = args["file_path"].(string); !ok {
			return "", fmt.Errorf("parameter 'file' or 'file_path' must be provided")
		}
	}

	validationErrors, ok := args["errors"].([]interface{})
	if !ok {
		return "", fmt.Errorf("parameter 'errors' must be provided as array")
	}

	var errorList []string
	for _, err := range validationErrors {
		if errStr, ok := err.(string); ok {
			errorList = append(errorList, errStr)
		}
	}

	if len(errorList) == 0 {
		return "No validation errors to fix", nil
	}

	currentContent, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %v", filePath, err)
	}

	fixedContent, err := requestLLMToFixErrors(filePath, string(currentContent), errorList)
	if err != nil {
		return "", fmt.Errorf("failed to fix errors with LLM: %v", err)
	}

	if err := os.WriteFile(filePath, []byte(fixedContent), 0600); err != nil {
		return "", fmt.Errorf("failed to write fixed content: %v", err)
	}

	engine := getValidationEngine()
	ctx := context.Background()
	results := engine.ValidateFile(ctx, filePath)

	if len(results) == 0 {
		return fmt.Sprintf("File %s fixed successfully (no validators applied)", filePath), nil
	}

	stillHasErrors := false
	for _, result := range results {
		if !result.Success {
			stillHasErrors = true
			break
		}
	}

	if stillHasErrors {
		formatted := FormatValidationResults(map[string][]*ValidationResult{filePath: results})
		return fmt.Sprintf("File %s partially fixed, remaining issues:\n%s", filePath, formatted), nil
	}

	return fmt.Sprintf("File %s fixed successfully, all validation errors resolved", filePath), nil
}

func requestLLMToFixErrors(filePath, content string, errors []string) (string, error) {
	prompt := buildFixErrorsPrompt(filePath, content, errors)

	args := map[string]interface{}{
		"file":        filePath,
		"old_content": content,
		"prompt":      prompt,
		"task":        "Fix validation errors in this file",
	}

	_, err := Execute("apply_diff", args)
	if err != nil {
		return "", fmt.Errorf("LLM fix request failed: %v", err)
	}

	updatedContent, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read updated file: %v", err)
	}

	return string(updatedContent), nil
}

func buildFixErrorsPrompt(filePath, content string, errors []string) string {
	var prompt strings.Builder

	prompt.WriteString("Fix the following validation errors in this code file:\n\n")
	prompt.WriteString(fmt.Sprintf("File: %s\n\n", filePath))

	prompt.WriteString("Validation Errors:\n")
	for i, err := range errors {
		prompt.WriteString(fmt.Sprintf("%d. %s\n", i+1, err))
	}

	prompt.WriteString("\nCurrent Code:\n```")
	if strings.HasSuffix(filePath, ".go") {
		prompt.WriteString("go")
	} else if strings.HasSuffix(filePath, ".js") {
		prompt.WriteString("javascript")
	} else if strings.HasSuffix(filePath, ".py") {
		prompt.WriteString("python")
	}
	prompt.WriteString("\n")
	prompt.WriteString(content)
	prompt.WriteString("\n```\n\n")

	prompt.WriteString("Please provide the corrected code that fixes all validation errors. ")
	prompt.WriteString("Return ONLY the corrected code without explanations or markdown formatting. ")
	prompt.WriteString("Preserve the original functionality and structure as much as possible.\n")

	return prompt.String()
}

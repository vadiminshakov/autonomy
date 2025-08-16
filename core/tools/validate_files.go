package tools

import (
	"context"
	"fmt"
	"time"
)

func init() {
	Register("validate_files", validateFiles)
	Register("validate_modified_files", validateModifiedFiles)
}

var defaultValidationEngine *FileValidationEngine

func getValidationEngine() *FileValidationEngine {
	if defaultValidationEngine == nil {
		config := ValidationConfig{
			EnableCompilation: true,
			EnableLinting:     true,
			EnableTesting:     false,
			SkipOnErrors:      false,
			Timeout:           30 * time.Second,
		}
		defaultValidationEngine = NewFileValidationEngine(config)
	}
	return defaultValidationEngine
}

func validateFiles(args map[string]interface{}) (string, error) {
	files, ok := args["files"].([]interface{})
	if !ok {
		if filePath, ok := args["file"].(string); ok {
			files = []interface{}{filePath}
		} else {
			return "", fmt.Errorf("parameter 'files' or 'file' must be provided")
		}
	}

	engine := getValidationEngine()
	ctx := context.Background()

	results := make(map[string][]*ValidationResult)

	for _, fileInterface := range files {
		filePath, ok := fileInterface.(string)
		if !ok {
			continue
		}

		fileResults := engine.ValidateFile(ctx, filePath)
		if len(fileResults) > 0 {
			results[filePath] = fileResults
		}
	}

	if len(results) == 0 {
		return "No validation performed (no applicable validators found)", nil
	}

	formatted := FormatValidationResults(results)

	state := getTaskState()
	if hasErrors(results) {
		state.mu.Lock()
		state.Errors = append(state.Errors, "File validation failed")
		state.mu.Unlock()
	}

	return formatted, nil
}

func validateModifiedFiles(args map[string]interface{}) (string, error) {
	engine := getValidationEngine()
	ctx := context.Background()

	results := engine.ValidateModifiedFiles(ctx)

	if len(results) == 0 {
		return "No recently modified files to validate", nil
	}

	formatted := FormatValidationResults(results)

	state := getTaskState()
	if hasErrors(results) {
		state.mu.Lock()
		state.Errors = append(state.Errors, "Modified file validation failed")
		state.mu.Unlock()
	}

	return formatted, nil
}

func hasErrors(results map[string][]*ValidationResult) bool {
	for _, fileResults := range results {
		for _, result := range fileResults {
			if !result.Success {
				return true
			}
		}
	}
	return false
}

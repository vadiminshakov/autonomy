package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

func init() {
	Register("validate_files", validateFiles)
	Register("validate_modified_files", validateModifiedFiles)
}

var defaultValidationEngine *FileValidationEngine

// getValidationEngine returns the default validation engine
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

// validateFiles validates specific files provided in args
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

	// format results for output
	if len(results) == 0 {
		return "No validation performed (no applicable validators found)", nil
	}

	formatted := FormatValidationResults(results)

	// record validation results in task state
	state := getTaskState()
	if hasErrors(results) {
		state.mu.Lock()
		state.Errors = append(state.Errors, "File validation failed")
		state.mu.Unlock()
	}

	return formatted, nil
}

// validateModifiedFiles validates all recently modified files
func validateModifiedFiles(args map[string]interface{}) (string, error) {
	engine := getValidationEngine()
	ctx := context.Background()

	results := engine.ValidateModifiedFiles(ctx)

	if len(results) == 0 {
		return "No recently modified files to validate", nil
	}

	formatted := FormatValidationResults(results)

	// record validation results in task state
	state := getTaskState()
	if hasErrors(results) {
		state.mu.Lock()
		state.Errors = append(state.Errors, "Modified file validation failed")
		state.mu.Unlock()
	}

	return formatted, nil
}

// hasErrors checks if any validation results contain errors
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

// autoValidateAfterFileChange automatically validates a file after it's been modified
func autoValidateAfterFileChange(filePath string) {
	// only validate if the file is in a language we can validate
	engine := getValidationEngine()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	results := engine.ValidateFile(ctx, filePath)
	if len(results) == 0 {
		return // No applicable validators
	}

	// log validation results
	resultsMap := map[string][]*ValidationResult{filePath: results}
	formatted := FormatValidationResults(resultsMap)

	fmt.Printf("🔍 Auto-validation after file change:\n%s\n", formatted)

	// if there are errors, try to fix them using LLM
	if hasErrors(resultsMap) {
		fmt.Printf("🔧 Attempting to fix validation errors with LLM...\n")

		// extract error messages
		var errorMessages []interface{}
		for _, fileResults := range resultsMap {
			for _, result := range fileResults {
				for _, err := range result.Errors {
					errorMessages = append(errorMessages, err)
				}
			}
		}

		// Try to fix errors using LLM
		fixArgs := map[string]interface{}{
			"file":   filePath,
			"errors": errorMessages,
		}

		fixResult, err := Execute("fix_validation_errors", fixArgs)
		if err != nil {
			fmt.Printf("❌ Failed to fix validation errors: %v\n", err)
		} else {
			fmt.Printf("✅ %s\n", fixResult)
		}

		// record any remaining errors in task state
		state := getTaskState()
		state.mu.Lock()
		state.Errors = append(state.Errors, fmt.Sprintf("Auto-validation failed for %s", filePath))
		state.mu.Unlock()
	}
}

// ValidationSummary provides a summary of validation results
type ValidationSummary struct {
	TotalFiles      int                            `json:"total_files"`
	FilesWithErrors int                            `json:"files_with_errors"`
	TotalErrors     int                            `json:"total_errors"`
	TotalWarnings   int                            `json:"total_warnings"`
	Results         map[string][]*ValidationResult `json:"results"`
}

// GetValidationSummary returns a structured summary of validation results
func GetValidationSummary(results map[string][]*ValidationResult) *ValidationSummary {
	summary := &ValidationSummary{
		TotalFiles:      len(results),
		FilesWithErrors: 0,
		TotalErrors:     0,
		TotalWarnings:   0,
		Results:         results,
	}

	for _, fileResults := range results {
		hasFileErrors := false
		for _, result := range fileResults {
			summary.TotalErrors += len(result.Errors)
			summary.TotalWarnings += len(result.Warnings)
			if !result.Success {
				hasFileErrors = true
			}
		}
		if hasFileErrors {
			summary.FilesWithErrors++
		}
	}

	return summary
}

// getValidationSummaryJSON returns validation summary as JSON
func getValidationSummaryJSON(args map[string]interface{}) (string, error) {
	engine := getValidationEngine()
	ctx := context.Background()

	results := engine.ValidateModifiedFiles(ctx)
	summary := GetValidationSummary(results)

	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal validation summary: %v", err)
	}

	return string(data), nil
}

func init() {
	Register("get_validation_summary", getValidationSummaryJSON)
}

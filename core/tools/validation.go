package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// FileValidator defines the interface for validating files after modification
type FileValidator interface {
	// Name returns the name of the validator
	Name() string

	// CanValidate returns true if this validator can handle the given file
	CanValidate(filePath string) bool

	// Validate performs validation on the file and returns any errors
	Validate(ctx context.Context, filePath string) *ValidationResult
}

// ValidationResult contains the result of file validation
type ValidationResult struct {
	Success       bool          `json:"success"`
	ValidatorName string        `json:"validator_name"`
	FilePath      string        `json:"file_path"`
	Errors        []string      `json:"errors,omitempty"`
	Warnings      []string      `json:"warnings,omitempty"`
	Duration      time.Duration `json:"duration"`
	Details       string        `json:"details,omitempty"`
}

// ValidationConfig controls validation behavior
type ValidationConfig struct {
	EnableCompilation bool          `json:"enable_compilation"`
	EnableLinting     bool          `json:"enable_linting"`
	EnableTesting     bool          `json:"enable_testing"`
	SkipOnErrors      bool          `json:"skip_on_errors"`
	Timeout           time.Duration `json:"timeout"`
}

// FileValidationEngine manages file validation
type FileValidationEngine struct {
	validators []FileValidator
	config     ValidationConfig
}

// NewFileValidationEngine creates a new validation engine
func NewFileValidationEngine(config ValidationConfig) *FileValidationEngine {
	engine := &FileValidationEngine{
		validators: make([]FileValidator, 0),
		config:     config,
	}

	// Register default validators
	engine.RegisterValidator(&GoValidator{})
	engine.RegisterValidator(&JavaScriptValidator{})
	engine.RegisterValidator(&PythonValidator{})
	engine.RegisterValidator(&TypeScriptValidator{})

	return engine
}

// RegisterValidator registers a new validator
func (fve *FileValidationEngine) RegisterValidator(validator FileValidator) {
	fve.validators = append(fve.validators, validator)
}

// ValidateFile validates a single file using all applicable validators
func (fve *FileValidationEngine) ValidateFile(ctx context.Context, filePath string) []*ValidationResult {
	var results []*ValidationResult

	// Set timeout context
	if fve.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, fve.config.Timeout)
		defer cancel()
	}

	for _, validator := range fve.validators {
		if validator.CanValidate(filePath) {
			result := validator.Validate(ctx, filePath)
			results = append(results, result)

			// Skip remaining validators if there are errors and SkipOnErrors is true
			if fve.config.SkipOnErrors && !result.Success {
				break
			}
		}
	}

	return results
}

// ValidateModifiedFiles validates all recently modified files
func (fve *FileValidationEngine) ValidateModifiedFiles(ctx context.Context) map[string][]*ValidationResult {
	state := getTaskState()
	state.mu.RLock()
	modifiedFiles := make([]string, len(state.ModifiedFiles))
	copy(modifiedFiles, state.ModifiedFiles)
	state.mu.RUnlock()

	results := make(map[string][]*ValidationResult)

	for _, filePath := range modifiedFiles {
		fileResults := fve.ValidateFile(ctx, filePath)
		if len(fileResults) > 0 {
			results[filePath] = fileResults
		}
	}

	return results
}

// GetFileExtension returns the file extension without the dot
func GetFileExtension(filePath string) string {
	ext := filepath.Ext(filePath)
	if len(ext) > 0 {
		return strings.ToLower(ext[1:]) // remove the dot
	}
	return ""
}

// IsCompiledLanguage returns true if the file is in a compiled language
func IsCompiledLanguage(filePath string) bool {
	ext := GetFileExtension(filePath)
	compiledExts := []string{"go", "java", "c", "cpp", "cc", "cxx", "cs", "rs", "kt"}

	for _, compiledExt := range compiledExts {
		if ext == compiledExt {
			return true
		}
	}
	return false
}

// FormatValidationResults formats validation results for display
func FormatValidationResults(results map[string][]*ValidationResult) string {
	if len(results) == 0 {
		return "No validation results"
	}

	var output strings.Builder
	output.WriteString("File Validation Results:\n")

	totalFiles := 0
	totalErrors := 0
	totalWarnings := 0

	for filePath, fileResults := range results {
		totalFiles++
		output.WriteString(fmt.Sprintf("\n📁 %s:\n", filePath))

		for _, result := range fileResults {
			if result.Success {
				output.WriteString(fmt.Sprintf("  ✅ %s (%.2fs)\n", result.ValidatorName, result.Duration.Seconds()))
			} else {
				output.WriteString(fmt.Sprintf("  ❌ %s (%.2fs)\n", result.ValidatorName, result.Duration.Seconds()))
			}

			for _, err := range result.Errors {
				output.WriteString(fmt.Sprintf("    🔴 %s\n", err))
				totalErrors++
			}

			for _, warning := range result.Warnings {
				output.WriteString(fmt.Sprintf("    🟡 %s\n", warning))
				totalWarnings++
			}

			if result.Details != "" {
				output.WriteString(fmt.Sprintf("    ℹ️  %s\n", result.Details))
			}
		}
	}

	output.WriteString(fmt.Sprintf("\n📊 Summary: %d files, %d errors, %d warnings\n",
		totalFiles, totalErrors, totalWarnings))

	return output.String()
}

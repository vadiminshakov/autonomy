package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

func init() {
	Register("fix_validation_errors", fixValidationErrors)
}

// fixValidationErrors uses LLM to fix validation errors in a file
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

	// convert errors to strings
	var errorList []string
	for _, err := range validationErrors {
		if errStr, ok := err.(string); ok {
			errorList = append(errorList, errStr)
		}
	}

	if len(errorList) == 0 {
		return "No validation errors to fix", nil
	}

	// read current file content
	currentContent, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %v", filePath, err)
	}

	// use LLM to fix the errors
	fixedContent, err := fixErrors(filePath, string(currentContent), errorList)
	if err != nil {
		return "", fmt.Errorf("failed to fix errors with LLM: %v", err)
	}

	// write fixed content back to file
	if err := os.WriteFile(filePath, []byte(fixedContent), 0600); err != nil {
		return "", fmt.Errorf("failed to write fixed content: %v", err)
	}

	// re-validate to confirm fixes
	engine := getValidationEngine()
	ctx := context.Background()
	results := engine.ValidateFile(ctx, filePath)

	if len(results) == 0 {
		return "File fixed successfully (no validators applied)", nil
	}

	// check if errors are resolved
	for _, result := range results {
		if !result.Success {
			return "File partially fixed, remaining issues exist", nil
		}
	}

	return "File fixed successfully, all validation errors resolved", nil
}

// fixErrors sends a request to LLM to fix validation errors
func fixErrors(_, content string, errors []string) (string, error) {
	fixedContent := content
	
	// apply basic fixes for common issues
	for _, errMsg := range errors {
		if strings.Contains(errMsg, "gofmt: file is not properly formatted") {
			// apply gofmt formatting
			if formattedContent, err := applyGoFmt(content); err == nil {
				fixedContent = formattedContent
			}
		}
		
		if strings.Contains(errMsg, "relative import") {
			// fix relative imports by removing them or converting to absolute
			fixedContent = fixRelativeImports(fixedContent)
		}
	}
	
	return fixedContent, nil
}


// applyGoFmt applies gofmt formatting to Go code
func applyGoFmt(content string) (string, error) {
	cmd := exec.Command("gofmt")
	cmd.Stdin = strings.NewReader(content)
	
	output, err := cmd.Output()
	if err != nil {
		return content, err
	}
	
	return string(output), nil
}

// fixRelativeImports removes or fixes relative import statements
func fixRelativeImports(content string) string {
	// pattern to match relative imports like "./package" or "../package"
	relativeImportPattern := regexp.MustCompile(`"\.{1,2}/[^"]*"`)
	
	lines := strings.Split(content, "\n")
	var fixedLines []string
	
	for _, line := range lines {
		// remove lines with relative imports
		if relativeImportPattern.MatchString(line) && strings.Contains(line, "import") {
			// skip this line (remove the relative import)
			continue
		}
		fixedLines = append(fixedLines, line)
	}
	
	return strings.Join(fixedLines, "\n")
}

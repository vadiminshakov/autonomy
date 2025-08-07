package tools

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

func init() {
	Register("diagnostics", GetDiagnostics)
}

// GetDiagnostics runs basic language diagnostics on files
func GetDiagnostics(args map[string]interface{}) (string, error) {
	// get optional file_path parameter
	var filePath string
	if pathVal, ok := args["file_path"]; ok {
		if pathStr, ok := pathVal.(string); ok {
			filePath = pathStr
		}
	}

	if filePath == "" {
		// run diagnostics on current directory
		return runProjectDiagnostics(".")
	}

	// run diagnostics on specific file
	return runFileDiagnostics(filePath)
}

// runFileDiagnostics runs diagnostics on a specific file
func runFileDiagnostics(filePath string) (string, error) {
	cleanPath := filepath.Clean(filePath)
	ext := strings.ToLower(filepath.Ext(cleanPath))
	
	var results []string
	
	switch ext {
	case ".go":
		result, err := runGoDiagnostics(cleanPath)
		if err != nil {
			results = append(results, fmt.Sprintf("Go diagnostics error: %v", err))
		} else if result != "" {
			results = append(results, result)
		}
		
	case ".js", ".jsx", ".ts", ".tsx":
		result, err := runJavaScriptDiagnostics(cleanPath)
		if err != nil {
			results = append(results, fmt.Sprintf("JavaScript/TypeScript diagnostics error: %v", err))
		} else if result != "" {
			results = append(results, result)
		}
		
	case ".py":
		result, err := runPythonDiagnostics(cleanPath)
		if err != nil {
			results = append(results, fmt.Sprintf("Python diagnostics error: %v", err))
		} else if result != "" {
			results = append(results, result)
		}
		
	default:
		return "", fmt.Errorf("diagnostics not supported for file type: %s", ext)
	}
	
	if len(results) == 0 {
		return fmt.Sprintf("No diagnostic issues found in %s", cleanPath), nil
	}
	
	output := fmt.Sprintf("Diagnostics for %s:\n%s", cleanPath, strings.Join(results, "\n"))
	
	// create structured response
	metadata := &DiagnosticsMetadata{
		FilePath:    cleanPath,
		FileType:    ext,
		IssueCount:  len(results),
		HasIssues:   len(results) > 0,
	}
	
	return CreateStructuredResponse(output, metadata), nil
}

// runProjectDiagnostics runs diagnostics on the entire project
func runProjectDiagnostics(projectPath string) (string, error) {
	var results []string
	
	// try Go project diagnostics
	if goResult, err := runGoProjectDiagnostics(projectPath); err == nil && goResult != "" {
		results = append(results, "Go Project Diagnostics:\n"+goResult)
	}
	
	// try JavaScript/TypeScript project diagnostics
	if jsResult, err := runJavaScriptProjectDiagnostics(projectPath); err == nil && jsResult != "" {
		results = append(results, "JavaScript/TypeScript Project Diagnostics:\n"+jsResult)
	}
	
	// try Python project diagnostics
	if pyResult, err := runPythonProjectDiagnostics(projectPath); err == nil && pyResult != "" {
		results = append(results, "Python Project Diagnostics:\n"+pyResult)
	}
	
	if len(results) == 0 {
		return "No diagnostic tools found or no issues detected in project", nil
	}
	
	output := strings.Join(results, "\n\n")
	
	// create structured response
	metadata := &DiagnosticsMetadata{
		FilePath:    projectPath,
		FileType:    "project",
		IssueCount:  len(results),
		HasIssues:   len(results) > 0,
	}
	
	return CreateStructuredResponse(output, metadata), nil
}

// runGoDiagnostics runs Go-specific diagnostics
func runGoDiagnostics(filePath string) (string, error) {
	// try go vet
	cmd := exec.Command("go", "vet", filePath)
	output, err := cmd.CombinedOutput()
	if err != nil && len(output) > 0 {
		return fmt.Sprintf("go vet issues:\n%s", string(output)), nil
	}
	
	// try go fmt check
	cmd = exec.Command("gofmt", "-l", filePath)
	output, err = cmd.CombinedOutput()
	if err == nil && len(output) > 0 {
		return fmt.Sprintf("go fmt issues: file needs formatting"), nil
	}
	
	return "", nil
}

// runGoProjectDiagnostics runs Go project-wide diagnostics
func runGoProjectDiagnostics(projectPath string) (string, error) {
	var results []string
	
	// try go vet ./...
	cmd := exec.Command("go", "vet", "./...")
	cmd.Dir = projectPath
	output, err := cmd.CombinedOutput()
	if err != nil && len(output) > 0 {
		results = append(results, fmt.Sprintf("go vet issues:\n%s", string(output)))
	}
	
	// try go mod tidy check
	cmd = exec.Command("go", "mod", "tidy", "-v")
	cmd.Dir = projectPath
	output, err = cmd.CombinedOutput()
	if err != nil && len(output) > 0 {
		results = append(results, fmt.Sprintf("go mod issues:\n%s", string(output)))
	}
	
	return strings.Join(results, "\n"), nil
}

// runJavaScriptDiagnostics runs JavaScript/TypeScript-specific diagnostics
func runJavaScriptDiagnostics(filePath string) (string, error) {
	var results []string
	
	// try eslint if available
	cmd := exec.Command("eslint", filePath)
	output, err := cmd.CombinedOutput()
	if err == nil || len(output) > 0 {
		results = append(results, fmt.Sprintf("ESLint issues:\n%s", string(output)))
	}
	
	// try TypeScript compiler if .ts/.tsx file
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == ".ts" || ext == ".tsx" {
		cmd = exec.Command("tsc", "--noEmit", filePath)
		output, err = cmd.CombinedOutput()
		if err != nil && len(output) > 0 {
			results = append(results, fmt.Sprintf("TypeScript compiler issues:\n%s", string(output)))
		}
	}
	
	return strings.Join(results, "\n"), nil
}

// runJavaScriptProjectDiagnostics runs JavaScript/TypeScript project-wide diagnostics
func runJavaScriptProjectDiagnostics(projectPath string) (string, error) {
	var results []string
	
	// try eslint on project
	cmd := exec.Command("eslint", ".")
	cmd.Dir = projectPath
	output, err := cmd.CombinedOutput()
	if err == nil || len(output) > 0 {
		results = append(results, fmt.Sprintf("ESLint project issues:\n%s", string(output)))
	}
	
	// try TypeScript project check
	cmd = exec.Command("tsc", "--noEmit")
	cmd.Dir = projectPath
	output, err = cmd.CombinedOutput()
	if err != nil && len(output) > 0 {
		results = append(results, fmt.Sprintf("TypeScript project issues:\n%s", string(output)))
	}
	
	return strings.Join(results, "\n"), nil
}

// runPythonDiagnostics runs Python-specific diagnostics
func runPythonDiagnostics(filePath string) (string, error) {
	var results []string
	
	// try pylint if available
	cmd := exec.Command("pylint", filePath)
	output, err := cmd.CombinedOutput()
	if err == nil || len(output) > 0 {
		results = append(results, fmt.Sprintf("Pylint issues:\n%s", string(output)))
	}
	
	// try flake8 if available
	cmd = exec.Command("flake8", filePath)
	output, err = cmd.CombinedOutput()
	if err == nil || len(output) > 0 {
		results = append(results, fmt.Sprintf("Flake8 issues:\n%s", string(output)))
	}
	
	return strings.Join(results, "\n"), nil
}

// runPythonProjectDiagnostics runs Python project-wide diagnostics
func runPythonProjectDiagnostics(projectPath string) (string, error) {
	var results []string
	
	// try pylint on project
	cmd := exec.Command("pylint", ".")
	cmd.Dir = projectPath
	output, err := cmd.CombinedOutput()
	if err == nil || len(output) > 0 {
		results = append(results, fmt.Sprintf("Pylint project issues:\n%s", string(output)))
	}
	
	return strings.Join(results, "\n"), nil
}

// DiagnosticsMetadata contains metadata for diagnostics operations
type DiagnosticsMetadata struct {
	FilePath   string `json:"file_path"`
	FileType   string `json:"file_type"`
	IssueCount int    `json:"issue_count"`
	HasIssues  bool   `json:"has_issues"`
}
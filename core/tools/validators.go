package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// validationCheck represents a single validation operation
type validationCheck struct {
	name    string
	checkFn func(ctx context.Context, filePath string) error
	isError bool // true for errors, false for warnings
}

// runValidationChecks executes a series of validation checks and returns the result
func runValidationChecks(validatorName, filePath string, checks []validationCheck, ctx context.Context) *ValidationResult {
	start := time.Now()
	result := &ValidationResult{
		ValidatorName: validatorName,
		FilePath:      filePath,
		Success:       true,
		Errors:        []string{},
		Warnings:      []string{},
	}

	for _, check := range checks {
		if err := check.checkFn(ctx, filePath); err != nil {
			if check.isError {
				result.Success = false
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", check.name, err))
			} else {
				result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %v", check.name, err))
			}
		}
	}

	result.Duration = time.Since(start)
	return result
}

// GoValidator validates Go files
type GoValidator struct{}

func (gv *GoValidator) Name() string {
	return "Go Validator"
}

func (gv *GoValidator) CanValidate(filePath string) bool {
	return GetFileExtension(filePath) == "go"
}

func (gv *GoValidator) Validate(ctx context.Context, filePath string) *ValidationResult {
	start := time.Now()
	result := &ValidationResult{
		ValidatorName: gv.Name(),
		FilePath:      filePath,
		Success:       true,
		Errors:        []string{},
		Warnings:      []string{},
	}

	// run go fmt check
	if err := gv.checkGoFmt(ctx, filePath); err != nil {
		result.Success = false
		result.Errors = append(result.Errors, fmt.Sprintf("gofmt: %v", err))
	}

	// run go vet check
	if err := gv.checkGoVet(ctx, filePath); err != nil {
		result.Success = false
		result.Errors = append(result.Errors, fmt.Sprintf("go vet: %v", err))
	}

	// try to compile
	if err := gv.checkCompilation(ctx, filePath); err != nil {
		result.Success = false
		result.Errors = append(result.Errors, fmt.Sprintf("compilation: %v", err))
	}

	result.Duration = time.Since(start)
	return result
}

func (gv *GoValidator) checkGoFmt(ctx context.Context, filePath string) error {
	cmd := exec.CommandContext(ctx, "gofmt", "-l", filePath)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gofmt failed: %v", err)
	}

	// if gofmt outputs the filename, it means the file is not formatted
	if strings.TrimSpace(stdout.String()) != "" {
		return fmt.Errorf("file is not properly formatted")
	}

	return nil
}

func (gv *GoValidator) checkGoVet(ctx context.Context, filePath string) error {
	// run go vet on the directory containing the file
	dir := filepath.Dir(filePath)
	cmd := exec.CommandContext(ctx, "go", "vet", dir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(stderr.String()))
	}

	return nil
}

func (gv *GoValidator) checkCompilation(ctx context.Context, filePath string) error {
	// try to build the package containing the file
	dir := filepath.Dir(filePath)
	cmd := exec.CommandContext(ctx, "go", "build", "-o", "/dev/null", dir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(stderr.String()))
	}

	return nil
}

// JavaScriptValidator validates JavaScript files
type JavaScriptValidator struct{}

func (jsv *JavaScriptValidator) Name() string {
	return "JavaScript Validator"
}

func (jsv *JavaScriptValidator) CanValidate(filePath string) bool {
	ext := GetFileExtension(filePath)
	return ext == "js" || ext == "jsx"
}

func (jsv *JavaScriptValidator) Validate(ctx context.Context, filePath string) *ValidationResult {
	checks := []validationCheck{
		{"syntax", jsv.checkNodeSyntax, true},
		{"eslint", jsv.checkESLint, false}, // ESLint errors are warnings since it might not be configured
	}
	return runValidationChecks(jsv.Name(), filePath, checks, ctx)
}

func (jsv *JavaScriptValidator) checkNodeSyntax(ctx context.Context, filePath string) error {
	cmd := exec.CommandContext(ctx, "node", "-c", filePath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(stderr.String()))
	}

	return nil
}

func (jsv *JavaScriptValidator) checkESLint(ctx context.Context, filePath string) error {
	cmd := exec.CommandContext(ctx, "eslint", filePath)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		output := strings.TrimSpace(stdout.String())
		if output != "" {
			return fmt.Errorf("%s", output)
		}
		return fmt.Errorf("eslint check failed")
	}

	return nil
}

// TypeScriptValidator validates TypeScript files
type TypeScriptValidator struct{}

func (tsv *TypeScriptValidator) Name() string {
	return "TypeScript Validator"
}

func (tsv *TypeScriptValidator) CanValidate(filePath string) bool {
	ext := GetFileExtension(filePath)
	return ext == "ts" || ext == "tsx"
}

func (tsv *TypeScriptValidator) Validate(ctx context.Context, filePath string) *ValidationResult {
	start := time.Now()
	result := &ValidationResult{
		ValidatorName: tsv.Name(),
		FilePath:      filePath,
		Success:       true,
		Errors:        []string{},
		Warnings:      []string{},
	}

	// try TypeScript compilation
	if err := tsv.checkTypeScriptCompilation(ctx, filePath); err != nil {
		result.Success = false
		result.Errors = append(result.Errors, fmt.Sprintf("tsc: %v", err))
	}

	result.Duration = time.Since(start)
	return result
}

func (tsv *TypeScriptValidator) checkTypeScriptCompilation(ctx context.Context, filePath string) error {
	cmd := exec.CommandContext(ctx, "tsc", "--noEmit", filePath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(stderr.String()))
	}

	return nil
}

// PythonValidator validates Python files
type PythonValidator struct{}

func (pv *PythonValidator) Name() string {
	return "Python Validator"
}

func (pv *PythonValidator) CanValidate(filePath string) bool {
	return GetFileExtension(filePath) == "py"
}

func (pv *PythonValidator) Validate(ctx context.Context, filePath string) *ValidationResult {
	checks := []validationCheck{
		{"syntax", pv.checkPythonSyntax, true},
		{"flake8", pv.checkFlake8, false}, // Flake8 errors are warnings since it might not be configured
	}
	return runValidationChecks(pv.Name(), filePath, checks, ctx)
}

func (pv *PythonValidator) checkPythonSyntax(ctx context.Context, filePath string) error {
	cmd := exec.CommandContext(ctx, "python3", "-m", "py_compile", filePath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(stderr.String()))
	}

	return nil
}

func (pv *PythonValidator) checkFlake8(ctx context.Context, filePath string) error {
	cmd := exec.CommandContext(ctx, "flake8", filePath)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		output := strings.TrimSpace(stdout.String())
		if output != "" {
			return fmt.Errorf("%s", output)
		}
		return fmt.Errorf("flake8 check failed")
	}

	return nil
}

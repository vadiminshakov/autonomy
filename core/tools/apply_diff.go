package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/vadiminshakov/autonomy/ui"
)

func init() {
	Register("apply_diff", applyDiff)
}

// applyDiff applies a unified diff to files in the working directory.
// args["diff"] contains the patch content
// args["path"] or args["file"] is the relative file path
func applyDiff(args map[string]interface{}) (string, error) {
	diffVal, ok := args["diff"].(string)
	if !ok || strings.TrimSpace(diffVal) == "" {
		return "", fmt.Errorf("parameter 'diff' must be a non-empty string")
	}

	// determine target file path
	var pathVal string
	if v, ok := args["path"].(string); ok && strings.TrimSpace(v) != "" {
		pathVal = strings.TrimSpace(v)
	} else if v, ok := args["file"].(string); ok && strings.TrimSpace(v) != "" {
		pathVal = strings.TrimSpace(v)
	} else {
		return "", fmt.Errorf("either 'path' or 'file' parameter must be provided")
	}

	// validate file exists
	if _, err := os.Stat(pathVal); os.IsNotExist(err) {
		return "", fmt.Errorf("target file does not exist: %s", pathVal)
	}

	// validate and normalize diff format
	normalizedDiff, err := validateAndNormalizeDiff(diffVal, pathVal)
	if err != nil {
		return "", fmt.Errorf("invalid diff format: %v", err)
	}

	// log what we're applying
	logDiffApplication(pathVal, normalizedDiff)

	// create temporary file for the patch
	tmpFile, err := os.CreateTemp("", "patch-*.diff")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary diff file: %v", err)
	}
	defer func() {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
	}()

	if _, err := tmpFile.WriteString(normalizedDiff); err != nil {
		return "", fmt.Errorf("failed to write diff to temporary file: %v", err)
	}
	tmpFile.Close()

	// apply the patch with comprehensive error handling
	result, err := applyPatchWithValidation(tmpFile.Name(), pathVal)
	if err != nil {
		return "", err
	}

	return result, nil
}

// validateAndNormalizeDiff ensures the diff is in proper unified format
func validateAndNormalizeDiff(diff, filePath string) (string, error) {
	diff = strings.TrimSpace(diff)

	// check if diff already has proper headers
	hasHeaders := strings.Contains(diff, "--- ") && strings.Contains(diff, "+++ ")

	if !hasHeaders {
		// add unified diff headers if missing
		if !strings.Contains(diff, "@@") {
			return "", fmt.Errorf("diff must contain hunk headers (@@)")
		}

		header := fmt.Sprintf("--- %s\n+++ %s\n", filePath, filePath)
		diff = header + diff
	}

	// validate hunk headers format
	hunkRegex := regexp.MustCompile(`@@\s*-(\d+)(?:,(\d+))?\s*\+(\d+)(?:,(\d+))?\s*@@`)
	if !hunkRegex.MatchString(diff) {
		return "", fmt.Errorf("invalid hunk header format")
	}

	// validate diff lines format
	lines := strings.Split(diff, "\n")
	inHunk := false
	for i, line := range lines {
		if strings.HasPrefix(line, "@@") {
			inHunk = true
			continue
		}
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") {
			continue
		}
		if inHunk && line != "" {
			if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "-") {
				return "", fmt.Errorf("invalid diff line format at line %d: %s", i+1, line)
			}
		}
	}

	return diff, nil
}

// logDiffApplication logs information about what diff is being applied
func logDiffApplication(filePath, diff string) {
	const maxDiffLength = 200

	// count changes
	addedLines := 0
	removedLines := 0
	lines := strings.Split(diff, "\n")

	for _, line := range lines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			addedLines++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			removedLines++
		}
	}

	// prepare diff preview
	diffPreview := diff
	if len(diff) > maxDiffLength {
		diffPreview = diff[:maxDiffLength] + "..."
	}

	// replace newlines with visual indicators for logging
	diffPreview = strings.ReplaceAll(diffPreview, "\n", "\\n")

	fmt.Printf("%s Applying diff to %s\n", ui.Tool("DIFF"), ui.BrightWhite(filePath))
	fmt.Printf("%s Changes: %s added, %s removed\n",
		ui.Info("STATS"),
		ui.BrightGreen(fmt.Sprintf("+%d", addedLines)),
		ui.BrightRed(fmt.Sprintf("-%d", removedLines)))
	fmt.Printf("%s Preview: %s\n", ui.Info("PREVIEW"), ui.Dim(diffPreview))
}

// applyPatchWithValidation applies the patch with comprehensive validation
func applyPatchWithValidation(patchFile, targetFile string) (string, error) {
	// create backup before applying
	backupFile := targetFile + ".backup." + fmt.Sprintf("%d", time.Now().Unix())
	if err := copyFile(targetFile, backupFile); err != nil {
		return "", fmt.Errorf("failed to create backup: %v", err)
	}
	defer os.Remove(backupFile) // clean up backup on success

	// apply patch with timeout and detailed error handling
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "patch", "-u", "-p0", "-i", patchFile, "--no-backup-if-mismatch", "--batch", "--verbose")
	cmd.Dir = "./"

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	if ctx.Err() == context.DeadlineExceeded {
		// restore from backup on timeout
		restoreFile(backupFile, targetFile)
		return "", fmt.Errorf("patch command timed out after 30 seconds")
	}

	if err != nil {
		// restore from backup on failure
		restoreFile(backupFile, targetFile)

		stderrStr := stderr.String()
		stdoutStr := stdout.String()

		// provide detailed error information
		errorMsg := fmt.Sprintf("failed to apply patch: %v", err)
		if stderrStr != "" {
			errorMsg += fmt.Sprintf("\nStderr: %s", stderrStr)
		}
		if stdoutStr != "" {
			errorMsg += fmt.Sprintf("\nStdout: %s", stdoutStr)
		}

		// check for common patch errors and provide helpful suggestions
		if strings.Contains(stderrStr, "malformed patch") {
			errorMsg += "\nSuggestion: Check diff format - ensure proper unified diff format with @@ headers"
		} else if strings.Contains(stderrStr, "can't find file") {
			errorMsg += "\nSuggestion: Verify file path is correct and file exists"
		} else if strings.Contains(stderrStr, "Hunk") && strings.Contains(stderrStr, "FAILED") {
			errorMsg += "\nSuggestion: The diff may not match current file content - check line numbers and context"
		}

		return "", fmt.Errorf("%s", errorMsg)
	}

	// validate that the file was actually modified
	if err := validatePatchApplication(targetFile, backupFile); err != nil {
		restoreFile(backupFile, targetFile)
		return "", fmt.Errorf("patch validation failed: %v", err)
	}

	fmt.Printf("%s Diff applied successfully to %s\n", ui.Success("SUCCESS"), ui.BrightWhite(targetFile))

	getTaskState().RecordFileModified(targetFile)

	return fmt.Sprintf("diff applied successfully to %s", targetFile), nil
}

// copyFile creates a copy of the source file
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	// determine file permissions - preserve existing or use safe defaults
	var fileMode os.FileMode = 0600 // safe default for new files
	if info, err := os.Stat(dst); err == nil {
		fileMode = info.Mode().Perm() // preserve existing permissions
	} else if srcInfo, err := os.Stat(src); err == nil {
		fileMode = srcInfo.Mode().Perm() // copy permissions from source
	}

	return os.WriteFile(dst, data, fileMode)
}

// restoreFile restores a file from backup
func restoreFile(backupFile, targetFile string) {
	if err := copyFile(backupFile, targetFile); err != nil {
		fmt.Printf("%s Failed to restore file from backup: %v\n", ui.Error("ERROR"), err)
	}
}

// validatePatchApplication checks if the patch was actually applied
func validatePatchApplication(targetFile, backupFile string) error {
	originalContent, err := os.ReadFile(backupFile)
	if err != nil {
		return fmt.Errorf("failed to read backup file: %v", err)
	}

	modifiedContent, err := os.ReadFile(targetFile)
	if err != nil {
		return fmt.Errorf("failed to read modified file: %v", err)
	}

	// check if files are different (patch should have changed something)
	if bytes.Equal(originalContent, modifiedContent) {
		return fmt.Errorf("file content unchanged after patch application")
	}

	// basic syntax validation for common file types
	if err := validateFileSyntax(targetFile); err != nil {
		return fmt.Errorf("syntax validation failed: %v", err)
	}

	return nil
}

// validateFileSyntax performs basic syntax validation for common file types
func validateFileSyntax(filePath string) error {
	if strings.HasSuffix(filePath, ".go") {
		return validateGoSyntax(filePath)
	}
	// add more file type validations as needed
	return nil
}

// validateGoSyntax checks if Go file has valid syntax
func validateGoSyntax(filePath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gofmt", "-l", filePath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("syntax check timed out")
		}
		return fmt.Errorf("go fmt failed: %v, stderr: %s", err, stderr.String())
	}

	return nil
}

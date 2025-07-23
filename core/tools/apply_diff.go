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

	// normalize line endings to Unix style
	diff = strings.ReplaceAll(diff, "\r\n", "\n")
	diff = strings.ReplaceAll(diff, "\r", "\n")

	// check if diff already has proper headers
	hasHeaders := strings.Contains(diff, "--- ") && strings.Contains(diff, "+++ ")

	if !hasHeaders {
		// add unified diff headers if missing
		if !strings.Contains(diff, "@@") {
			return "", fmt.Errorf("diff must contain hunk headers. Example: '@@ -1,3 +1,4 @@' to show changes starting at line 1")
		}

		header := fmt.Sprintf("--- %s\n+++ %s\n", filePath, filePath)
		diff = header + diff
	}

	// improve hunk header validation and auto-fix some common issues
	lines := strings.Split(diff, "\n")
	var fixedLines []string
	inHunk := false

	hunkRegex := regexp.MustCompile(`@@\s*-(\d+)(?:,(\d+))?\s*\+(\d+)(?:,(\d+))?\s*@@`)

	for i, line := range lines {
		if strings.HasPrefix(line, "@@") {
			// validate and potentially fix hunk header
			if !hunkRegex.MatchString(line) {
				// try to auto-fix common hunk header issues
				fixedLine := fixHunkHeader(line)
				if fixedLine != "" && hunkRegex.MatchString(fixedLine) {
					fmt.Printf("%s Auto-fixed hunk header: '%s' -> '%s'\n",
						ui.Warning("FIX"), line, fixedLine)
					line = fixedLine
				} else {
					return "", fmt.Errorf("invalid hunk header format at line %d: '%s'. Required format: '@@ -startLine,count +startLine,count @@' (example: '@@ -1,3 +1,4 @@')", i+1, line)
				}
			}
			inHunk = true
			fixedLines = append(fixedLines, line)
			continue
		}

		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") {
			fixedLines = append(fixedLines, line)
			continue
		}

		if inHunk && line != "" {
			// auto-fix common line prefix issues
			if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "-") {
				// try to auto-fix: if line looks like it should be context, add space prefix
				if !strings.Contains(line, "{{") && !strings.Contains(line, "}}") &&
					!strings.HasPrefix(line, "\\") { // ignore "\ No newline at end of file"
					fixedLine := " " + line
					fmt.Printf("%s Auto-fixed line format: added space prefix to line %d\n",
						ui.Warning("FIX"), i+1)
					line = fixedLine
				} else {
					return "", fmt.Errorf("invalid diff line format at line %d: '%s'. Each line must start with space ' ' (context), '+' (added), or '-' (removed)", i+1, line)
				}
			}
		}

		fixedLines = append(fixedLines, line)
	}

	return strings.Join(fixedLines, "\n"), nil
}

// fixHunkHeader attempts to auto-fix common hunk header format issues
func fixHunkHeader(line string) string {
	// remove extra spaces and normalize format
	line = strings.TrimSpace(line)

	// handle @@ without proper spacing
	if strings.HasPrefix(line, "@@") && strings.HasSuffix(line, "@@") {
		inner := strings.TrimSpace(line[2 : len(line)-2])

		// handle cases like "@@-1,3+1,4@@" (no spaces)
		if strings.Contains(inner, "-") && strings.Contains(inner, "+") {
			// split by + to get old and new parts
			plusIndex := strings.Index(inner, "+")
			if plusIndex > 0 {
				oldPart := strings.TrimSpace(inner[:plusIndex])
				newPart := strings.TrimSpace(inner[plusIndex:])

				// ensure parts start with - and + respectively
				if !strings.HasPrefix(oldPart, "-") {
					oldPart = "-" + oldPart
				}
				if !strings.HasPrefix(newPart, "+") {
					newPart = "+" + newPart
				}

				return fmt.Sprintf("@@ %s %s @@", oldPart, newPart)
			}
		}

		// try to parse with spaces (handle extra whitespace)
		parts := strings.Fields(inner)
		if len(parts) >= 2 {
			// look for patterns like "-1,3 +1,4" or "-1 +1"
			var oldPart, newPart string
			for _, part := range parts {
				if strings.HasPrefix(part, "-") && oldPart == "" {
					oldPart = part
				} else if strings.HasPrefix(part, "+") && newPart == "" {
					newPart = part
				}
			}

			if oldPart != "" && newPart != "" {
				return fmt.Sprintf("@@ %s %s @@", oldPart, newPart)
			}
		}
	}

	return ""
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

	// try multiple patch strategies for better compatibility
	strategies := []patchStrategy{
		{
			name: "strict",
			args: []string{"-u", "-p0", "--no-backup-if-mismatch", "--reject-file=/dev/null", "--batch", "--verbose"},
		},
		{
			name: "with fuzz",
			args: []string{"-u", "-p0", "--fuzz=3", "--no-backup-if-mismatch", "--reject-file=/dev/null", "--batch", "--verbose"},
		},
		{
			name: "ignore whitespace",
			args: []string{"-u", "-p0", "-l", "--fuzz=3", "--no-backup-if-mismatch", "--reject-file=/dev/null", "--batch", "--verbose"},
		},
		{
			name: "p1 strip",
			args: []string{"-u", "-p1", "--fuzz=3", "--no-backup-if-mismatch", "--reject-file=/dev/null", "--batch", "--verbose"},
		},
	}

	var lastError error
	var lastStderr, lastStdout string

	for _, strategy := range strategies {
		// restore from backup before each attempt
		if err := copyFile(backupFile, targetFile); err != nil {
			return "", fmt.Errorf("failed to restore file from backup: %v", err)
		}

		fmt.Printf("%s Trying patch strategy: %s\n", ui.Info("PATCH"), strategy.name)

		success, stdout, stderr, err := attemptPatch(patchFile, targetFile, strategy.args)

		if success {
			// validate that the file was actually modified
			if err := validatePatchApplication(targetFile, backupFile); err != nil {
				fmt.Printf("%s Patch applied but validation failed: %v\n", ui.Warning("WARNING"), err)
				continue // try next strategy
			}

			fmt.Printf("%s Diff applied successfully to %s (using %s strategy)\n",
				ui.Success("SUCCESS"), ui.BrightWhite(targetFile), strategy.name)

			getTaskState().RecordFileModified(targetFile)
			return fmt.Sprintf("diff applied successfully to %s", targetFile), nil
		}

		// record last error for reporting
		lastError = err
		lastStderr = stderr
		lastStdout = stdout

		fmt.Printf("%s Strategy '%s' failed: %v\n", ui.Warning("WARNING"), strategy.name, err)
	}

	// all strategies failed, restore file and return error
	restoreFile(backupFile, targetFile)

	// provide detailed error information from the last attempt
	errorMsg := fmt.Sprintf("all patch strategies failed. Last error: %v", lastError)
	if lastStderr != "" {
		errorMsg += fmt.Sprintf("\nStderr: %s", lastStderr)
	}
	if lastStdout != "" {
		errorMsg += fmt.Sprintf("\nStdout: %s", lastStdout)
	}

	// check for common patch errors and provide helpful suggestions
	if strings.Contains(lastStderr, "malformed patch") {
		errorMsg += "\nSuggestion: Check diff format - ensure proper unified diff format with @@ headers"
	} else if strings.Contains(lastStderr, "can't find file") {
		errorMsg += "\nSuggestion: Verify file path is correct and file exists"
	} else if strings.Contains(lastStderr, "Hunk") && strings.Contains(lastStderr, "FAILED") {
		errorMsg += "\nSuggestion: The diff may not match current file content - check line numbers and context"
	} else if strings.Contains(lastStderr, "No such file") {
		errorMsg += "\nSuggestion: Check that the target file path is correct"
	}

	return "", fmt.Errorf("%s", errorMsg)
}

// patchStrategy defines a patch application strategy
type patchStrategy struct {
	name string
	args []string
}

// attemptPatch tries to apply patch with given arguments
func attemptPatch(patchFile, targetFile string, args []string) (success bool, stdout, stderr string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// build command with patch file input
	cmdArgs := append(args, "-i", patchFile, targetFile)
	cmd := exec.CommandContext(ctx, "patch", cmdArgs...)
	cmd.Dir = "./"

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err = cmd.Run()

	stdout = stdoutBuf.String()
	stderr = stderrBuf.String()

	if ctx.Err() == context.DeadlineExceeded {
		return false, stdout, stderr, fmt.Errorf("patch command timed out after 30 seconds")
	}

	if err != nil {
		return false, stdout, stderr, fmt.Errorf("patch command failed: %v", err)
	}

	return true, stdout, stderr, nil
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

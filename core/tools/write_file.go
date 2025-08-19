package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func init() {
	Register("write_file", WriteFile)
}

//nolint:gocyclo
func WriteFile(args map[string]interface{}) (string, error) {
	var pathVal string
	var ok bool
	if pathVal, ok = args["path"].(string); !ok {
		if pathVal, ok = args["file"].(string); !ok {
			if pathVal, ok = args["fileName"].(string); !ok {
				if pathVal, ok = args["file_path"].(string); !ok {
					return "", fmt.Errorf("parameter path, file, fileName or file_path must be a non-empty string")
				}
			}
		}
	}
	// support aliases: content, code, fileContent
	var contentVal string
	if contentVal, ok = args["content"].(string); !ok {
		if contentVal, ok = args["code"].(string); !ok {
			if contentVal, ok = args["fileContent"].(string); !ok {
				return "", fmt.Errorf("parameter content, code or fileContent must be a non-empty string")
			}
		}
	}

	// ensure content is not empty or whitespace
	if len(strings.TrimSpace(contentVal)) == 0 {
		return "", fmt.Errorf("parameter 'content' is empty – file will not be created")
	}

	// clean path to prevent directory traversal
	cleanPath := filepath.Clean(pathVal)

	// check if file exists with the same content
	if existingContent, err := os.ReadFile(cleanPath); err == nil {
		if string(existingContent) == contentVal {
			return fmt.Sprintf("file %s already exists with identical content (%d bytes). no changes made", pathVal, len(contentVal)), nil
		}
	}

	// create parent directory if it does not exist
	dir := filepath.Dir(cleanPath)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("failed to create directory %s: %v", dir, err)
		}
	}

	// determine file permissions - preserve existing or use safe defaults
	var fileMode os.FileMode = 0600 // safe default for new files
	if info, err := os.Stat(cleanPath); err == nil {
		fileMode = info.Mode().Perm() // preserve existing permissions
	}

	// write file atomically using temp file
	tempFile := cleanPath + ".tmp"
	if err := os.WriteFile(tempFile, []byte(contentVal), fileMode); err != nil {
		return "", fmt.Errorf("failed to write temp file %s: %v", tempFile, err)
	}

	// atomic rename
	if err := os.Rename(tempFile, cleanPath); err != nil {
		// cleanup temp file on error
		os.Remove(tempFile)
		return "", fmt.Errorf("failed to rename temp file to %s: %v", pathVal, err)
	}

	// validate code if it's a Go file
	if filepath.Ext(cleanPath) == ".go" {
		if err := validateGoFile(cleanPath); err != nil {
			return fmt.Sprintf("file %s written but has syntax errors: %v", pathVal, err), nil
		}
	}

	state := getTaskState()
	if _, err := os.Stat(cleanPath); err == nil {
		state.RecordFileModified(cleanPath)
	} else {
		state.RecordFileCreated(cleanPath)
	}

	return fmt.Sprintf("file %s successfully written (%d bytes)", pathVal, len(contentVal)), nil
}

// validateGoFile validates Go file syntax
func validateGoFile(filePath string) error {
	cmd := exec.Command("go", "build", "-o", "/dev/null", filePath)
	if output, err := cmd.CombinedOutput(); err != nil {
		// extract only the important part of the error
		errorMsg := string(output)
		if strings.Contains(errorMsg, "syntax error") {
			lines := strings.Split(errorMsg, "\n")
			for _, line := range lines {
				if strings.Contains(line, "syntax error") {
					return fmt.Errorf("syntax error in Go code: %s", strings.TrimSpace(line))
				}
			}
		}
		return fmt.Errorf("go build failed: %s", strings.TrimSpace(errorMsg))
	}
	return nil
}

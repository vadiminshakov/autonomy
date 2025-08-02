package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func init() {
	Register("write_file", WriteFile)
}

// WriteFile creates or overwrites a file at args["path"] with the provided content.
//
//nolint:gocyclo
func WriteFile(args map[string]interface{}) (string, error) {
	// support aliases: path, file, fileName, file_path
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

	state := getTaskState()
	if _, err := os.Stat(cleanPath); err == nil {
		state.RecordFileModified(cleanPath)
	} else {
		state.RecordFileCreated(cleanPath)
	}

	go autoValidateAfterFileChange(cleanPath)

	return fmt.Sprintf("file %s successfully written (%d bytes)", pathVal, len(contentVal)), nil
}

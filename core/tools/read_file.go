package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func init() {
	Register("read_file", ReadFile)
}

const (
	maxFileSize = 1024 * 1024
)

func ReadFile(args map[string]interface{}) (string, error) {
	pathVal, ok := args["path"].(string)
	if !ok {
		return "", fmt.Errorf("parameter 'path' must be a non-empty string")
	}

	if isSensitiveFile(pathVal) {
		return "", fmt.Errorf("reading file '%s' blocked for security reasons", pathVal)
	}

	cleanPath := filepath.Clean(pathVal)

	info, err := os.Stat(cleanPath)
	if err != nil {
		return "", fmt.Errorf("failed to get info for file %s: %v", pathVal, err)
	}

	if info.Size() > maxFileSize {
		return "", fmt.Errorf("file %s is too large (%d bytes), maximum %d bytes", pathVal, info.Size(), maxFileSize)
	}

	if info.IsDir() {
		return "", fmt.Errorf("path %s points to a directory, not a file", pathVal)
	}

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %v", pathVal, err)
	}

	state := getTaskState()
	state.RecordFileRead(cleanPath)

	return string(data), nil
}

func isSensitiveFile(path string) bool {
	path = strings.ToLower(filepath.Clean(path))

	sensitiveFiles := []string{
		"/etc/passwd", "/etc/shadow", "/etc/sudoers",
		"/home/.ssh/id_rsa", "/root/.ssh/id_rsa",
		"/.env", "/.env.local", "/.env.production",
		"/id_rsa", "/id_dsa", "/id_ecdsa", "/id_ed25519",
		"/private.key", "/server.key", "/cert.key",
	}

	for _, sensitive := range sensitiveFiles {
		if strings.Contains(path, sensitive) {
			return true
		}
	}

	if strings.Contains(path, "/.ssh/") || strings.Contains(path, "/.gnupg/") {
		return true
	}

	return false
}

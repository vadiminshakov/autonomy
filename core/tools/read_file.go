package tools

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

func init() {
	Register("view", ViewFile)
}

const (
	MaxReadSize      = 250 * 1024
	DefaultReadLimit = 2000
	MaxLineLength    = 2000
)

// ViewFile returns file contents for args["file_path"] with line numbers and structured metadata.
func ViewFile(args map[string]interface{}) (string, error) {
	pathVal, ok := args["file_path"].(string)
	if !ok {
		return "", fmt.Errorf("parameter 'file_path' must be a non-empty string")
	}
	
	// check for empty or whitespace-only path
	if strings.TrimSpace(pathVal) == "" {
		return "", fmt.Errorf("parameter 'file_path' must be a non-empty string (received empty or whitespace-only path)")
	}

	// get optional parameters
	offset := 0
	if offsetVal, ok := args["offset"]; ok {
		if offsetInt, ok := offsetVal.(float64); ok {
			offset = int(offsetInt)
		}
	}
	
	limit := DefaultReadLimit
	if limitVal, ok := args["limit"]; ok {
		if limitInt, ok := limitVal.(float64); ok {
			limit = int(limitInt)
		}
	}

	if isSensitiveFile(pathVal) {
		return "", fmt.Errorf("reading file '%s' blocked for security reasons", pathVal)
	}

	cleanPath := filepath.Clean(pathVal)

	info, err := os.Stat(cleanPath)
	if err != nil {
		return "", fmt.Errorf("failed to get info for file %s: %v", pathVal, err)
	}

	if info.Size() > MaxReadSize {
		return "", fmt.Errorf("file %s is too large (%d bytes), maximum %d bytes", pathVal, info.Size(), MaxReadSize)
	}

	if info.IsDir() {
		return "", fmt.Errorf("path %s points to a directory, not a file", pathVal)
	}

	// check if it's an image file
	isImage, imageType := isImageFile(cleanPath)
	if isImage {
		return "", fmt.Errorf("this is an image file of type: %s", imageType)
	}

	// read the file content with offset and limit
	content, totalLineCount, err := readTextFile(cleanPath, offset, limit)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %v", pathVal, err)
	}

	// validate UTF-8
	if !utf8.ValidString(content) {
		return "", fmt.Errorf("file content is not valid UTF-8")
	}

	state := getTaskState()
	state.RecordFileRead(cleanPath)

	// format output with line numbers
	output := "<file>\n"
	output += addLineNumbers(content, offset+1)
	
	// add truncation note if needed
	if totalLineCount > offset+len(strings.Split(content, "\n")) {
		output += fmt.Sprintf("\n\n(File has more lines. Use 'offset' parameter to read beyond line %d)",
			offset+len(strings.Split(content, "\n")))
	}
	output += "\n</file>\n"

	// create metadata for the file read operation
	lineCount := strings.Count(content, "\n") + 1
	if content == "" {
		lineCount = 0
	}
	
	metadata := &FileReadMetadata{
		FilePath:  cleanPath,
		FileSize:  info.Size(),
		LineCount: lineCount,
		Language:  GetLanguageFromExtension(cleanPath),
	}

	return CreateStructuredResponse(output, metadata), nil
}

// isSensitiveFile checks if a file is sensitive and should not be read
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

	// check for hidden files in sensitive directories
	if strings.Contains(path, "/.ssh/") || strings.Contains(path, "/.gnupg/") {
		return true
	}

	return false
}

// addLineNumbers adds line numbers to content starting from startLine
func addLineNumbers(content string, startLine int) string {
	if content == "" {
		return ""
	}

	lines := strings.Split(content, "\n")
	var result []string
	
	for i, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		lineNum := i + startLine
		numStr := fmt.Sprintf("%d", lineNum)
		
		if len(numStr) >= 6 {
			result = append(result, fmt.Sprintf("%s|%s", numStr, line))
		} else {
			paddedNum := fmt.Sprintf("%6s", numStr)
			result = append(result, fmt.Sprintf("%s|%s", paddedNum, line))
		}
	}
	
	return strings.Join(result, "\n")
}

// readTextFile reads a text file with offset and limit support
func readTextFile(filePath string, offset, limit int) (string, int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()

	lineCount := 0
	scanner := bufio.NewScanner(file)
	
	// skip to offset
	if offset > 0 {
		for lineCount < offset && scanner.Scan() {
			lineCount++
		}
		if err = scanner.Err(); err != nil {
			return "", 0, err
		}
	}

	// read limited lines
	lines := make([]string, 0, limit)
	lineCount = offset
	
	for scanner.Scan() && len(lines) < limit {
		lineCount++
		lineText := scanner.Text()
		if len(lineText) > MaxLineLength {
			lineText = lineText[:MaxLineLength] + "..."
		}
		lines = append(lines, lineText)
	}

	// continue scanning to get total line count
	for scanner.Scan() {
		lineCount++
	}

	if err := scanner.Err(); err != nil {
		return "", 0, err
	}

	return strings.Join(lines, "\n"), lineCount, nil
}

// isImageFile checks if the file is an image based on extension
func isImageFile(filePath string) (bool, string) {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".jpg", ".jpeg":
		return true, "JPEG"
	case ".png":
		return true, "PNG"
	case ".gif":
		return true, "GIF"
	case ".bmp":
		return true, "BMP"
	case ".svg":
		return true, "SVG"
	case ".webp":
		return true, "WebP"
	default:
		return false, ""
	}
}
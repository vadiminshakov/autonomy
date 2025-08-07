package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func init() {
	Register("multiedit", MultiEdit)
}

// MultiEditOperation represents a single edit operation
type MultiEditOperation struct {
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

// MultiEdit performs multiple find-and-replace operations on a single file atomically
func MultiEdit(args map[string]interface{}) (string, error) {
	// Get file_path parameter
	pathVal, ok := args["file_path"].(string)
	if !ok {
		return "", fmt.Errorf("parameter 'file_path' must be a non-empty string")
	}

	// Get edits parameter
	editsVal, ok := args["edits"]
	if !ok {
		return "", fmt.Errorf("parameter 'edits' is required")
	}

	// Parse edits
	editsSlice, ok := editsVal.([]interface{})
	if !ok {
		return "", fmt.Errorf("parameter 'edits' must be an array")
	}

	if len(editsSlice) == 0 {
		return "", fmt.Errorf("at least one edit operation is required")
	}

	// Parse edit operations
	var edits []MultiEditOperation
	for i, editVal := range editsSlice {
		editMap, ok := editVal.(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("edit %d must be an object", i+1)
		}

		oldString, ok := editMap["old_string"].(string)
		if !ok {
			return "", fmt.Errorf("edit %d: old_string is required and must be a string", i+1)
		}

		newString, ok := editMap["new_string"].(string)
		if !ok {
			return "", fmt.Errorf("edit %d: new_string is required and must be a string", i+1)
		}

		replaceAll := false
		if replaceAllVal, exists := editMap["replace_all"]; exists {
			if replaceAllBool, ok := replaceAllVal.(bool); ok {
				replaceAll = replaceAllBool
			}
		}

		edits = append(edits, MultiEditOperation{
			OldString:  oldString,
			NewString:  newString,
			ReplaceAll: replaceAll,
		})
	}

	// Validate edits
	if err := validateEdits(edits); err != nil {
		return "", err
	}

	cleanPath := filepath.Clean(pathVal)

	// Handle file creation case (first edit has empty old_string)
	if len(edits) > 0 && edits[0].OldString == "" {
		return processMultiEditWithCreation(cleanPath, edits)
	}

	return processMultiEditExistingFile(cleanPath, edits)
}

// validateEdits validates edit operations
func validateEdits(edits []MultiEditOperation) error {
	for i, edit := range edits {
		if edit.OldString == edit.NewString {
			return fmt.Errorf("edit %d: old_string and new_string are identical", i+1)
		}
		// Only the first edit can have empty old_string (for file creation)
		if i > 0 && edit.OldString == "" {
			return fmt.Errorf("edit %d: only the first edit can have empty old_string (for file creation)", i+1)
		}
	}
	return nil
}

// processMultiEditWithCreation handles file creation with multiple edits
func processMultiEditWithCreation(filePath string, edits []MultiEditOperation) (string, error) {
	// First edit creates the file
	firstEdit := edits[0]
	if firstEdit.OldString != "" {
		return "", fmt.Errorf("first edit must have empty old_string for file creation")
	}

	// Check if file already exists
	if _, err := os.Stat(filePath); err == nil {
		return "", fmt.Errorf("file already exists: %s", filePath)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("failed to access file: %v", err)
	}

	// Create parent directories
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create parent directories: %v", err)
	}

	// Start with the content from the first edit
	currentContent := firstEdit.NewString

	// Apply remaining edits to the content
	for i := 1; i < len(edits); i++ {
		edit := edits[i]
		newContent, err := applyEditToContent(currentContent, edit)
		if err != nil {
			return "", fmt.Errorf("edit %d failed: %s", i+1, err.Error())
		}
		currentContent = newContent
	}

	// Write the file
	if err := os.WriteFile(filePath, []byte(currentContent), 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %v", err)
	}

	// Record operations
	state := getTaskState()
	state.RecordFileCreated(filePath)

	// Generate metadata
	additions := strings.Count(currentContent, "\n") + 1
	metadata := &MultiEditMetadata{
		FilePath:     filePath,
		EditsApplied: len(edits),
		Additions:    additions,
		Removals:     0,
		OldContent:   "",
		NewContent:   currentContent,
	}

	result := fmt.Sprintf("<result>\nFile created with %d edits: %s\n</result>", len(edits), filePath)
	
	
	return CreateStructuredResponse(result, metadata), nil
}

// processMultiEditExistingFile handles editing existing files
func processMultiEditExistingFile(filePath string, edits []MultiEditOperation) (string, error) {
	// Validate file exists and is readable
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("file not found: %s", filePath)
		}
		return "", fmt.Errorf("failed to access file: %v", err)
	}

	if fileInfo.IsDir() {
		return "", fmt.Errorf("path is a directory, not a file: %s", filePath)
	}

	// Read current file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %v", err)
	}

	oldContent := string(content)
	currentContent := oldContent

	// Apply all edits sequentially
	for i, edit := range edits {
		newContent, err := applyEditToContent(currentContent, edit)
		if err != nil {
			return "", fmt.Errorf("edit %d failed: %s", i+1, err.Error())
		}
		currentContent = newContent
	}

	// Check if content actually changed
	if oldContent == currentContent {
		return "", fmt.Errorf("no changes made - all edits resulted in identical content")
	}

	// Write the updated content
	if err := os.WriteFile(filePath, []byte(currentContent), 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %v", err)
	}

	// Record operations
	state := getTaskState()
	state.RecordFileModified(filePath)

	// Generate metadata
	oldLines := strings.Count(oldContent, "\n") + 1
	newLines := strings.Count(currentContent, "\n") + 1
	
	metadata := &MultiEditMetadata{
		FilePath:     filePath,
		EditsApplied: len(edits),
		Additions:    newLines,
		Removals:     oldLines,
		OldContent:   oldContent,
		NewContent:   currentContent,
	}

	result := fmt.Sprintf("<result>\nApplied %d edits to file: %s\n</result>", len(edits), filePath)
	
	
	return CreateStructuredResponse(result, metadata), nil
}

// applyEditToContent applies a single edit operation to content
func applyEditToContent(content string, edit MultiEditOperation) (string, error) {
	if edit.OldString == "" && edit.NewString == "" {
		return content, nil
	}

	if edit.OldString == "" {
		return "", fmt.Errorf("old_string cannot be empty for content replacement")
	}

	var newContent string
	var replacementCount int

	if edit.ReplaceAll {
		newContent = strings.ReplaceAll(content, edit.OldString, edit.NewString)
		replacementCount = strings.Count(content, edit.OldString)
		if replacementCount == 0 {
			return "", fmt.Errorf("old_string not found in content. Make sure it matches exactly, including whitespace and line breaks")
		}
	} else {
		index := strings.Index(content, edit.OldString)
		if index == -1 {
			return "", fmt.Errorf("old_string not found in content. Make sure it matches exactly, including whitespace and line breaks")
		}

		lastIndex := strings.LastIndex(content, edit.OldString)
		if index != lastIndex {
			return "", fmt.Errorf("old_string appears multiple times in the content. Please provide more context to ensure a unique match, or set replace_all to true")
		}

		newContent = content[:index] + edit.NewString + content[index+len(edit.OldString):]
		replacementCount = 1
	}

	return newContent, nil
}

// MultiEditMetadata contains metadata for multiedit operations
type MultiEditMetadata struct {
	FilePath     string `json:"file_path"`
	EditsApplied int    `json:"edits_applied"`
	Additions    int    `json:"additions"`
	Removals     int    `json:"removals"`
	OldContent   string `json:"old_content,omitempty"`
	NewContent   string `json:"new_content,omitempty"`
}
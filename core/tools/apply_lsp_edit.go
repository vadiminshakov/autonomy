package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vadiminshakov/autonomy/ui"
)

func init() {
	Register("apply_lsp_edit", applyLSPEdit)
}

// applyLSPEdit applies code changes using Language Server Protocol
//
// Parameters:
// - file: target file path
// - edits: array of edit operations in LSP TextEdit format
// - position_based: if true, use exact line/column positions; if false, use content-based matching
func applyLSPEdit(args map[string]interface{}) (string, error) {
	filePath, ok := args["file"].(string)
	if !ok || strings.TrimSpace(filePath) == "" {
		return "", fmt.Errorf("parameter 'file' must be a non-empty string")
	}

	// validate file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return "", fmt.Errorf("target file does not exist: %s", filePath)
	}

	// parse edits
	editsData, ok := args["edits"]
	if !ok {
		return "", fmt.Errorf("parameter 'edits' is required")
	}

	var textEdits []TextEdit

	// handle different input formats
	switch v := editsData.(type) {
	case string:
		// JSON string format
		if err := json.Unmarshal([]byte(v), &textEdits); err != nil {
			return "", fmt.Errorf("failed to parse edits JSON: %v", err)
		}
	case []interface{}:
		// array format
		for i, editInterface := range v {
			editBytes, err := json.Marshal(editInterface)
			if err != nil {
				return "", fmt.Errorf("failed to marshal edit %d: %v", i, err)
			}

			var edit TextEdit
			if err := json.Unmarshal(editBytes, &edit); err != nil {
				return "", fmt.Errorf("failed to parse edit %d: %v", i, err)
			}

			textEdits = append(textEdits, edit)
		}
	default:
		return "", fmt.Errorf("edits must be a JSON string or array")
	}

	if len(textEdits) == 0 {
		return "", fmt.Errorf("no edits provided")
	}

	// check if position-based editing is enabled
	positionBased := false
	if pb, ok := args["position_based"].(bool); ok {
		positionBased = pb
	}

	// get working directory for lsp client
	workingDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %v", err)
	}

	// try lsp-based editing first
	if positionBased {
		result, err := applyWithLSP(filePath, textEdits, workingDir)
		if err == nil {
			return result, nil
		}
		fmt.Printf("%s LSP editing failed, falling back to manual application: %v\n",
			ui.Warning("LSP"), err)
	}

	// fallback to manual text editing
	return applyEditsManually(filePath, textEdits)
}

// applyWithLSP attempts to apply edits using a language server
func applyWithLSP(filePath string, edits []TextEdit, workingDir string) (string, error) {
	// get or create lsp client for this file
	client, err := GetLSPClient(filePath, workingDir)
	if err != nil {
		return "", fmt.Errorf("failed to get LSP client: %v", err)
	}

	// read file content to open document
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %v", err)
	}

	// open document in language server
	if err := client.OpenDocument(filePath, string(content)); err != nil {
		return "", fmt.Errorf("failed to open document: %v", err)
	}

	// convert file path to uri
	fileURI := "file://" + filepath.ToSlash(filePath)

	// create workspace edit
	workspaceEdit := WorkspaceEdit{
		Changes: map[string][]TextEdit{
			fileURI: edits,
		},
	}

	// apply the edit
	if err := client.ApplyEdit(workspaceEdit); err != nil {
		return "", fmt.Errorf("failed to apply LSP edit: %v", err)
	}

	fmt.Printf("%s Successfully applied %d edits to %s using LSP\n",
		ui.Success("LSP"), len(edits), ui.BrightWhite(filePath))

	getTaskState().RecordFileModified(filePath)
	go autoValidateAfterFileChange(filePath)

	return fmt.Sprintf("successfully applied %d LSP edits to %s", len(edits), filePath), nil
}

// applyEditsManually applies text edits without using LSP
func applyEditsManually(filePath string, edits []TextEdit) (string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %v", err)
	}

	lines := strings.Split(string(content), "\n")

	// sort edits by position (reverse order to avoid offset issues)
	sortedEdits := make([]TextEdit, len(edits))
	copy(sortedEdits, edits)

	// apply edits in reverse order (last to first) to avoid position shifting
	for i := len(sortedEdits) - 1; i >= 0; i-- {
		edit := sortedEdits[i]

		// validate positions
		if edit.Range.Start.Line < 0 || edit.Range.Start.Line >= len(lines) {
			return "", fmt.Errorf("invalid start line %d (file has %d lines)",
				edit.Range.Start.Line, len(lines))
		}

		if edit.Range.End.Line < 0 || edit.Range.End.Line >= len(lines) {
			return "", fmt.Errorf("invalid end line %d (file has %d lines)",
				edit.Range.End.Line, len(lines))
		}

		// apply single-line edits
		if edit.Range.Start.Line == edit.Range.End.Line {
			line := lines[edit.Range.Start.Line]

			if edit.Range.Start.Character < 0 || edit.Range.Start.Character > len(line) {
				return "", fmt.Errorf("invalid start character %d on line %d (line length: %d)",
					edit.Range.Start.Character, edit.Range.Start.Line, len(line))
			}

			if edit.Range.End.Character < 0 || edit.Range.End.Character > len(line) {
				return "", fmt.Errorf("invalid end character %d on line %d (line length: %d)",
					edit.Range.End.Character, edit.Range.End.Line, len(line))
			}

			// replace text in the line
			newLine := line[:edit.Range.Start.Character] +
				edit.NewText +
				line[edit.Range.End.Character:]

			lines[edit.Range.Start.Line] = newLine
		} else {
			// multi-line edits
			startLine := lines[edit.Range.Start.Line]
			endLine := lines[edit.Range.End.Line]

			// create new content
			newLine := startLine[:edit.Range.Start.Character] +
				edit.NewText +
				endLine[edit.Range.End.Character:]

				// replace the range of lines with the new content
			newLines := append(lines[:edit.Range.Start.Line], newLine)
			newLines = append(newLines, lines[edit.Range.End.Line+1:]...)
			lines = newLines
		}
	}

	// write the modified content back to file
	newContent := strings.Join(lines, "\n")
	if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %v", err)
	}

	fmt.Printf("%s Successfully applied %d manual edits to %s\n",
		ui.Success("EDIT"), len(edits), ui.BrightWhite(filePath))

	getTaskState().RecordFileModified(filePath)
	go autoValidateAfterFileChange(filePath)

	return fmt.Sprintf("successfully applied %d manual edits to %s", len(edits), filePath), nil
}

// createtextedit is a helper function to create textedit objects
func CreateTextEdit(startLine, startChar, endLine, endChar int, newText string) TextEdit {
	return TextEdit{
		Range: Range{
			Start: Position{Line: startLine, Character: startChar},
			End:   Position{Line: endLine, Character: endChar},
		},
		NewText: newText,
	}
}

// createsimplereplacement creates a simple text replacement edit
func CreateSimpleReplacement(line int, startChar, endChar int, newText string) TextEdit {
	return CreateTextEdit(line, startChar, line, endChar, newText)
}

// createlinereplacement replaces an entire line
func CreateLineReplacement(line int, newText string) TextEdit {
	return TextEdit{
		Range: Range{
			Start: Position{Line: line, Character: 0},
			End:   Position{Line: line + 1, Character: 0},
		},
		NewText: newText + "\n",
	}
}

// createinsertatposition inserts text at a specific position
func CreateInsertAtPosition(line, character int, text string) TextEdit {
	return TextEdit{
		Range: Range{
			Start: Position{Line: line, Character: character},
			End:   Position{Line: line, Character: character},
		},
		NewText: text,
	}
}

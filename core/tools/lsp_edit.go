package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/vadiminshakov/autonomy/ui"
)

func init() {
	Register("lsp_edit", lspEdit)
}

// EditRequest represents a single edit operation
type EditRequest struct {
	StartLine   int    `json:"start_line"`
	EndLine     int    `json:"end_line"`
	NewText     string `json:"new_text"`
	Description string `json:"description,omitempty"`
}

//nolint:gocyclo
func lspEdit(args map[string]interface{}) (string, error) {
	pathVal, ok := args["path"].(string)
	if !ok || strings.TrimSpace(pathVal) == "" {
		return "", fmt.Errorf("parameter 'path' must be a non-empty string")
	}

	pathVal = strings.TrimSpace(pathVal)

	// check that the file exists
	if _, err := os.Stat(pathVal); os.IsNotExist(err) {
		return "", fmt.Errorf("file does not exist: %s", pathVal)
	}

	// read file content
	content, err := os.ReadFile(pathVal)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %v", err)
	}

	lines := strings.Split(string(content), "\n")

	var edits []EditRequest

	// try to get edits as JSON string or array
	if editsVal, ok := args["edits"]; ok {
		switch v := editsVal.(type) {
		case string:
			// if JSON string is passed
			if err := json.Unmarshal([]byte(v), &edits); err != nil {
				return "", fmt.Errorf("invalid edits JSON: %v", err)
			}
		case []interface{}:
			// if array of objects is passed
			for _, editVal := range v {
				if editMap, ok := editVal.(map[string]interface{}); ok {
					edit, err := parseEditRequest(editMap)
					if err != nil {
						return "", fmt.Errorf("invalid edit request: %v", err)
					}
					edits = append(edits, edit)
				} else {
					return "", fmt.Errorf("each edit must be an object")
				}
			}
		default:
			return "", fmt.Errorf("edits must be a JSON string or array of objects")
		}
	} else {
		// fallback: create one edit from old parameters for compatibility
		startLine := 1
		endLine := len(lines)
		newText := ""

		if sl, ok := args["start_line"]; ok {
			if slInt, ok := sl.(float64); ok {
				startLine = int(slInt)
			} else if slStr, ok := sl.(string); ok {
				if parsed, err := strconv.Atoi(slStr); err == nil {
					startLine = parsed
				}
			}
		}

		if el, ok := args["end_line"]; ok {
			if elInt, ok := el.(float64); ok {
				endLine = int(elInt)
			} else if elStr, ok := el.(string); ok {
				if parsed, err := strconv.Atoi(elStr); err == nil {
					endLine = parsed
				}
			}
		}

		if nt, ok := args["new_text"].(string); ok {
			newText = nt
		}

		edits = []EditRequest{{
			StartLine: startLine,
			EndLine:   endLine,
			NewText:   newText,
		}}
	}

	if len(edits) == 0 {
		return "", fmt.Errorf("no edits provided")
	}

	// apply edits in reverse order (top to bottom) to avoid messing up line numbers
	fmt.Printf("%s Applying %d edit(s) to %s\n", ui.Tool("LSP_EDIT"), len(edits), ui.BrightWhite(pathVal))

	// sort edits by line numbers in reverse order
	sortedEdits := make([]EditRequest, len(edits))
	copy(sortedEdits, edits)

	// simple sort by start line (descending)
	for i := 0; i < len(sortedEdits)-1; i++ {
		for j := i + 1; j < len(sortedEdits); j++ {
			if sortedEdits[i].StartLine < sortedEdits[j].StartLine {
				sortedEdits[i], sortedEdits[j] = sortedEdits[j], sortedEdits[i]
			}
		}
	}

	// create backup
	backupPath := pathVal + ".backup." + fmt.Sprintf("%d", time.Now().Unix())
	if err := os.WriteFile(backupPath, content, 0600); err != nil {
		return "", fmt.Errorf("failed to create backup: %v", err)
	}
	defer os.Remove(backupPath)

	// apply edits
	modifiedLines := lines
	totalChanges := 0

	for i, edit := range sortedEdits {
		if err := validateEdit(edit, len(modifiedLines)); err != nil {
			return "", fmt.Errorf("invalid edit #%d: %v", i+1, err)
		}

		// log the change
		logEdit(edit, i+1, len(sortedEdits))

		// apply the edit
		newLines, changes := applyEdit(modifiedLines, edit)
		modifiedLines = newLines
		totalChanges += changes
	}

	// write the result
	newContent := strings.Join(modifiedLines, "\n")
	if err := os.WriteFile(pathVal, []byte(newContent), 0600); err != nil {
		// restore from backup on error
		if rerr := os.WriteFile(pathVal, content, 0600); rerr != nil {
			return "", fmt.Errorf("failed to write modified file: %v; also failed to restore backup: %v", err, rerr)
		}
		return "", fmt.Errorf("failed to write modified file: %v", err)
	}

	// validate syntax if this is a Go file
	if strings.HasSuffix(pathVal, ".go") {
		if err := validateGoSyntax(pathVal); err != nil {
			// restore from backup on syntax error
			if rerr := os.WriteFile(pathVal, content, 0600); rerr != nil {
				return "", fmt.Errorf("syntax validation failed: %v; also failed to restore backup: %v", err, rerr)
			}
			return "", fmt.Errorf("syntax validation failed: %v", err)
		}
	}

	getTaskState().RecordFileModified(pathVal)

	result := fmt.Sprintf("applied %d edit(s) to %s (%d lines changed)", len(edits), pathVal, totalChanges)
	fmt.Printf("%s %s\n", ui.Success("SUCCESS"), result)

	return result, nil
}

// parseEditRequest parses edit object from map
func parseEditRequest(editMap map[string]interface{}) (EditRequest, error) {
	var edit EditRequest

	if sl, ok := editMap["start_line"]; ok {
		switch v := sl.(type) {
		case float64:
			edit.StartLine = int(v)
		case int:
			edit.StartLine = v
		case string:
			if parsed, err := strconv.Atoi(v); err == nil {
				edit.StartLine = parsed
			} else {
				return edit, fmt.Errorf("invalid start_line: %v", v)
			}
		default:
			return edit, fmt.Errorf("start_line must be a number")
		}
	} else {
		return edit, fmt.Errorf("start_line is required")
	}

	if el, ok := editMap["end_line"]; ok {
		switch v := el.(type) {
		case float64:
			edit.EndLine = int(v)
		case int:
			edit.EndLine = v
		case string:
			if parsed, err := strconv.Atoi(v); err == nil {
				edit.EndLine = parsed
			} else {
				return edit, fmt.Errorf("invalid end_line: %v", v)
			}
		default:
			return edit, fmt.Errorf("end_line must be a number")
		}
	} else {
		return edit, fmt.Errorf("end_line is required")
	}

	if nt, ok := editMap["new_text"].(string); ok {
		edit.NewText = nt
	} // new_text is not required (can be empty for deletion)

	if desc, ok := editMap["description"].(string); ok {
		edit.Description = desc
	}

	return edit, nil
}

// validateEdit checks validity of edit operation
func validateEdit(edit EditRequest, totalLines int) error {
	if edit.StartLine < 1 {
		return fmt.Errorf("start_line must be >= 1, got %d", edit.StartLine)
	}

	if edit.EndLine < edit.StartLine {
		return fmt.Errorf("end_line (%d) must be >= start_line (%d)", edit.EndLine, edit.StartLine)
	}

	if edit.StartLine > totalLines+1 {
		return fmt.Errorf("start_line (%d) is beyond file length (%d lines)", edit.StartLine, totalLines)
	}

	return nil
}

// logEdit logs edit operation
func logEdit(edit EditRequest, editNum, totalEdits int) {
	desc := edit.Description
	if desc == "" {
		desc = fmt.Sprintf("lines %d-%d", edit.StartLine, edit.EndLine)
	}

	newLineCount := len(strings.Split(edit.NewText, "\n"))
	if edit.NewText == "" {
		newLineCount = 0
	}

	oldLineCount := edit.EndLine - edit.StartLine + 1
	delta := newLineCount - oldLineCount

	var deltaStr string
	if delta > 0 {
		deltaStr = ui.BrightGreen(fmt.Sprintf("+%d", delta))
	} else if delta < 0 {
		deltaStr = ui.BrightRed(fmt.Sprintf("%d", delta))
	} else {
		deltaStr = ui.Dim("0")
	}

	fmt.Printf("%s Edit %d/%d: %s (%s lines)\n",
		ui.Info("EDIT"), editNum, totalEdits, ui.BrightWhite(desc), deltaStr)
}

// applyEdit applies one edit operation to array of lines
func applyEdit(lines []string, edit EditRequest) ([]string, int) {
	startIdx := edit.StartLine - 1 // convert to 0-based index
	endIdx := edit.EndLine - 1

	// protect against going out of bounds
	if startIdx < 0 {
		startIdx = 0
	}
	if endIdx >= len(lines) {
		endIdx = len(lines) - 1
	}
	if startIdx > len(lines) {
		startIdx = len(lines)
	}

	// split new text into lines
	var newLines []string
	if edit.NewText != "" {
		newLines = strings.Split(edit.NewText, "\n")
	}

	// create new array of lines
	result := make([]string, 0, len(lines)+len(newLines))

	// add lines before edited area
	result = append(result, lines[:startIdx]...)

	// add new lines
	result = append(result, newLines...)

	// add lines after edited area
	if endIdx+1 < len(lines) {
		result = append(result, lines[endIdx+1:]...)
	}

	// count number of changes
	changes := 0
	if len(newLines) > 0 || endIdx >= startIdx {
		changes = 1 // count each edit as one change
	}

	return result, changes
}

// validateGoSyntax checks Go file syntax
func validateGoSyntax(filePath string) error {
	// use gofmt to check syntax
	cmd := exec.Command("gofmt", "-e", filePath)
	cmd.Dir = filepath.Dir(filePath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("syntax error: %v\nOutput: %s", err, string(output))
	}

	return nil
}

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

// lspEdit applies text edits to files using Language Server Protocol approach
// args["path"] - file path to edit
// args["edits"] - JSON array of edit operations
func lspEdit(args map[string]interface{}) (string, error) {
	pathVal, ok := args["path"].(string)
	if !ok || strings.TrimSpace(pathVal) == "" {
		return "", fmt.Errorf("parameter 'path' must be a non-empty string")
	}

	pathVal = strings.TrimSpace(pathVal)

	// проверяем что файл существует
	if _, err := os.Stat(pathVal); os.IsNotExist(err) {
		return "", fmt.Errorf("file does not exist: %s", pathVal)
	}

	// читаем содержимое файла
	content, err := os.ReadFile(pathVal)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %v", err)
	}

	lines := strings.Split(string(content), "\n")

	var edits []EditRequest

	// пробуем получить edits как JSON строку или массив
	if editsVal, ok := args["edits"]; ok {
		switch v := editsVal.(type) {
		case string:
			// если передана JSON строка
			if err := json.Unmarshal([]byte(v), &edits); err != nil {
				return "", fmt.Errorf("invalid edits JSON: %v", err)
			}
		case []interface{}:
			// если передан массив объектов
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
		// fallback: создаем один edit из старых параметров для совместимости
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

	// применяем редактирования в обратном порядке (сверху вниз) чтобы не сбивать номера строк
	fmt.Printf("%s Applying %d edit(s) to %s\n", ui.Tool("LSP_EDIT"), len(edits), ui.BrightWhite(pathVal))

	// сортируем edits по номерам строк в обратном порядке
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

	// создаем backup
	backupPath := pathVal + ".backup." + fmt.Sprintf("%d", time.Now().Unix())
	if err := os.WriteFile(backupPath, content, 0644); err != nil {
		return "", fmt.Errorf("failed to create backup: %v", err)
	}
	defer os.Remove(backupPath)

	// применяем редактирования
	modifiedLines := lines
	totalChanges := 0

	for i, edit := range sortedEdits {
		if err := validateEdit(edit, len(modifiedLines)); err != nil {
			return "", fmt.Errorf("invalid edit #%d: %v", i+1, err)
		}

		// логируем изменение
		logEdit(edit, i+1, len(sortedEdits))

		// применяем редактирование
		newLines, changes := applyEdit(modifiedLines, edit)
		modifiedLines = newLines
		totalChanges += changes
	}

	// записываем результат
	newContent := strings.Join(modifiedLines, "\n")
	if err := os.WriteFile(pathVal, []byte(newContent), 0644); err != nil {
		// восстанавливаем из backup при ошибке
		os.WriteFile(pathVal, content, 0644)
		return "", fmt.Errorf("failed to write modified file: %v", err)
	}

	// валидируем синтаксис если это Go файл
	if strings.HasSuffix(pathVal, ".go") {
		if err := validateGoSyntax(pathVal); err != nil {
			// восстанавливаем из backup при ошибке синтаксиса
			os.WriteFile(pathVal, content, 0644)
			return "", fmt.Errorf("syntax validation failed: %v", err)
		}
	}

	getTaskState().RecordFileModified(pathVal)

	result := fmt.Sprintf("applied %d edit(s) to %s (%d lines changed)", len(edits), pathVal, totalChanges)
	fmt.Printf("%s %s\n", ui.Success("SUCCESS"), result)

	return result, nil
}

// parseEditRequest парсит объект редактирования из map
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
	} // new_text не обязательный (может быть пустым для удаления)

	if desc, ok := editMap["description"].(string); ok {
		edit.Description = desc
	}

	return edit, nil
}

// validateEdit проверяет валидность операции редактирования
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

// logEdit логирует операцию редактирования
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

// applyEdit применяет одну операцию редактирования к массиву строк
func applyEdit(lines []string, edit EditRequest) ([]string, int) {
	startIdx := edit.StartLine - 1 // convert to 0-based index
	endIdx := edit.EndLine - 1

	// защищаемся от выхода за границы
	if startIdx < 0 {
		startIdx = 0
	}
	if endIdx >= len(lines) {
		endIdx = len(lines) - 1
	}
	if startIdx > len(lines) {
		startIdx = len(lines)
	}

	// разделяем новый текст на строки
	var newLines []string
	if edit.NewText != "" {
		newLines = strings.Split(edit.NewText, "\n")
	}

	// создаем новый массив строк
	result := make([]string, 0, len(lines)+len(newLines))

	// добавляем строки до редактируемой области
	result = append(result, lines[:startIdx]...)

	// добавляем новые строки
	result = append(result, newLines...)

	// добавляем строки после редактируемой области
	if endIdx+1 < len(lines) {
		result = append(result, lines[endIdx+1:]...)
	}

	// подсчитываем количество изменений
	changes := 0
	if len(newLines) > 0 || endIdx >= startIdx {
		changes = 1 // считаем каждый edit как одно изменение
	}

	return result, changes
}

// validateGoSyntax проверяет синтаксис Go файла
func validateGoSyntax(filePath string) error {
	// используем gofmt для проверки синтаксиса
	cmd := exec.Command("gofmt", "-e", filePath)
	cmd.Dir = filepath.Dir(filePath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("syntax error: %v\nOutput: %s", err, string(output))
	}

	return nil
}

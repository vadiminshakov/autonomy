package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLspEdit_SimpleEdit(t *testing.T) {
	// создаем временный файл для тестирования
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")

	originalContent := `package main

import "fmt"

func main() {
	fmt.Println("hello")
}`

	if err := os.WriteFile(testFile, []byte(originalContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// применяем одно редактирование - заменяем "hello" на "world"
	args := map[string]interface{}{
		"path": testFile,
		"edits": []interface{}{
			map[string]interface{}{
				"start_line":  6,
				"end_line":    6,
				"new_text":    `	fmt.Println("world")`,
				"description": "change hello to world",
			},
		},
	}

	result, err := lspEdit(args)
	if err != nil {
		t.Fatalf("lspEdit failed: %v", err)
	}

	if !strings.Contains(result, "applied 1 edit(s)") {
		t.Errorf("Expected result to mention 1 edit, got: %s", result)
	}

	// проверяем результат
	modifiedContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read modified file: %v", err)
	}

	expectedContent := `package main

import "fmt"

func main() {
	fmt.Println("world")
}`

	if string(modifiedContent) != expectedContent {
		t.Errorf("Expected:\n%s\nGot:\n%s", expectedContent, string(modifiedContent))
	}
}

func TestLspEdit_MultipleEdits(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")

	originalContent := `package main

import "fmt"

func main() {
	fmt.Println("first")
	fmt.Println("second")
	fmt.Println("third")
}`

	if err := os.WriteFile(testFile, []byte(originalContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// применяем несколько редактирований
	args := map[string]interface{}{
		"path": testFile,
		"edits": []interface{}{
			map[string]interface{}{
				"start_line": 6,
				"end_line":   6,
				"new_text":   `	fmt.Println("FIRST")`,
			},
			map[string]interface{}{
				"start_line": 8,
				"end_line":   8,
				"new_text":   `	fmt.Println("THIRD")`,
			},
		},
	}

	result, err := lspEdit(args)
	if err != nil {
		t.Fatalf("lspEdit failed: %v", err)
	}

	if !strings.Contains(result, "applied 2 edit(s)") {
		t.Errorf("Expected result to mention 2 edits, got: %s", result)
	}

	// проверяем результат
	modifiedContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read modified file: %v", err)
	}

	expectedContent := `package main

import "fmt"

func main() {
	fmt.Println("FIRST")
	fmt.Println("second")
	fmt.Println("THIRD")
}`

	if string(modifiedContent) != expectedContent {
		t.Errorf("Expected:\n%s\nGot:\n%s", expectedContent, string(modifiedContent))
	}
}

func TestLspEdit_AddLines(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")

	originalContent := `package main

func main() {
	// empty
}`

	if err := os.WriteFile(testFile, []byte(originalContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// добавляем импорт и код
	args := map[string]interface{}{
		"path": testFile,
		"edits": []interface{}{
			map[string]interface{}{
				"start_line": 2,
				"end_line":   2, // вставка в строку 2
				"new_text":   "\nimport \"fmt\"",
			},
			map[string]interface{}{
				"start_line": 4,
				"end_line":   4, // replace "// empty" line
				"new_text":   `	fmt.Println("hello world")`,
			},
		},
	}

	result, err := lspEdit(args)
	if err != nil {
		t.Fatalf("lspEdit failed: %v", err)
	}

	if !strings.Contains(result, "applied 2 edit(s)") {
		t.Errorf("Expected result to mention 2 edits, got: %s", result)
	}

	// проверяем результат
	modifiedContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read modified file: %v", err)
	}

	if !strings.Contains(string(modifiedContent), `import "fmt"`) {
		t.Error("Expected import statement to be added")
	}

	if !strings.Contains(string(modifiedContent), `fmt.Println("hello world")`) {
		t.Error("Expected println statement to be added")
	}
}

func TestLspEdit_InvalidEdits(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")

	originalContent := `package main

func main() {
}`

	if err := os.WriteFile(testFile, []byte(originalContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// тест недопустимых номеров строк
	args := map[string]interface{}{
		"path": testFile,
		"edits": []interface{}{
			map[string]interface{}{
				"start_line": 0, // недопустимо - должно быть >= 1
				"end_line":   1,
				"new_text":   "// comment",
			},
		},
	}

	_, err := lspEdit(args)
	if err == nil {
		t.Error("Expected error for invalid start_line, but got none")
	}

	// тест end_line < start_line
	args = map[string]interface{}{
		"path": testFile,
		"edits": []interface{}{
			map[string]interface{}{
				"start_line": 3,
				"end_line":   2, // недопустимо
				"new_text":   "// comment",
			},
		},
	}

	_, err = lspEdit(args)
	if err == nil {
		t.Error("Expected error for end_line < start_line, but got none")
	}
}

func TestLspEdit_FileNotFound(t *testing.T) {
	args := map[string]interface{}{
		"path": "/non/existent/file.go",
		"edits": []interface{}{
			map[string]interface{}{
				"start_line": 1,
				"end_line":   1,
				"new_text":   "// comment",
			},
		},
	}

	_, err := lspEdit(args)
	if err == nil {
		t.Error("Expected error for non-existent file, but got none")
	}

	if !strings.Contains(err.Error(), "file does not exist") {
		t.Errorf("Expected 'file does not exist' error, got: %v", err)
	}
}

func TestLspEdit_MissingPath(t *testing.T) {
	args := map[string]interface{}{
		"edits": []interface{}{
			map[string]interface{}{
				"start_line": 1,
				"end_line":   1,
				"new_text":   "// comment",
			},
		},
	}

	_, err := lspEdit(args)
	if err == nil {
		t.Error("Expected error for missing path, but got none")
	}
}

func TestLspEdit_EmptyEdits(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")

	if err := os.WriteFile(testFile, []byte("package main"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	args := map[string]interface{}{
		"path":  testFile,
		"edits": []interface{}{},
	}

	_, err := lspEdit(args)
	if err == nil {
		t.Error("Expected error for empty edits, but got none")
	}

	if !strings.Contains(err.Error(), "no edits provided") {
		t.Errorf("Expected 'no edits provided' error, got: %v", err)
	}
}

package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchDir(t *testing.T) {
	// create temporary directory for tests
	tempDir, err := os.MkdirTemp("", "search_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// create test files
	testFiles := map[string]string{
		"file1.txt":        "hello world\ntest line\nfoo bar",
		"file2.txt":        "nothing here\nsearch target\nrandom text",
		"subdir/file3.txt": "another file\nsearch target again\nend",
	}

	for path, content := range testFiles {
		fullPath := filepath.Join(tempDir, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("failed to create directory %s: %v", dir, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to create file %s: %v", fullPath, err)
		}
	}

	// search with expected results
	args := map[string]interface{}{
		"path":  tempDir,
		"query": "search target",
	}

	result, err := SearchDir(args)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	// verify that result contains expected elements
	if !strings.Contains(result, "search target") {
		t.Errorf("result does not contain target text: %s", result)
	}
	if !strings.Contains(result, "file2.txt") {
		t.Errorf("result does not contain file2.txt: %s", result)
	}
	if !strings.Contains(result, "file3.txt") {
		t.Errorf("result does not contain file3.txt: %s", result)
	}
	if !strings.Contains(result, "Found") {
		t.Errorf("Result missing summary line: %s", result)
	}
}

func TestSearchDirNoResults(t *testing.T) {
	// create temporary directory for tests
	tempDir, err := os.MkdirTemp("", "search_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// create test file
	testFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello world"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	// search with no results
	args := map[string]interface{}{
		"path":  tempDir,
		"query": "nonexistent",
	}

	result, err := SearchDir(args)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if !strings.Contains(result, "no matches found") {
		t.Errorf("Result should contain 'no matches found' message: %s", result)
	}
}

func TestSearchDirCaseInsensitive(t *testing.T) {
	// create temporary directory for tests
	tempDir, err := os.MkdirTemp("", "search_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// create test file
	testFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("Hello World\nTEST LINE"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	// case-insensitive search test
	args := map[string]interface{}{
		"path":             tempDir,
		"query":            "hello",
		"case_insensitive": true,
	}

	result, err := SearchDir(args)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if !strings.Contains(result, "Hello World") {
		t.Errorf("result should contain found string: %s", result)
	}
}

func TestSearchDirMissingQuery(t *testing.T) {
	args := map[string]interface{}{
		"path": ".",
	}

	_, err := SearchDir(args)
	if err == nil {
		t.Error("expected error when 'query' parameter is missing")
	}
	if !strings.Contains(err.Error(), "parameter 'query' is required") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestFindFiles(t *testing.T) {
	// create temporary directory for tests
	tempDir, err := os.MkdirTemp("", "find_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// create test files
	testFiles := []string{
		"test.go",
		"main.go",
		"config.json",
		"subdir/helper.go",
	}

	for _, file := range testFiles {
		fullPath := filepath.Join(tempDir, file)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("failed to create directory %s: %v", dir, err)
		}
		if err := os.WriteFile(fullPath, []byte("content"), 0644); err != nil {
			t.Fatalf("failed to create file %s: %v", fullPath, err)
		}
	}

	// search for .go files
	args := map[string]interface{}{
		"path":    tempDir,
		"pattern": ".go",
	}

	result, err := FindFiles(args)
	if err != nil {
		t.Fatalf("file search failed: %v", err)
	}

	// verify .go files found
	if !strings.Contains(result, "test.go") {
		t.Errorf("result does not contain test.go: %s", result)
	}
	if !strings.Contains(result, "main.go") {
		t.Errorf("result does not contain main.go: %s", result)
	}
	if !strings.Contains(result, "helper.go") {
		t.Errorf("result does not contain helper.go: %s", result)
	}
	if strings.Contains(result, "config.json") {
		t.Errorf("result should not contain config.json: %s", result)
	}
}

func TestFindFilesMissingPattern(t *testing.T) {
	args := map[string]interface{}{
		"path": ".",
	}

	_, err := FindFiles(args)
	if err == nil {
		t.Error("expected error when 'pattern' parameter is missing")
	}
	if !strings.Contains(err.Error(), "parameter 'pattern' is required") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

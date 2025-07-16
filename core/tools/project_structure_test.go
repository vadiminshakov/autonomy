package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetProjectStructure(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_project_structure")
	require.NoError(t, err, "Failed to create temp dir")
	defer os.RemoveAll(tempDir)

	testFiles := []string{
		"main.go",
		"README.md",
		"src/app.go",
		"src/utils/helper.go",
		"tests/main_test.go",
		".git/config",  // should be ignored
		".DS_Store",    // should be ignored
		"logs/app.log", // .log should be ignored
	}

	for _, file := range testFiles {
		fullPath := filepath.Join(tempDir, file)
		dir := filepath.Dir(fullPath)

		require.NoError(t, os.MkdirAll(dir, 0755), "Failed to create dir %s", dir)
		require.NoError(t, os.WriteFile(fullPath, []byte("test content"), 0644), "Failed to create file %s", fullPath)
	}

	args := map[string]interface{}{
		"path": tempDir,
	}

	result, err := GetProjectStructure(args)
	require.NoError(t, err, "GetProjectStructure failed")

	// verify result contains expected files
	expectedFiles := []string{
		"main.go",
		"README.md",
		"src/",
		"app.go",
		"utils/",
		"helper.go",
		"tests/",
		"main_test.go",
		"logs/",
	}

	for _, expected := range expectedFiles {
		require.Contains(t, result, expected, "Expected to find '%s' in result, but didn't. Result:\n%s", expected, result)
	}

	// verify ignored files are NOT present in result
	ignoredFiles := []string{
		".git",
		".DS_Store",
		"app.log",
	}

	for _, ignored := range ignoredFiles {
		require.NotContains(t, result, ignored, "Expected NOT to find '%s' in result, but found it. Result:\n%s", ignored, result)
	}

	// verify tree-like structure present
	require.True(t, strings.Contains(result, "├──") || strings.Contains(result, "└──"), "Expected tree-like structure with ├── or └── symbols, but didn't find them. Result:\n%s", result)
}

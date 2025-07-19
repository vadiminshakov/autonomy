package tools

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func init() {
	Register("search_dir", SearchDir)
	Register("find_files", FindFiles)
}

// SearchDir searches for a text query inside files under a directory.
func SearchDir(args map[string]interface{}) (string, error) {
	rootDir, ok := args["path"].(string)
	if !ok || rootDir == "" {
		rootDir = "." // default to current directory
	}

	query, ok := args["query"].(string)
	if !ok || query == "" {
		return "", fmt.Errorf("parameter 'query' is required for search_dir")
	}

	caseInsensitive := false
	if val, ok := args["case_insensitive"].(bool); ok {
		caseInsensitive = val
	} else if val, ok := args["case_insensitive"].(string); ok {
		caseInsensitive = (val == "true" || val == "1")
	}

	results, err := searchInDir(rootDir, query, caseInsensitive)
	if err != nil {
		return "", fmt.Errorf("search error: %v", err)
	}

	if len(results) == 0 {
		return fmt.Sprintf("Search '%s' in '%s': no matches found", query, rootDir), nil
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Search '%s' in '%s':\n", query, rootDir))

	totalMatches := 0
	for filePath, matches := range results {
		output.WriteString(fmt.Sprintf("\n📄 %s (%d matches):\n", filePath, len(matches)))
		for _, match := range matches {
			output.WriteString(fmt.Sprintf("  %d: %s\n", match.LineNumber, match.Text))
			totalMatches++
		}
	}

	output.WriteString(fmt.Sprintf("\nFound %d matches in %d files", totalMatches, len(results)))

	return output.String(), nil
}

// searchMatch describes a single match inside a file.
type searchMatch struct {
	LineNumber int    `json:"line"`
	Text       string `json:"text"`
}

// searchInDir searches for a given substring (case sensitive by default)
// in all files under the given directory (including subdirectories).
// If caseInsensitive is true, search ignores letter case.
//
// Returns a map: file path => list of matched lines (line numbers and text)
func searchInDir(rootDir string, query string, caseInsensitive bool) (map[string][]searchMatch, error) {
	if rootDir == "" || query == "" {
		return nil, errors.New("rootDir and query must be non-empty")
	}
	results := make(map[string][]searchMatch)

	walker := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}

		matches, err := searchFileForQuery(path, query, caseInsensitive)
		if err != nil {
			return nil // ignore read errors for robustness
		}
		if len(matches) > 0 {
			results[path] = matches
		}
		return nil
	}

	if err := filepath.WalkDir(rootDir, walker); err != nil {
		return nil, err
	}
	return results, nil
}

// searchFileForQuery opens a file and finds all lines containing the query.
// Returns slice of line numbers and text where match found.
func searchFileForQuery(path, query string, caseInsensitive bool) ([]searchMatch, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	matches := []searchMatch{}
	lines := strings.Split(string(data), "\n")

	q := query
	if caseInsensitive {
		q = strings.ToLower(query)
	}
	for i, line := range lines {
		candidate := line
		if caseInsensitive {
			candidate = strings.ToLower(line)
		}
		if strings.Contains(candidate, q) {
			matches = append(matches, searchMatch{
				LineNumber: i + 1,
				Text:       line,
			})
		}
	}
	return matches, nil
}

// FindFiles searches for files by name/pattern
func FindFiles(args map[string]interface{}) (string, error) {
	rootDir, ok := args["path"].(string)
	if !ok || rootDir == "" {
		rootDir = "." // default to current directory
	}

	pattern, ok := args["pattern"].(string)
	if !ok || pattern == "" {
		return "", fmt.Errorf("parameter 'pattern' is required for find_files")
	}

	caseInsensitive := false
	if val, ok := args["case_insensitive"].(bool); ok {
		caseInsensitive = val
	} else if val, ok := args["case_insensitive"].(string); ok {
		caseInsensitive = (val == "true" || val == "1")
	}

	foundFiles, err := findFilesByName(rootDir, pattern, caseInsensitive)
	if err != nil {
		return "", fmt.Errorf("file search error: %v", err)
	}

	if len(foundFiles) == 0 {
		return fmt.Sprintf("Search for files with pattern '%s' in '%s': no files found", pattern, rootDir), nil
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Files with pattern '%s' in '%s':\n", pattern, rootDir))

	for _, filePath := range foundFiles {
		output.WriteString(fmt.Sprintf("📄 %s\n", filePath))
	}

	output.WriteString(fmt.Sprintf("\nFound %d files", len(foundFiles)))

	return output.String(), nil
}

// findFilesByName searches files by name/pattern inside a directory.
func findFilesByName(rootDir, pattern string, caseInsensitive bool) ([]string, error) {
	var foundFiles []string

	searchPattern := pattern
	if caseInsensitive {
		searchPattern = strings.ToLower(pattern)
	}

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // ignore access errors to files
		}

		if info.IsDir() {
			return nil
		}

		fileName := info.Name()
		searchName := fileName
		if caseInsensitive {
			searchName = strings.ToLower(fileName)
		}

		if strings.Contains(searchName, searchPattern) {
			relPath, err := filepath.Rel(rootDir, path)
			if err != nil {
				relPath = path
			}

			foundFiles = append(foundFiles, relPath)
		}

		return nil
	})

	return foundFiles, err
}

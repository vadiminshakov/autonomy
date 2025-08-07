package tools

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func init() {
	Register("grep", GrepSearch)
}

// GrepSearch is a universal search tool that replaces both search_dir and find_files
func GrepSearch(args map[string]interface{}) (string, error) {
	// get query parameter (required)
	query, ok := args["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("parameter 'query' must be a non-empty string")
	}

	// get search mode: "content" (default) or "files"
	mode := "content"
	if modeVal, ok := args["mode"].(string); ok {
		mode = strings.ToLower(strings.TrimSpace(modeVal))
	}
	if mode != "content" && mode != "files" {
		return "", fmt.Errorf("mode must be 'content' or 'files'")
	}

	// get path parameter (optional, default to current directory)
	searchPath := "."
	if pathVal, ok := args["path"].(string); ok && strings.TrimSpace(pathVal) != "" {
		searchPath = strings.TrimSpace(pathVal)
	}

	// get regex flag (optional, default false)
	useRegex := false
	if regexVal, ok := args["regex"].(bool); ok {
		useRegex = regexVal
	}

	// get case_sensitive flag (optional, default false for case-insensitive)
	caseSensitive := false
	if caseVal, ok := args["case_sensitive"].(bool); ok {
		caseSensitive = caseVal
	}

	// get file_types filter (optional)
	var fileTypes []string
	if typesVal, ok := args["file_types"].([]interface{}); ok {
		for _, t := range typesVal {
			if typeStr, ok := t.(string); ok {
				fileTypes = append(fileTypes, strings.TrimSpace(typeStr))
			}
		}
	}

	// get exclude patterns (optional)
	var excludePatterns []string
	if excludeVal, ok := args["exclude"].([]interface{}); ok {
		for _, e := range excludeVal {
			if excludeStr, ok := e.(string); ok {
				excludePatterns = append(excludePatterns, strings.TrimSpace(excludeStr))
			}
		}
	}

	// get max_results limit (optional, default 100)
	maxResults := 100
	if maxVal, ok := args["max_results"]; ok {
		if maxFloat, ok := maxVal.(float64); ok {
			maxResults = int(maxFloat)
		}
	}
	if maxResults <= 0 || maxResults > 1000 {
		maxResults = 100
	}

	// try ripgrep first, fallback to built-in search
	result, err := tryRipgrep(query, searchPath, mode, useRegex, caseSensitive, fileTypes, excludePatterns, maxResults)
	if err == nil {
		return result, nil
	}

	// fallback to built-in search
	return builtinSearch(query, searchPath, mode, useRegex, caseSensitive, fileTypes, excludePatterns, maxResults)
}

// tryRipgrep attempts to use ripgrep if available
func tryRipgrep(query, searchPath, mode string, useRegex, caseSensitive bool, fileTypes, excludePatterns []string, maxResults int) (string, error) {
	// check if ripgrep is available
	if _, err := exec.LookPath("rg"); err != nil {
		return "", fmt.Errorf("ripgrep not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var args []string

	if mode == "files" {
		args = append(args, "--files")
		// for file search, we'll filter the file list
		if query != "" {
			args = append(args, "-g", fmt.Sprintf("*%s*", query))
		}
	} else {
		// content search
		args = append(args, query)
		
		if !useRegex {
			args = append(args, "--fixed-strings")
		}
		
		if !caseSensitive {
			args = append(args, "--ignore-case")
		}
		
		args = append(args, "--line-number", "--with-filename")
	}

	// add file type filters
	for _, fileType := range fileTypes {
		args = append(args, "--type", fileType)
	}

	// add exclude patterns
	for _, exclude := range excludePatterns {
		args = append(args, "--glob", fmt.Sprintf("!*%s*", exclude))
	}

	// limit results
	args = append(args, "--max-count", fmt.Sprintf("%d", maxResults))

	// add search path
	args = append(args, searchPath)

	cmd := exec.CommandContext(ctx, "rg", args...)
	output, err := cmd.CombinedOutput()
	
	if err != nil {
		// ripgrep returns exit code 1 when no matches found, which is normal
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("ripgrep timeout")
		}
	}

	return formatRipgrepOutput(string(output), query, searchPath, mode), nil
}

// builtinSearch implements search using Go's built-in functionality
func builtinSearch(query, searchPath, mode string, useRegex, caseSensitive bool, fileTypes, excludePatterns []string, maxResults int) (string, error) {
	var results []SearchResult
	resultCount := 0

	// compile regex if needed
	var queryRegex *regexp.Regexp
	if useRegex {
		flags := ""
		if !caseSensitive {
			flags = "(?i)"
		}
		var err error
		queryRegex, err = regexp.Compile(flags + query)
		if err != nil {
			return "", fmt.Errorf("invalid regex pattern: %v", err)
		}
	}

	err := filepath.WalkDir(searchPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil || resultCount >= maxResults {
			return err
		}

		// skip if excluded
		if shouldExclude(path, excludePatterns) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			return nil
		}

		// check file type
		if len(fileTypes) > 0 && !matchesFileType(path, fileTypes) {
			return nil
		}

		if mode == "files" {
			// search in filename
			fileName := filepath.Base(path)
			if matchesQuery(fileName, query, queryRegex, useRegex, caseSensitive) {
				results = append(results, SearchResult{
					FilePath: path,
					Type:     "file",
				})
				resultCount++
			}
		} else {
			// search in file content
			matches, err := searchFileContent(path, query, queryRegex, useRegex, caseSensitive, maxResults-resultCount)
			if err != nil {
				return nil // ignore file read errors
			}
			if len(matches) > 0 {
				results = append(results, SearchResult{
					FilePath: path,
					Type:     "content",
					Matches:  matches,
				})
				resultCount += len(matches)
			}
		}

		return nil
	})

	if err != nil {
		return "", fmt.Errorf("search error: %v", err)
	}

	return formatBuiltinResults(results, query, searchPath, mode), nil
}

// SearchResult represents a search result
type SearchResult struct {
	FilePath string          `json:"file_path"`
	Type     string          `json:"type"` // "file" or "content"
	Matches  []ContentMatch  `json:"matches,omitempty"`
}

// ContentMatch represents a content match within a file
type ContentMatch struct {
	LineNumber int    `json:"line_number"`
	Line       string `json:"line"`
	Column     int    `json:"column,omitempty"`
}

// searchFileContent searches for query in file content
func searchFileContent(filePath, query string, queryRegex *regexp.Regexp, useRegex, caseSensitive bool, maxMatches int) ([]ContentMatch, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	// skip binary files
	if isBinaryFile(data) {
		return nil, nil
	}

	content := string(data)
	lines := strings.Split(content, "\n")
	var matches []ContentMatch

	for i, line := range lines {
		if len(matches) >= maxMatches {
			break
		}

		if matchesQuery(line, query, queryRegex, useRegex, caseSensitive) {
			column := 0
			if useRegex && queryRegex != nil {
				if loc := queryRegex.FindStringIndex(line); loc != nil {
					column = loc[0] + 1
				}
			} else {
				searchQuery := query
				searchLine := line
				if !caseSensitive {
					searchQuery = strings.ToLower(query)
					searchLine = strings.ToLower(line)
				}
				if idx := strings.Index(searchLine, searchQuery); idx != -1 {
					column = idx + 1
				}
			}

			matches = append(matches, ContentMatch{
				LineNumber: i + 1,
				Line:       line,
				Column:     column,
			})
		}
	}

	return matches, nil
}

// matchesQuery checks if text matches the query
func matchesQuery(text, query string, queryRegex *regexp.Regexp, useRegex, caseSensitive bool) bool {
	if useRegex && queryRegex != nil {
		return queryRegex.MatchString(text)
	}

	searchText := text
	searchQuery := query
	if !caseSensitive {
		searchText = strings.ToLower(text)
		searchQuery = strings.ToLower(query)
	}

	return strings.Contains(searchText, searchQuery)
}

// shouldExclude checks if path should be excluded
func shouldExclude(path string, excludePatterns []string) bool {
	for _, pattern := range excludePatterns {
		if strings.Contains(strings.ToLower(path), strings.ToLower(pattern)) {
			return true
		}
	}
	
	// exclude common directories
	excludeDirs := []string{".git", "node_modules", ".svn", ".hg", "vendor", "__pycache__", ".DS_Store"}
	pathLower := strings.ToLower(path)
	for _, dir := range excludeDirs {
		if strings.Contains(pathLower, dir) {
			return true
		}
	}
	
	return false
}

// matchesFileType checks if file matches any of the specified types
func matchesFileType(filePath string, fileTypes []string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	for _, fileType := range fileTypes {
		typeExt := "." + strings.TrimPrefix(strings.ToLower(fileType), ".")
		if ext == typeExt {
			return true
		}
	}
	return false
}

// isBinaryFile checks if data appears to be binary
func isBinaryFile(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	
	// check first 512 bytes for null bytes
	checkLen := 512
	if len(data) < checkLen {
		checkLen = len(data)
	}
	
	for i := 0; i < checkLen; i++ {
		if data[i] == 0 {
			return true
		}
	}
	
	return false
}

// formatRipgrepOutput formats ripgrep output with size limits
func formatRipgrepOutput(output, query, searchPath, mode string) string {
	const maxOutputSize = 4000 // Limit output to 4KB to prevent token overflow
	const maxLinesPerFile = 10 // Show max 10 matches per file
	
	if strings.TrimSpace(output) == "" {
		return fmt.Sprintf("No matches found for '%s' in '%s'", query, searchPath)
	}

	var result strings.Builder
	if mode == "files" {
		result.WriteString(fmt.Sprintf("# Files matching '%s' in '%s'\n\n", query, searchPath))
	} else {
		result.WriteString(fmt.Sprintf("# Content search for '%s' in '%s'\n\n", query, searchPath))
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	fileCount := 0
	matchCount := 0
	totalMatchCount := 0

	currentFile := ""
	currentFileMatches := 0
	truncated := false
	
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Check if we're approaching size limit
		if result.Len() > maxOutputSize {
			truncated = true
			break
		}

		if mode == "files" {
			result.WriteString(fmt.Sprintf("📄 %s\n", line))
			fileCount++
		} else {
			// ripgrep format: filename:line_number:content
			parts := strings.SplitN(line, ":", 3)
			if len(parts) >= 3 {
				file := parts[0]
				lineNum := parts[1]
				content := parts[2]
				
				totalMatchCount++

				if file != currentFile {
					if currentFile != "" {
						if currentFileMatches > maxLinesPerFile {
							result.WriteString(fmt.Sprintf("  ... and %d more matches\n", currentFileMatches - maxLinesPerFile))
						}
						result.WriteString("\n")
					}
					result.WriteString(fmt.Sprintf("## 📄 %s\n", file))
					currentFile = file
					currentFileMatches = 0
					fileCount++
				}

				currentFileMatches++
				if currentFileMatches <= maxLinesPerFile {
					// Truncate very long lines
					if len(content) > 120 {
						content = content[:117] + "..."
					}
					result.WriteString(fmt.Sprintf("  %s: %s\n", lineNum, content))
					matchCount++
				}
			}
		}
	}

	// Handle last file's overflow
	if currentFile != "" && currentFileMatches > maxLinesPerFile {
		result.WriteString(fmt.Sprintf("  ... and %d more matches\n", currentFileMatches - maxLinesPerFile))
	}

	if truncated {
		result.WriteString(fmt.Sprintf("\n⚠️ Output truncated. Showing %d of %d total matches", matchCount, totalMatchCount))
	}

	if mode == "files" {
		result.WriteString(fmt.Sprintf("\nFound %d files", fileCount))
	} else {
		result.WriteString(fmt.Sprintf("\nFound %d matches in %d files", matchCount, fileCount))
		if totalMatchCount > matchCount {
			result.WriteString(fmt.Sprintf(" (showing %d of %d total)", matchCount, totalMatchCount))
		}
	}

	// record operation
	state := getTaskState()
	state.mu.Lock()
	if state.CompletedTools == nil {
		state.CompletedTools = make(map[string]int)
	}
	state.CompletedTools["grep"]++
	state.mu.Unlock()

	// create structured response
	metadata := &GrepMetadata{
		Query:      query,
		SearchPath: searchPath,
		Mode:       mode,
		Tool:       "ripgrep",
		FileCount:  fileCount,
		MatchCount: matchCount,
	}

	return CreateStructuredResponse(result.String(), metadata)
}

// formatBuiltinResults formats built-in search results with size limits
func formatBuiltinResults(results []SearchResult, query, searchPath, mode string) string {
	const maxOutputSize = 4000 // Limit output to 4KB
	const maxLinesPerFile = 10 // Show max 10 matches per file
	const maxFiles = 20        // Show max 20 files
	
	if len(results) == 0 {
		return fmt.Sprintf("No matches found for '%s' in '%s'", query, searchPath)
	}

	var result strings.Builder
	if mode == "files" {
		result.WriteString(fmt.Sprintf("# Files matching '%s' in '%s'\n\n", query, searchPath))
	} else {
		result.WriteString(fmt.Sprintf("# Content search for '%s' in '%s'\n\n", query, searchPath))
	}

	fileCount := 0
	totalMatches := 0
	displayedMatches := 0
	truncated := false

	for i, res := range results {
		// Check size limit
		if result.Len() > maxOutputSize || i >= maxFiles {
			truncated = true
			break
		}

		if mode == "files" {
			result.WriteString(fmt.Sprintf("📄 %s\n", res.FilePath))
			fileCount++
		} else {
			totalMatches += len(res.Matches)
			matchesToShow := len(res.Matches)
			if matchesToShow > maxLinesPerFile {
				matchesToShow = maxLinesPerFile
			}
			
			result.WriteString(fmt.Sprintf("## 📄 %s (%d matches)\n", res.FilePath, len(res.Matches)))
			
			for j, match := range res.Matches {
				if j >= matchesToShow {
					result.WriteString(fmt.Sprintf("  ... and %d more matches\n", len(res.Matches) - matchesToShow))
					break
				}
				
				// Truncate very long lines
				line := match.Line
				if len(line) > 120 {
					line = line[:117] + "..."
				}
				result.WriteString(fmt.Sprintf("  %d: %s\n", match.LineNumber, line))
				displayedMatches++
			}
			result.WriteString("\n")
			fileCount++
		}
	}

	if truncated {
		if mode == "files" {
			result.WriteString(fmt.Sprintf("\n⚠️ Output truncated. Showing %d of %d files", fileCount, len(results)))
		} else {
			result.WriteString(fmt.Sprintf("\n⚠️ Output truncated. Showing %d files of %d total", fileCount, len(results)))
		}
	}

	if mode == "files" {
		result.WriteString(fmt.Sprintf("\nFound %d files", fileCount))
		if len(results) > fileCount {
			result.WriteString(fmt.Sprintf(" (showing %d of %d)", fileCount, len(results)))
		}
	} else {
		result.WriteString(fmt.Sprintf("\nFound %d matches in %d files", displayedMatches, fileCount))
		if totalMatches > displayedMatches {
			result.WriteString(fmt.Sprintf(" (showing %d of %d total matches)", displayedMatches, totalMatches))
		}
	}

	// record operation
	state := getTaskState()
	state.mu.Lock()
	if state.CompletedTools == nil {
		state.CompletedTools = make(map[string]int)
	}
	state.CompletedTools["grep"]++
	state.mu.Unlock()

	// create structured response
	metadata := &GrepMetadata{
		Query:      query,
		SearchPath: searchPath,
		Mode:       mode,
		Tool:       "builtin",
		FileCount:  fileCount,
		MatchCount: totalMatches,
	}

	return CreateStructuredResponse(result.String(), metadata)
}

// GrepMetadata contains metadata for grep operations
type GrepMetadata struct {
	Query      string `json:"query"`
	SearchPath string `json:"search_path"`
	Mode       string `json:"mode"`
	Tool       string `json:"tool"` // "ripgrep" or "builtin"
	FileCount  int    `json:"file_count"`
	MatchCount int    `json:"match_count"`
}
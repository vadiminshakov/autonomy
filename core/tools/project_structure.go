package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func init() {
	Register("get_project_structure", GetProjectStructure)
}

// GetProjectStructure returns a tree-like structure of the project starting from the working directory or the provided path
func GetProjectStructure(args map[string]interface{}) (string, error) {
	root := "."
	if val, ok := args["path"].(string); ok && val != "" {
		root = val
	}

	ignorePatterns := []string{
		".git",
		".DS_Store",
		"node_modules",
		".idea",
		".vscode",
		"*.tmp",
		"*.log",
		".env",
		"coverage.out",
	}

	sb := &strings.Builder{}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}

	sb.WriteString(fmt.Sprintf("%s/\n", filepath.Base(absRoot)))

	err = buildTree(root, "", sb, ignorePatterns)
	if err != nil {
		return "", fmt.Errorf("failed to build project structure: %v", err)
	}

	return sb.String(), nil
}

// buildTree constructs the file tree recursively
func buildTree(dir, prefix string, sb *strings.Builder, ignorePatterns []string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	var filteredEntries []os.DirEntry
	for _, entry := range entries {
		if shouldIgnore(entry.Name(), ignorePatterns) {
			continue
		}
		filteredEntries = append(filteredEntries, entry)
	}

	// sort: directories first, then files; alphabetical inside groups
	sort.Slice(filteredEntries, func(i, j int) bool {
		if filteredEntries[i].IsDir() != filteredEntries[j].IsDir() {
			return filteredEntries[i].IsDir()
		}
		return filteredEntries[i].Name() < filteredEntries[j].Name()
	})

	for i, entry := range filteredEntries {
		isLast := i == len(filteredEntries)-1

		var treeSymbol, nextPrefix string
		if isLast {
			treeSymbol = "└── "
			nextPrefix = prefix + "    "
		} else {
			treeSymbol = "├── "
			nextPrefix = prefix + "│   "
		}

		// add entry to the tree
		if entry.IsDir() {
			sb.WriteString(fmt.Sprintf("%s%s%s/\n", prefix, treeSymbol, entry.Name()))

			// recursively process subdirectory
			subDir := filepath.Join(dir, entry.Name())
			err := buildTree(subDir, nextPrefix, sb, ignorePatterns)
			if err != nil {
				// continue even if subdirectory processing fails
				sb.WriteString(fmt.Sprintf("%s    [error reading directory: %v]\n", nextPrefix, err))
			}
		} else {
			sb.WriteString(fmt.Sprintf("%s%s%s\n", prefix, treeSymbol, entry.Name()))
		}
	}

	return nil
}

// shouldIgnore determines whether a file or directory should be ignored
func shouldIgnore(name string, patterns []string) bool {
	for _, pattern := range patterns {
		if pattern == name {
			return true
		}
		// simple wildcard support
		if strings.HasPrefix(pattern, "*.") {
			ext := pattern[1:]
			if strings.HasSuffix(name, ext) {
				return true
			}
		}
	}
	
	return false
}

package tools

import (
	"path/filepath"
	"strings"
	"sync"
)

// languageServerRegistry holds configurations for different language servers
var (
	languageServerRegistry = map[string]LanguageServerConfig{
	// Go
	"go": {
		Command:    "gopls",
		Args:       []string{},
		Extensions: []string{".go"},
	},
	
	// TypeScript/JavaScript
	"typescript": {
		Command:    "typescript-language-server",
		Args:       []string{"--stdio"},
		Extensions: []string{".ts", ".tsx", ".js", ".jsx"},
	},
	
	// Python
	"python": {
		Command:    "pylsp",
		Args:       []string{},
		Extensions: []string{".py"},
	},
	
	// Rust
	"rust": {
		Command:    "rust-analyzer",
		Args:       []string{},
		Extensions: []string{".rs"},
	},
	
	// Java
	"java": {
		Command:    "jdtls",
		Args:       []string{},
		Extensions: []string{".java"},
	},
	
	// C/C++
	"cpp": {
		Command:    "clangd",
		Args:       []string{},
		Extensions: []string{".c", ".cpp", ".cc", ".cxx", ".h", ".hpp"},
	},
	
	// C#
	"csharp": {
		Command:    "omnisharp",
		Args:       []string{"--languageserver"},
		Extensions: []string{".cs"},
	},
	
	// HTML
	"html": {
		Command:    "vscode-html-languageserver",
		Args:       []string{"--stdio"},
		Extensions: []string{".html", ".htm"},
	},
	
	// CSS
	"css": {
		Command:    "vscode-css-languageserver",
		Args:       []string{"--stdio"},
		Extensions: []string{".css", ".scss", ".sass", ".less"},
	},
	
	// JSON
	"json": {
		Command:    "vscode-json-languageserver",
		Args:       []string{"--stdio"},
		Extensions: []string{".json"},
	},
	
	// Ruby
	"ruby": {
		Command:    "ruby-lsp",
		Args:       []string{},
		Extensions: []string{".rb"},
	},
	
	// PHP
	"php": {
		Command:    "phpactor",
		Args:       []string{"language-server"},
		Extensions: []string{".php"},
	},
	
	// Kotlin
	"kotlin": {
		Command:    "kotlin-language-server",
		Args:       []string{},
		Extensions: []string{".kt", ".kts"},
	},
	
	// Scala
	"scala": {
		Command:    "metals",
		Args:       []string{},
		Extensions: []string{".scala", ".sc"},
	},
	
	// Elixir
	"elixir": {
		Command:    "elixir-ls",
		Args:       []string{},
		Extensions: []string{".ex", ".exs"},
	},
}
	lspRegistryMu sync.RWMutex
)

// GetLanguageServerForFile determines which language server to use for a given file
func GetLanguageServerForFile(filePath string) (*LanguageServerConfig, bool) {
	ext := strings.ToLower(filepath.Ext(filePath))
	
	lspRegistryMu.RLock()
	defer lspRegistryMu.RUnlock()
	
	for _, config := range languageServerRegistry {
		for _, supportedExt := range config.Extensions {
			if ext == supportedExt {
				return &config, true
			}
		}
	}
	
	return nil, false
}

// GetLanguageServerByName returns a language server configuration by name
func GetLanguageServerByName(name string) (*LanguageServerConfig, bool) {
	lspRegistryMu.RLock()
	config, exists := languageServerRegistry[name]
	lspRegistryMu.RUnlock()
	
	if !exists {
		return nil, false
	}
	return &config, true
}

// ListSupportedLanguages returns all supported language identifiers
func ListSupportedLanguages() []string {
	lspRegistryMu.RLock()
	defer lspRegistryMu.RUnlock()
	
	languages := make([]string, 0, len(languageServerRegistry))
	for lang := range languageServerRegistry {
		languages = append(languages, lang)
	}
	return languages
}

// GetLanguageIDForFile determines the LSP language identifier for a file
func GetLanguageIDForFile(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	
	switch ext {
	case ".go":
		return "go"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "typescriptreact"
	case ".js":
		return "javascript"
	case ".jsx":
		return "javascriptreact"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".c":
		return "c"
	case ".cpp", ".cc", ".cxx":
		return "cpp"
	case ".h":
		return "c" // could be cpp, but c is safer default
	case ".hpp":
		return "cpp"
	case ".cs":
		return "csharp"
	case ".html", ".htm":
		return "html"
	case ".css":
		return "css"
	case ".scss":
		return "scss"
	case ".sass":
		return "sass"
	case ".less":
		return "less"
	case ".json":
		return "json"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".kt":
		return "kotlin"
	case ".kts":
		return "kotlin"
	case ".scala", ".sc":
		return "scala"
	case ".ex", ".exs":
		return "elixir"
	default:
		return "plaintext"
	}
}

// RegisterCustomLanguageServer allows adding custom language server configurations
func RegisterCustomLanguageServer(name string, config LanguageServerConfig) {
	lspRegistryMu.Lock()
	defer lspRegistryMu.Unlock()
	languageServerRegistry[name] = config
}
package index

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type Language string

const (
	LanguageGo         Language = "go"
	LanguageJavaScript Language = "javascript"
	LanguageTypeScript Language = "typescript"
	LanguagePython     Language = "python"
	LanguageJava       Language = "java"
	LanguageRust       Language = "rust"
	LanguageC          Language = "c"
	LanguageCPP        Language = "cpp"
	LanguageUnknown    Language = "unknown"
)

type SymbolKind string

const (
	SymbolFunction  SymbolKind = "function"
	SymbolMethod    SymbolKind = "method"
	SymbolClass     SymbolKind = "class"
	SymbolInterface SymbolKind = "interface"
	SymbolStruct    SymbolKind = "struct"
	SymbolType      SymbolKind = "type"
	SymbolVariable  SymbolKind = "variable"
	SymbolConstant  SymbolKind = "constant"
	SymbolEnum      SymbolKind = "enum"
	SymbolModule    SymbolKind = "module"
	SymbolNamespace SymbolKind = "namespace"
	SymbolProperty  SymbolKind = "property"
	SymbolField     SymbolKind = "field"
)

type Visibility string

const (
	VisibilityPublic    Visibility = "public"
	VisibilityPrivate   Visibility = "private"
	VisibilityProtected Visibility = "protected"
	VisibilityInternal  Visibility = "internal"
)

type CodeSymbol struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	FullName    string            `json:"full_name"`
	Kind        SymbolKind        `json:"kind"`
	Language    Language          `json:"language"`
	Package     string            `json:"package,omitempty"`
	Namespace   string            `json:"namespace,omitempty"`
	File        string            `json:"file"`
	StartLine   int               `json:"start_line"`
	EndLine     int               `json:"end_line,omitempty"`
	StartColumn int               `json:"start_column,omitempty"`
	EndColumn   int               `json:"end_column,omitempty"`
	Signature   string            `json:"signature"`
	Comment     string            `json:"comment,omitempty"`
	DocString   string            `json:"doc_string,omitempty"`
	Visibility  Visibility        `json:"visibility"`
	IsStatic    bool              `json:"is_static,omitempty"`
	IsAsync     bool              `json:"is_async,omitempty"`
	IsAbstract  bool              `json:"is_abstract,omitempty"`
	ReturnType  string            `json:"return_type,omitempty"`
	Parameters  []Parameter       `json:"parameters,omitempty"`
	Fields      []Field           `json:"fields,omitempty"`
	Methods     []string          `json:"methods,omitempty"`
	Parent      string            `json:"parent,omitempty"`
	Children    []string          `json:"children,omitempty"`
	Imports     []string          `json:"imports,omitempty"`
	Exports     []string          `json:"exports,omitempty"`
	Tags        map[string]string `json:"tags,omitempty"`
	Metadata    map[string]any    `json:"metadata,omitempty"`
}

type Parameter struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	DefaultValue string `json:"default_value,omitempty"`
	IsOptional   bool   `json:"is_optional,omitempty"`
	IsVariadic   bool   `json:"is_variadic,omitempty"`
}

type Field struct {
	Name       string     `json:"name"`
	Type       string     `json:"type"`
	Visibility Visibility `json:"visibility"`
	IsStatic   bool       `json:"is_static,omitempty"`
	IsReadOnly bool       `json:"is_readonly,omitempty"`
	Comment    string     `json:"comment,omitempty"`
}

type UniversalImportInfo struct {
	Path      string   `json:"path"`
	Alias     string   `json:"alias,omitempty"`
	IsDefault bool     `json:"is_default,omitempty"`
	Items     []string `json:"items,omitempty"`
	Language  Language `json:"language"`
	File      string   `json:"file"`
}

type LanguageParser interface {
	GetLanguage() Language
	GetSupportedExtensions() []string
	ParseFile(filePath string) ([]CodeSymbol, []UniversalImportInfo, error)
	ParseContent(content string, filePath string) ([]CodeSymbol, []UniversalImportInfo, error)
}

type Index struct {
	Symbols     map[string]*CodeSymbol `json:"symbols"`
	Imports     []UniversalImportInfo  `json:"imports"`
	Languages   map[Language][]string  `json:"languages"`
	Files       map[string]Language    `json:"files"`
	Packages    map[string][]string    `json:"packages"`
	LastUpdated time.Time              `json:"last_updated"`
	ProjectPath string                 `json:"project_path"`
	parsers     map[Language]LanguageParser
	mu          sync.RWMutex
}

func NewIndex(projectPath string) *Index {
	ui := &Index{
		Symbols:     make(map[string]*CodeSymbol),
		Imports:     make([]UniversalImportInfo, 0),
		Languages:   make(map[Language][]string),
		Files:       make(map[string]Language),
		Packages:    make(map[string][]string),
		ProjectPath: projectPath,
		parsers:     make(map[Language]LanguageParser),
	}

	ui.registerBuiltinParsers()
	return ui
}

func (idx *Index) registerBuiltinParsers() {
	idx.RegisterParser(NewGoParser())
	idx.RegisterParser(NewJSParser())
	idx.RegisterParser(NewTSParser())
	idx.RegisterParser(NewPythonParser())
}

func (idx *Index) RegisterParser(parser LanguageParser) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.parsers[parser.GetLanguage()] = parser
}

func (idx *Index) GetParser(lang Language) (LanguageParser, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	parser, exists := idx.parsers[lang]
	return parser, exists
}

func (idx *Index) DetectLanguage(filePath string) Language {
	ext := strings.ToLower(filepath.Ext(filePath))

	switch ext {
	case ".go":
		return LanguageGo
	case ".js", ".jsx", ".mjs", ".cjs":
		return LanguageJavaScript
	case ".ts", ".tsx":
		return LanguageTypeScript
	case ".py", ".pyx", ".pyi":
		return LanguagePython
	case ".java":
		return LanguageJava
	case ".rs":
		return LanguageRust
	case ".c", ".h":
		return LanguageC
	case ".cpp", ".cxx", ".cc", ".hpp", ".hxx":
		return LanguageCPP
	default:
		return LanguageUnknown
	}
}

type fileTask struct {
	path    string
	relPath string
	lang    Language
	parser  LanguageParser
}

type fileResult struct {
	symbols []CodeSymbol
	imports []UniversalImportInfo
	lang    Language
	relPath string
	err     error
}

func (idx *Index) BuildIndex() error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.Symbols = make(map[string]*CodeSymbol)
	idx.Imports = make([]UniversalImportInfo, 0)
	idx.Languages = make(map[Language][]string)
	idx.Files = make(map[string]Language)
	idx.Packages = make(map[string][]string)

	var fileTasks []fileTask

	err := filepath.WalkDir(idx.ProjectPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || strings.Contains(path, "vendor/") || strings.Contains(path, "node_modules/") {
			return nil
		}

		// skip index file to prevent self-indexing
		if strings.HasSuffix(path, "index.json") {
			return nil
		}

		if strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, ".test.js") || strings.HasSuffix(path, ".test.ts") {
			return nil
		}

		lang := idx.DetectLanguage(path)
		if lang == LanguageUnknown {
			return nil
		}

		parser, exists := idx.parsers[lang]
		if !exists {
			return nil
		}

		relPath, _ := filepath.Rel(idx.ProjectPath, path)
		
		fileTasks = append(fileTasks, fileTask{
			path:    path,
			relPath: relPath,
			lang:    lang,
			parser:  parser,
		})

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to collect files: %w", err)
	}

	numWorkers := runtime.NumCPU()
	if numWorkers > len(fileTasks) {
		numWorkers = len(fileTasks)
	}

	taskChan := make(chan fileTask, len(fileTasks))
	resultChan := make(chan fileResult, len(fileTasks))

	var wg sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range taskChan {
				symbols, imports, err := task.parser.ParseFile(task.path)
				resultChan <- fileResult{
					symbols: symbols,
					imports: imports,
					lang:    task.lang,
					relPath: task.relPath,
					err:     err,
				}
			}
		}()
	}

	for _, task := range fileTasks {
		taskChan <- task
	}
	close(taskChan)

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	var firstErr error
	for result := range resultChan {
		if result.err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("failed to parse file %s: %w", result.relPath, result.err)
			}
			continue
		}

		idx.Files[result.relPath] = result.lang

		if idx.Languages[result.lang] == nil {
			idx.Languages[result.lang] = make([]string, 0)
		}
		idx.Languages[result.lang] = append(idx.Languages[result.lang], result.relPath)

		for _, symbol := range result.symbols {
			symbol.Language = result.lang
			symbol.File = result.relPath
			idx.Symbols[symbol.ID] = &symbol

			if symbol.Package != "" {
				if idx.Packages[symbol.Package] == nil {
					idx.Packages[symbol.Package] = make([]string, 0)
				}
				
				idx.Packages[symbol.Package] = append(idx.Packages[symbol.Package], result.relPath)
			}
		}

		for _, imp := range result.imports {
			idx.Imports = append(idx.Imports, imp)
		}
	}

	if firstErr != nil {
		return firstErr
	}

	idx.LastUpdated = time.Now()
	return nil
}

func (idx *Index) SearchSymbols(query string, filters ...SearchFilter) []*CodeSymbol {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var results []*CodeSymbol
	query = strings.ToLower(query)

	for _, symbol := range idx.Symbols {
		if idx.matchesQuery(symbol, query) && idx.matchesFilters(symbol, filters) {
			results = append(results, symbol)
		}
	}

	return results
}

func (idx *Index) matchesQuery(symbol *CodeSymbol, query string) bool {
	return strings.Contains(strings.ToLower(symbol.Name), query) ||
		strings.Contains(strings.ToLower(symbol.FullName), query) ||
		strings.Contains(strings.ToLower(symbol.Comment), query) ||
		strings.Contains(strings.ToLower(symbol.DocString), query) ||
		strings.Contains(strings.ToLower(symbol.Signature), query)
}

func (idx *Index) matchesFilters(symbol *CodeSymbol, filters []SearchFilter) bool {
	for _, filter := range filters {
		if !filter(symbol) {
			return false
		}
	}
	return true
}

type SearchFilter func(*CodeSymbol) bool

func FilterByLanguage(lang Language) SearchFilter {
	return func(symbol *CodeSymbol) bool {
		return symbol.Language == lang
	}
}

func FilterByKind(kind SymbolKind) SearchFilter {
	return func(symbol *CodeSymbol) bool {
		return symbol.Kind == kind
	}
}

func FilterByPackage(pkg string) SearchFilter {
	return func(symbol *CodeSymbol) bool {
		return symbol.Package == pkg
	}
}

func FilterByVisibility(vis Visibility) SearchFilter {
	return func(symbol *CodeSymbol) bool {
		return symbol.Visibility == vis
	}
}

func (idx *Index) GetSymbol(id string) (*CodeSymbol, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	symbol, exists := idx.Symbols[id]
	return symbol, exists
}

func (idx *Index) GetSymbolsByLanguage(lang Language) []*CodeSymbol {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var results []*CodeSymbol
	for _, symbol := range idx.Symbols {
		if symbol.Language == lang {
			results = append(results, symbol)
		}
	}

	return results
}

func (idx *Index) GetSymbolsByPackage(pkg string) []*CodeSymbol {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var results []*CodeSymbol
	for _, symbol := range idx.Symbols {
		if symbol.Package == pkg {
			results = append(results, symbol)
		}
	}

	return results
}

func (idx *Index) GetSymbolsByKind(kind SymbolKind) []*CodeSymbol {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var results []*CodeSymbol
	for _, symbol := range idx.Symbols {
		if symbol.Kind == kind {
			results = append(results, symbol)
		}
	}

	return results
}

func (idx *Index) SaveToFile(filePath string) error {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal universal index: %w", err)
	}

	return os.WriteFile(filePath, data, 0644)
}

func (idx *Index) LoadFromFile(filePath string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read universal index file: %w", err)
	}

	temp := &Index{}
	if err := json.Unmarshal(data, temp); err != nil {
		return fmt.Errorf("failed to unmarshal universal index: %w", err)
	}

	idx.Symbols = temp.Symbols
	idx.Imports = temp.Imports
	idx.Languages = temp.Languages
	idx.Files = temp.Files
	idx.Packages = temp.Packages
	idx.LastUpdated = temp.LastUpdated
	idx.ProjectPath = temp.ProjectPath

	idx.registerBuiltinParsers()

	return nil
}

func (idx *Index) GetStats() map[string]any {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	languageStats := make(map[Language]int)
	kindStats := make(map[SymbolKind]int)

	for _, symbol := range idx.Symbols {
		languageStats[symbol.Language]++
		kindStats[symbol.Kind]++
	}

	return map[string]any{
		"total_symbols":  len(idx.Symbols),
		"total_imports":  len(idx.Imports),
		"total_files":    len(idx.Files),
		"total_packages": len(idx.Packages),
		"languages":      languageStats,
		"symbol_kinds":   kindStats,
		"last_updated":   idx.LastUpdated.Format(time.RFC3339),
		"project_path":   idx.ProjectPath,
	}
}

package index

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type JSParser struct {
	language Language
}

func NewJSParser() *JSParser {
	return &JSParser{language: LanguageJavaScript}
}

func NewTSParser() *JSParser {
	return &JSParser{language: LanguageTypeScript}
}

func (p *JSParser) GetLanguage() Language {
	return p.language
}

func (p *JSParser) GetSupportedExtensions() []string {
	if p.language == LanguageTypeScript {
		return []string{".ts", ".tsx"}
	}
	return []string{".js", ".jsx", ".mjs", ".cjs"}
}

func (p *JSParser) ParseFile(filePath string) ([]CodeSymbol, []UniversalImportInfo, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	return p.ParseContent(string(content), filePath)
}

func (p *JSParser) ParseContent(content string, filePath string) ([]CodeSymbol, []UniversalImportInfo, error) {
	var symbols []CodeSymbol
	var imports []UniversalImportInfo

	relPath := filepath.Base(filePath)
	lines := strings.Split(content, "\n")

	packageName := p.extractPackageName(content, filePath)

	imports = p.extractImports(content, relPath)
	symbols = append(symbols, p.extractFunctions(content, packageName, relPath)...)
	symbols = append(symbols, p.extractClasses(content, packageName, relPath)...)
	symbols = append(symbols, p.extractInterfaces(content, packageName, relPath)...)
	symbols = append(symbols, p.extractVariables(content, packageName, relPath)...)
	symbols = append(symbols, p.extractTypes(content, packageName, relPath)...)

	for i := range symbols {
		symbols[i].Language = p.language
		symbols[i].StartLine = p.findLineNumber(lines, symbols[i].Name)
	}

	return symbols, imports, nil
}

func (p *JSParser) extractPackageName(content string, filePath string) string {
	packageRe := regexp.MustCompile(`"name":\s*"([^"]+)"`)
	if match := packageRe.FindStringSubmatch(content); len(match) > 1 {
		return match[1]
	}

	base := filepath.Base(filePath)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func (p *JSParser) extractImports(content string, filePath string) []UniversalImportInfo {
	var imports []UniversalImportInfo

	importRe := regexp.MustCompile(`(?m)^import\s+(?:(?:\{([^}]+)\}|\*\s+as\s+(\w+)|(\w+))\s+from\s+)?['""]([^'"]+)['""]\s*;?$`)
	matches := importRe.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		path := match[4]
		alias := ""
		items := []string{}
		isDefault := false

		if match[1] != "" {
			itemStrs := strings.Split(match[1], ",")
			for _, item := range itemStrs {
				item = strings.TrimSpace(item)
				if item != "" {
					items = append(items, item)
				}
			}
		} else if match[2] != "" {
			alias = match[2]
		} else if match[3] != "" {
			alias = match[3]
			isDefault = true
		}

		imports = append(imports, UniversalImportInfo{
			Path:      path,
			Alias:     alias,
			IsDefault: isDefault,
			Items:     items,
			Language:  p.language,
			File:      filePath,
		})
	}

	requireRe := regexp.MustCompile(`const\s+(\w+)\s*=\s*require\s*\(\s*['""]([^'"]+)['"]\s*\)`)
	requireMatches := requireRe.FindAllStringSubmatch(content, -1)

	for _, match := range requireMatches {
		alias := match[1]
		path := match[2]

		imports = append(imports, UniversalImportInfo{
			Path:      path,
			Alias:     alias,
			IsDefault: true,
			Language:  p.language,
			File:      filePath,
		})
	}

	return imports
}

func (p *JSParser) extractFunctions(content string, packageName string, filePath string) []CodeSymbol {
	var symbols []CodeSymbol

	funcRe := regexp.MustCompile(`(?m)^(?:export\s+)?(?:async\s+)?function\s+(\w+)\s*\(([^)]*)\)\s*(?::\s*([^{]+))?\s*\{`)
	matches := funcRe.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		name := match[1]
		paramsStr := match[2]
		returnType := strings.TrimSpace(match[3])

		symbol := p.createFunctionSymbol(name, paramsStr, returnType, match[0], packageName, filePath)
		symbols = append(symbols, symbol)
	}

	arrowRe := regexp.MustCompile(`(?m)^(?:export\s+)?(?:const|let|var)\s+(\w+)\s*=\s*(?:async\s+)?\(([^)]*)\)\s*(?::\s*([^=]+))?\s*=>\s*`)
	arrowMatches := arrowRe.FindAllStringSubmatch(content, -1)

	for _, match := range arrowMatches {
		name := match[1]
		paramsStr := match[2]
		returnType := strings.TrimSpace(match[3])

		symbol := p.createFunctionSymbol(name, paramsStr, returnType, match[0], packageName, filePath)
		symbols = append(symbols, symbol)
	}

	return symbols
}

func (p *JSParser) createFunctionSymbol(name, paramsStr, returnType, fullMatch, packageName, filePath string) CodeSymbol {
	params := p.parseParameters(paramsStr)

	visibility := VisibilityPrivate
	if strings.Contains(fullMatch, "export") {
		visibility = VisibilityPublic
	}

	isAsync := strings.Contains(fullMatch, "async")

	signature := p.buildJSFunctionSignature(name, params, returnType, isAsync)
	fullName := fmt.Sprintf("%s.%s", packageName, name)

	return CodeSymbol{
		ID:         fullName,
		Name:       name,
		FullName:   fullName,
		Kind:       SymbolFunction,
		Package:    packageName,
		File:       filePath,
		Signature:  signature,
		Visibility: visibility,
		IsAsync:    isAsync,
		ReturnType: returnType,
		Parameters: params,
		Tags:       make(map[string]string),
		Metadata:   make(map[string]any),
	}
}

func (p *JSParser) extractClasses(content string, packageName string, filePath string) []CodeSymbol {
	var symbols []CodeSymbol

	classRe := regexp.MustCompile(`(?m)^(?:export\s+)?class\s+(\w+)(?:\s+extends\s+(\w+))?\s*\{`)
	matches := classRe.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		name := match[1]
		parent := match[2]

		visibility := VisibilityPrivate
		if strings.Contains(match[0], "export") {
			visibility = VisibilityPublic
		}

		signature := fmt.Sprintf("class %s", name)
		if parent != "" {
			signature += fmt.Sprintf(" extends %s", parent)
		}

		fullName := fmt.Sprintf("%s.%s", packageName, name)

		symbols = append(symbols, CodeSymbol{
			ID:         fullName,
			Name:       name,
			FullName:   fullName,
			Kind:       SymbolClass,
			Package:    packageName,
			File:       filePath,
			Signature:  signature,
			Visibility: visibility,
			Parent:     parent,
			Tags:       make(map[string]string),
			Metadata:   make(map[string]any),
		})
	}

	return symbols
}

func (p *JSParser) extractInterfaces(content string, packageName string, filePath string) []CodeSymbol {
	var symbols []CodeSymbol

	if p.language != LanguageTypeScript {
		return symbols
	}

	interfaceRe := regexp.MustCompile(`(?m)^(?:export\s+)?interface\s+(\w+)(?:\s+extends\s+([^{]+))?\s*\{`)
	matches := interfaceRe.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		name := match[1]
		parent := strings.TrimSpace(match[2])

		visibility := VisibilityPrivate
		if strings.Contains(match[0], "export") {
			visibility = VisibilityPublic
		}

		signature := fmt.Sprintf("interface %s", name)
		if parent != "" {
			signature += fmt.Sprintf(" extends %s", parent)
		}

		fullName := fmt.Sprintf("%s.%s", packageName, name)

		symbols = append(symbols, CodeSymbol{
			ID:         fullName,
			Name:       name,
			FullName:   fullName,
			Kind:       SymbolInterface,
			Package:    packageName,
			File:       filePath,
			Signature:  signature,
			Visibility: visibility,
			Parent:     parent,
			Tags:       make(map[string]string),
			Metadata:   make(map[string]any),
		})
	}

	return symbols
}

func (p *JSParser) extractVariables(content string, packageName string, filePath string) []CodeSymbol {
	var symbols []CodeSymbol

	// Go's regex doesn't support negative lookahead, so we'll use a simpler pattern
	varPattern := `(?m)^(?:export\s+)?(?:const|let|var)\s+(\w+)(?:\s*:\s*([^=]+))?\s*=\s*`
	varRe := regexp.MustCompile(varPattern)
	matches := varRe.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		name := match[1]
		varType := strings.TrimSpace(match[2])

		visibility := VisibilityPrivate
		if strings.Contains(match[0], "export") {
			visibility = VisibilityPublic
		}

		kind := SymbolVariable
		if strings.Contains(match[0], "const") {
			kind = SymbolConstant
		}

		signature := fmt.Sprintf("%s %s", kind, name)
		if varType != "" {
			signature += fmt.Sprintf(": %s", varType)
		}

		fullName := fmt.Sprintf("%s.%s", packageName, name)

		symbols = append(symbols, CodeSymbol{
			ID:         fullName,
			Name:       name,
			FullName:   fullName,
			Kind:       kind,
			Package:    packageName,
			File:       filePath,
			Signature:  signature,
			Visibility: visibility,
			Tags:       make(map[string]string),
			Metadata:   make(map[string]any),
		})
	}

	return symbols
}

func (p *JSParser) extractTypes(content string, packageName string, filePath string) []CodeSymbol {
	var symbols []CodeSymbol

	if p.language != LanguageTypeScript {
		return symbols
	}

	typeRe := regexp.MustCompile(`(?m)^(?:export\s+)?type\s+(\w+)(?:\s*<[^>]+>)?\s*=\s*([^;]+)`)
	matches := typeRe.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		name := match[1]
		typeDefinition := strings.TrimSpace(match[2])

		visibility := VisibilityPrivate
		if strings.Contains(match[0], "export") {
			visibility = VisibilityPublic
		}

		signature := fmt.Sprintf("type %s = %s", name, typeDefinition)
		fullName := fmt.Sprintf("%s.%s", packageName, name)

		symbols = append(symbols, CodeSymbol{
			ID:         fullName,
			Name:       name,
			FullName:   fullName,
			Kind:       SymbolType,
			Package:    packageName,
			File:       filePath,
			Signature:  signature,
			Visibility: visibility,
			Tags:       make(map[string]string),
			Metadata:   make(map[string]any),
		})
	}

	return symbols
}

func (p *JSParser) parseParameters(paramsStr string) []Parameter {
	var params []Parameter

	if strings.TrimSpace(paramsStr) == "" {
		return params
	}

	paramList := strings.Split(paramsStr, ",")
	for _, param := range paramList {
		param = strings.TrimSpace(param)
		if param == "" {
			continue
		}

		var name, paramType, defaultValue string
		isOptional := false
		isVariadic := false

		if strings.Contains(param, "...") {
			isVariadic = true
			param = strings.TrimPrefix(param, "...")
		}

		if strings.Contains(param, "?") {
			isOptional = true
			param = strings.Replace(param, "?", "", 1)
		}

		if strings.Contains(param, "=") {
			parts := strings.SplitN(param, "=", 2)
			param = strings.TrimSpace(parts[0])
			defaultValue = strings.TrimSpace(parts[1])
		}

		if strings.Contains(param, ":") {
			parts := strings.SplitN(param, ":", 2)
			name = strings.TrimSpace(parts[0])
			paramType = strings.TrimSpace(parts[1])
		} else {
			name = param
			paramType = "any"
		}

		params = append(params, Parameter{
			Name:         name,
			Type:         paramType,
			DefaultValue: defaultValue,
			IsOptional:   isOptional,
			IsVariadic:   isVariadic,
		})
	}

	return params
}

func (p *JSParser) buildJSFunctionSignature(name string, params []Parameter, returnType string, isAsync bool) string {
	var sig strings.Builder

	if isAsync {
		sig.WriteString("async ")
	}

	sig.WriteString("function ")
	sig.WriteString(name)
	sig.WriteString("(")

	var paramStrs []string
	for _, param := range params {
		paramStr := ""
		if param.IsVariadic {
			paramStr += "..."
		}
		paramStr += param.Name
		if param.IsOptional {
			paramStr += "?"
		}
		if param.Type != "" && param.Type != "any" {
			paramStr += ": " + param.Type
		}
		if param.DefaultValue != "" {
			paramStr += " = " + param.DefaultValue
		}
		paramStrs = append(paramStrs, paramStr)
	}

	sig.WriteString(strings.Join(paramStrs, ", "))
	sig.WriteString(")")

	if returnType != "" {
		sig.WriteString(": ")
		sig.WriteString(returnType)
	}

	return sig.String()
}

func (p *JSParser) findLineNumber(lines []string, symbolName string) int {
	for i, line := range lines {
		if strings.Contains(line, symbolName) {
			return i + 1
		}
	}
	return 1
}

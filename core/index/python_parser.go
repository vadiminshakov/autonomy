package index

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type PythonParser struct{}

func NewPythonParser() *PythonParser {
	return &PythonParser{}
}

func (p *PythonParser) GetLanguage() Language {
	return LanguagePython
}

func (p *PythonParser) GetSupportedExtensions() []string {
	return []string{".py", ".pyx", ".pyi"}
}

func (p *PythonParser) ParseFile(filePath string) ([]CodeSymbol, []UniversalImportInfo, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	return p.ParseContent(string(content), filePath)
}

func (p *PythonParser) ParseContent(content string, filePath string) ([]CodeSymbol, []UniversalImportInfo, error) {
	var symbols []CodeSymbol
	var imports []UniversalImportInfo

	relPath := filepath.Base(filePath)
	lines := strings.Split(content, "\n")

	packageName := p.extractPackageName(filePath)

	imports = p.extractImports(content, relPath)
	symbols = append(symbols, p.extractFunctions(content, packageName, relPath, lines)...)
	symbols = append(symbols, p.extractClasses(content, packageName, relPath, lines)...)
	symbols = append(symbols, p.extractVariables(content, packageName, relPath, lines)...)

	for i := range symbols {
		symbols[i].Language = LanguagePython
	}

	return symbols, imports, nil
}

func (p *PythonParser) extractPackageName(filePath string) string {
	dir := filepath.Dir(filePath)
	if filepath.Base(dir) == "." {
		base := filepath.Base(filePath)
		return strings.TrimSuffix(base, filepath.Ext(base))
	}
	return filepath.Base(dir)
}

func (p *PythonParser) extractImports(content string, filePath string) []UniversalImportInfo {
	var imports []UniversalImportInfo

	importRe := regexp.MustCompile(`(?m)^(?:from\s+([a-zA-Z_][a-zA-Z0-9_]*(?:\.[a-zA-Z_][a-zA-Z0-9_]*)*)\s+)?import\s+([a-zA-Z_][a-zA-Z0-9_]*(?:\s*,\s*[a-zA-Z_][a-zA-Z0-9_]*)*)(?:\s+as\s+([a-zA-Z_][a-zA-Z0-9_]*))?`)
	matches := importRe.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		fromModule := match[1]
		importItems := match[2]
		alias := match[3]

		if fromModule != "" {
			items := strings.Split(importItems, ",")
			for i, item := range items {
				items[i] = strings.TrimSpace(item)
			}

			imports = append(imports, UniversalImportInfo{
				Path:     fromModule,
				Items:    items,
				Alias:    alias,
				Language: LanguagePython,
				File:     filePath,
			})
		} else {
			imports = append(imports, UniversalImportInfo{
				Path:     importItems,
				Alias:    alias,
				Language: LanguagePython,
				File:     filePath,
			})
		}
	}

	return imports
}

func (p *PythonParser) extractFunctions(content string, packageName string, filePath string, lines []string) []CodeSymbol {
	var symbols []CodeSymbol

	funcRe := regexp.MustCompile(`(?m)^(\s*)def\s+(\w+)\s*\(([^)]*)\)\s*(?:->\s*([^:]+))?\s*:`)
	matches := funcRe.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		indent := match[1]
		name := match[2]
		paramsStr := match[3]
		returnType := strings.TrimSpace(match[4])

		params := p.parseParameters(paramsStr)
		
		visibility := VisibilityPrivate
		if !strings.HasPrefix(name, "_") {
			visibility = VisibilityPublic
		}

		isStatic := false
		kind := SymbolFunction
		
		if len(params) > 0 && (params[0].Name == "self" || params[0].Name == "cls") {
			kind = SymbolMethod
			if params[0].Name == "cls" {
				isStatic = true
			}
		}

		docstring := p.extractDocstring(content, match[0])
		
		signature := p.buildPythonFunctionSignature(name, params, returnType, isStatic)
		fullName := fmt.Sprintf("%s.%s", packageName, name)

		lineNum := p.findLineNumber(lines, match[0])

		symbols = append(symbols, CodeSymbol{
			ID:         fullName,
			Name:       name,
			FullName:   fullName,
			Kind:       kind,
			Package:    packageName,
			File:       filePath,
			StartLine:  lineNum,
			Signature:  signature,
			DocString:  docstring,
			Visibility: visibility,
			IsStatic:   isStatic,
			ReturnType: returnType,
			Parameters: params,
			Tags:       make(map[string]string),
			Metadata:   map[string]any{"indent": len(indent)},
		})
	}

	return symbols
}

func (p *PythonParser) extractClasses(content string, packageName string, filePath string, lines []string) []CodeSymbol {
	var symbols []CodeSymbol

	classRe := regexp.MustCompile(`(?m)^(\s*)class\s+(\w+)(?:\s*\(([^)]*)\))?\s*:`)
	matches := classRe.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		indent := match[1]
		name := match[2]
		parentStr := match[3]

		var parent string
		if parentStr != "" {
			parents := strings.Split(parentStr, ",")
			if len(parents) > 0 {
				parent = strings.TrimSpace(parents[0])
			}
		}

		visibility := VisibilityPrivate
		if !strings.HasPrefix(name, "_") {
			visibility = VisibilityPublic
		}

		docstring := p.extractDocstring(content, match[0])
		
		signature := fmt.Sprintf("class %s", name)
		if parent != "" {
			signature += fmt.Sprintf("(%s)", parent)
		}

		fullName := fmt.Sprintf("%s.%s", packageName, name)
		lineNum := p.findLineNumber(lines, match[0])

		symbols = append(symbols, CodeSymbol{
			ID:         fullName,
			Name:       name,
			FullName:   fullName,
			Kind:       SymbolClass,
			Package:    packageName,
			File:       filePath,
			StartLine:  lineNum,
			Signature:  signature,
			DocString:  docstring,
			Visibility: visibility,
			Parent:     parent,
			Tags:       make(map[string]string),
			Metadata:   map[string]any{"indent": len(indent)},
		})
	}

	return symbols
}

func (p *PythonParser) extractVariables(content string, packageName string, filePath string, lines []string) []CodeSymbol {
	var symbols []CodeSymbol

	varRe := regexp.MustCompile(`(?m)^(\s*)([A-Z_][A-Z0-9_]*)\s*=\s*(.+)`)
	matches := varRe.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		indent := match[1]
		name := match[2]
		value := strings.TrimSpace(match[3])

		if len(indent) > 0 {
			continue
		}

		visibility := VisibilityPrivate
		if !strings.HasPrefix(name, "_") {
			visibility = VisibilityPublic
		}

		signature := fmt.Sprintf("%s = %s", name, value)
		if len(value) > 50 {
			signature = fmt.Sprintf("%s = %s...", name, value[:50])
		}

		fullName := fmt.Sprintf("%s.%s", packageName, name)
		lineNum := p.findLineNumber(lines, match[0])

		symbols = append(symbols, CodeSymbol{
			ID:         fullName,
			Name:       name,
			FullName:   fullName,
			Kind:       SymbolConstant,
			Package:    packageName,
			File:       filePath,
			StartLine:  lineNum,
			Signature:  signature,
			Visibility: visibility,
			Tags:       make(map[string]string),
			Metadata:   make(map[string]any),
		})
	}

	return symbols
}

func (p *PythonParser) parseParameters(paramsStr string) []Parameter {
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

		if strings.HasPrefix(param, "**") {
			isVariadic = true
			param = param[2:]
		} else if strings.HasPrefix(param, "*") {
			isVariadic = true
			param = param[1:]
		}

		if strings.Contains(param, "=") {
			parts := strings.SplitN(param, "=", 2)
			param = strings.TrimSpace(parts[0])
			defaultValue = strings.TrimSpace(parts[1])
			isOptional = true
		}

		if strings.Contains(param, ":") {
			parts := strings.SplitN(param, ":", 2)
			name = strings.TrimSpace(parts[0])
			paramType = strings.TrimSpace(parts[1])
		} else {
			name = param
			paramType = "Any"
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

func (p *PythonParser) buildPythonFunctionSignature(name string, params []Parameter, returnType string, isStatic bool) string {
	var sig strings.Builder

	if isStatic {
		sig.WriteString("@staticmethod\n")
	}

	sig.WriteString("def ")
	sig.WriteString(name)
	sig.WriteString("(")

	var paramStrs []string
	for _, param := range params {
		paramStr := ""
		if param.IsVariadic {
			if param.Name == "kwargs" || strings.Contains(param.Type, "dict") {
				paramStr += "**"
			} else {
				paramStr += "*"
			}
		}
		paramStr += param.Name
		if param.Type != "" && param.Type != "Any" {
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
		sig.WriteString(" -> ")
		sig.WriteString(returnType)
	}

	return sig.String()
}

func (p *PythonParser) extractDocstring(content string, defLine string) string {
	defIndex := strings.Index(content, defLine)
	if defIndex == -1 {
		return ""
	}

	remaining := content[defIndex+len(defLine):]
	lines := strings.Split(remaining, "\n")
	
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		
		if strings.HasPrefix(trimmed, `"""`) || strings.HasPrefix(trimmed, `'''`) {
			quote := trimmed[:3]
			docstring := trimmed[3:]
			
			if strings.HasSuffix(trimmed, quote) && len(trimmed) > 6 {
				return strings.TrimSuffix(docstring, quote)
			}
			
			for j := i + 1; j < len(lines); j++ {
				if strings.Contains(lines[j], quote) {
					endIndex := strings.Index(lines[j], quote)
					docstring += "\n" + lines[j][:endIndex]
					return docstring
				}
				docstring += "\n" + lines[j]
			}
		}
		
		break
	}
	
	return ""
}

func (p *PythonParser) findLineNumber(lines []string, matchText string) int {
	for i, line := range lines {
		if strings.Contains(line, matchText) {
			return i + 1
		}
	}
	return 1
}
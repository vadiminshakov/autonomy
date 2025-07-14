package index

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

type GoParser struct{}

func NewGoParser() *GoParser {
	return &GoParser{}
}

func (p *GoParser) GetLanguage() Language {
	return LanguageGo
}

func (p *GoParser) GetSupportedExtensions() []string {
	return []string{".go"}
}

func (p *GoParser) ParseFile(filePath string) ([]CodeSymbol, []UniversalImportInfo, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	return p.ParseContent(string(content), filePath)
}

func (p *GoParser) ParseContent(content string, filePath string) ([]CodeSymbol, []UniversalImportInfo, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, content, parser.ParseComments)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse Go file %s: %w", filePath, err)
	}

	var symbols []CodeSymbol
	var imports []UniversalImportInfo

	packageName := node.Name.Name
	relPath := filepath.Base(filePath)

	for _, imp := range node.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		alias := ""
		if imp.Name != nil {
			alias = imp.Name.Name
		}

		imports = append(imports, UniversalImportInfo{
			Path:      importPath,
			Alias:     alias,
			Language:  LanguageGo,
			File:      relPath,
		})
	}

	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			if symbol := p.parseFunction(x, fset, packageName, relPath); symbol != nil {
				symbols = append(symbols, *symbol)
			}
		case *ast.TypeSpec:
			if symbol := p.parseType(x, fset, packageName, relPath); symbol != nil {
				symbols = append(symbols, *symbol)
			}
		case *ast.GenDecl:
			if symbol := p.parseGenDecl(x, fset, packageName, relPath); symbol != nil {
				symbols = append(symbols, *symbol)
			}
		}
		
		return true
	})

	return symbols, imports, nil
}

func (p *GoParser) parseFunction(fn *ast.FuncDecl, fset *token.FileSet, packageName, filePath string) *CodeSymbol {
	if fn.Name == nil {
		return nil
	}

	pos := fset.Position(fn.Pos())
	endPos := fset.Position(fn.End())
	funcName := fn.Name.Name
	
	var receiver string
	var kind SymbolKind = SymbolFunction
	
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		if field := fn.Recv.List[0]; field.Type != nil {
			receiver = fmt.Sprintf("%s", field.Type)
			receiver = strings.TrimPrefix(receiver, "*")
			kind = SymbolMethod
		}
	}

	var params []Parameter
	if fn.Type.Params != nil {
		for _, param := range fn.Type.Params.List {
			paramType := fmt.Sprintf("%s", param.Type)
			if len(param.Names) > 0 {
				for _, name := range param.Names {
					params = append(params, Parameter{
						Name: name.Name,
						Type: paramType,
					})
				}
			} else {
				params = append(params, Parameter{
					Name: "",
					Type: paramType,
				})
			}
		}
	}

	var returnType string
	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		if len(fn.Type.Results.List) == 1 && len(fn.Type.Results.List[0].Names) == 0 {
			returnType = fmt.Sprintf("%s", fn.Type.Results.List[0].Type)
		} else {
			var returns []string
			for _, result := range fn.Type.Results.List {
				if len(result.Names) > 0 {
					for _, name := range result.Names {
						returns = append(returns, fmt.Sprintf("%s %s", name.Name, result.Type))
					}
				} else {
					returns = append(returns, fmt.Sprintf("%s", result.Type))
				}
			}
			returnType = "(" + strings.Join(returns, ", ") + ")"
		}
	}

	signature := p.buildFunctionSignature(funcName, params, returnType, receiver)
	
	var comment string
	if fn.Doc != nil {
		comment = strings.TrimSpace(fn.Doc.Text())
	}

	visibility := VisibilityPrivate
	if ast.IsExported(funcName) {
		visibility = VisibilityPublic
	}

	fullName := fmt.Sprintf("%s.%s", packageName, funcName)
	if receiver != "" {
		fullName = fmt.Sprintf("%s.%s.%s", packageName, receiver, funcName)
	}

	id := fullName

	return &CodeSymbol{
		ID:          id,
		Name:        funcName,
		FullName:    fullName,
		Kind:        kind,
		Language:    LanguageGo,
		Package:     packageName,
		File:        filePath,
		StartLine:   pos.Line,
		EndLine:     endPos.Line,
		StartColumn: pos.Column,
		EndColumn:   endPos.Column,
		Signature:   signature,
		Comment:     comment,
		Visibility:  visibility,
		ReturnType:  returnType,
		Parameters:  params,
		Parent:      receiver,
		Tags:        make(map[string]string),
		Metadata:    make(map[string]any),
	}
}

func (p *GoParser) parseType(ts *ast.TypeSpec, fset *token.FileSet, packageName, filePath string) *CodeSymbol {
	if ts.Name == nil {
		return nil
	}

	pos := fset.Position(ts.Pos())
	endPos := fset.Position(ts.End())
	typeName := ts.Name.Name

	var kind SymbolKind
	var fields []Field
	var methods []string

	switch t := ts.Type.(type) {
	case *ast.StructType:
		kind = SymbolStruct
		if t.Fields != nil {
			for _, field := range t.Fields.List {
				fieldType := fmt.Sprintf("%s", field.Type)
				fieldVis := VisibilityPrivate
				
				if len(field.Names) > 0 {
					for _, name := range field.Names {
						if ast.IsExported(name.Name) {
							fieldVis = VisibilityPublic
						}
						fields = append(fields, Field{
							Name:       name.Name,
							Type:       fieldType,
							Visibility: fieldVis,
						})
					}
				} else {
					fields = append(fields, Field{
						Name:       "",
						Type:       fieldType,
						Visibility: fieldVis,
					})
				}
			}
		}
	case *ast.InterfaceType:
		kind = SymbolInterface
		if t.Methods != nil {
			for _, method := range t.Methods.List {
				if len(method.Names) > 0 {
					for _, name := range method.Names {
						methods = append(methods, name.Name)
					}
				}
			}
		}
	default:
		kind = SymbolType
	}

	var comment string
	if ts.Doc != nil {
		comment = strings.TrimSpace(ts.Doc.Text())
	}

	visibility := VisibilityPrivate
	if ast.IsExported(typeName) {
		visibility = VisibilityPublic
	}

	fullName := fmt.Sprintf("%s.%s", packageName, typeName)
	id := fullName

	return &CodeSymbol{
		ID:          id,
		Name:        typeName,
		FullName:    fullName,
		Kind:        kind,
		Language:    LanguageGo,
		Package:     packageName,
		File:        filePath,
		StartLine:   pos.Line,
		EndLine:     endPos.Line,
		StartColumn: pos.Column,
		EndColumn:   endPos.Column,
		Signature:   fmt.Sprintf("type %s %s", typeName, kind),
		Comment:     comment,
		Visibility:  visibility,
		Fields:      fields,
		Methods:     methods,
		Tags:        make(map[string]string),
		Metadata:    make(map[string]any),
	}
}

func (p *GoParser) parseGenDecl(gd *ast.GenDecl, fset *token.FileSet, packageName, filePath string) *CodeSymbol {
	if gd.Tok != token.CONST && gd.Tok != token.VAR {
		return nil
	}

	pos := fset.Position(gd.Pos())
	endPos := fset.Position(gd.End())

	var kind SymbolKind
	if gd.Tok == token.CONST {
		kind = SymbolConstant
	} else {
		kind = SymbolVariable
	}

	var names []string
	var typeStr string
	
	for _, spec := range gd.Specs {
		if valueSpec, ok := spec.(*ast.ValueSpec); ok {
			if valueSpec.Type != nil {
				typeStr = fmt.Sprintf("%s", valueSpec.Type)
			}
			for _, name := range valueSpec.Names {
				names = append(names, name.Name)
			}
		}
	}

	if len(names) == 0 {
		return nil
	}

	var comment string
	if gd.Doc != nil {
		comment = strings.TrimSpace(gd.Doc.Text())
	}

	visibility := VisibilityPrivate
	if len(names) > 0 && ast.IsExported(names[0]) {
		visibility = VisibilityPublic
	}

	name := strings.Join(names, ", ")
	fullName := fmt.Sprintf("%s.%s", packageName, name)
	id := fullName

	return &CodeSymbol{
		ID:          id,
		Name:        name,
		FullName:    fullName,
		Kind:        kind,
		Language:    LanguageGo,
		Package:     packageName,
		File:        filePath,
		StartLine:   pos.Line,
		EndLine:     endPos.Line,
		StartColumn: pos.Column,
		EndColumn:   endPos.Column,
		Signature:   fmt.Sprintf("%s %s %s", gd.Tok.String(), name, typeStr),
		Comment:     comment,
		Visibility:  visibility,
		Tags:        make(map[string]string),
		Metadata:    make(map[string]any),
	}
}

func (p *GoParser) buildFunctionSignature(name string, params []Parameter, returnType, receiver string) string {
	var sig strings.Builder
	
	if receiver != "" {
		sig.WriteString(fmt.Sprintf("func (%s) ", receiver))
	} else {
		sig.WriteString("func ")
	}
	
	sig.WriteString(name)
	sig.WriteString("(")
	
	var paramStrs []string
	for _, param := range params {
		if param.Name != "" {
			paramStrs = append(paramStrs, fmt.Sprintf("%s %s", param.Name, param.Type))
		} else {
			paramStrs = append(paramStrs, param.Type)
		}
	}
	sig.WriteString(strings.Join(paramStrs, ", "))
	sig.WriteString(")")
	
	if returnType != "" {
		sig.WriteString(" ")
		sig.WriteString(returnType)
	}
	
	return sig.String()
}
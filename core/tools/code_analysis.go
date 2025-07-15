package tools

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
)

func init() {
	Register("analyze_code_go", AnalyzeCodeGo)
}

// AnalyzeCode analyzes Go code structure and complexity before suggesting changes
func AnalyzeCodeGo(args map[string]interface{}) (string, error) {
	pathVal, ok := args["path"].(string)
	if !ok {
		return "", fmt.Errorf("parameter 'path' must be a non-empty string")
	}

	content, err := os.ReadFile(pathVal)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %v", pathVal, err)
	}

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, pathVal, content, parser.ParseComments)
	if err != nil {
		return "", fmt.Errorf("failed to parse Go file %s: %v", pathVal, err)
	}

	analysis := analyzeGoFile(node, fset)

	return analysis, nil
}

// analyzeGoFile performs detailed analysis of a Go file
func analyzeGoFile(node *ast.File, fset *token.FileSet) string {
	var result strings.Builder
	
	result.WriteString(fmt.Sprintf("Package: %s\n", node.Name.Name))
	result.WriteString(fmt.Sprintf("Imports: %d\n", len(node.Imports)))
	
	// count declarations
	var (
		typeCount     int
		funcCount     int
		varCount      int
		constCount    int
		maxFuncLines  int
		maxFuncName   string
		totalLines    int
	)
	
	// analyze each declaration
	for _, decl := range node.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			switch d.Tok {
			case token.TYPE:
				typeCount++
			case token.VAR:
				varCount++
			case token.CONST:
				constCount++
			}

		case *ast.FuncDecl:
			funcCount++
			if d.Name != nil {
				start := fset.Position(d.Pos())
				end := fset.Position(d.End())
				funcLines := end.Line - start.Line + 1
				if funcLines > maxFuncLines {
					maxFuncLines = funcLines
					maxFuncName = d.Name.Name
				}
			}
		}
	}
	
	// calculate total lines
	if len(node.Decls) > 0 {
		lastDecl := node.Decls[len(node.Decls)-1]
		totalLines = fset.Position(lastDecl.End()).Line
	}
	
	result.WriteString(fmt.Sprintf("Total lines: %d\n", totalLines))
	result.WriteString(fmt.Sprintf("Functions: %d\n", funcCount))
	result.WriteString(fmt.Sprintf("Types: %d\n", typeCount))
	result.WriteString(fmt.Sprintf("Variables: %d\n", varCount))
	result.WriteString(fmt.Sprintf("Constants: %d\n", constCount))
	
	if maxFuncName != "" {
		result.WriteString(fmt.Sprintf("Largest function: %s (%d lines)\n", maxFuncName, maxFuncLines))
	}
	
	// complexity assessment
	complexity := assessComplexity(totalLines, funcCount, maxFuncLines)
	result.WriteString(fmt.Sprintf("Complexity: %s\n", complexity))
	
	// recommendations
	recommendations := generateRecommendations(totalLines, funcCount, maxFuncLines, typeCount)
	if recommendations != "" {
		result.WriteString(fmt.Sprintf("Recommendations: %s\n", recommendations))
	}
	
	return result.String()
}

// assessComplexity determines if code is appropriately complex
func assessComplexity(totalLines, funcCount, maxFuncLines int) string {
	if totalLines < 50 {
		return "LOW - Simple, well-structured"
	}

	if totalLines < 200 && funcCount < 10 && maxFuncLines < 50 {
		return "MEDIUM - Appropriately structured"
	}

	if totalLines < 500 && funcCount < 20 && maxFuncLines < 100 {
		return "HIGH - Complex but manageable"
	}

	return "VERY HIGH - Consider refactoring"
}

// generateRecommendations provides specific suggestions
func generateRecommendations(totalLines, funcCount, maxFuncLines, typeCount int) string {
	var recommendations []string
	
	if maxFuncLines > 100 {
		recommendations = append(recommendations, "Consider breaking down large functions")
	}
	
	if funcCount > 20 && typeCount < 3 {
		recommendations = append(recommendations, "Consider using more types to organize code")
	}
	
	if totalLines > 500 && funcCount < 10 {
		recommendations = append(recommendations, "Consider breaking into multiple files")
	}
	
	if totalLines < 100 && funcCount > 15 {
		recommendations = append(recommendations, "Code may be over-engineered")
	}
	
	return strings.Join(recommendations, "; ")
}
package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vadiminshakov/autonomy/core/index"
)

func init() {
	Register("search_index", searchIndex)
	Register("get_index_stats", getIndexStats)
	Register("get_function", getFunction)
	Register("get_type", getType)
	Register("get_package_info", getPackageInfo)
}

func searchIndex(args map[string]any) (string, error) {
	queryArg, exists := args["query"]
	if !exists {
		return "", fmt.Errorf("parameter 'query' is required")
	}

	query, ok := queryArg.(string)
	if !ok {
		return "", fmt.Errorf("parameter 'query' must be a string")
	}

	manager := index.GetIndexManager()
	symbols := manager.SearchSymbols(query)

	var results strings.Builder
	results.WriteString(fmt.Sprintf("Search results for query: '%s'\n\n", query))

	if manager.IsBuilding() {
		results.WriteString("Index is still building in background. Results may be incomplete.\n\n")
	}

	if len(symbols) > 0 {
		languageGroups := make(map[index.Language][]*index.CodeSymbol)
		for _, symbol := range symbols {
			languageGroups[symbol.Language] = append(languageGroups[symbol.Language], symbol)
		}

		for lang, langSymbols := range languageGroups {
			results.WriteString(fmt.Sprintf("%s:\n", strings.ToUpper(string(lang))))

			kindGroups := make(map[index.SymbolKind][]*index.CodeSymbol)
			for _, symbol := range langSymbols {
				kindGroups[symbol.Kind] = append(kindGroups[symbol.Kind], symbol)
			}

			for kind, kindSymbols := range kindGroups {
				results.WriteString(fmt.Sprintf("  %s:\n", strings.ToUpper(string(kind))))
				for _, symbol := range kindSymbols {
					results.WriteString(fmt.Sprintf("    • %s (%s:%d)\n", symbol.Signature, symbol.File, symbol.StartLine))
					if symbol.Comment != "" {
						comment := strings.Split(symbol.Comment, "\n")[0]
						if len(comment) > 80 {
							comment = comment[:80] + "..."
						}
						results.WriteString(fmt.Sprintf("      %s\n", comment))
					}
					if symbol.DocString != "" {
						docstring := strings.Split(symbol.DocString, "\n")[0]
						if len(docstring) > 80 {
							docstring = docstring[:80] + "..."
						}
						results.WriteString(fmt.Sprintf("      %s\n", docstring))
					}
				}
				results.WriteString("\n")
			}
		}
	} else {
		if manager.IsBuilding() {
			results.WriteString("Index is still building. Try again later.\n")
		} else {
			results.WriteString("No results found.\n")
		}
	}

	return results.String(), nil
}

func getIndexStats(args map[string]any) (string, error) {
	manager := index.GetIndexManager()
	stats := manager.GetStats()
	statsJSON, _ := json.MarshalIndent(stats, "", "  ")

	return fmt.Sprintf("Universal index statistics:\n%s", string(statsJSON)), nil
}

func getFunction(args map[string]any) (string, error) {
	keyArg, exists := args["key"]
	if !exists {
		return "", fmt.Errorf("parameter 'key' is required")
	}

	key, ok := keyArg.(string)
	if !ok {
		return "", fmt.Errorf("parameter 'key' must be a string")
	}

	manager := index.GetIndexManager()
	symbol, exists := manager.GetSymbol(key)
	if !exists {
		return fmt.Sprintf("Symbol with key '%s' not found.", key), nil
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("%s: %s\n\n", strings.ToUpper(string(symbol.Kind)), symbol.Name))
	result.WriteString(fmt.Sprintf("Language: %s\n", symbol.Language))
	result.WriteString(fmt.Sprintf("Package: %s\n", symbol.Package))
	result.WriteString(fmt.Sprintf("File: %s:%d\n", symbol.File, symbol.StartLine))
	result.WriteString(fmt.Sprintf("Signature: %s\n", symbol.Signature))
	result.WriteString(fmt.Sprintf("Visibility: %s\n", symbol.Visibility))

	if symbol.Parent != "" {
		result.WriteString(fmt.Sprintf("Parent: %s\n", symbol.Parent))
	}

	if len(symbol.Parameters) > 0 {
		result.WriteString("Parameters:\n")
		for _, param := range symbol.Parameters {
			paramStr := fmt.Sprintf("  • %s: %s", param.Name, param.Type)
			if param.DefaultValue != "" {
				paramStr += fmt.Sprintf(" = %s", param.DefaultValue)
			}
			if param.IsOptional {
				paramStr += " (optional)"
			}
			if param.IsVariadic {
				paramStr += " (variadic)"
			}
			result.WriteString(paramStr + "\n")
		}
	}

	if symbol.ReturnType != "" {
		result.WriteString(fmt.Sprintf("Returns: %s\n", symbol.ReturnType))
	}

	if symbol.Comment != "" {
		result.WriteString(fmt.Sprintf("\nComment:\n%s\n", symbol.Comment))
	}

	if symbol.DocString != "" {
		result.WriteString(fmt.Sprintf("\nDocstring:\n%s\n", symbol.DocString))
	}

	return result.String(), nil
}

func getType(args map[string]any) (string, error) {
	return getFunction(args)
}

func getPackageInfo(args map[string]any) (string, error) {
	packageArg, exists := args["package"]
	if !exists {
		return "", fmt.Errorf("parameter 'package' is required")
	}

	packageName, ok := packageArg.(string)
	if !ok {
		return "", fmt.Errorf("parameter 'package' must be a string")
	}

	manager := index.GetIndexManager()
	symbols := manager.GetSymbolsByPackage(packageName)

	if len(symbols) == 0 {
		return fmt.Sprintf("Package '%s' not found.", packageName), nil
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("PACKAGE: %s\n\n", packageName))

	languageGroups := make(map[index.Language][]*index.CodeSymbol)
	for _, symbol := range symbols {
		languageGroups[symbol.Language] = append(languageGroups[symbol.Language], symbol)
	}

	for lang, langSymbols := range languageGroups {
		result.WriteString(fmt.Sprintf("%s:\n", strings.ToUpper(string(lang))))

		kindGroups := make(map[index.SymbolKind][]*index.CodeSymbol)
		for _, symbol := range langSymbols {
			kindGroups[symbol.Kind] = append(kindGroups[symbol.Kind], symbol)
		}

		for kind, kindSymbols := range kindGroups {
			result.WriteString(fmt.Sprintf("  %s:\n", strings.ToUpper(string(kind))))
			for _, symbol := range kindSymbols {
				result.WriteString(fmt.Sprintf("    • %s (%s:%d)\n", symbol.Name, symbol.File, symbol.StartLine))
			}
		}
		result.WriteString("\n")
	}

	return result.String(), nil
}

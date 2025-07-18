package index

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDetectLanguage(t *testing.T) {
	cases := map[string]Language{
		"main.go":     LanguageGo,
		"script.js":   LanguageJavaScript,
		"module.ts":   LanguageTypeScript,
		"app.py":      LanguagePython,
		"unknown.xyz": LanguageUnknown,
	}

	idx := NewIndex(".")
	for file, want := range cases {
		require.Equal(t, want, idx.DetectLanguage(file), "file %s", file)
	}
}

func TestSearchSymbolsAndFilters(t *testing.T) {
	idx := NewIndex(".")

	s1 := &CodeSymbol{ID: "1", Name: "Add", Kind: SymbolFunction, Language: LanguageGo, Package: "math", Signature: "func Add(int,int) int", Comment: "adds two numbers"}
	s2 := &CodeSymbol{ID: "2", Name: "Sub", Kind: SymbolFunction, Language: LanguageGo, Package: "math", Signature: "func Sub(int,int) int"}
	s3 := &CodeSymbol{ID: "3", Name: "connect", Kind: SymbolFunction, Language: LanguageJavaScript, Package: "net", Comment: "connect to server"}

	idx.Symbols[s1.ID] = s1
	idx.Symbols[s2.ID] = s2
	idx.Symbols[s3.ID] = s3

	// basic query matches name case-insensitively
	res := idx.SearchSymbols("add")
	require.Len(t, res, 1)
	require.Equal(t, "1", res[0].ID)

	// filter by language
	res = idx.SearchSymbols("", FilterByLanguage(LanguageGo))
	require.Len(t, res, 2)

	// filter by package
	res = idx.SearchSymbols("", FilterByPackage("math"))
	require.Len(t, res, 2)

	// combined filter yields 1
	res = idx.SearchSymbols("sub", FilterByLanguage(LanguageGo), FilterByPackage("math"))
	require.Len(t, res, 1)
	require.Equal(t, "2", res[0].ID)
}

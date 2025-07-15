package tools

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
)

func init() {
	Register("calc", calc)
}

// calc evaluates a basic arithmetic expression such as "2+2*3".
// Supported operators: +, -, *, / and parentheses.
func calc(args map[string]interface{}) (string, error) {
	expr, ok := args["expr"].(string)
	if !ok {
		expr, ok = args["expression"].(string)
	}

	if !ok || strings.TrimSpace(expr) == "" {
		return "", fmt.Errorf("parameter 'expr' or 'expression' must be a non-empty string")
	}

	astExpr, err := parser.ParseExpr(expr)
	if err != nil {
		return "", fmt.Errorf("expression parse error: %v", err)
	}

	val, err := evalAST(astExpr)
	if err != nil {
		return "", err
	}

	if val == float64(int64(val)) {
		return fmt.Sprintf("%d", int64(val)), nil
	}

	return fmt.Sprintf("%g", val), nil
}

func evalAST(expr ast.Expr) (float64, error) {
	switch n := expr.(type) {
	case *ast.BasicLit:
		switch n.Kind {
		case token.INT:
			return strconv.ParseFloat(n.Value, 64)
		case token.FLOAT:
			return strconv.ParseFloat(n.Value, 64)
		default:
			return 0, fmt.Errorf("unsupported literal: %s", n.Value)
		}

	case *ast.BinaryExpr:
		left, err := evalAST(n.X)
		if err != nil {
			return 0, err
		}
		right, err := evalAST(n.Y)
		if err != nil {
			return 0, err
		}

		switch n.Op {
		case token.ADD:
			return left + right, nil
		case token.SUB:
			return left - right, nil
		case token.MUL:
			return left * right, nil
		case token.QUO:
			return left / right, nil
		default:
			return 0, fmt.Errorf("unsupported operator: %s", n.Op.String())
		}

	case *ast.ParenExpr:
		return evalAST(n.X)
		
	default:
		return 0, fmt.Errorf("unsupported expression: %T", n)
	}
}

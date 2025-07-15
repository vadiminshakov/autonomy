package tools

import (
	"testing"
)

func TestCalc_Positive(t *testing.T) {
	tests := []struct {
		name      string
		expr      string
		want      string
		wantError bool
	}{
		{"Addition", "2+2", "4", false},
		{"Subtraction", "10-3", "7", false},
		{"Multiplication", "5*6", "30", false},
		{"Division", "8/2", "4", false},
		{"Parentheses", "2*(3+4)", "14", false},
		{"Float division", "7/2", "3.5", false},
		{"Complex", "1+2*3-4/2", "5", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := calc(map[string]interface{}{"expr": tc.expr})
			if (err != nil) != tc.wantError {
				t.Fatalf("expr %s: unexpected error: %v", tc.expr, err)
			}
			if !tc.wantError && got != tc.want {
				t.Errorf("expr %s: want %v, got %v", tc.expr, tc.want, got)
			}
		})
	}
}

func TestCalc_Negative(t *testing.T) {
	invalidExpr := []struct {
		name string
		expr interface{}
	}{
		{"Empty String", ""},
		{"Non-string", 123},
		{"Invalid Operator", "2++2"},
		{"Unsupported Func", "sin(2)"},
		{"Alphabetic", "abc+1"},
	}

	for _, tc := range invalidExpr {
		t.Run(tc.name, func(t *testing.T) {
			_, err := calc(map[string]interface{}{"expr": tc.expr})
			if err == nil {
				t.Errorf("expr %v: expected error but got nil", tc.expr)
			}
		})
	}
}

package math

import "testing"

func TestFactorial(t *testing.T) {
	// Test cases for factorial function
	testCases := []struct {
		input    int
		expected int
	}{
		{0, 1},
		{1, 1},
		{2, 2},
		{3, 6},
		{4, 24},
		{5, 120},
		{10, 3628800},
	}

	for _, tc := range testCases {
		result := Factorial(tc.input)
		if result != tc.expected {
			t.Errorf("Factorial(%d) = %d; expected %d", tc.input, result, tc.expected)
		}
	}
}

func TestFactorialNegative(t *testing.T) {
	// Test that factorial of negative number panics
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Factorial(-1) should panic, but it didn't")
		}
	}()
	Factorial(-1)
}
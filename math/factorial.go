package math

// Factorial calculates the factorial of a given non-negative integer
func Factorial(n int) int {
	if n < 0 {
		panic("factorial is not defined for negative numbers")
	}
	if n == 0 || n == 1 {
		return 1
	}
	result := 1
	for i := 2; i <= n; i++ {
		result *= i
	}
	return result
}
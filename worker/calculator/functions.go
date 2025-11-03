package calculator

import "math"

var functions map[string]func(float64) float64

func init() {

	functions = map[string]func(float64) float64{
		// --- Polinomios ---
		"x^2":             func(x float64) float64 { return x * x },
		"x^3":             func(x float64) float64 { return x * x * x },
		"2*x^3 - 5*x + 1": func(x float64) float64 { return 2*x*x*x - 5*x + 1 },
		"x^4 - 4*x^2":     func(x float64) float64 { return math.Pow(x, 4) - 4*x*x },

		// --- Trigonométricas ---
		"sin(x)":          func(x float64) float64 { return math.Sin(x) },
		"cos(x)":          func(x float64) float64 { return math.Cos(x) },
		"sin(x) + cos(x)": func(x float64) float64 { return math.Sin(x) + math.Cos(x) },
		"sin(x) * cos(x)": func(x float64) float64 { return math.Sin(x) * math.Cos(x) },

		// --- Exponenciales y Logaritmos ---
		"exp(x)": func(x float64) float64 { return math.Exp(x) },
		"log(x)": func(x float64) float64 {
			if x <= 0 {
				return 0
			}
			return math.Log(x) // math.Log es el logaritmo natural (ln)
		},
		"exp(-x^2)": func(x float64) float64 { return math.Exp(-x * x) },

		// --- Racionales y Complejas ---
		"1/x": func(x float64) float64 {
			if x == 0 {
				// Evitar la división por cero; devolver NaN (Not a Number) para un resultado indefinido.
				return math.NaN()
			}
			return 1 / x
		},
		"1/(1 + x^2)": func(x float64) float64 { return 1 / (1 + x*x) },
		"sqrt(x)": func(x float64) float64 {
			if x < 0 {
				return 0
			}
			return math.Sqrt(x)
		},
		"x * sin(x)":         func(x float64) float64 { return x * math.Sin(x) },
		"exp(x) / (1 + x^2)": func(x float64) float64 { return math.Exp(x) / (1 + x*x) },
	}
}

func getFunction(f string) func(float64) float64 {
	return functions[f]
}

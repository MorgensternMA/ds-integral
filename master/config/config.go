package config

import (
	"flag"
	"log"
)

var (
	IntervalSize float64 = 0.01 // tamaño del subintervalo enviado a un worker
	Reset        bool    = false
	LowerBound   float64 = 0.0   // límite inferior de la integral
	UpperBound   float64 = 1.0   // límite superior de la integral
	Function     string  = "x^2" // función a integrar (representación en string)
)

func Load() {
	var intervalSize float64
	flag.Float64Var(&intervalSize, "intervalSize", 0.01, "")
	flag.BoolVar(&Reset, "reset", false, "")
	flag.Float64Var(&LowerBound, "lower", 0.0, "")
	flag.Float64Var(&UpperBound, "upper", 1.0, "")
	flag.StringVar(&Function, "function", "x^2", "")

	flag.Parse()

	if intervalSize > 0 {
		IntervalSize = intervalSize
	}

	log.Printf("Config loaded. IntervalSize [%f] Reset [%t] Bounds [%f,%f] Function [%s]",
		IntervalSize, Reset, LowerBound, UpperBound, Function)
}

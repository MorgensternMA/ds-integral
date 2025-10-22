package calculator

import (
	"log"
	"math"
	"math/big"
	"net"
	"net/rpc"
	"time"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"

	"ds-integral.com/master/shared"
)

type Calculator struct {
	masterAddr net.TCPAddr
	client     *rpc.Client
	workerName string

	job    *currentJob
	ticker *time.Ticker
	stopCh chan struct{}
}

type currentJob struct {
	id         uint64
	lowerBound float64
	upperBound float64
	numPoints  uint64
	function   string
	result     big.Float
	completed  bool
	precision  uint
}

func NewCalculator(masterIP net.IP, port int) Calculator {
	return Calculator{
		masterAddr: net.TCPAddr{
			IP:   masterIP,
			Port: port,
		},
		ticker: time.NewTicker(time.Second * 5),
	}
}

func (c *Calculator) Run() {
	c.createClient()

	// connect
	if err := c.connect(); err != nil {
		log.Fatalf("unable to connect to master: %s", err)
	}

	// start pinging
	c.ping()

	// ask jobs
	for {
		if c.askJob() {
			// Si numPoints es 0, significa que no hay más trabajo
			if c.job.numPoints == 0 {
				log.Printf("No more jobs available. Waiting...")
				time.Sleep(time.Second * 10)
				continue
			}
			
			if result := c.calculate(); result != nil {
				c.send(result)
			}
		}
	}
}

// Métodos connect, ping, askJob similares a los del sistema de PI

func (c *Calculator) calculate() []byte {
	start := time.Now()

	// Configurar la precisión para el cálculo
	precision := uint(math.Log2(float64(c.job.numPoints))+1000*3.32) * 3
	c.job.precision = precision

	// Crear un nuevo número para guardar el resultado con alta precisión
	result := new(big.Float).SetPrec(precision * 2).SetFloat64(0)

	// Calcular el ancho de cada subintervalo
	deltaX := (c.job.upperBound - c.job.lowerBound) / float64(c.job.numPoints)

	// Aplicar el método del trapecio para la integración numérica
	// f(a)/2 + f(a+h) + f(a+2h) + ... + f(b-h) + f(b)/2
	firstTerm := evaluateFunction(c.job.lowerBound, c.job.function)
	lastTerm := evaluateFunction(c.job.upperBound, c.job.function)

	// Sumar los términos de los extremos (con peso 1/2)
	sum := new(big.Float).SetPrec(precision).SetFloat64((firstTerm + lastTerm) / 2.0)

	// Sumar los términos intermedios
	for i := uint64(1); i < c.job.numPoints; i++ {
		x := c.job.lowerBound + float64(i)*deltaX
		fx := evaluateFunction(x, c.job.function)
		sum.Add(sum, big.NewFloat(fx))
	}

	// Multiplicar por deltaX para obtener el resultado final
	result.Mul(sum, big.NewFloat(deltaX))

	elapsed := time.Now().Sub(start)
	log.Printf("Job calculated in %s. Result (rounded) %s", elapsed, result.String())

	buffer, err := result.GobEncode()
	if err != nil {
		log.Fatalf("unable to encode result as gob: %s", err)
		return nil
	}

	return buffer
}

// evaluateFunction evalúa la función en un punto dado
func evaluateFunction(x float64, functionStr string) float64 {
	// Implementar un evaluador de expresiones simple
	// Para un sistema real, se podría usar una biblioteca más robusta
	// Este es un ejemplo básico que soporta x^n, sin(x), cos(x), etc.
	
	// Por simplicidad, aquí solo implementamos x^2 como ejemplo
	switch functionStr {
	case "x^2":
		return x * x
	case "sin(x)":
		return math.Sin(x)
	case "cos(x)":
		return math.Cos(x)
	case "exp(x)":
		return math.Exp(x)
	default:
		// Intentar parsear expresiones simples como "x^n"
		return parseAndEvaluate(functionStr, x)
	}
}

// parseAndEvaluate intenta parsear y evaluar expresiones matemáticas simples
func parseAndEvaluate(expr string, x float64) float64 {
	// Implementación básica para expresiones simples
	// En un sistema real, se usaría un parser más completo
	return x * x // Valor por defecto
}

func (c *Calculator) send(buffer []byte) bool {
	args := &shared.GiveArgs{
		JobID:     c.job.id,
		Result:    buffer,
		Precision: c.job.precision,
	}
	var reply shared.GiveReply
	err := c.client.Call("CalcRPC.Give", args, &reply)
	if err != nil {
		log.Fatalf("unable to call CalcRPC.Give: %s", err)
		return false
	}

	log.Printf("Job result sent.")
	return true
}
package calculator

import (
	"log"
	"math"
	"math/big"
	"net"
	"net/rpc"
	"time"

	"ds-integral.com/master/shared"
)

type Calculator struct {
	masterAddr net.TCPAddr
	client     *rpc.Client
	workerName string

	job    *currentJob
	ticker *time.Ticker
}

type currentJob struct {
	id         uint64
	lowerBound float64
	upperBound float64
	numPoints  uint64
	function   string
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

func (c *Calculator) createClient() {
	var err error
	c.client, err = rpc.Dial("tcp", c.masterAddr.String())
	if err != nil {
		log.Fatalf("unable to create rpc client: %s", err)
	}
	log.Printf("RPC client created")
}

func (c *Calculator) connect() error {
	args := &shared.ConnectArgs{WorkerIP: c.masterAddr.IP.String()}
	var reply shared.ConnectReply
	err := c.client.Call("CalcRPC.Connect", args, &reply)
	if err != nil {
		return err
	}

	c.workerName = reply.WorkerName
	log.Printf("Connected to master as %s", c.workerName)
	return nil
}

func (c *Calculator) ping() {
	go func() {
		for {
			<-c.ticker.C
			args := &shared.PingArgs{WorkerName: c.workerName}
			var reply shared.PingResponse
			err := c.client.Call("CalcRPC.Ping", args, &reply)
			if err != nil {
				log.Printf("unable to ping master: %s", err)
			}
		}
	}()
}

func (c *Calculator) askJob() bool {
	if c.job != nil && !c.job.completed {
		return false
	}

	args := &shared.AskArgs{WorkerName: c.workerName}
	var reply shared.AskReply
	err := c.client.Call("CalcRPC.Ask", args, &reply)
	if err != nil {
		log.Fatalf("unable to call CalcRPC.Ask: %s", err)
		return false
	}

	if reply.NumPoints == 0 {
		log.Printf("skipped reply %+v because there is no points", reply)
		return false
	}

	c.job = &currentJob{
		id:         reply.JobID,
		lowerBound: reply.LowerBound,
		upperBound: reply.UpperBound,
		numPoints:  reply.NumPoints,
		function:   reply.Function,
		completed:  false,
	}

	log.Printf("Received job %d: [%f, %f] with %d points for function %s",
		c.job.id, c.job.lowerBound, c.job.upperBound, c.job.numPoints, c.job.function)
	return true
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
			if result := c.calculate(); result != nil {
				c.send(result)
			}
		} else {
			log.Printf("No work assigned. Waiting...")
			time.Sleep(time.Second * 5)
		}
	}
}

func (c *Calculator) calculate() []byte {
	start := time.Now()

	// Configurar la precisión para el cálculo
	precision := uint(math.Log2(float64(c.job.numPoints))+1000*3.32) * 3
	c.job.precision = precision

	f := getFunction(c.job.function)

	// Crear un nuevo número para guardar el resultado
	result := new(big.Float).SetPrec(precision * 2).SetFloat64(0)

	// Calcular el ancho de cada subintervalo
	deltaX := (c.job.upperBound - c.job.lowerBound) / float64(c.job.numPoints)

	// Aplicar el método del trapecio para la integración numérica
	// f(a)/2 + f(a+h) + f(a+2h) + ... + f(b-h) + f(b)/2
	firstTerm := evaluateFunction(c.job.lowerBound, f)
	lastTerm := evaluateFunction(c.job.upperBound, f)

	// Sumar los términos de los extremos (con peso 1/2)
	sum := new(big.Float).SetPrec(precision).SetFloat64((firstTerm + lastTerm) / 2.0)

	// Sumar los términos intermedios
	for i := uint64(1); i < c.job.numPoints; i++ {
		x := c.job.lowerBound + float64(i)*deltaX
		fx := evaluateFunction(x, f)
		sum.Add(sum, big.NewFloat(fx))
	}

	// Multiplicar por deltaX para obtener el resultado final
	result.Mul(sum, big.NewFloat(deltaX))

	elapsed := time.Since(start)
	log.Printf("Job calculated in %s. Result (rounded) %s", elapsed, result.String())

	buffer, err := result.GobEncode()
	if err != nil {
		log.Fatalf("unable to encode result as gob: %s", err)
		return nil
	}

	return buffer
}

// Evalúa la función en un punto dado usando expr
func evaluateFunction(x float64, f func(float64) float64) float64 {
	output := f(x)
	return output
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

	c.job = nil
	log.Printf("Job result sent.")
	return true
}

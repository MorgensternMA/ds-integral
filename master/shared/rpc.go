package shared

import (
	"log"
)

// CalcInterface defines the methods required by CalcRPC
type CalcInterface interface {
	GenerateWorkerName() string
	AddWorker(name, ip string)
	UpdateWorkerPing(workerName string)
	GetJob(workerName string) WorkerJob
	CompleteJob(jobId uint64, result []byte, precision uint)
	GetCurrentFunction() string
}

// WorkerJob mirrors the structure from icalc package
type WorkerJob struct {
	ID         uint64
	LowerBound float64
	UpperBound float64
	NumPoints  uint64
	Function   string
}

// CalcRPC handles RPC calls for the integral calculator
type CalcRPC struct {
	calc CalcInterface
}

// NewCalcRPC creates a new CalcRPC instance
func NewCalcRPC(calc CalcInterface) *CalcRPC {
	return &CalcRPC{
		calc: calc,
	}
}

// Connect handles worker connection requests
func (c *CalcRPC) Connect(args *ConnectArgs, reply *ConnectReply) error {
	workerName := c.calc.GenerateWorkerName()
	c.calc.AddWorker(workerName, args.WorkerIP)
	reply.WorkerName = workerName
	log.Printf("Worker connected from %s as %s", args.WorkerIP, workerName)
	return nil
}

// Ping handles ping requests from workers
func (c *CalcRPC) Ping(args *PingArgs, reply *PingResponse) error {
	c.calc.UpdateWorkerPing(args.WorkerName)
	return nil
}

// Ask handles job requests from workers
func (c *CalcRPC) Ask(args *AskArgs, reply *AskReply) error {
	job := c.calc.GetJob(args.WorkerName)

	reply.JobID = job.ID
	reply.LowerBound = job.LowerBound
	reply.UpperBound = job.UpperBound
	reply.NumPoints = job.NumPoints
	reply.Function = job.Function

	return nil
}

// Give handles result submissions from workers
func (c *CalcRPC) Give(args *GiveArgs, reply *GiveReply) error {
	c.calc.CompleteJob(args.JobID, args.Result, args.Precision)
	log.Printf("Received result for job %d", args.JobID)
	return nil
}

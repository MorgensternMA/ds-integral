package icalc

import (
	"fmt"
	"log"
	"math/big"
	"sync"
	"time"

	"ds-integral.com/master/config"
	"ds-integral.com/master/shared"
)

type WorkerRange struct {
	WorkerName   string
	Function     string
	LowerBound   float64
	UpperBound   float64
	IntervalSize float64
	CurrentPoint float64
}

// Calc keeps track of calculations, intervals and workers available
type Calc struct {
	jobMutex     sync.Mutex
	sumMutex     sync.Mutex
	saveMutex    sync.Mutex
	bufferMutex  sync.RWMutex
	workerMutex  sync.RWMutex
	JobsBuffer   []mergeRequest
	Jobs         map[uint64]WorkerJob    // key is job id
	Workers      map[string]Worker       // key is worker name
	WorkerRanges map[string]*WorkerRange // key is worker name
	LastPoint    float64                 // This contains the last point given by the master
	LastJobID    uint64                  // keeps track of given job ids
	Result       *big.Float              // The calculated integral result

	// Dynamic configuration
	CurrentFunction string
	CurrentLower    float64
	CurrentUpper    float64
	CurrentInterval float64
	JobDelay        int // Delay in milliseconds between job assignments
}

// Worker represents a connected worker
type Worker struct {
	Name     string
	IP       string
	LastPing time.Time
}

// WorkerJob contains information about the job sent to a worker
type WorkerJob struct {
	ID         uint64
	SendAt     time.Time
	ReturnedAt *time.Time
	Completed  bool    // a flag that indicates the job was completed
	Lost       bool    // a flag that indicates if the connection with the actual worker was lost
	WorkerName string  // the name of the worker who owns the job
	LowerBound float64 // lower bound of the subinterval
	UpperBound float64 // upper bound of the subinterval
	NumPoints  uint64  // number of points for calculation
	Result     []byte  `json:"-"`
}

type mergeRequest struct {
	jobId     uint64
	result    []byte
	precision uint
}

func NewCalc() *Calc {
	return &Calc{
		jobMutex:        sync.Mutex{},
		saveMutex:       sync.Mutex{},
		sumMutex:        sync.Mutex{},
		workerMutex:     sync.RWMutex{},
		Jobs:            make(map[uint64]WorkerJob),
		Workers:         make(map[string]Worker),
		WorkerRanges:    make(map[string]*WorkerRange),
		LastPoint:       config.LowerBound,
		Result:          big.NewFloat(0).SetPrec(50_000),
		CurrentFunction: config.Function,
		CurrentLower:    config.LowerBound,
		CurrentUpper:    config.UpperBound,
		CurrentInterval: config.IntervalSize,
		JobDelay:        0,
	}
}

func (c *Calc) GetJob(workerName string) shared.WorkerJob {
	c.jobMutex.Lock()
	defer c.jobMutex.Unlock()

	// Check if this worker has a specific range assigned
	if workerRange, exists := c.WorkerRanges[workerName]; exists {
		// This worker has a specific range to work on
		if workerRange.CurrentPoint >= workerRange.UpperBound {
			// This worker has completed its assigned range
			log.Printf("Worker %s has completed its assigned range [%f, %f]",
				workerName, workerRange.LowerBound, workerRange.UpperBound)
			return shared.WorkerJob{NumPoints: 0} // No more work for this worker
		}

		lowerBound := workerRange.CurrentPoint
		upperBound := lowerBound + workerRange.IntervalSize

		// Don't exceed the assigned upper bound
		if upperBound > workerRange.UpperBound {
			upperBound = workerRange.UpperBound
		}

		jobId := c.LastJobID
		job := WorkerJob{
			ID:         jobId,
			SendAt:     time.Now(),
			WorkerName: workerName,
			LowerBound: lowerBound,
			UpperBound: upperBound,
			NumPoints:  uint64((upperBound - lowerBound) / 0.001),
		}

		c.Jobs[job.ID] = job
		c.LastJobID++
		workerRange.CurrentPoint = upperBound

		log.Printf("Gave job [id=%d] to worker %s from assigned range [%f, %f] - progress: [%f/%f]",
			job.ID, workerName, lowerBound, upperBound, workerRange.CurrentPoint, workerRange.UpperBound)

		return shared.WorkerJob{
			ID:         job.ID,
			LowerBound: job.LowerBound,
			UpperBound: job.UpperBound,
			NumPoints:  job.NumPoints,
		}
	}

	// If worker has NO assigned range, return empty job (wait for assignment)
	return shared.WorkerJob{NumPoints: 0}
}

func (c *Calc) CompleteJob(jobId uint64, result []byte, precision uint) {
	c.bufferMutex.Lock()
	defer c.bufferMutex.Unlock()
	c.JobsBuffer = append(c.JobsBuffer, mergeRequest{
		jobId:     jobId,
		result:    result,
		precision: precision,
	})
}

func (c *Calc) GenerateWorkerName() string {
	c.workerMutex.Lock()
	defer c.workerMutex.Unlock()

	// Simple sequential naming: worker-1, worker-2, etc.
	workerNum := len(c.Workers) + 1
	return fmt.Sprintf("worker-%d", workerNum)
}

func (c *Calc) AddWorker(name, ip string) {
	c.workerMutex.Lock()
	defer c.workerMutex.Unlock()
	c.Workers[name] = Worker{
		Name:     name,
		IP:       ip,
		LastPing: time.Now(),
	}
}

func (c *Calc) UpdateWorkerPing(workerName string) {
	c.workerMutex.Lock()
	defer c.workerMutex.Unlock()
	if worker, exists := c.Workers[workerName]; exists {
		worker.LastPing = time.Now()
		c.Workers[workerName] = worker
	}
}

// Resets and starts a new integral calculation
func (c *Calc) StartNewIntegral(function string, lower, upper, intervalSize float64, delayMs int) {
	c.jobMutex.Lock()
	defer c.jobMutex.Unlock()

	// Reset state
	c.Jobs = make(map[uint64]WorkerJob)
	c.JobsBuffer = []mergeRequest{}
	c.LastPoint = lower
	c.LastJobID = 0
	c.Result = big.NewFloat(0).SetPrec(50_000)

	// Set new parameters
	c.CurrentFunction = function
	c.CurrentLower = lower
	c.CurrentUpper = upper
	c.CurrentInterval = intervalSize
	c.JobDelay = delayMs

	// Update global config
	config.Function = function
	config.LowerBound = lower
	config.UpperBound = upper
	config.IntervalSize = intervalSize

	log.Printf("New integral: %s from %f to %f (interval: %f, delay: %dms)",
		function, lower, upper, intervalSize, delayMs)
}

// Assigns a specific interval to a specific worker
func (c *Calc) AssignSpecificInterval(workerName, function string, lower, upper float64) uint64 {
	c.jobMutex.Lock()
	defer c.jobMutex.Unlock()

	jobId := c.LastJobID
	c.LastJobID++

	job := WorkerJob{
		ID:         jobId,
		SendAt:     time.Now(),
		WorkerName: workerName,
		LowerBound: lower,
		UpperBound: upper,
		NumPoints:  uint64((upper - lower) / 0.001),
		Completed:  false,
		Lost:       false,
	}

	c.Jobs[jobId] = job

	log.Printf("Manually assigned job [id=%d] to worker %s: %s [%f, %f]",
		jobId, workerName, function, lower, upper)

	return jobId
}

// Holds status information
type CalculationStatus struct {
	Function      string
	LowerBound    float64
	UpperBound    float64
	CurrentPoint  float64
	Progress      float64
	CompletedJobs int
	TotalJobs     int
	IsComplete    bool
}

// GetStatus returns current calculation status
func (c *Calc) GetStatus() CalculationStatus {
	c.jobMutex.Lock()
	defer c.jobMutex.Unlock()

	completed := 0
	for _, job := range c.Jobs {
		if job.Completed {
			completed++
		}
	}

	totalRange := c.CurrentUpper - c.CurrentLower
	progress := 0.0
	if totalRange > 0 {
		progress = ((c.LastPoint - c.CurrentLower) / totalRange) * 100
	}

	expectedTotal := int(totalRange / c.CurrentInterval)
	if expectedTotal == 0 {
		expectedTotal = len(c.Jobs)
	}

	return CalculationStatus{
		Function:      c.CurrentFunction,
		LowerBound:    c.CurrentLower,
		UpperBound:    c.CurrentUpper,
		CurrentPoint:  c.LastPoint,
		Progress:      progress,
		CompletedJobs: completed,
		TotalJobs:     expectedTotal,
		IsComplete:    c.LastPoint >= c.CurrentUpper && completed == len(c.Jobs),
	}
}

// GetResult returns the current accumulated result
func (c *Calc) GetResult() *big.Float {
	c.sumMutex.Lock()
	defer c.sumMutex.Unlock()
	return c.Result
}

// Stats holds statistics information
type Stats struct {
	TotalJobs      int
	CompletedJobs  int
	PendingJobs    int
	TotalWorkers   int
	WorkerJobCount map[string]int
}

// GetStats returns calculation statistics
func (c *Calc) GetStats() Stats {
	c.jobMutex.Lock()
	defer c.jobMutex.Unlock()

	completed := 0
	workerJobs := make(map[string]int)

	for _, job := range c.Jobs {
		if job.Completed {
			completed++
		}
		workerJobs[job.WorkerName]++
	}

	c.workerMutex.RLock()
	totalWorkers := len(c.Workers)
	c.workerMutex.RUnlock()

	return Stats{
		TotalJobs:      len(c.Jobs),
		CompletedJobs:  completed,
		PendingJobs:    len(c.Jobs) - completed,
		TotalWorkers:   totalWorkers,
		WorkerJobCount: workerJobs,
	}
}

// GetWorkers returns a list of connected workers
func (c *Calc) GetWorkers() []Worker {
	c.workerMutex.RLock()
	defer c.workerMutex.RUnlock()

	workers := make([]Worker, 0, len(c.Workers))
	for _, w := range c.Workers {
		workers = append(workers, w)
	}
	return workers
}

// GetCurrentFunction returns the current function being integrated
func (c *Calc) GetCurrentFunction() string {
	return c.CurrentFunction
}

// AssignRangeToWorker assigns a specific range of the integral to a specific worker
func (c *Calc) AssignRangeToWorker(workerName, function string, lower, upper, intervalSize float64) {
	c.jobMutex.Lock()
	defer c.jobMutex.Unlock()

	c.WorkerRanges[workerName] = &WorkerRange{
		WorkerName:   workerName,
		Function:     function,
		LowerBound:   lower,
		UpperBound:   upper,
		IntervalSize: intervalSize,
		CurrentPoint: lower, // Start from the lower bound
	}

	log.Printf("Assigned range [%f, %f] of function '%s' to worker %s (interval size: %f)",
		lower, upper, function, workerName, intervalSize)
}

// GetWorkerRanges returns all assigned worker ranges
func (c *Calc) GetWorkerRanges() map[string]*WorkerRange {
	c.jobMutex.Lock()
	defer c.jobMutex.Unlock()

	// Return a copy to avoid race conditions
	ranges := make(map[string]*WorkerRange)
	for k, v := range c.WorkerRanges {
		ranges[k] = v
	}
	return ranges
}

// ClearWorkerRanges clears all worker range assignments
func (c *Calc) ClearWorkerRanges() {
	c.jobMutex.Lock()
	defer c.jobMutex.Unlock()

	c.WorkerRanges = make(map[string]*WorkerRange)
	log.Println("Cleared all worker range assignments")
}

// AutoAssignRanges automatically divides the integral among connected workers
func (c *Calc) AutoAssignRanges(function string, lower, upper, intervalSize float64) int {
	c.workerMutex.RLock()
	workers := make([]string, 0, len(c.Workers))
	for name := range c.Workers {
		workers = append(workers, name)
	}
	c.workerMutex.RUnlock()

	if len(workers) == 0 {
		log.Println("No workers connected for auto-assignment")
		return 0
	}

	// Clear previous assignments
	c.ClearWorkerRanges()

	// Divide the range equally among workers
	totalRange := upper - lower
	rangePerWorker := totalRange / float64(len(workers))

	c.jobMutex.Lock()
	defer c.jobMutex.Unlock()

	for i, workerName := range workers {
		workerLower := lower + (float64(i) * rangePerWorker)
		workerUpper := lower + (float64(i+1) * rangePerWorker)

		// Ensure last worker reaches exactly the upper bound
		if i == len(workers)-1 {
			workerUpper = upper
		}

		c.WorkerRanges[workerName] = &WorkerRange{
			WorkerName:   workerName,
			Function:     function,
			LowerBound:   workerLower,
			UpperBound:   workerUpper,
			IntervalSize: intervalSize,
			CurrentPoint: workerLower,
		}

		log.Printf("Auto-assigned range [%f, %f] of '%s' to %s",
			workerLower, workerUpper, function, workerName)
	}

	return len(workers)
}

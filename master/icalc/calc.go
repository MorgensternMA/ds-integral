package icalc

import (
	"fmt"
	"log"
	"math/big"
	"strings"
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
	StartedAt    *time.Time              // time when the last job started
	FinishedAt   *time.Time              // time when the last job finished

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
	c := &Calc{
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

	// Start the background goroutine to process results
	go c.processResultsBuffer()

	return c
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

		// Calculate points ensuring a minimum of 1000 points even for small intervals
		intervalSize := upperBound - lowerBound
		numPoints := uint64(intervalSize / 0.001)
		if numPoints < 1000 {
			numPoints = 1000
		}

		job := WorkerJob{
			ID:         jobId,
			SendAt:     time.Now(),
			WorkerName: workerName,
			LowerBound: lowerBound,
			UpperBound: upperBound,
			NumPoints:  numPoints,
		}

		c.Jobs[job.ID] = job
		c.LastJobID++
		workerRange.CurrentPoint = upperBound

		log.Printf("Gave job [id=%d] to worker %s from assigned range [%f, %f] with %d points",
			job.ID, workerName, lowerBound, upperBound, numPoints)

		return shared.WorkerJob{
			ID:         job.ID,
			LowerBound: job.LowerBound,
			UpperBound: job.UpperBound,
			NumPoints:  job.NumPoints,
			Function:   workerRange.Function,
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

// processResultsBuffer continuously processes completed jobs from the buffer
func (c *Calc) processResultsBuffer() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		c.bufferMutex.Lock()
		if len(c.JobsBuffer) == 0 {
			c.bufferMutex.Unlock()
			continue
		}

		// Copy buffer and clear it
		buffer := make([]mergeRequest, len(c.JobsBuffer))
		copy(buffer, c.JobsBuffer)
		c.JobsBuffer = []mergeRequest{}
		c.bufferMutex.Unlock()

		// Process each result
		for _, req := range buffer {
			c.mergeResult(req.jobId, req.result, req.precision)
		}

		if c.IsFinished() {
			now := time.Now()
			c.FinishedAt = &now
		}
	}
}

// mergeResult adds a job result to the final accumulated result
func (c *Calc) mergeResult(jobId uint64, result []byte, precision uint) {
	// Decode the result
	partialResult := new(big.Float).SetPrec(precision)
	err := partialResult.GobDecode(result)
	if err != nil {
		log.Printf("Error decoding result for job %d: %v", jobId, err)
		return
	}

	// Add to accumulated result
	c.sumMutex.Lock()
	c.Result.Add(c.Result, partialResult)
	c.sumMutex.Unlock()

	// Mark job as completed
	c.jobMutex.Lock()
	if job, exists := c.Jobs[jobId]; exists {
		now := time.Now()
		job.Completed = true
		job.ReturnedAt = &now
		job.Result = result
		c.Jobs[jobId] = job
		log.Printf("Job %d completed and merged. Partial result: %s", jobId, partialResult.Text('f', 10))
	}
	c.jobMutex.Unlock()
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

	exists := false
	// for i, worker := range c.Workers {
	// 	if worker.IP == ip {
	// 		worker.LastPing = time.Now()
	// 		c.Workers[i] = worker
	// 		exists = true
	// 		break
	// 	}
	// }

	if !exists {
		c.Workers[name] = Worker{
			Name:     name,
			IP:       ip,
			LastPing: time.Now(),
		}
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

	// Calcular número de puntos con un mínimo de 1000 puntos
	intervalSize := upper - lower
	numPoints := uint64(intervalSize / 0.001)
	if numPoints < 1000 {
		numPoints = 1000
	}

	job := WorkerJob{
		ID:         jobId,
		SendAt:     time.Now(),
		WorkerName: workerName,
		LowerBound: lower,
		UpperBound: upper,
		NumPoints:  numPoints,
		Completed:  false,
		Lost:       false,
	}

	c.Jobs[jobId] = job

	log.Printf("Manually assigned job [id=%d] to worker %s: %s [%f, %f] with %d points",
		jobId, workerName, function, lower, upper, numPoints)

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

// IsFinished returns a bool when the current calculation finished
func (c *Calc) IsFinished() bool {
	return c.FinishedAt != nil
}

// GetResult returns the current accumulated result
func (c *Calc) GetResult() *big.Float {
	c.sumMutex.Lock()
	defer c.sumMutex.Unlock()

	if c.IsFinished() {
		return c.Result
	}

	return nil
}

// GetTimeToComplete returns the time it took to calculate
func (c *Calc) GetTimeToComplete() string {
	if c.StartedAt != nil && c.FinishedAt != nil {
		sub := c.FinishedAt.Sub(*c.StartedAt).Abs()
		return humanizeDuration(sub)
	}
	return ""
}

// Stats holds statistics information
type Stats struct {
	TotalJobs      int            `json:"total_jobs"`
	CompletedJobs  int            `json:"completed_jobs"`
	PendingJobs    int            `json:"pending_jobs"`
	TotalWorkers   int            `json:"total_workers"`
	WorkerJobCount map[string]int `json:"worker_job_count"`
}

// GetStats returns calculation statistics
func (c *Calc) GetStats() Stats {
	// First snapshot buffered job IDs under buffer lock
	c.bufferMutex.RLock()
	buffered := make(map[uint64]struct{}, len(c.JobsBuffer))
	for _, req := range c.JobsBuffer {
		buffered[req.jobId] = struct{}{}
	}
	c.bufferMutex.RUnlock()

	c.jobMutex.Lock()
	defer c.jobMutex.Unlock()

	completed := 0
	workerJobs := make(map[string]int)

	for id, job := range c.Jobs {
		// Count job as completed if merged or present in the buffer waiting to be merged
		_, inBuffer := buffered[id]
		if job.Completed || inBuffer {
			completed++
		}
		workerJobs[job.WorkerName]++
	}

	c.workerMutex.RLock()
	totalWorkers := 0
	for _, worker := range c.Workers {
		// skip if offline
		if time.Now().Sub(worker.LastPing).Abs().Seconds() > 6 {
			continue
		}
		totalWorkers++
	}
	c.workerMutex.RUnlock()

	totalJobs := len(c.Jobs)
	pending := totalJobs - completed
	if pending < 0 {
		pending = 0
	}

	return Stats{
		TotalJobs:      totalJobs,
		CompletedJobs:  completed,
		PendingJobs:    pending,
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

	now := time.Now()
	c.StartedAt = &now
	c.WorkerRanges = make(map[string]*WorkerRange)
	c.Jobs = make(map[uint64]WorkerJob)
	c.FinishedAt = nil
	c.JobsBuffer = make([]mergeRequest, 0)
	log.Println("Cleared all worker range assignments")
}

// AutoAssignRanges automatically divides the integral among connected workers
func (c *Calc) AutoAssignRanges(function string, lower, upper, intervalSize float64) int {
	c.workerMutex.Lock()
	defer c.workerMutex.Unlock()

	// Clear previous assignments
	c.ClearWorkerRanges()

	// Get active workers
	activeWorkers := make([]Worker, 0)
	for _, worker := range c.Workers {
		if time.Now().Sub(worker.LastPing).Abs().Seconds() <= 6 {
			activeWorkers = append(activeWorkers, worker)
		}
	}

	if len(activeWorkers) == 0 {
		return 0
	}

	// Calculate range per worker
	totalRange := upper - lower
	rangePerWorker := totalRange / float64(len(activeWorkers))

	// Assign ranges to each worker
	assigned := 0
	currentLower := lower

	for _, worker := range activeWorkers {
		workerUpper := currentLower + rangePerWorker
		if workerUpper > upper {
			workerUpper = upper
		}

		c.WorkerRanges[worker.Name] = &WorkerRange{
			WorkerName:   worker.Name,
			Function:     function,
			LowerBound:   currentLower,
			UpperBound:   workerUpper,
			IntervalSize: intervalSize,
			CurrentPoint: currentLower,
		}

		assigned++
		log.Printf("Auto-assigned range to worker %s: [%f, %f]",
			worker.Name, currentLower, workerUpper)

		currentLower = workerUpper
		if currentLower >= upper {
			break
		}
	}

	return assigned
}

func humanizeDuration(d time.Duration) string {
	if d.Seconds() < 1.0 {
		return "hace menos de 1 segundo"
	}

	// Calculate components
	seconds := int64(d.Seconds())
	days := seconds / (24 * 60 * 60)
	seconds %= (24 * 60 * 60)
	hours := seconds / (60 * 60)
	seconds %= (60 * 60)
	minutes := seconds / 60
	seconds %= 60

	// Build the string
	var parts []string

	if days > 0 {
		parts = append(parts, fmt.Sprintf("%d dia(s)", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%d hora(s)", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%d minuto(s)", minutes))
	}
	// Only show seconds if there are no larger units, or if it's the last non-zero unit
	if len(parts) == 0 || (seconds > 0 && len(parts) < 2) {
		parts = append(parts, fmt.Sprintf("%d segundo(s)", seconds))
	}

	// Join the parts, using a max of 2 units for brevity
	if len(parts) > 2 {
		parts = parts[:2]
	}

	// Simple conjunction for two units, otherwise just join
	if len(parts) == 2 {
		return fmt.Sprintf("%s y %s", parts[0], parts[1])
	}

	return strings.Join(parts, ", ")
}

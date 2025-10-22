package icalc

import (
	"log"
	"math/big"
	"sync"
	"time"

	"ds-integral.com/master/config"
)

const filename = "icalc.state"

// Calc keeps track of calculations, intervals and workers available
type Calc struct {
	jobMutex    sync.Mutex
	sumMutex    sync.Mutex
	saveMutex   sync.Mutex
	bufferMutex sync.RWMutex
	JobsBuffer  []mergeRequest
	Jobs        map[uint64]WorkerJob // key is job id
	LastPoint   float64              // This contains the last point given by the master
	LastJobID   uint64               // keeps track of given job ids
	Result      *big.Float           // The calculated integral result
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
	Result     []byte  `json:"-"`
}

type mergeRequest struct {
	jobId     uint64
	result    []byte
	precision uint
}

func NewCalc() *Calc {
	return &Calc{
		jobMutex:  sync.Mutex{},
		saveMutex: sync.Mutex{},
		sumMutex:  sync.Mutex{},
		Jobs:      make(map[uint64]WorkerJob),
		LastPoint: config.LowerBound,
		Result:    big.NewFloat(0).SetPrec(50_000),
	}
}

func (c *Calc) GetJob(workerName string) WorkerJob {
	c.jobMutex.Lock()
	defer c.jobMutex.Unlock()

	// first check if there is a lost job
	var lostJob *WorkerJob
	for _, job := range c.Jobs {
		if job.Lost {
			lostJob = &job
		}
	}

	var job WorkerJob
	if lostJob != nil {
		lostJob.Lost = false
		lostJob.WorkerName = workerName
		lostJob.SendAt = time.Now()
		c.Jobs[lostJob.ID] = *lostJob
		job = *lostJob
		log.Printf("Gave lost job %d to worker %s", lostJob.ID, workerName)
	} else {
		// Verificar si hemos llegado al límite superior
		if c.LastPoint >= config.UpperBound {
			// No hay más trabajo por hacer
			return WorkerJob{NumPoints: 0} // Indicar que no hay más trabajo
		}

		lowerBound := c.LastPoint
		upperBound := lowerBound + config.IntervalSize

		// Asegurarse de no exceder el límite superior
		if upperBound > config.UpperBound {
			upperBound = config.UpperBound
		}

		jobId := c.LastJobID

		job = WorkerJob{
			ID:         jobId,
			SendAt:     time.Now(),
			WorkerName: workerName,
			LowerBound: lowerBound,
			UpperBound: upperBound,
			NumPoints:  uint64((upperBound - lowerBound) / 0.001), // Puntos para cálculo
		}

		c.Jobs[job.ID] = job
		c.LastJobID++
		c.LastPoint = upperBound
		log.Printf("Gave new job [id=%d] to worker %s [interval=%f,%f]",
			job.ID, workerName, job.LowerBound, job.UpperBound)
	}

	go c.Save()
	return job
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

// Resto de métodos (Save, Restore, delete, merge) similares a los del sistema de PI
// pero adaptados para el cálculo de integrales

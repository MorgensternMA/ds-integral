package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"ds-integral.com/master/icalc"
)

type Server struct {
	calc *icalc.Calc
}

func NewServer(calc *icalc.Calc) *Server {
	return &Server{calc: calc}
}

// StartIntegralRequest represents a request to start a new integral calculation
type StartIntegralRequest struct {
	Function     string  `json:"function"`
	LowerBound   float64 `json:"lower_bound"`
	UpperBound   float64 `json:"upper_bound"`
	IntervalSize float64 `json:"interval_size"`
	AutoDivide   bool    `json:"auto_divide"` // If true, automatically divide among workers
}

// AssignRangeRequest assigns a specific range to a worker for distributed calculation
type AssignRangeRequest struct {
	WorkerName   string  `json:"worker_name"`
	Function     string  `json:"function"`
	LowerBound   float64 `json:"lower_bound"`
	UpperBound   float64 `json:"upper_bound"`
	IntervalSize float64 `json:"interval_size"`
}

// MultiRangeAssignment for assigning multiple ranges at once
type MultiRangeAssignment struct {
	Function     string              `json:"function"`
	IntervalSize float64             `json:"interval_size"`
	Ranges       []WorkerRangeAssign `json:"ranges"`
}

type WorkerRangeAssign struct {
	WorkerName string  `json:"worker_name"`
	LowerBound float64 `json:"lower_bound"`
	UpperBound float64 `json:"upper_bound"`
}

// WorkerRangeInfo represents info about a worker's assigned range
type WorkerRangeInfo struct {
	WorkerName   string  `json:"worker_name"`
	Function     string  `json:"function"`
	LowerBound   float64 `json:"lower_bound"`
	UpperBound   float64 `json:"upper_bound"`
	CurrentPoint float64 `json:"current_point"`
	Progress     float64 `json:"progress_percent"`
}

// Response structures
type WorkerResponse struct {
	Name     string `json:"name"`
	IP       string `json:"ip"`
	LastPing string `json:"last_ping"`
}

type StatusResponse struct {
	Function      string  `json:"function"`
	LowerBound    float64 `json:"lower_bound"`
	UpperBound    float64 `json:"upper_bound"`
	CurrentPoint  float64 `json:"current_point"`
	Progress      float64 `json:"progress_percent"`
	CompletedJobs int     `json:"completed_jobs"`
	TotalJobs     int     `json:"total_jobs"`
	IsComplete    bool    `json:"is_complete"`
}

type StatsResponse struct {
	TotalJobs      int            `json:"total_jobs"`
	CompletedJobs  int            `json:"completed_jobs"`
	PendingJobs    int            `json:"pending_jobs"`
	TotalWorkers   int            `json:"total_workers"`
	WorkerJobCount map[string]int `json:"worker_job_count"`
}

type ResultResponse struct {
	Result string `json:"result"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func (s *Server) Start(port string) error {
	mux := http.NewServeMux()

	// Endpoints
	mux.HandleFunc("/integrals", s.handleIntegrals)
	mux.HandleFunc("/ranges/assign", s.handleAssignRange)
	mux.HandleFunc("/ranges/assign-multi", s.handleAssignMultiRange)
	mux.HandleFunc("/ranges/list", s.handleListRanges)
	mux.HandleFunc("/ranges/clear", s.handleClearRanges)
	mux.HandleFunc("/workers", s.handleWorkers)
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/stats", s.handleStats)
	mux.HandleFunc("/result", s.handleResult)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/", s.handleWeb)

	log.Printf("API Server listening on %s", port)
	log.Println("Available endpoints:")
	log.Println("  GET  /             		   - Render a dashboard)")
	log.Println("  POST /integrals             - Start integral (with auto_divide=true to auto-assign)")
	log.Println("  POST /ranges/assign         - Assign range to single worker")
	log.Println("  POST /ranges/assign-multi   - Assign multiple ranges at once")
	log.Println("  GET  /ranges/list           - List all assigned ranges")
	log.Println("  POST /ranges/clear          - Clear all range assignments")
	log.Println("  GET  /workers               - List connected workers")
	log.Println("  GET  /status                - Get calculation status")
	log.Println("  GET  /stats                 - Get statistics")
	log.Println("  GET  /result                - Get current result")
	log.Println("  GET  /health                - Health check")

	return http.ListenAndServe(port, s.enableCORS(mux))
}

// POST /integrals - Start new integral calculation
func (s *Server) handleIntegrals(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req StartIntegralRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate
	if req.Function == "" {
		req.Function = "x^2"
	}
	if req.IntervalSize <= 0 {
		req.IntervalSize = 0.01
	}
	if req.UpperBound <= req.LowerBound {
		s.sendError(w, "Upper bound must be greater than lower bound", http.StatusBadRequest)
		return
	}

	// Auto-divide if requested
	if req.AutoDivide {
		numWorkers := s.calc.AutoAssignRanges(req.Function, req.LowerBound, req.UpperBound, req.IntervalSize)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message":          "Integral auto-divided among workers",
			"function":         req.Function,
			"range":            []float64{req.LowerBound, req.UpperBound},
			"interval_size":    req.IntervalSize,
			"workers_assigned": numWorkers,
		})
		return
	}

	// Legacy: just set the configuration without auto-divide
	s.calc.StartNewIntegral(req.Function, req.LowerBound, req.UpperBound, req.IntervalSize, 0)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":       "Integral configuration set",
		"function":      req.Function,
		"range":         []float64{req.LowerBound, req.UpperBound},
		"interval_size": req.IntervalSize,
		"note":          "Workers will not receive jobs until ranges are assigned",
	})
}

// POST /ranges/assign-multi - Assign multiple ranges at once
func (s *Server) handleAssignMultiRange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req MultiRangeAssignment
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate
	if req.Function == "" {
		s.sendError(w, "function is required", http.StatusBadRequest)
		return
	}
	if req.IntervalSize <= 0 {
		req.IntervalSize = 0.01
	}
	if len(req.Ranges) == 0 {
		s.sendError(w, "ranges array cannot be empty", http.StatusBadRequest)
		return
	}

	// Clear previous assignments
	s.calc.ClearWorkerRanges()

	// Assign each range
	assigned := []map[string]interface{}{}
	for _, r := range req.Ranges {
		if r.UpperBound <= r.LowerBound {
			s.sendError(w, fmt.Sprintf("Invalid range for %s: upper_bound must be > lower_bound", r.WorkerName), http.StatusBadRequest)
			return
		}

		s.calc.AssignRangeToWorker(r.WorkerName, req.Function, r.LowerBound, r.UpperBound, req.IntervalSize)
		assigned = append(assigned, map[string]interface{}{
			"worker": r.WorkerName,
			"range":  []float64{r.LowerBound, r.UpperBound},
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":       "Multiple ranges assigned",
		"function":      req.Function,
		"interval_size": req.IntervalSize,
		"assignments":   assigned,
	})
}

// GET /workers - List connected workers
func (s *Server) handleWorkers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	workers := s.calc.GetWorkers()
	response := make([]WorkerResponse, 0, len(workers))

	for _, w := range workers {
		response = append(response, WorkerResponse{
			Name:     w.Name,
			IP:       w.IP,
			LastPing: w.LastPing.Format("2006-01-02 15:04:05"),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":   len(response),
		"workers": response,
	})
}

// GET /status - Get calculation status
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := s.calc.GetStatus()

	response := StatusResponse{
		Function:      status.Function,
		LowerBound:    status.LowerBound,
		UpperBound:    status.UpperBound,
		CurrentPoint:  status.CurrentPoint,
		Progress:      status.Progress,
		CompletedJobs: status.CompletedJobs,
		TotalJobs:     status.TotalJobs,
		IsComplete:    status.IsComplete,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GET /stats - Get statistics
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats := s.calc.GetStats()

	response := StatsResponse{
		TotalJobs:      stats.TotalJobs,
		CompletedJobs:  stats.CompletedJobs,
		PendingJobs:    stats.PendingJobs,
		TotalWorkers:   stats.TotalWorkers,
		WorkerJobCount: stats.WorkerJobCount,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GET /result - Get current result
func (s *Server) handleResult(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	result := s.calc.GetResult()

	// Get precision from query parameter
	precision := 10
	if precStr := r.URL.Query().Get("precision"); precStr != "" {
		if p, err := strconv.Atoi(precStr); err == nil && p > 0 {
			precision = p
		}
	}

	response := ResultResponse{
		Result: result.Text('f', precision),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GET /health - Health check
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

// GET / - Web dashboard
func (s *Server) handleWeb(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "api/index.html")
}

// POST /ranges/assign - Assign a specific range to a worker
func (s *Server) handleAssignRange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AssignRangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate
	if req.WorkerName == "" {
		s.sendError(w, "worker_name is required", http.StatusBadRequest)
		return
	}
	if req.Function == "" {
		s.sendError(w, "function is required", http.StatusBadRequest)
		return
	}
	if req.UpperBound <= req.LowerBound {
		s.sendError(w, "upper_bound must be greater than lower_bound", http.StatusBadRequest)
		return
	}
	if req.IntervalSize <= 0 {
		req.IntervalSize = 0.01 // default
	}

	// Assign the range
	s.calc.AssignRangeToWorker(req.WorkerName, req.Function, req.LowerBound, req.UpperBound, req.IntervalSize)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":       "Range assigned to worker",
		"worker_name":   req.WorkerName,
		"function":      req.Function,
		"lower_bound":   req.LowerBound,
		"upper_bound":   req.UpperBound,
		"interval_size": req.IntervalSize,
	})
}

// GET /ranges/list - List all assigned ranges
func (s *Server) handleListRanges(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ranges := s.calc.GetWorkerRanges()
	rangeInfos := make([]WorkerRangeInfo, 0, len(ranges))

	for _, wr := range ranges {
		totalRange := wr.UpperBound - wr.LowerBound
		progress := 0.0
		if totalRange > 0 {
			progress = ((wr.CurrentPoint - wr.LowerBound) / totalRange) * 100
		}

		rangeInfos = append(rangeInfos, WorkerRangeInfo{
			WorkerName:   wr.WorkerName,
			Function:     wr.Function,
			LowerBound:   wr.LowerBound,
			UpperBound:   wr.UpperBound,
			CurrentPoint: wr.CurrentPoint,
			Progress:     progress,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ranges": rangeInfos,
		"total":  len(rangeInfos),
	})
}

// POST /ranges/clear - Clear all range assignments
func (s *Server) handleClearRanges(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.calc.ClearWorkerRanges()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "All range assignments cleared",
	})
}

func (s *Server) sendError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(ErrorResponse{Error: message})
}

func (s *Server) enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

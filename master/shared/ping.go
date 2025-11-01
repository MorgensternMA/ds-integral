package shared

// PingArgs is sent to a worker
type PingArgs struct {
	WorkerName string
}

// PingResponse is sent to a master
type PingResponse struct{}
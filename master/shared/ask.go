package shared

// AskArgs are passed to Ask method
type AskArgs struct {
	WorkerName string
}

// AskReply is returned from an Ask method call
type AskReply struct {
	LowerBound float64
	UpperBound float64
	NumPoints  uint64
	Function   string
	JobID      uint64
}
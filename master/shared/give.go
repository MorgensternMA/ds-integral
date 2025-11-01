package shared

type GiveArgs struct {
	JobID     uint64
	Result    []byte
	Precision uint
}

type GiveReply struct{}
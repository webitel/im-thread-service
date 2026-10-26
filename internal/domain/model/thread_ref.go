package model

// SourceRef is one row of im_thread.source_ref: display data of a provider gate.
type SourceRef struct {
	GateID   string
	DomainID int
	Name     string
	Type     string
}

// QueueRef is one row of im_thread.queue_ref.
type QueueRef struct {
	DomainID int
	QueueID  int64
	Name     string
}

// QueueInfo is the call_center queue through which an agent receives a thread.
type QueueInfo struct {
	ID   int64
	Name string
}

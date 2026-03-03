package dto

type ReadMessageRequest struct {
	MessageID string `json:"message_id"`
	ThreadID  string `json:"thread_id"`
	UserID    string `json:"user_id"`
	DomainID  int32  `json:"domain_id"`
}

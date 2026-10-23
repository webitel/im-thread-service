package dto

import (
	"time"

	"github.com/google/uuid"

	"github.com/webitel/im-thread-service/internal/domain/model"
)

type GivePreviewRequest struct {
	ThreadID           uuid.UUID
	ContactID          uuid.UUID
	InitiatorContactID uuid.UUID
	DomainID           int
	Duration           time.Duration
}

type RemoveFromPreviewRequest struct {
	ThreadID           uuid.UUID
	ContactID          uuid.UUID
	InitiatorContactID uuid.UUID // optional, logged only
	DomainID           int
}

type UpgradePreviewRequest struct {
	ThreadID           uuid.UUID
	ContactID          uuid.UUID
	InitiatorContactID uuid.UUID
	DomainID           int
	Role               model.ThreadRole
	// Queue is the queue through which the member receives the thread; nil when unknown.
	Queue *model.QueueInfo
}

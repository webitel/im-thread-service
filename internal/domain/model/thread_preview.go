package model

import (
	"time"

	"github.com/google/uuid"
)

type PreviewRevokeReason string

const (
	PreviewRevokeReasonRemove  PreviewRevokeReason = "remove"
	PreviewRevokeReasonUpgrade PreviewRevokeReason = "upgrade"
	// PreviewRevokeReasonExpired closes a lapsed preview so a fresh grant can take its unique slot.
	PreviewRevokeReasonExpired PreviewRevokeReason = "expired"
)

// ThreadPreview is a read-only, time-limited look at a thread for a contact who is not
// a member; active while RevokedAt is nil and ExpiresAt is in the future.
type ThreadPreview struct {
	ID           uuid.UUID            `json:"id" db:"id"`
	DomainID     int                  `json:"domain_id" db:"domain_id"`
	ThreadID     uuid.UUID            `json:"thread_id" db:"thread_id"`
	ContactID    uuid.UUID            `json:"contact_id" db:"contact_id"`
	InitiatorID  uuid.UUID            `json:"initiator_id" db:"initiator_id"`
	CreatedAt    time.Time            `json:"created_at" db:"created_at"`
	ExpiresAt    time.Time            `json:"expires_at" db:"expires_at"`
	RevokedAt    *time.Time           `json:"revoked_at" db:"revoked_at"`
	RevokeReason *PreviewRevokeReason `json:"revoke_reason" db:"revoke_reason"`
}

func (p *ThreadPreview) CreatedAtUnixMilli() int64 {
	return p.CreatedAt.UnixMilli()
}

func (p *ThreadPreview) ExpiresAtUnixMilli() int64 {
	return p.ExpiresAt.UnixMilli()
}

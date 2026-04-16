package model

import (
	"maps"
	"time"

	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/shared"
)

type SystemMessage struct {
	ID             uuid.UUID      `json:"id" db:"id"`
	ThreadID       uuid.UUID      `json:"threadId" db:"thread_id"`
	DomainID       int32          `json:"domainId" db:"domain_id"`
	IdempotencyKey string         `json:"idempotencyKey" db:"-"`
	From           shared.Peer    `json:"from" db:"-"`
	To             shared.Peer    `json:"to" db:"-"`
	Members        uuid.UUIDs     `json:"members" db:"-"`
	Type           string         `json:"type" db:"type"`
	Body           string         `json:"body" db:"body"`
	Metadata       map[string]any `json:"metadata" db:"metadata"`
	CreatedAt      time.Time      `json:"createdAt" db:"created_at"`

	domainEvents []event.Outboxer
}

func (m *SystemMessage) AddEvent(e event.Outboxer) {
	m.domainEvents = append(m.domainEvents, e)
}

func (m *SystemMessage) Events() []event.Outboxer {
	e := m.domainEvents
	m.domainEvents = nil
	return e
}

func (m *SystemMessage) WithCreatedEvent(sendID string) *SystemMessage {
	if m == nil {
		return m
	}

	metadata := maps.Clone(m.Metadata)
	if metadata == nil {
		metadata = make(map[string]any)
	}
	metadata["system_type"] = m.Type

	e := event.MessageCreated{
		MessageID:  m.ID,
		ThreadID:   m.ThreadID,
		DomainID:   m.DomainID,
		To:         m.Members,
		SendID:     sendID,
		Body:       m.Body,
		Type:       int16(MessageTypeSystem),
		OccurredAt: m.CreatedAt,
		Metadata:   metadata,
	}

	m.AddEvent(e)
	return m
}

package event

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

const (
	MemberRemovedTopicV1   = "thread.member_added.v1.%s.%s" // thread_id, recipient_id
	MemberRemovedVersionV1 = "v1"
)

type MemberRemovedV1 struct {
	Recipient         uuid.UUID
	ThreadID          uuid.UUID `json:"thread_id"`
	MemberID          uuid.UUID `json:"member_id"`
	NewThreadDialogID uuid.UUID `json:"thread_dialog_id"`
	InitiatorMemberID uuid.UUID `json:"initiator_id"`
}

func (e *MemberRemovedV1) Topic() string {
	return fmt.Sprintf(MemberAddedTopicV1, e.ThreadID.String(), e.Recipient.String())
}

func (e *MemberRemovedV1) MustBeThreadEvent() {}

func (e *MemberRemovedV1) EventType() string { return "member_added" }

func (e *MemberRemovedV1) Version() string { return MemberRemovedVersionV1 }

func (e *MemberRemovedV1) RecipientID() uuid.UUID { return e.Recipient }

func (e *MemberRemovedV1) ToOutbox() (OutboxEvent, error) {
	return e.serialize(e, e.Version())
}
func (e *MemberRemovedV1) serialize(data any, version string) (OutboxEvent, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return OutboxEvent{}, err
	}

	return OutboxEvent{
		ID:      uuid.Must(uuid.NewV7()),
		Payload: payload,
		Metadata: map[string]string{
			"event_type": ThreadCreatedEvent,
			"version":    version,
		},
	}, nil
}

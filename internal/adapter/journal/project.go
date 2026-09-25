package journal

import (
	"encoding/json"
	"maps"

	"github.com/webitel/im-thread-service/internal/domain/event"
)

// Journal entry kinds: canonical strings for thread_updates.kind.
const (
	KindMessageNew      = "message.new"
	KindMessageEdited   = "message.edited"
	KindMessageDeleted  = "message.deleted"
	KindMessageReaction = "message.reaction"
	KindMemberChanged   = "member.changed"
	KindThreadCreated   = "thread.created"
)

// Journal field keys. Entries keep only references; message content is read from
// history at catch-up time, so the journal never stores bodies.
const (
	FieldMsgID     = "msg_id"
	FieldActor     = "actor"
	FieldEmoji     = "emoji"
	FieldAction    = "action"
	FieldContactID = "contact_id"
)

// Member change actions stored under FieldAction.
const (
	ActionJoined = "joined"
	ActionLeft   = "left"
)

// ProjectEvent maps an outbox event to a journal Update; ok=false means not journaled.
func ProjectEvent(eventType string, payload []byte) (Update, bool, error) {
	switch eventType {
	case event.ThreadCreatedEvent:
		var e event.ThreadCreated
		if err := json.Unmarshal(payload, &e); err != nil {
			return Update{}, false, err
		}

		return Update{ThreadID: e.ID.String(), Kind: KindThreadCreated, Fields: make(map[string]any)}, true, nil
	case event.MessageCreatedEvent:
		var e event.MessageCreated
		if err := json.Unmarshal(payload, &e); err != nil {
			return Update{}, false, err
		}

		return messageUpdate(e.ThreadID.String(), KindMessageNew, e.MessageID.String(), memberContact(e.From), nil), true, nil

	case event.MessageEditedEvent:
		var e event.MessageEdited
		if err := json.Unmarshal(payload, &e); err != nil {
			return Update{}, false, err
		}

		return messageUpdate(e.ThreadID.String(), KindMessageEdited, e.MessageID.String(), memberContact(e.EditedBy), nil), true, nil

	case event.MessageDeletedEvent:
		var e event.MessageDeleted
		if err := json.Unmarshal(payload, &e); err != nil {
			return Update{}, false, err
		}

		return messageUpdate(e.ThreadID.String(), KindMessageDeleted, e.MessageID.String(), memberContactFromMember(e.DeletedBy), nil), true, nil

	case event.MessageReactionEvent:
		var e event.MessageReaction
		if err := json.Unmarshal(payload, &e); err != nil {
			return Update{}, false, err
		}

		return messageUpdate(e.ThreadID.String(), KindMessageReaction, e.MessageID.String(), memberContact(e.Reactor),
			map[string]any{FieldEmoji: e.Emoji, FieldAction: e.Action}), true, nil

	case event.MemberJoinedEvent:
		var e event.MemberJoined
		if err := json.Unmarshal(payload, &e); err != nil {
			return Update{}, false, err
		}

		return memberUpdate(e.ThreadID.String(), e.ContactID.String(), ActionJoined), true, nil

	case event.MemberLeftEvent:
		var e event.MemberLeft
		if err := json.Unmarshal(payload, &e); err != nil {
			return Update{}, false, err
		}

		return memberUpdate(e.ThreadID.String(), e.ContactID.String(), ActionLeft), true, nil

	default:
		return Update{}, false, nil // status receipts, typing and the rest are not journaled
	}
}

func messageUpdate(threadID, kind, msgID, actor string, extra map[string]any) Update {
	fields := map[string]any{FieldMsgID: msgID, FieldActor: actor}
	maps.Copy(fields, extra)

	return Update{ThreadID: threadID, Kind: kind, Fields: fields}
}

func memberUpdate(threadID, contactID, action string) Update {
	return Update{ThreadID: threadID, Kind: KindMemberChanged, Fields: map[string]any{
		FieldContactID: contactID, FieldActor: contactID, FieldAction: action,
	}}
}

func memberContact(m *event.ThreadMember) string {
	if m == nil {
		return ""
	}

	return m.ContactID.String()
}

// memberContactFromMember mirrors memberContact for *event.Member.
func memberContactFromMember(m *event.Member) string {
	if m == nil || m.Contact == nil {
		return ""
	}

	return m.Contact.ID
}

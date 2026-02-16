package model

import (
	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/shared"
)

type ThreadKind = int

const (
	ThreadUnspecified ThreadKind = iota
	ThreadDirect                 // user to user
	ThreadGroup                  //group with many users, bots etc.
	ThreadChannel                //channel, right now not implemented!
)

type Thread struct {
	shared.BaseModel

	Kind       ThreadKind      `json:"kind"`
	Owner      uuid.UUID       `json:"owner"`
	Admins     []uuid.UUID     `json:"admins"`
	MembersIds []uuid.UUID     `json:"members_ids"`
	Members    []*ThreadMember `json:"members"`

	Subject     string `json:"subject"`
	Description string `json:"description"`

	events []event.Base
}

func (t *Thread) AddEvents(e ...event.Base) {
	t.events = append(t.events, e...)
}

func (t *Thread) PullEvents() []event.Base {
	events := t.events
    t.events = nil
    return events
}


type ThreadBuilder struct {
	thread *Thread
}

func NewThreadBuilder() *ThreadBuilder {
	return &ThreadBuilder{
		// Initialize the thread with default values
		thread: &Thread{
			BaseModel: shared.BaseModel{},
			Kind:      ThreadUnspecified,
			Owner:     uuid.Nil,
			Admins:    []uuid.UUID{},
			MembersIds: []uuid.UUID{},
			Members:    []*ThreadMember{},
			Subject:    "",
			Description: "",
		},
	}
}

func (b *ThreadBuilder) WithKind(kind ThreadKind) *ThreadBuilder {
	b.thread.Kind = kind
	return b
}

func (b *ThreadBuilder) WithOwner(owner uuid.UUID) *ThreadBuilder {
	b.thread.Owner = owner
	return b
}

func (b *ThreadBuilder) WithAdmins(admins []uuid.UUID) *ThreadBuilder {
	b.thread.Admins = admins
	return b
}

func (b *ThreadBuilder) WithMembersIds(memberIds []uuid.UUID) *ThreadBuilder {
	b.thread.MembersIds = memberIds
	return b
}

func (b *ThreadBuilder) WithMembers(members []*ThreadMember) *ThreadBuilder {
	b.thread.Members = members
	return b
}

func (b *ThreadBuilder) WithSubject(subject string) *ThreadBuilder {
	b.thread.Subject = subject
	return b
}

func (b *ThreadBuilder) WithDescription(description string) *ThreadBuilder {
	b.thread.Description = description
	return b
}

func (b *ThreadBuilder) WithID(id uuid.UUID) *ThreadBuilder {
	b.thread.ID = id
	return b
}

func (b *ThreadBuilder) WithDomainID(domainID int) *ThreadBuilder {
	b.thread.DomainID = domainID
	return b
}

func (b *ThreadBuilder) Build() *Thread {
	return b.thread
}

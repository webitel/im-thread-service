// Package model defines the core domain entities and business logic rules
// for the IM Thread service. This package is the heart of the application
// and must remain independent of any external frameworks or transport layers.
package shared

import (
	"time"

	"github.com/google/uuid"
)

type PeerType int16

//go:generate stringer -type=PeerType
const (
	PeerContact PeerType = iota + 1
	PeerGroup
	PeerChannel
	PeerThread
)

type Identity struct {
	Issuer string
	Name   string
	Via    *string
}

type Peer struct {
	ID       uuid.UUID `json:"id" db:"id"`
	Type     PeerType  `json:"type" db:"type"`
	Identity *Identity `json:"identity"`
}

func (p Peer) ResolveContactID() *uuid.UUID {
	if p.Type != PeerContact {
		return nil
	}
	return &p.ID
}

func (p Peer) ResolveThreadID() *uuid.UUID {
	if p.Type != PeerThread {
		return nil
	}
	return &p.ID
}

func (p Peer) ResolveVia() *string {
	identity := p.Identity
	if identity == nil {
		return nil
	}

	return identity.Via
}

type BaseModel struct {
	ID        uuid.UUID `json:"id"`
	DomainID  int       `json:"domain_id"`
	CreatedBy int       `json:"created_by"`
	UpdatedBy int       `json:"updated_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (b *BaseModel) CreatedAtMilliseconds() int64 {
	return max(b.CreatedAt.UnixMilli(), 0)
}

func (b *BaseModel) UpdatedAtMilliseconds() int64 {
	return max(b.UpdatedAt.UnixMilli(), 0)
}

type Entity struct {
	Type   string `json:"type"`
	Offset int    `json:"offset"`
	Length int    `json:"length"`
	Value  string `json:"value"`
}

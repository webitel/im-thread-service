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

// ViaInfo is the gate (im-providers) an identity came through.
type ViaInfo struct {
	// ID is the gate id.
	ID string
	// Name is the gate display name.
	Name string
	// Type is the provider type, e.g. facebook | whatsapp | telegram_bot.
	Type string
}

type Identity struct {
	Issuer   string
	Name     string
	ChatName string
	ViaInfo  *ViaInfo
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

// ResolveVia returns the gate id of the peer, or nil when it has none.
func (p Peer) ResolveVia() *string {
	identity := p.Identity
	if identity == nil || identity.ViaInfo == nil || identity.ViaInfo.ID == "" {
		return nil
	}

	gateID := identity.ViaInfo.ID

	return &gateID
}

func (p Peer) ResolveViaInfo() *ViaInfo {
	identity := p.Identity
	if identity == nil {
		return nil
	}

	return identity.ViaInfo
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
	Value  string `json:"value,omitempty"`
}

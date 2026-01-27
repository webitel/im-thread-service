// Package model defines the core domain entities and business logic rules
// for the IM Thread service. This package is the heart of the application
// and must remain independent of any external frameworks or transport layers.
package model

import (
	"time"

	"github.com/google/uuid"
)

type PeerType int

const (
	PeerUser PeerType = iota
	PeerChat
	PeerChannel
	PeerBot
)

type Identity struct {
	Issuer string
	Name   string
}

type Peer struct {
	ID       uuid.UUID `json:"id" db:"id"`
	Type     PeerType  `json:"type" db:"type"`
	Identity *Identity `json:"identity"`
}

type BaseModel struct {
	ID        uuid.UUID `json:"id"`
	DomainID  int       `json:"domain_id"`
	CreatedBy int       `json:"created_by"`
	UpdatedBy int       `json:"updated_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Entity struct {
	Type   string `json:"type"`
	Offset int    `json:"offset"`
	Length int    `json:"length"`
	Value  string `json:"value"`
}

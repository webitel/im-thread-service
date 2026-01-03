package model

import "github.com/google/uuid"

type PeerType = int

const (
	PeerUser = iota
	PeerChat
	PeerChannel
)

type Peer struct {
	Id   uuid.UUID `json:"id" db:"id"`
	Type PeerType  `json:"type" db:"type"`
}

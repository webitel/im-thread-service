package dto

import (
	"github.com/google/uuid"
	impb "github.com/webitel/im-thread-service/gen/go/api/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
)

type (
	SendTextRequest struct {
		From model.Peer `json:"from"`
		To   model.Peer `json:"to"`
		Body string     `json:"body"`
	}

	SendTextResponse struct {
		To model.Peer `json:"to"`
		Id uuid.UUID  `json:"id"`
	}
)

func NewPeerFromProto(pb *impb.Peer) model.Peer {
	if pb == nil {
		return model.Peer{}
	}

	var p model.Peer
	switch kind := pb.Kind.(type) {
	case *impb.Peer_UserId:
		p.Id, _ = uuid.Parse(kind.UserId)
		p.Type = model.PeerUser
	case *impb.Peer_ChatId:
		p.Id, _ = uuid.Parse(kind.ChatId)
		p.Type = model.PeerChat
	case *impb.Peer_ChannelId:
		p.Id, _ = uuid.Parse(kind.ChannelId)
		p.Type = model.PeerChannel
	}
	return p
}

func NewSendTextRequest(in *impb.SendTextRequest) *SendTextRequest {
	return &SendTextRequest{
		From: NewPeerFromProto(in.GetFrom()),
		To:   NewPeerFromProto(in.GetTo()),
		Body: in.GetBody(),
	}
}

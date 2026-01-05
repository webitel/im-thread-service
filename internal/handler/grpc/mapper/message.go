package mapper

import (
	"github.com/google/uuid"
	impb "github.com/webitel/im-thread-service/gen/go/api/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

func MapToSendTextRequest(in *impb.SendTextRequest) *dto.SendTextRequest {
	if in == nil {
		return nil
	}
	return &dto.SendTextRequest{
		From: MapPeerFromProto(in.GetFrom()),
		To:   MapPeerFromProto(in.GetTo()),
		Body: in.GetBody(),
	}
}

func MapPeerFromProto(pb *impb.Peer) model.Peer {
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

func MapToSendTextResponse(out *dto.SendTextResponse) *impb.SendTextResponse {
	if out == nil {
		return nil
	}
	return &impb.SendTextResponse{
		Id: out.Id.String(),
		To: MapPeerToProto(out.To),
	}
}

func MapPeerToProto(p model.Peer) *impb.Peer {
	idStr := p.Id.String()
	res := &impb.Peer{}
	switch p.Type {
	case model.PeerUser:
		res.Kind = &impb.Peer_UserId{UserId: idStr}
	case model.PeerChat:
		res.Kind = &impb.Peer_ChatId{ChatId: idStr}
	case model.PeerChannel:
		res.Kind = &impb.Peer_ChannelId{ChannelId: idStr}
	}
	return res
}

package mapper

import (
	"github.com/google/uuid"
	impb "github.com/webitel/im-thread-service/gen/go/api/v1"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

func MapToSendTextRequest(in *impb.SendTextRequest) *dto.SendTextRequest {
	if in == nil {
		return nil
	}
	return &dto.SendTextRequest{
		From:     MapPeerFromProto(in.GetFrom()),
		To:       MapPeerFromProto(in.GetTo()),
		Body:     in.GetBody(),
		DomainID: in.GetDomainId(),
	}
}

func MapPeerFromProto(pb *impb.Peer) shared.Peer {
	if pb == nil {
		return shared.Peer{}
	}

	var p shared.Peer
	switch kind := pb.Kind.(type) {
	case *impb.Peer_UserId:
		p.ID, _ = uuid.Parse(kind.UserId)
		p.Type = shared.PeerContact
	case *impb.Peer_ChatId:
		p.ID, _ = uuid.Parse(kind.ChatId)
		p.Type = shared.PeerGroup
	case *impb.Peer_ChannelId:
		p.ID, _ = uuid.Parse(kind.ChannelId)
		p.Type = shared.PeerChannel
	}
	return p
}

func MapToSendTextResponse(out *dto.SendTextResponse) *impb.SendTextResponse {
	if out == nil {
		return nil
	}
	return &impb.SendTextResponse{
		Id: out.ID.String(),
		To: MapPeerToProto(out.To),
	}
}

func MapPeerToProto(p shared.Peer) *impb.Peer {
	idStr := p.ID.String()
	res := &impb.Peer{}
	switch p.Type {
	case shared.PeerContact:
		res.Kind = &impb.Peer_UserId{UserId: idStr}
	case shared.PeerGroup:
		res.Kind = &impb.Peer_ChatId{ChatId: idStr}
	case shared.PeerChannel:
		res.Kind = &impb.Peer_ChannelId{ChannelId: idStr}
	}
	return res
}

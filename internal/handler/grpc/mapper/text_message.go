package mapper

import (
	"github.com/google/uuid"
	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
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
	case *impb.Peer_ContactId:
		p.ID, _ = uuid.Parse(kind.ContactId)
		p.Type = shared.PeerContact
	case *impb.Peer_GroupId:
		p.ID, _ = uuid.Parse(kind.GroupId)
		p.Type = shared.PeerGroup
	case *impb.Peer_ChannelId:
		p.ID, _ = uuid.Parse(kind.ChannelId)
		p.Type = shared.PeerChannel
	}

	p.Identity = &shared.Identity{
		Issuer: pb.GetIdentity().GetIssuer(),
		Name:   pb.GetIdentity().GetName(),
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
		res.Kind = &impb.Peer_ContactId{ContactId: idStr}
	case shared.PeerGroup:
		res.Kind = &impb.Peer_GroupId{GroupId: idStr}
	case shared.PeerChannel:
		res.Kind = &impb.Peer_ChannelId{ChannelId: idStr}
	}
	return res
}

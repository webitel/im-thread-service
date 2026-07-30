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

	sendAs, _ := uuid.Parse(in.GetSendAs())

	return &dto.SendTextRequest{
		From:              MapPeerFromProto(in.GetFrom()),
		To:                MapPeerFromProto(in.GetTo()),
		Body:              in.GetBody(),
		DomainID:          in.GetDomainId(),
		SendID:            in.GetSendId(),
		SendAs:            &sendAs,
		ReplyToMessageID:  ParseOptionalUUID(in.GetReplyToMessageId()),
		ExternalID:        in.GetExternalId(),
		ReplyToExternalID: in.GetReplyToExternalId(),
	}
}

// MapToSendTypingRequest maps a proto typing request to its DTO. The target
// thread is taken from the `to` peer's thread_id variant; other peer kinds
// leave ThreadID zero and are rejected by the guard.
func MapToSendTypingRequest(in *impb.SendTypingRequest) *dto.SendTypingRequest {
	if in == nil {
		return nil
	}

	var threadID uuid.UUID
	if tid := MapPeerFromProto(in.GetTo()).ResolveThreadID(); tid != nil {
		threadID = *tid
	}

	return &dto.SendTypingRequest{
		From:        MapPeerFromProto(in.GetFrom()),
		ThreadID:    threadID,
		DomainID:    in.GetDomainId(),
		TimeoutMS:   in.GetTimeoutMs(),
		PreviewText: in.GetPreviewText(),
	}
}

func MapPeerFromProto(pb *impb.Peer) shared.Peer {
	if pb == nil {
		return shared.Peer{}
	}

	var p shared.Peer

	switch kind := pb.GetKind().(type) {
	case *impb.Peer_ContactId:
		p.ID, _ = uuid.Parse(kind.ContactId)

		p.Type = shared.PeerContact
		if id := pb.GetIdentity(); id != nil {
			p.Identity = &shared.Identity{
				Issuer: id.GetIssuer(),
				Name:   id.GetName(),
				Via:    id.Via,
			}
		}
	case *impb.Peer_GroupId:
		p.ID, _ = uuid.Parse(kind.GroupId)
		p.Type = shared.PeerGroup
	case *impb.Peer_ChannelId:
		p.ID, _ = uuid.Parse(kind.ChannelId)
		p.Type = shared.PeerChannel
	case *impb.Peer_ThreadId:
		p.ID, _ = uuid.Parse(kind.ThreadId)
		p.Type = shared.PeerThread
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
	case shared.PeerThread:
		res.Kind = &impb.Peer_ThreadId{ThreadId: idStr}
	}

	return res
}

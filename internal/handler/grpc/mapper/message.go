package mapper

import (
	impb "github.com/webitel/im-thread-service/gen/go/api/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
)

func ToProtoPeer(p model.Peer) *impb.Peer {
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

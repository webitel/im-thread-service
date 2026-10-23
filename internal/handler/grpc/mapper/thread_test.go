package mapper

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
)

func TestConvertGet_EmptyCallerID(t *testing.T) {
	id := uuid.NewString()
	in := &impb.GetThreadRequest{
		Id:       id,
		CallerId: "", // empty caller_id
	}

	out, err := (&ThreadInConverter{}).ConvertGet(in)
	require.NoError(t, err)
	require.Equal(t, uuid.Nil, out.CallerID)
}

func TestConvertGet_ValidCallerID(t *testing.T) {
	id := uuid.NewString()
	callerID := uuid.NewString()
	in := &impb.GetThreadRequest{
		Id:       id,
		CallerId: callerID,
	}

	out, err := (&ThreadInConverter{}).ConvertGet(in)
	require.NoError(t, err)

	parsedCaller, _ := uuid.Parse(callerID)
	require.Equal(t, parsedCaller, out.CallerID)
}

func TestConvertGet_InvalidCallerID(t *testing.T) {
	id := uuid.NewString()
	in := &impb.GetThreadRequest{
		Id:       id,
		CallerId: "not-a-uuid",
	}

	out, err := (&ThreadInConverter{}).ConvertGet(in)
	require.Nil(t, out)
	require.Error(t, err)
	require.Equal(t, codes.InvalidArgument, errors.Code(err))
}

func TestConvertGet_NilUUID(t *testing.T) {
	id := uuid.NewString()
	nilUUID := "00000000-0000-0000-0000-000000000000"
	in := &impb.GetThreadRequest{
		Id:       id,
		CallerId: nilUUID,
	}

	out, err := (&ThreadInConverter{}).ConvertGet(in)
	require.Nil(t, out)
	require.Error(t, err)
	require.Equal(t, codes.InvalidArgument, errors.Code(err))
}

func TestMapSearchVariablesRequestToQuery_CopiesCallerAndDomain(t *testing.T) {
	callerID := uuid.NewString()
	req := &impb.SearchVariablesRequest{
		Size:     10,
		Page:     1,
		CallerId: callerID,
		DomainId: 42,
	}

	query, err := MapSearchVariablesRequestToQuery(req)
	require.NoError(t, err)

	parsedCaller, _ := uuid.Parse(callerID)
	require.Equal(t, parsedCaller, query.CallerID)
	require.Equal(t, 42, query.DomainID)
}

func TestConvertGivePreviewRequest_ValidRequest(t *testing.T) {
	threadID := uuid.NewString()
	contactID := uuid.NewString()
	initiatorID := uuid.NewString()

	in := &impb.GivePreviewRequest{
		ThreadId:           threadID,
		ContactId:          contactID,
		InitiatorContactId: initiatorID,
		DomainId:           1,
		DurationMs:         1500,
	}

	out, err := ConvertGivePreviewRequest(in)
	require.NoError(t, err)
	require.Equal(t, time.Duration(1500)*time.Millisecond, out.Duration)
}

func TestConvertToThreadPreview_Nil(t *testing.T) {
	result := ConvertToThreadPreview(nil)
	require.Nil(t, result)
}

func TestConvertToThreadPreview_ValidPreview(t *testing.T) {
	id := uuid.New()
	threadID := uuid.New()
	contactID := uuid.New()
	initiatorID := uuid.New()
	createdAt := time.Now().UTC()
	expiresAt := createdAt.Add(time.Hour)

	preview := &model.ThreadPreview{
		ID:          id,
		ThreadID:    threadID,
		ContactID:   contactID,
		InitiatorID: initiatorID,
		DomainID:    5,
		CreatedAt:   createdAt,
		ExpiresAt:   expiresAt,
	}

	result := ConvertToThreadPreview(preview)
	require.NotNil(t, result)
	require.Equal(t, id.String(), result.GetId())
	require.Equal(t, threadID.String(), result.GetThreadId())
	require.Equal(t, contactID.String(), result.GetContactId())
	require.Equal(t, initiatorID.String(), result.GetInitiatorContactId())
	require.Equal(t, int32(5), result.GetDomainId())
	require.Equal(t, createdAt.UnixMilli(), result.GetCreatedAt())
	require.Equal(t, expiresAt.UnixMilli(), result.GetExpiresAt())
}

func TestMapPeerFromProto_ViaRef(t *testing.T) {
	withRef := MapPeerFromProto(&impb.Peer{
		Kind: &impb.Peer_ContactId{ContactId: uuid.NewString()},
		Identity: &impb.Identity{
			ViaRef: &impb.Via{Id: "gate-1", Name: "Bot", Type: "telegram_bot"},
		},
	})
	require.NotNil(t, withRef.Identity)
	require.Equal(t, &shared.ViaInfo{ID: "gate-1", Name: "Bot", Type: "telegram_bot"}, withRef.Identity.ViaInfo)

	idOnly := MapPeerFromProto(&impb.Peer{
		Kind:     &impb.Peer_ContactId{ContactId: uuid.NewString()},
		Identity: &impb.Identity{ViaRef: &impb.Via{Id: "gate-2"}},
	})
	require.Equal(t, &shared.ViaInfo{ID: "gate-2"}, idOnly.Identity.ViaInfo)
}

func TestMapPeerFromProto_LegacyVia(t *testing.T) {
	legacy := "gate-legacy"

	// Old clients send only the deprecated via field: it becomes ViaInfo.ID.
	onlyLegacy := MapPeerFromProto(&impb.Peer{
		Kind:     &impb.Peer_ContactId{ContactId: uuid.NewString()},
		Identity: &impb.Identity{Via: &legacy}, //nolint:staticcheck // covers the deprecated field
	})
	require.Equal(t, &shared.ViaInfo{ID: "gate-legacy"}, onlyLegacy.Identity.ViaInfo)

	// Legacy via fills the id when via_ref carries only name/type.
	legacyWithInfo := MapPeerFromProto(&impb.Peer{
		Kind: &impb.Peer_ContactId{ContactId: uuid.NewString()},
		Identity: &impb.Identity{
			Via:    &legacy, //nolint:staticcheck // covers the deprecated field
			ViaRef: &impb.Via{Name: "Support", Type: "facebook"},
		},
	})
	require.Equal(t, &shared.ViaInfo{ID: "gate-legacy", Name: "Support", Type: "facebook"}, legacyWithInfo.Identity.ViaInfo)

	// On conflict via_ref.id wins.
	both := MapPeerFromProto(&impb.Peer{
		Kind: &impb.Peer_ContactId{ContactId: uuid.NewString()},
		Identity: &impb.Identity{
			Via:    &legacy, //nolint:staticcheck // covers the deprecated field
			ViaRef: &impb.Via{Id: "gate-new", Name: "Support", Type: "facebook"},
		},
	})
	require.Equal(t, &shared.ViaInfo{ID: "gate-new", Name: "Support", Type: "facebook"}, both.Identity.ViaInfo)

	none := MapPeerFromProto(&impb.Peer{
		Kind:     &impb.Peer_ContactId{ContactId: uuid.NewString()},
		Identity: &impb.Identity{Name: "customer"},
	})
	require.Nil(t, none.Identity.ViaInfo)
}

func TestConvertAddMemberRequest_Queue(t *testing.T) {
	base := func(q *impb.ThreadQueue) *impb.AddMemberRequest {
		return &impb.AddMemberRequest{
			ThreadId:           uuid.NewString(),
			NewMemberContactId: uuid.NewString(),
			Role:               impb.ThreadRole_ROLE_MEMBER,
			DomainId:           1,
			Queue:              q,
		}
	}

	converter := &ThreadInConverter{}

	withQueue, err := converter.ConvertAddMemberRequest(base(&impb.ThreadQueue{Id: 7, Name: "Sales"}))
	require.NoError(t, err)
	require.Equal(t, &model.QueueInfo{ID: 7, Name: "Sales"}, withQueue.Queue)

	noQueue, err := converter.ConvertAddMemberRequest(base(nil))
	require.NoError(t, err)
	require.Nil(t, noQueue.Queue)

	zeroQueue, err := converter.ConvertAddMemberRequest(base(&impb.ThreadQueue{Id: 0, Name: "Sales"}))
	require.NoError(t, err)
	require.Nil(t, zeroQueue.Queue)
}

func TestConvertTransferThreadRequest_Queue(t *testing.T) {
	base := func(q *impb.ThreadQueue) *impb.TransferRequest {
		return &impb.TransferRequest{
			ThreadId:           uuid.NewString(),
			InitiatorContactId: uuid.NewString(),
			NewMemberContactId: uuid.NewString(),
			Role:               impb.ThreadRole_ROLE_MEMBER,
			Queue:              q,
		}
	}

	converter := &ThreadInConverter{}

	withQueue, err := converter.ConvertTransferThreadRequest(base(&impb.ThreadQueue{Id: 7, Name: "Sales"}))
	require.NoError(t, err)
	require.Equal(t, &model.QueueInfo{ID: 7, Name: "Sales"}, withQueue.Queue)

	noQueue, err := converter.ConvertTransferThreadRequest(base(nil))
	require.NoError(t, err)
	require.Nil(t, noQueue.Queue)

	zeroQueue, err := converter.ConvertTransferThreadRequest(base(&impb.ThreadQueue{Id: 0}))
	require.NoError(t, err)
	require.Nil(t, zeroQueue.Queue)
}

func TestConvertUpgradePreviewRequest_Queue(t *testing.T) {
	base := func(q *impb.ThreadQueue) *impb.UpgradePreviewToFullMemberRequest {
		return &impb.UpgradePreviewToFullMemberRequest{
			ThreadId:           uuid.NewString(),
			ContactId:          uuid.NewString(),
			InitiatorContactId: uuid.NewString(),
			DomainId:           1,
			Role:               impb.ThreadRole_ROLE_MEMBER,
			Queue:              q,
		}
	}

	withQueue, err := ConvertUpgradePreviewRequest(base(&impb.ThreadQueue{Id: 7, Name: "Sales"}))
	require.NoError(t, err)
	require.Equal(t, &model.QueueInfo{ID: 7, Name: "Sales"}, withQueue.Queue)

	noQueue, err := ConvertUpgradePreviewRequest(base(nil))
	require.NoError(t, err)
	require.Nil(t, noQueue.Queue)
}

func TestConvertToThread_SourceAndQueue(t *testing.T) {
	converter := &ThreadOutConverter{}

	withFields := converter.ConvertToThread(&model.Thread{
		ID:     uuid.New(),
		Source: &model.ThreadSource{ID: "gate-1", Name: "Support", Type: "facebook"},
		Queue:  &model.ThreadQueue{ID: 7, Name: "Sales"},
	})
	require.NotNil(t, withFields.GetSource())
	require.Equal(t, "gate-1", withFields.GetSource().GetId())
	require.Equal(t, "Support", withFields.GetSource().GetName())
	require.Equal(t, "facebook", withFields.GetSource().GetType())
	require.NotNil(t, withFields.GetQueue())
	require.Equal(t, int64(7), withFields.GetQueue().GetId())
	require.Equal(t, "Sales", withFields.GetQueue().GetName())

	withoutFields := converter.ConvertToThread(&model.Thread{ID: uuid.New()})
	require.Nil(t, withoutFields.GetSource())
	require.Nil(t, withoutFields.GetQueue())
}

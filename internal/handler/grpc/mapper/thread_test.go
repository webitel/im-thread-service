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
	require.Equal(t, id.String(), result.Id)
	require.Equal(t, threadID.String(), result.ThreadId)
	require.Equal(t, contactID.String(), result.ContactId)
	require.Equal(t, initiatorID.String(), result.InitiatorContactId)
	require.Equal(t, int32(5), result.DomainId)
	require.Equal(t, createdAt.UnixMilli(), result.CreatedAt)
	require.Equal(t, expiresAt.UnixMilli(), result.ExpiresAt)
}

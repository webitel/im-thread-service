package mapper

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
)

func TestMapSendRequests_RetryOf(t *testing.T) {
	t.Parallel()

	retryOf := uuid.New()

	text := MapToSendTextRequest(&impb.SendTextRequest{Body: "hi", RetryOf: proto.String(retryOf.String())})
	require.NotNil(t, text.RetryOf)
	require.Equal(t, retryOf, *text.RetryOf)

	doc := MapToSendDocumentRequest(&impb.SendDocumentRequest{RetryOf: proto.String(retryOf.String())})
	require.NotNil(t, doc.RetryOf)
	require.Equal(t, retryOf, *doc.RetryOf)

	require.Nil(t, MapToSendTextRequest(&impb.SendTextRequest{Body: "hi"}).RetryOf)
	require.Nil(t, MapToSendDocumentRequest(&impb.SendDocumentRequest{}).RetryOf)
}

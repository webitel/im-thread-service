package mapper

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/webitel/im-thread-service/internal/domain/model"
)

func TestMapHistoryMessage_Failures(t *testing.T) {
	t.Parallel()

	member := uuid.New()

	tests := []struct {
		name string
		msg  *model.Message
		want int
	}{
		{
			name: "live message carries its failures",
			msg: &model.Message{ID: uuid.New(), Failures: []*model.DeliveryFailure{
				{MemberID: member, Code: "403", Message: "Forbidden: bot was blocked by the user"},
			}},
			want: 1,
		},
		{
			name: "delivered message has none",
			msg:  &model.Message{ID: uuid.New()},
		},
		{
			name: "deleted message hides them like the rest of its content",
			msg: &model.Message{ID: uuid.New(), DeletedAt: new(time.Now()), Failures: []*model.DeliveryFailure{
				{MemberID: member, Code: "403"},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := mapHistoryMessage(tt.msg, uuid.Nil).GetFailures()
			require.Len(t, got, tt.want)

			if tt.want > 0 {
				assert.Equal(t, member.String(), got[0].GetMemberId())
				assert.Equal(t, "403", got[0].GetError().GetCode())
				assert.Equal(t, "Forbidden: bot was blocked by the user", got[0].GetError().GetMessage())
			}
		})
	}
}

// last_msg arrives as the jsonb the thread query builds; its failures must survive decoding and mapping.
func TestConvertToThread_LastMessageFailures(t *testing.T) {
	t.Parallel()

	member := uuid.New()
	raw := `{"id":"` + uuid.NewString() + `","body":"hi","failures":[{"member_id":"` + member.String() + `","code":"403","message":"blocked"}]}`

	var last model.Message
	require.NoError(t, json.Unmarshal([]byte(raw), &last))

	got := new(ThreadOutConverter).ConvertToThread(&model.Thread{ID: uuid.New(), LastMessage: &last}).GetLastMsg().GetFailures()
	require.Len(t, got, 1)
	assert.Equal(t, member.String(), got[0].GetMemberId())
	assert.Equal(t, "403", got[0].GetError().GetCode())
	assert.Equal(t, "blocked", got[0].GetError().GetMessage())
}

package guards

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/webitel/im-thread-service/internal/service/dto"
)

// The read point is a message seq; a message id is still accepted for older clients.
func TestValidateReadMessage(t *testing.T) {
	thread, user := uuid.NewString(), uuid.NewString()

	tests := map[string]struct {
		req     *dto.ReadMessageRequest
		wantErr bool
	}{
		"by seq":           {req: &dto.ReadMessageRequest{ThreadID: thread, UserID: user, UpToSeq: 9}},
		"by message id":    {req: &dto.ReadMessageRequest{ThreadID: thread, UserID: user, MessageID: uuid.NewString()}},
		"neither":          {req: &dto.ReadMessageRequest{ThreadID: thread, UserID: user}, wantErr: true},
		"negative seq":     {req: &dto.ReadMessageRequest{ThreadID: thread, UserID: user, UpToSeq: -1}, wantErr: true},
		"bad id, no seq":   {req: &dto.ReadMessageRequest{ThreadID: thread, UserID: user, MessageID: "x"}, wantErr: true},
		"bad id, seq wins": {req: &dto.ReadMessageRequest{ThreadID: thread, UserID: user, MessageID: "x", UpToSeq: 3}},
		"no thread":        {req: &dto.ReadMessageRequest{UserID: user, UpToSeq: 9}, wantErr: true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := ValidateReadMessage(tt.req)
			assert.Equal(t, tt.wantErr, err != nil, "err = %v", err)
		})
	}
}

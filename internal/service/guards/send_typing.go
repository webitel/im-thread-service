package guards

import (
	"errors"

	"github.com/google/uuid"

	"github.com/webitel/im-thread-service/internal/service/dto"
)

// SendTypingGuard validates an ephemeral typing request. Like the other guards
// it only inspects the request; timeout clamping and preview truncation are
// mutations handled by the service.
func SendTypingGuard(req *dto.SendTypingRequest) error {
	if req == nil {
		return errors.New("request is nil")
	}

	if req.From.ID == uuid.Nil {
		return errors.New("typing sender id is required")
	}

	if req.ThreadID == uuid.Nil {
		return errors.New("typing thread id is required")
	}

	return nil
}

// Package guards provides functional validation logic for service-layer DTOs.
// It implements a pipeline-based validation pattern where multiple specialized
// guard functions are executed sequentially to ensure data integrity before
// reaching the business logic.
//
// Design Principles:
//   - Immutability: Guards do not modify the request, only inspect it.
//   - Fail-fast: The validation stops and returns at the first encountered error.
//   - Separation of Concerns: Keeps service methods clean by offloading input
//     sanity checks to dedicated validation functions.
package guards

import (
	"errors"

	"github.com/google/uuid"

	"github.com/webitel/im-thread-service/internal/service/dto"
)

type MessageGuard func(*dto.SendTextRequest) error

func SendTextGuard(request *dto.SendTextRequest) error {
	guards := []MessageGuard{
		checkNilRequest(),
		checkEmptyPeers(),
		checkEmptyBody(),
		checkReplyTarget(),
	}

	for _, guard := range guards {
		if err := guard(request); err != nil {
			return err
		}
	}

	return nil
}

func checkNilRequest() MessageGuard {
	return func(req *dto.SendTextRequest) error {
		if req == nil {
			return errors.New("request is nil")
		}

		return nil
	}
}

func checkEmptyPeers() MessageGuard {
	return func(req *dto.SendTextRequest) error {
		if req.From.ID == uuid.Nil || req.To.ID == uuid.Nil {
			return errors.New("sender and receiver ids are required")
		}

		return nil
	}
}

func checkEmptyBody() MessageGuard {
	return func(req *dto.SendTextRequest) error {
		if req.Body == "" {
			return errors.New("message body is empty")
		}

		return nil
	}
}

func checkReplyTarget() MessageGuard {
	return func(req *dto.SendTextRequest) error {
		if req.ReplyToMessageID != nil && *req.ReplyToMessageID == uuid.Nil {
			return errors.New("reply_to_message_id is not a valid uuid")
		}

		return nil
	}
}

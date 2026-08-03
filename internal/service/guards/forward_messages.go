package guards

import (
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/webitel/im-thread-service/internal/service/dto"
)

type ForwardGuard func(*dto.ForwardMessagesRequest) error

func ForwardMessagesGuard(request *dto.ForwardMessagesRequest) error {
	guards := []ForwardGuard{
		checkNilForwardRequest(),
		checkEmptyForwardPeers(),
		checkForwardTargets(),
	}

	for _, guard := range guards {
		if err := guard(request); err != nil {
			return err
		}
	}

	return nil
}

func checkNilForwardRequest() ForwardGuard {
	return func(req *dto.ForwardMessagesRequest) error {
		if req == nil {
			return errors.New("request is nil")
		}

		return nil
	}
}

func checkEmptyForwardPeers() ForwardGuard {
	return func(req *dto.ForwardMessagesRequest) error {
		if req.From.ID == uuid.Nil || req.To.ID == uuid.Nil {
			return errors.New("sender and receiver ids are required")
		}

		return nil
	}
}

func checkForwardTargets() ForwardGuard {
	return func(req *dto.ForwardMessagesRequest) error {
		if len(req.MessageIDs) == 0 {
			return errors.New("message_ids is empty")
		}

		if len(req.MessageIDs) > dto.MaxForwardBatch {
			return fmt.Errorf("message_ids holds %d ids, at most %d may be forwarded at once",
				len(req.MessageIDs), dto.MaxForwardBatch)
		}

		seen := make(map[uuid.UUID]struct{}, len(req.MessageIDs))

		for _, id := range req.MessageIDs {
			if id == uuid.Nil {
				return errors.New("message_ids holds an invalid uuid")
			}

			if _, dup := seen[id]; dup {
				return fmt.Errorf("message_ids holds %s more than once", id)
			}

			seen[id] = struct{}{}
		}

		return nil
	}
}

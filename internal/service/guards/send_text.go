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

package guards

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

// ReadMessageGuard defines the functional guard type for read requests
type ReadMessageGuard func(*dto.ReadMessageRequest) error

// ValidateReadMessage coordinates the execution of all validation rules
func ValidateReadMessage(request *dto.ReadMessageRequest) error {
	guards := []ReadMessageGuard{
		checkNilReadRequest(),
		checkReadIdentifiers(),
	}

	for _, guard := range guards {
		if err := guard(request); err != nil {
			return err
		}
	}
	return nil
}

// checkNilReadRequest ensures the request pointer is not nil
func checkNilReadRequest() ReadMessageGuard {
	return func(req *dto.ReadMessageRequest) error {
		if req == nil {
			return errors.New("request is nil")
		}
		return nil
	}
}

// checkReadIdentifiers validates that IDs are provided and are valid UUIDs
func checkReadIdentifiers() ReadMessageGuard {
	return func(req *dto.ReadMessageRequest) error {
		// Validate MessageID
		if req.MessageID == "" {
			return errors.New("message id is required")
		}
		if _, err := uuid.Parse(req.MessageID); err != nil {
			return fmt.Errorf("invalid message id format: %w", err)
		}

		// Validate ThreadID
		if req.ThreadID == "" {
			return errors.New("thread id is required")
		}
		if _, err := uuid.Parse(req.ThreadID); err != nil {
			return fmt.Errorf("invalid thread id format: %w", err)
		}

		// Validate UserID (Peer ID)
		if req.UserID == "" {
			return errors.New("user id is required")
		}

		return nil
	}
}

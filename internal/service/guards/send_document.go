package guards

import (
	"errors"

	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

type DocumentMessageGuard func(*dto.SendDocumentRequest) error

// SendDocumentGuard executes a chain of validation rules for document sending requests.
func SendDocumentGuard(request *dto.SendDocumentRequest) error {
	guards := []DocumentMessageGuard{
		checkNilDocumentRequest(),
		checkEmptyDocumentPeers(),
		checkDocumentsPresence(),
	}

	for _, guard := range guards {
		if err := guard(request); err != nil {
			return err
		}
	}
	return nil
}

func checkNilDocumentRequest() DocumentMessageGuard {
	return func(req *dto.SendDocumentRequest) error {
		if req == nil {
			return errors.New("request is nil")
		}
		return nil
	}
}

func checkEmptyDocumentPeers() DocumentMessageGuard {
	return func(req *dto.SendDocumentRequest) error {
		if req.From.ID == uuid.Nil || req.To.ID == uuid.Nil {
			return errors.New("sender and receiver ids are required")
		}
		return nil
	}
}

func checkDocumentsPresence() DocumentMessageGuard {
	return func(req *dto.SendDocumentRequest) error {
		// Ensure the Document container has at least one document entry
		if len(req.Document.Documents) == 0 {
			return errors.New("at least one document is required")
		}

		// Validate mandatory fields for each document attachment
		for _, doc := range req.Document.Documents {
			if doc.ID == 0 {
				return errors.New("document file id is required")
			}
			if doc.Name == "" {
				return errors.New("document name is required")
			}
		}
		return nil
	}
}

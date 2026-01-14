package guards

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

type DocumentMessageGuard func(*dto.SendDocumentRequest) error

func SendDocumentGuard(req *dto.SendDocumentRequest) error {
	guards := []DocumentMessageGuard{
		checkDocumentBase(),
		checkDocumentsIntegrity(),
	}

	for _, guard := range guards {
		if err := guard(req); err != nil {
			return err
		}
	}
	return nil
}

func checkDocumentBase() DocumentMessageGuard {
	return func(req *dto.SendDocumentRequest) error {
		if req == nil {
			return errors.New("request is nil")
		}
		if req.From.ID == uuid.Nil || req.To.ID == uuid.Nil {
			return errors.New("sender and receiver ids are required")
		}
		return nil
	}
}

func checkDocumentsIntegrity() DocumentMessageGuard {
	return func(req *dto.SendDocumentRequest) error {
		if len(req.Document.Documents) == 0 {
			return errors.New("at least one document is required")
		}

		for i, doc := range req.Document.Documents {
			prefix := fmt.Sprintf("document[%d]", i)

			// [ID] Basic integrity check
			if doc.ID == 0 {
				return fmt.Errorf("%s: file id is required", prefix)
			}

			// [URL] VALIDATE FORMAT IF LINK PROVIDED
			if doc.URL != "" {
				if err := validateURL(doc.URL); err != nil {
					// [ERROR] LOG AND RETURN SPECIFIC ERROR
					return fmt.Errorf("%s: %w", prefix, err)
				}
			}

			// [NAME] Security & Length validation
			if err := validateFilename(doc.Name); err != nil {
				return fmt.Errorf("%s: %w", prefix, err)
			}

			// [MIME] Format validation
			if err := validateMime(doc.MimeType, false); err != nil {
				return fmt.Errorf("%s: %w", prefix, err)
			}

			// [SIZE] Reasonable limits (e.g., max 100MB)
			if doc.Size <= 0 || doc.Size > 100*1024*1024 {
				return fmt.Errorf("%s: size must be between 1 byte and 100MB", prefix)
			}
		}
		return nil
	}
}

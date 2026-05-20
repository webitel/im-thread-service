package guards

import (
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/webitel/im-thread-service/internal/service/dto"
)

type ImageMessageGuard func(*dto.SendImageRequest) error

func SendImageGuard(req *dto.SendImageRequest) error {
	guards := []ImageMessageGuard{
		checkImageBase(),
		checkImagesIntegrity(),
	}

	for _, guard := range guards {
		if err := guard(req); err != nil {
			return err
		}
	}

	return nil
}

func checkImageBase() ImageMessageGuard {
	return func(req *dto.SendImageRequest) error {
		if req == nil {
			return errors.New("request is nil")
		}

		if req.From.ID == uuid.Nil || req.To.ID == uuid.Nil {
			return errors.New("sender and receiver ids are required")
		}

		return nil
	}
}

func checkImagesIntegrity() ImageMessageGuard {
	return func(req *dto.SendImageRequest) error {
		if len(req.Image.Images) == 0 {
			return errors.New("at least one image is required")
		}

		for i, img := range req.Image.Images {
			prefix := fmt.Sprintf("image[%d]", i)

			// [SOURCE] Must have ID (internal) or Link (external)
			if img.ID == 0 && img.URL == "" {
				return fmt.Errorf("%s: file id or link is required", prefix)
			}

			// [URL] VALIDATE FORMAT IF LINK PROVIDED
			if img.URL != "" {
				if err := validateURL(img.URL); err != nil {
					// [ERROR] LOG AND RETURN SPECIFIC ERROR
					return fmt.Errorf("%s: %w", prefix, err)
				}
			}

			// [URL] Validate format if link provided
			if img.URL != "" {
				if err := validateURL(img.URL); err != nil {
					return fmt.Errorf("%s: %w", prefix, err)
				}
			}

			// [MIME] Strict image type validation
			if err := validateMime(img.MimeType, true); err != nil {
				return fmt.Errorf("%s: %w", prefix, err)
			}
		}

		return nil
	}
}

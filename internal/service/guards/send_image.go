package guards

import (
	"errors"

	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

type ImageMessageGuard func(*dto.SendImageRequest) error

func SendImageGuard(request *dto.SendImageRequest) error {
	guards := []ImageMessageGuard{
		checkNilImageRequest(),
		checkEmptyImagePeers(),
		checkImagesPresence(),
	}

	for _, guard := range guards {
		if err := guard(request); err != nil {
			return err
		}
	}
	return nil
}

func checkNilImageRequest() ImageMessageGuard {
	return func(req *dto.SendImageRequest) error {
		if req == nil {
			return errors.New("request is nil")
		}
		return nil
	}
}

func checkEmptyImagePeers() ImageMessageGuard {
	return func(req *dto.SendImageRequest) error {
		if req.From.ID == uuid.Nil || req.To.ID == uuid.Nil {
			return errors.New("sender and receiver ids are required")
		}
		return nil
	}
}

func checkImagesPresence() ImageMessageGuard {
	return func(req *dto.SendImageRequest) error {
		// Check if the Image object exists and contains at least one image
		if len(req.Image.Images) == 0 {
			return errors.New("at least one image is required")
		}

		// Optional: Validate that each image has a valid link or ID
		for _, img := range req.Image.Images {
			if img.Link == "" && img.ID == "" {
				return errors.New("image link or id is required for all items")
			}
		}
		return nil
	}
}

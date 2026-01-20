package service

import (
	"context"
	"fmt"

	"github.com/webitel/im-thread-service/gen/go/client/storage"
	storageclient "github.com/webitel/im-thread-service/infra/webitel/storage"
)

// [ATTACHMENT] CONTRACT FOR ANY MEDIA DATA
type AttachmentProcessor interface {
	GetID() int64
	GetURL() string
	GetMimeType() string
	GetName() string
	SetID(int64)
	SetMime(string)
	SetName(string)
}

// [MEDIA_PROCESSOR] DEFINES THE LOGIC FOR EXTERNAL STORAGE OPERATIONS
type MediaProcessor interface {
	Process(ctx context.Context, domainID int64, items []AttachmentProcessor) error
}

type storageProcessor struct {
	client storageclient.FileService
}

func NewMediaProcessor(client storageclient.FileService) MediaProcessor {
	return &storageProcessor{client: client}
}

// [PROCESS] HANDLES VALIDATION AND UPLOADS FOR ALL ATTACHMENTS
func (p *storageProcessor) Process(ctx context.Context, domainID int64, items []AttachmentProcessor) error {
	if len(items) == 0 {
		return nil
	}

	svc := p.client
	for _, item := range items {
		// [CASE_1] VERIFY IF ID EXISTS
		if id := item.GetID(); id != 0 {
			res, err := svc.SearchFiles(ctx, &storage.SearchFilesRequest{Id: []int64{id}, Size: 1})
			if err != nil || len(res.GetItems()) == 0 {
				return fmt.Errorf("storage: file %d check failed: %w", id, err)
			}
			// [UPDATE_POINTERS] set name and mime in case they were changed
			if len(res.GetItems()) > 0 {
				f := res.GetItems()[0]
				item.SetName(f.GetName())
				item.SetMime(f.GetMimeType())
			}
			continue
		}

		// [CASE_2] UPLOAD IF URL IS PROVIDED
		if url := item.GetURL(); url != "" {
			res, err := svc.UploadFileUrl(ctx, &storage.UploadFileUrlRequest{
				DomainId: domainID,
				Url:      url,
				Mime:     item.GetMimeType(),
				Channel:  storage.UploadFileChannel_ChatChannel,
			})
			if err != nil {
				return fmt.Errorf("storage: upload failed for %s: %w", url, err)
			}

			// [UPDATE_POINTERS] set new ID and update mime if changed
			item.SetID(res.GetId())
			if res.GetMime() != "" {
				item.SetMime(res.GetMime())
			}
		}
	}
	return nil
}

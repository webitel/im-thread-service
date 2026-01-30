package service

import (
	"context"
	"fmt"

	"github.com/webitel/im-thread-service/gen/go/storage"
	storageclient "github.com/webitel/im-thread-service/infra/webitel/storage"
	"github.com/webitel/im-thread-service/internal/utils"
)

// [ATTACHMENT] CONTRACT FOR ANY MEDIA DATA
type AttachmentProcessor interface {
	GetID() int64
	GetURL() string
	GetMimeType() string
	GetName() string
	SetID(int64)
	SetMime(string)
	SetURL(string)
	SetName(string)
}

// [MEDIA_PROCESSOR] DEFINES THE LOGIC FOR EXTERNAL STORAGE OPERATIONS
type MediaProcessor interface {
	Process(ctx context.Context, domainID int64, items []AttachmentProcessor) error
}

type storageProcessor struct {
	client *storageclient.Client
}

// NewMediaProcessor creates a new MediaProcessor instance from the given storage client.
//
//	Args:
//	client: The storage client to use for external storage operations.
//
//	Returns:
//	A new MediaProcessor instance.
func NewMediaProcessor(client *storageclient.Client) MediaProcessor {
	return &storageProcessor{client: client}
}

// Process processes media files in parallel by splitting them into two groups:
// files with IDs and files with URLs. It then calls statFile and uploadFile
// respectively for each group. The function waits for both channels to
// finish before returning the result. If any error occurs during the
// processing, it returns the error immediately.
//
//	Args:
//	ctx: The context of the request.
//	domainID: The ID of the domain to which the files belong.
//	items: The media files to process.
//
//	Returns:
//	An error if any error occurs during the processing.
func (p *storageProcessor) Process(ctx context.Context, domainID int64, items []AttachmentProcessor) error {
	if len(items) == 0 {
		return nil
	}

	var (
		itemsWithID  = make([]AttachmentProcessor, 0)
		itemsWithURL = make([]AttachmentProcessor, 0)
	)

	for _, item := range items {
		if id := item.GetID(); id != 0 {
			itemsWithID = append(itemsWithID, item)
		} else if url := item.GetURL(); url != "" {
			itemsWithURL = append(itemsWithURL, item)
		}
	}

	statChan := p.statFile(ctx, itemsWithID)
	uploadChan := p.uploadFile(ctx, domainID, itemsWithURL)

	var isStatFileFinished, isUploadFinished bool
	for !isStatFileFinished || !isUploadFinished {
		select {
		case err, ok := <-statChan:
			if ok && err != nil {
				return err
			}
			isStatFileFinished = true
		case err, ok := <-uploadChan:
			if ok && err != nil {
				return err
			}
			isUploadFinished = true
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

// UploadFile uploads media files to the storage service.
//
//	Args:
//	ctx: The context of the request.
//	domainID: The ID of the domain to which the files belong.
//	itemsWithURL: The media files to be uploaded.
//
//	Returns:
//	A channel of errors for each uploaded file. If an error occurs, the error is sent on the channel.
//	The channel is closed once all files have been uploaded.
func (p *storageProcessor) uploadFile(ctx context.Context, domainID int64, itemsWithURL []AttachmentProcessor) <-chan error {
	resultChan := make(chan error, 1)

	go func() {
		defer close(resultChan)

		if len(itemsWithURL) == 0 {
			resultChan <- nil
			return
		}

		for _, item := range itemsWithURL {
			res, err := p.client.UploadFileUrl(ctx, &storage.UploadFileUrlRequest{
				Url:      item.GetURL(),
				DomainId: domainID,
				Name:     item.GetName(),
				Mime:     item.GetMimeType(),
				Channel:  storage.UploadFileChannel_ChatChannel,
			})
			if err != nil {
				resultChan <- fmt.Errorf("[MEDIA_PROCESSOR] storage: upload failed for %s: %w", item.GetURL(), err)
				return
			}

			item.SetID(res.GetId())
			if res.GetMime() != "" {
				item.SetMime(res.GetMime())
			}
			item.SetURL(res.Url)
		}

		resultChan <- nil
	}()

	return resultChan
}

// StatFile checks if the given attachments exist in the storage service.
//
//	Args:
//	ctx: The context of the request.
//	itemsWithID: The attachments to check for existence.
//
//	Returns:
//	A channel of errors for each attachment. If an attachment does not exist, the error is sent on the channel.
//	The channel is closed once all attachments have been checked.
func (p *storageProcessor) statFile(ctx context.Context, itemsWithID []AttachmentProcessor) <-chan error {
	resultChan := make(chan error, 1)

	go func() {
		defer close(resultChan)

		ids := utils.Map(itemsWithID, func(ap AttachmentProcessor) int64 { return ap.GetID() })
		expectedSize := len(ids)
		if expectedSize == 0 {
			resultChan <- nil
			return
		}

		res, err := p.client.SearchFiles(ctx, &storage.SearchFilesRequest{Id: ids, Size: int32(expectedSize)})
		if err != nil {
			resultChan <- fmt.Errorf("[MEDIA_PROCESSOR] storage ids check failed: %w", err)
			return
		}

		fileMap := make(map[int64]*storage.File, len(res.GetItems()))
		for _, f := range res.GetItems() {
			fileMap[f.GetId()] = f
		}

		for _, item := range itemsWithID {
			f, ok := fileMap[item.GetID()]
			if !ok {
				resultChan <- fmt.Errorf("[MEDIA_PROCESSOR] file %d not found", item.GetID())
				return
			}

			item.SetName(f.GetName())
			item.SetMime(f.GetMimeType())
		}

		resultChan <- nil
	}()

	return resultChan
}

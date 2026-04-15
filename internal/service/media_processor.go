package service

import (
	"context"
	"fmt"
	"time"

	"github.com/webitel/im-thread-service/gen/go/storage"
	storageclient "github.com/webitel/im-thread-service/infra/webitel/storage"
	"github.com/webitel/im-thread-service/internal/utils"
	"github.com/webitel/webitel-go-kit/pkg/errors"
)

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

type MediaProcessor interface {
	Process(ctx context.Context, domainID int64, items []AttachmentProcessor) error
	FetchFileLinks(ctx context.Context, domainID int64, itemsWithID []AttachmentProcessor) <-chan fetchLinksResult
}

type storageProcessor struct {
	client *storageclient.Client
}

func NewMediaProcessor(client *storageclient.Client) MediaProcessor {
	return &storageProcessor{client: client}
}

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

	// statChan := p.statFile(ctx, itemsWithID)
	uploadChan := p.uploadFile(ctx, domainID, itemsWithURL)

	var isUploadFinished bool
	for !isUploadFinished {
		select {
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

type fetchLinksResult struct {
	Err       error
	FileLinks map[int64]string
}

func (p *storageProcessor) FetchFileLinks(ctx context.Context, domainID int64, itemsWithID []AttachmentProcessor) <-chan fetchLinksResult {
	resultChan := make(chan fetchLinksResult, 1)

	go func() {
		defer close(resultChan)
		if len(itemsWithID) == 0 {
			return
		}

		requests := make([]*storage.GenerateFileLinkRequest, 0, len(itemsWithID))
		for _, item := range itemsWithID {
			requests = append(requests, &storage.GenerateFileLinkRequest{
				DomainId: domainID,
				FileId:   item.GetID(),
				Action:   "download",
			})
		}

		ctx, cancel := context.WithTimeout(ctx, time.Second*3)
		defer cancel()

		response, err := p.client.BulkGenerateFileLink(ctx, &storage.BulkGenerateFileLinkRequest{
			Files: requests,
		})

		if err != nil {
			resultChan <- fetchLinksResult{
				Err: errors.Internal("fetch storage file links", errors.WithCause(err), errors.WithID("service.media_processor.fetch_file_links")),
			}

			return
		}

		linkedItems := make(map[int64]string, len(response.GetLinks()))
		for i, file := range response.GetLinks() {
			url, err := utils.ResolveFullURL(file.BaseUrl, file.Url)
			if err != nil {
				resultChan <- fetchLinksResult{Err: err}
				return
			}

			linkedItems[itemsWithID[i].GetID()] = url
		}
		resultChan <- fetchLinksResult{
			FileLinks: linkedItems,
		}
	}()

	return resultChan
}

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

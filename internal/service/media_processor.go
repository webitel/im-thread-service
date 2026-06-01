package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/status"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/gen/go/storage"
	storageclient "github.com/webitel/im-thread-service/infra/webitel/storage"
	"github.com/webitel/im-thread-service/internal/utils"
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
	SetSize(int64)
}

type MediaProcessor interface {
	Process(ctx context.Context, domainID int64, items []AttachmentProcessor) error
	FetchFileLinks(ctx context.Context, domainID int64, itemsWithID []AttachmentProcessor) <-chan fetchLinksResult
	FetchFileLinksWithMetadata(ctx context.Context, domainID int64, filesWithID []AttachmentProcessor) (storageFilesResponse, error)
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

	itemsWithURL := make([]AttachmentProcessor, 0)

	for _, item := range items {
		if url := item.GetURL(); url != "" {
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

type fileMetadata struct {
	Size int64
	Mime string
	Name string
	URL  string
}

type storageFilesResponse struct {
	FilesMetadata map[int64]*fileMetadata
}

func (p *storageProcessor) FetchFileLinksWithMetadata(ctx context.Context, domainID int64, filesWithID []AttachmentProcessor) (storageFilesResponse, error) {
	if len(filesWithID) == 0 {
		return storageFilesResponse{FilesMetadata: make(map[int64]*fileMetadata)}, nil
	}

	g, ctx := errgroup.WithContext(ctx)

	g.SetLimit(10)

	var mu sync.Mutex

	resultFilesMetadata := make(map[int64]*fileMetadata, len(filesWithID))

	for _, file := range filesWithID {
		f := file
		if f.GetID() <= 0 {
			continue
		}

		g.Go(func() error {
			fileID := f.GetID()

			result, err := p.client.GenerateFileLink(ctx, &storage.GenerateFileLinkRequest{
				DomainId: domainID,
				FileId:   fileID,
				Action:   "download",
				Source:   "file",
				Metadata: true,
			})
			if err != nil {
				if st, ok := status.FromError(err); ok {
					return errors.New(fmt.Sprintf("failed to generate file link for id %d", fileID), errors.WithCause(err), errors.WithID("service.media_processor"), errors.WithCode(st.Code()))
				}

				return errors.Internal(fmt.Sprintf("failed to generate file link for id %d", fileID), errors.WithCause(err), errors.WithID("service.media_processor"))
			}

			url, err := utils.ResolveFullURL(result.GetBaseUrl(), result.GetUrl())
			if err != nil {
				return errors.Internal(fmt.Sprintf("failed to resolve url for id: %d", fileID), errors.WithCause(err), errors.WithID("service.media_processor"))
			}

			meta := &fileMetadata{
				URL: url,
			}

			if metadata := result.GetMetadata(); metadata != nil {
				meta.Mime = metadata.GetMimeType()
				meta.Name = metadata.GetName()
				meta.Size = metadata.GetSize()
			}

			mu.Lock()
			resultFilesMetadata[fileID] = meta
			mu.Unlock()

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return storageFilesResponse{}, err
	}

	return storageFilesResponse{FilesMetadata: resultFilesMetadata}, nil
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
				Metadata: true,
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
			url, err := utils.ResolveFullURL(file.GetBaseUrl(), file.GetUrl())
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
			res, err := p.client.UploadFileURL(ctx, &storage.UploadFileUrlRequest{
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

			item.SetURL(res.GetUrl())
		}

		resultChan <- nil
	}()

	return resultChan
}

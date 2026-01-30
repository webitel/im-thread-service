package decorators

import (
	"context"
	"fmt"
	"net/url"
	"slices"

	"github.com/webitel/im-thread-service/gen/go/storage"
	storageclient "github.com/webitel/im-thread-service/infra/webitel/storage"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

const (
	StorageDownloadAction string = "download"
)

const (
	ImagesRequested    string = "images"
	DocumentsRequested string = "documents"
)

// Linked attachments [M]eta[D]ata
const (
	UseDocumentStorageMD int = 1 << iota
	UseImageStorageMD
)

type (
	messageHistoryEnricher struct {
		service.MessageHistorySearcher

		storage *storageclient.Client
	}
)

// NewMessageHistoryEnricher creates a new messageHistoryEnricher instance.
//
// Args:
// base: The base message history searcher.
// storage: The storage client.
//
// Returns:
// An instance of messageHistoryEnricher.
func NewMessageHistoryEnricher(base service.MessageHistorySearcher, storage *storageclient.Client) *messageHistoryEnricher {
	return &messageHistoryEnricher{
		MessageHistorySearcher: base,
		storage:                storage,
	}
}

// Search enriches the given messages with file links based on the given request.
//
// Args:
// ctx: The context of the request.
// hmiDTO: The history message input DTO containing the request.
//
// Returns:
// The enriched messages.
// An error if occurred during the enrichment process.
func (m *messageHistoryEnricher) Search(ctx context.Context, hmiDTO *dto.HistoryMessageInputDTO) (model.MessageSlice, error) {
	messages, err := m.MessageHistorySearcher.Search(ctx, hmiDTO)
	if err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return messages, nil
	}

	fileIDs := collectUniqueFileIDs(messages)
	if len(fileIDs) == 0 {
		return messages, nil
	}

	var (
		requestedMetadata int
	)

	requestedMetadata = shouldLoadMetadata(hmiDTO.Fields, requestedMetadata)

	loadMetadata := (requestedMetadata & (UseDocumentStorageMD | UseImageStorageMD)) != 0

	linkMap, err := m.fetchFileLinks(ctx, fileIDs, hmiDTO.DomainID, loadMetadata)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch file links: %w", err)
	}

	if err := m.enrichMessages(messages, linkMap, requestedMetadata); err != nil {
		return nil, fmt.Errorf("failed to enrich messages: %w", err)
	}

	return messages, nil
}

// fetchFileLinks fetches file links from the storage service for the given file IDs.
//
// Args:
//
//	ctx: The context of the request.
//	fileIDs: The file IDs to fetch links for.
//	domainID: The domain ID of the files.
//	loadMetadata: Whether to load metadata for the files.
//
// Returns:
//
//	A map of file IDs to their corresponding file links.
//	An error if occurred during the fetch process.
func (m *messageHistoryEnricher) fetchFileLinks(ctx context.Context, fileIDs []int64, domainID int, loadMetadata bool) (map[int64]*storage.GenerateFileLinkResponse, error) {
	requests := make([]*storage.GenerateFileLinkRequest, len(fileIDs))
	for i, fileID := range fileIDs {
		requests[i] = &storage.GenerateFileLinkRequest{
			DomainId: int64(domainID),
			FileId:   fileID,
			Metadata: loadMetadata,
			Action:   StorageDownloadAction,
		}
	}

	response, err := m.storage.BulkGenerateFileLink(ctx, &storage.BulkGenerateFileLinkRequest{
		Files: requests,
	})
	if err != nil {
		return nil, err
	}

	links := response.GetLinks()

	if len(links) != len(fileIDs) {
		return nil, fmt.Errorf(
			"storage returned %d links but expected %d",
			len(links),
			len(fileIDs),
		)
	}

	linkMap := make(map[int64]*storage.GenerateFileLinkResponse, len(fileIDs))
	for i, fileID := range fileIDs {
		linkMap[fileID] = links[i]
	}

	return linkMap, nil
}

// collectUniqueFileIDs collects unique file IDs from a slice of messages.
//
// Args:
//
// messages: The slice of messages to collect file IDs from.
//
// Returns:
//
//	A slice of unique file IDs.
func collectUniqueFileIDs(messages model.MessageSlice) []int64 {
	seen := make(map[int64]struct{})

	for _, message := range messages {
		for _, doc := range message.Documents {
			if doc.FileID != 0 {
				seen[doc.FileID] = struct{}{}
			}
		}

		for _, img := range message.Images {
			if img.FileID != 0 {
				seen[img.FileID] = struct{}{}
			}
		}
	}

	fileIDs := make([]int64, 0, len(seen))
	for id := range seen {
		fileIDs = append(fileIDs, id)
	}

	return fileIDs
}

// shouldLoadMetadata checks if the given fields require loading metadata from the storage.
//
// Args:
//
// fields: The slice of DB fields to check.
// requestedMetadata: The requested metadata to check against.
//
// Returns:
//
//	The requested metadata OR'd with the required metadata based on the fields.
func shouldLoadMetadata(fields []string, requestedMetadata int) int {
	if len(fields) == 0 {
		return 0
	}

	// in this case 'fields' means DB fields,
	// if field not 'lazy loaded' we don`t need to use 'storage' metadata
	if !slices.Contains(fields, DocumentsRequested) {
		requestedMetadata |= UseDocumentStorageMD
	}

	if !slices.Contains(fields, ImagesRequested) {
		requestedMetadata |= UseImageStorageMD
	}

	return requestedMetadata
}

// enrichMessages enriches the given messages with file links based on the given request.
//
// Args:
//
//	messages: The slice of messages to enrich.
//	linkMap: A map of file IDs to their corresponding file links.
//	requestedMetadata: The requested metadata to check against.
//
// Returns:
//
//	An error if occurred during the enrichment process.
func (m *messageHistoryEnricher) enrichMessages(messages model.MessageSlice, linkMap map[int64]*storage.GenerateFileLinkResponse, requestedMetadata int) error {
	useDocMD := (UseDocumentStorageMD&requestedMetadata != 0)
	useImgMD := (UseImageStorageMD&requestedMetadata != 0)

	for _, msg := range messages {
		if err := processAttachments(msg.Documents, linkMap, useDocMD, enrichDocument); err != nil {
			return err
		}

		if err := processAttachments(msg.Images, linkMap, useImgMD, enrichImage); err != nil {
			return err
		}
	}

	return nil
}

// processAttachments processes the given attachments and enriches them with file links
// based on the given request.
//
// Args:
//
//	attachments: The slice of attachments to process.
//	linkMap: A map of file IDs to their respective file links.
//	useMD: Whether to load metadata for the attachments.
//	enrichFunc: A function to enrich an attachment with a file link.
//
// Returns:
//
//	An error if occurred during the processing.
func processAttachments[T model.MessageAttachment](
	attachments []T,
	linkMap map[int64]*storage.GenerateFileLinkResponse,
	useMD bool,
	enrichFunc func(T, *storage.GenerateFileLinkResponse, bool) error,
) error {
	for _, item := range attachments {
		fileID := item.GetFileID()
		if link, ok := linkMap[fileID]; ok {
			if err := enrichFunc(item, link, useMD); err != nil {
				return fmt.Errorf("failed to enrich file %d: %w", fileID, err)
			}
		}
	}

	return nil
}

// enrichDocument enriches the given document with file links based on the given request.
//
// Args:
//
//	doc: The document to enrich.
//	link: The file link response containing the file link and metadata.
//	useMetadata: Whether to load metadata for the document.
//
// Returns:
//
//	An error if occurred during the enrichment process.
func enrichDocument(doc *model.MessageDocument, link *storage.GenerateFileLinkResponse, useMetadata bool) error {
	fullURL, err := resolveFullURL(link.GetBaseUrl(), link.GetUrl())
	if err != nil {
		return fmt.Errorf("failed to resolve URL: %w", err)
	}

	doc.URL = fullURL

	if useMetadata && link.Metadata != nil {
		doc.FileID = link.Metadata.GetId()
		doc.Size = link.Metadata.GetSize()
		doc.Mime = link.Metadata.GetMimeType()
		doc.Name = link.Metadata.GetName()
	}

	return nil
}

// enrichImage enriches the given image with file links based on the given request.
//
// Args:
//
//	img: The image to enrich.
//	link: The file link response containing the file link and metadata.
//	useMetadata: Whether to load metadata for the images.
//
// Returns:
//
//	An error if occurred during the enrichment process.
func enrichImage(img *model.MessageImage, link *storage.GenerateFileLinkResponse, useMetadata bool) error {
	fullURL, err := resolveFullURL(link.GetBaseUrl(), link.GetUrl())
	if err != nil {
		return fmt.Errorf("failed to resolve URL: %w", err)
	}

	img.URL = fullURL

	if useMetadata && link.Metadata != nil {
		img.FileID = link.Metadata.GetId()
		img.Mime = link.Metadata.GetMimeType()
		img.Name = link.Metadata.GetName()
	}

	return nil
}

// resolveFullURL resolves a full URL given a base URL and a relative URL.
//
// Args:
//
//	baseStr: The base URL to resolve the relative URL against.
//	relativeStr: The relative URL to resolve.
//
// Returns:
//
//	A fully resolved URL string, and an error if the base or relative URLs are invalid.
func resolveFullURL(baseStr, relativeStr string) (string, error) {
	if baseStr == "" || relativeStr == "" {
		return "", fmt.Errorf("base or relative URL is empty")
	}

	base, err := url.Parse(baseStr)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}

	rel, err := url.Parse(relativeStr)
	if err != nil {
		return "", fmt.Errorf("invalid relative URL: %w", err)
	}

	return base.ResolveReference(rel).String(), nil
}

package service

import (
	"context"
	"log/slog"
	"strconv"
	"sync"

	stderrs "errors"

	"github.com/webitel/im-thread-service/gen/go/provider/v1"
	improviders "github.com/webitel/im-thread-service/infra/webitel/im-providers"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/webitel-go-kit/pkg/errors"
)

type ProvidersAdapter interface {
	SendMessage(ctx context.Context, message *model.Message) error
}

type baseRpcProvidersAdapter struct {
	logger          *slog.Logger
	providersClient *improviders.Client
}

func newBaseRpcProvidersAdapter(logger *slog.Logger, providersClient *improviders.Client) *baseRpcProvidersAdapter {
	log := logger.With("component", "base_rpc_providers_adapter")

	return &baseRpcProvidersAdapter{
		logger:          log,
		providersClient: providersClient,
	}
}

func (a *baseRpcProvidersAdapter) SendMessage(ctx context.Context, message *model.Message) error {
	log := a.logger.With("operation", "send_message")

	if message == nil {
		log.Warn("received nil pointer message")
		return errors.InvalidArgument("received nil pointer message", errors.WithID("service.providers_adapter.send_message"))
	}

	externalPairs := model.ThreadDialogs.ExtractExternalPeers(message.To)
	if len(externalPairs) == 0 {
		return nil
	}

	var (
		wg          sync.WaitGroup
		errorsMu    sync.Mutex
		errorsArray []error
	)

	for _, externalPeer := range externalPairs {
		wg.Go(func() {
			var err error
			userID := externalPeer.ContactID.String()

			switch message.Type {
			case model.MessageTypeFile:
				_, err = a.providersClient.SendDocument(ctx, &provider.ProviderSendDocumentRequest{
					GateId:         externalPeer.Via,
					ExternalUserId: userID,
					Document:       extractFirstFile(message.Documents),
					Caption:        message.Body,
				})

			case model.MessageTypeImage:
				_, err = a.providersClient.SendImage(ctx, &provider.ProviderSendImageRequest{
					GateId:         externalPeer.Via,
					ExternalUserId: userID,
					Image:          extractFirstFile(message.Images),
					Caption:        message.Body,
				})

			case model.MessageTypeText:
				_, err = a.providersClient.SendText(ctx, &provider.ProviderSendTextRequest{
					GateId:         externalPeer.Via,
					ExternalUserId: userID,
					Text:           message.Body,
				})
			}

			if err != nil {
				errorsMu.Lock()
				errorsArray = append(errorsArray, err)
				errorsMu.Unlock()
			}
		})
	}

	wg.Wait()

	if len(errorsArray) > 0 {
		log.Error("sending message to external providers", "error", errorsArray)

		return stderrs.Join(errorsArray...)
	}

	return nil
}

func extractFirstFile[T AttachmentProcessor](media []T) *provider.ProviderFile {
	if len(media) == 0 {
		return nil
	}

	first := media[0]
	return &provider.ProviderFile{
		Id:       strconv.Itoa(int(first.GetID())),
		Url:      first.GetURL(),
		Name:     first.GetName(),
		MimeType: first.GetMimeType(),
	}
}

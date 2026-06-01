package service

import (
	"context"
	stderrs "errors"
	"log/slog"
	"strconv"
	"sync"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/gen/go/provider/v1"
	improviders "github.com/webitel/im-thread-service/infra/webitel/im-providers"
	"github.com/webitel/im-thread-service/internal/domain/model"
)

type ProvidersAdapter interface {
	SendMessage(ctx context.Context, message *model.Message) error
}

type baseRPCProvidersAdapter struct {
	logger          *slog.Logger
	providersClient *improviders.Client
}

func newBaseRPCProvidersAdapter(logger *slog.Logger, providersClient *improviders.Client) *baseRPCProvidersAdapter {
	log := logger.With("component", "base_rpc_providers_adapter")

	return &baseRPCProvidersAdapter{
		logger:          log,
		providersClient: providersClient,
	}
}

func (a *baseRPCProvidersAdapter) SendMessage(ctx context.Context, message *model.Message) error {
	log := a.logger.With("operation", "send_message")

	if message == nil {
		log.Warn("received nil pointer message")

		return errors.InvalidArgument("received nil pointer message", errors.WithID("service.providers_adapter.send_message"))
	}

	for i, member := range message.To {
		via := "<nil>"
		if member.Via != nil {
			via = *member.Via
		}
		log.Debug("recipient",
			slog.Int("index", i),
			slog.String("contact_id", member.ContactID.String()),
			slog.String("via", via),
		)
	}

	log.Debug("evaluating external peers",
		slog.String("message_id", message.ID.String()),
		slog.String("thread_id", message.ThreadID.String()),
		slog.String("message_type", message.Type.String()),
		slog.Int("total_recipients", len(message.To)),
	)

	externalPairs := model.ThreadDialogs.ExtractExternalPeers(message.To)
	if len(externalPairs) == 0 {
		log.Debug("no external peers found, skipping provider send",
			slog.String("message_id", message.ID.String()),
			slog.Int("total_recipients", len(message.To)),
		)

		return nil
	}

	log.Info("sending message to external providers",
		slog.String("message_id", message.ID.String()),
		slog.String("thread_id", message.ThreadID.String()),
		slog.String("message_type", message.Type.String()),
		slog.Int("external_peers_count", len(externalPairs)),
	)

	var (
		wg          sync.WaitGroup
		errorsMu    sync.Mutex
		errorsArray []error
	)

	for _, externalPeer := range externalPairs {
		wg.Go(func() {
			var err error

			userID := externalPeer.ContactID.String()
			peerLog := log.With(
				slog.String("external_user_id", userID),
				slog.String("gate_id", externalPeer.Via),
				slog.String("message_type", message.Type.String()),
			)

			peerLog.Debug("dispatching to external provider")

			switch message.Type {
			case model.MessageTypeFile:
				peerLog.Debug("sending document", slog.Int("documents_count", len(message.Documents)))
				_, err = a.providersClient.SendDocument(ctx, &provider.ProviderSendDocumentRequest{
					GateId:         externalPeer.Via,
					ExternalUserId: userID,
					Documents:      extratcFiles(message.Documents),
					Caption:        message.Body,
					DomainId:       message.DomainID,
				})

			case model.MessageTypeImage:
				peerLog.Debug("sending image", slog.Int("images_count", len(message.Images)))
				_, err = a.providersClient.SendImage(ctx, &provider.ProviderSendImageRequest{
					GateId:         externalPeer.Via,
					ExternalUserId: userID,
					Images:         extratcFiles(message.Images),
					Caption:        message.Body,
					DomainId:       message.DomainID,
				})

			case model.MessageTypeText:
				peerLog.Debug("sending text")
				_, err = a.providersClient.SendText(ctx, &provider.ProviderSendTextRequest{
					GateId:         externalPeer.Via,
					ExternalUserId: userID,
					Text:           message.Body,
					DomainId:       message.DomainID,
				})

			case model.MessageTypeContact:
				peerLog.Debug("message type contact: not implemented, skipping")
			case model.MessageTypeInteractive:
				peerLog.Debug("message type interactive: not implemented, skipping")
			case model.MessageTypeLocation:
				peerLog.Debug("message type location: not implemented, skipping")
			case model.MessageTypeSystem:
				peerLog.Debug("message type system: not implemented, skipping")
			case model.MessageTypeUnknown:
				peerLog.Warn("message type unknown: skipping")
			}

			if err != nil {
				peerLog.Error("provider RPC failed",
					slog.String("error", err.Error()),
				)

				errorsMu.Lock()
				errorsArray = append(errorsArray, err)
				errorsMu.Unlock()

				return
			}

			peerLog.Debug("provider RPC succeeded")
		})
	}

	wg.Wait()

	if len(errorsArray) > 0 {
		log.Error("one or more provider sends failed",
			slog.String("message_id", message.ID.String()),
			slog.Int("failures", len(errorsArray)),
			slog.Any("errors", errorsArray),
		)

		return stderrs.Join(errorsArray...)
	}

	log.Info("all external providers notified successfully",
		slog.String("message_id", message.ID.String()),
		slog.Int("external_peers_count", len(externalPairs)),
	)

	return nil
}

func extratcFiles[T AttachmentProcessor](files []T) []*provider.ProviderFile {
	providerFiles := make([]*provider.ProviderFile, 0, len(files))

	for _, file := range files {
		providerFiles = append(providerFiles, extractFile(file))
	}

	return providerFiles
}

func extractFile[T AttachmentProcessor](first T) *provider.ProviderFile {
	return &provider.ProviderFile{
		Id:       strconv.Itoa(int(first.GetID())),
		Url:      first.GetURL(),
		Name:     first.GetName(),
		MimeType: first.GetMimeType(),
	}
}

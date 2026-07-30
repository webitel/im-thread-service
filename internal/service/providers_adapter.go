package service

import (
	"context"
	stderrs "errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/gen/go/provider/v1"
	improviders "github.com/webitel/im-thread-service/infra/webitel/im-providers"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/store"
)

type ProvidersAdapter interface {
	SendMessage(ctx context.Context, message *model.Message) error
	// SendTyping forwards a fire-and-forget typing indicator to the external
	// peers of a thread whose channel supports it.
	SendTyping(ctx context.Context, req *model.TypingDispatch) error
}

type baseRPCProvidersAdapter struct {
	logger          *slog.Logger
	providersClient *improviders.Client
	externalIDs     store.MessageExternalStore
}

func newBaseRPCProvidersAdapter(logger *slog.Logger, providersClient *improviders.Client, externalIDs store.MessageExternalStore) *baseRPCProvidersAdapter {
	log := logger.With("component", "base_rpc_providers_adapter")

	return &baseRPCProvidersAdapter{
		logger:          log,
		providersClient: providersClient,
		externalIDs:     externalIDs,
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

	allExternal := model.ThreadDialogs.ExtractExternalPeers(message.To)

	externalPairs := make([]*model.ExternalPeerPair, 0, len(allExternal))
	for _, p := range allExternal {
		if p.ContactID != message.From.ID {
			externalPairs = append(externalPairs, p)
		}
	}

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
			var (
				err  error
				resp *provider.ProviderSendMessageResponse
			)

			userID := externalPeer.ContactID.String()
			peerLog := log.With(
				slog.String("external_user_id", userID),
				slog.String("gate_id", externalPeer.Via),
				slog.String("message_type", message.Type.String()),
			)

			peerLog.Debug("dispatching to external provider")

			replyToExternal := a.lookupReplyExternalID(ctx, message, externalPeer.Via, peerLog)

			switch message.Type {
			case model.MessageTypeFile:
				peerLog.Debug("sending document", slog.Int("documents_count", len(message.Documents)))
				resp, err = a.providersClient.SendDocument(ctx, &provider.ProviderSendDocumentRequest{
					GateId:            externalPeer.Via,
					ExternalUserId:    userID,
					Documents:         extratcFiles(message.Documents),
					Caption:           message.Body,
					DomainId:          message.DomainID,
					MessageId:         message.ID.String(),
					ThreadId:          message.ThreadID.String(),
					ReplyToExternalId: replyToExternal,
				})

			case model.MessageTypeText:
				peerLog.Debug("sending text")

				resp, err = a.providersClient.SendText(ctx, &provider.ProviderSendTextRequest{
					GateId:            externalPeer.Via,
					ExternalUserId:    userID,
					Text:              message.Body,
					DomainId:          message.DomainID,
					MessageId:         message.ID.String(),
					ThreadId:          message.ThreadID.String(),
					ReplyToExternalId: replyToExternal,
				})

			case model.MessageTypeContact:
				peerLog.Debug("message type contact: not implemented, skipping")
			case model.MessageTypeInteractive:
				if message.Interactive == nil {
					peerLog.Warn("interactive message has nil interactive payload, skipping")

					return
				}

				peerLog.Debug("sending interactive")

				body := message.Body
				sendID := message.ID.String()
				resp, err = a.providersClient.SendInteractive(ctx, &provider.ProviderSendInteractiveRequest{
					GateId:            externalPeer.Via,
					ExternalUserId:    userID,
					DomainId:          message.DomainID,
					Body:              &body,
					SendId:            &sendID,
					Interactive:       mapInteractive(message.Interactive),
					MessageId:         message.ID.String(),
					ThreadId:          message.ThreadID.String(),
					ReplyToExternalId: replyToExternal,
				})
			case model.MessageTypeLocation:
				peerLog.Debug("message type location: not implemented, skipping")
			case model.MessageTypeSystem:
				if message.System == nil {
					peerLog.Warn("system message has nil system payload, skipping")

					return
				}

				peerLog.Debug("sending system message", slog.String("event_type", message.System.Type))
				resp, err = a.providersClient.SendSystemMessage(ctx, &provider.ProviderSendSystemMessageRequest{
					GateId:         externalPeer.Via,
					ExternalUserId: userID,
					DomainId:       message.DomainID,
					EventType:      message.System.Type,
					Vars:           metadataToStringMap(message.System.Metadata),
					MessageId:      message.ID.String(),
					ThreadId:       message.ThreadID.String(),
				})
			case model.MessageTypeUnknown:
				peerLog.Warn("message type unknown: skipping")
			case model.MessageTypeImage:
				peerLog.Warn("message image type unsupported now")
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

			a.persistExternalID(ctx, message, externalPeer.Via, resp, peerLog)
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

func (a *baseRPCProvidersAdapter) SendTyping(ctx context.Context, req *model.TypingDispatch) error {
	if req == nil || len(req.Peers) == 0 {
		return nil
	}

	log := a.logger.With("operation", "send_typing", slog.String("thread_id", req.ThreadID.String()))

	var (
		wg          sync.WaitGroup
		errorsMu    sync.Mutex
		errorsArray []error
	)

	for _, peer := range req.Peers {
		wg.Go(func() {
			_, err := a.providersClient.SendTyping(ctx, &provider.ProviderSendTypingRequest{
				GateId:         peer.Via,
				ExternalUserId: peer.ContactID.String(),
				DomainId:       req.DomainID,
				TypingOn:       req.TypingOn,
				ThreadId:       req.ThreadID.String(),
			})
			if err == nil {
				return
			}

			log.Warn("provider SendTyping failed",
				slog.String("external_user_id", peer.ContactID.String()),
				slog.String("gate_id", peer.Via),
				slog.String("error", err.Error()),
			)

			errorsMu.Lock()

			errorsArray = append(errorsArray, err)
			errorsMu.Unlock()
		})
	}

	wg.Wait()

	if len(errorsArray) > 0 {
		return stderrs.Join(errorsArray...)
	}

	return nil
}

func (a *baseRPCProvidersAdapter) lookupReplyExternalID(ctx context.Context, message *model.Message, gateID string, log *slog.Logger) string {
	if message.ReplyTo == nil {
		return ""
	}

	ext, err := a.externalIDs.LookupExternalID(ctx, message.ReplyTo.MessageID, gateID)
	if err != nil {
		log.Warn("external id lookup failed; sending without native reply", slog.String("error", err.Error()))

		return ""
	}

	if ext == "" {
		log.Info("reply target has no external id for gate; sending without native reply")
	}

	return ext
}

func (a *baseRPCProvidersAdapter) persistExternalID(ctx context.Context, message *model.Message, gateID string, resp *provider.ProviderSendMessageResponse, log *slog.Logger) {
	extID := resp.GetExternalId()
	if extID == "" {
		return
	}

	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()

	err := a.externalIDs.Save(saveCtx, &model.MessageExternalID{
		MessageID:  message.ID,
		ThreadID:   message.ThreadID,
		GateID:     gateID,
		ExternalID: extID,
		Direction:  model.ExternalDirectionOutbound,
	})
	if err != nil {
		log.Warn("failed to persist external message id mapping", slog.String("error", err.Error()))
	}
}

func mapInteractive(m *model.MessageInteractive) *provider.ProviderInteractive {
	out := &provider.ProviderInteractive{SingleUse: m.SingleUse}

	if m.Kind.Markup != nil {
		rows := make([]*provider.ProviderKeyboardRow, 0, len(m.Kind.Markup.Rows))
		for _, r := range m.Kind.Markup.Rows {
			rows = append(rows, &provider.ProviderKeyboardRow{Buttons: mapButtons(r.Buttons)})
		}

		out.Kind = &provider.ProviderInteractive_Markup{
			Markup: &provider.ProviderKeyboardMarkup{Rows: rows},
		}

		return out
	}

	if m.Kind.ListReply != nil {
		sections := make([]*provider.ProviderKeyboardRowWithSection, 0, len(m.Kind.ListReply.Sections))
		for _, s := range m.Kind.ListReply.Sections {
			sections = append(sections, &provider.ProviderKeyboardRowWithSection{
				Section: s.Section,
				Buttons: mapButtons(s.Buttons),
			})
		}

		out.Kind = &provider.ProviderInteractive_ListReply{
			ListReply: &provider.ProviderKeyboardListReply{
				MainButtonTitle: m.Kind.ListReply.Title,
				Sections:        sections,
			},
		}
	}

	return out
}

func mapButtons(buttons []*model.KeyboardButton) []*provider.ProviderKeyboardButton {
	out := make([]*provider.ProviderKeyboardButton, 0, len(buttons))
	for _, b := range buttons {
		pb := &provider.ProviderKeyboardButton{Id: b.ID, Label: b.Label}
		switch b.Type {
		case model.ActionTypeURL:
			url := ""
			if b.URL != nil {
				url = *b.URL
			}

			pb.Kind = &provider.ProviderKeyboardButton_Url{
				Url: &provider.ProviderKeyboardButtonURL{Url: url},
			}
		case model.ActionTypeCallback:
			data := ""
			if b.Data != nil {
				data = *b.Data
			}

			pb.Kind = &provider.ProviderKeyboardButton_Callback{
				Callback: &provider.ProviderKeyboardButtonCallback{Data: data},
			}
		case model.ActionTypeRequest:
			action := ""
			if b.Action != nil {
				action = *b.Action
			}

			pb.Kind = &provider.ProviderKeyboardButton_Request{
				Request: &provider.ProviderKeyboardButtonRequest{Action: action},
			}
		}

		out = append(out, pb)
	}

	return out
}

// metadataToStringMap converts map[string]any to map[string]string by calling
// fmt.Sprint on each value. Non-string types (UUIDs, roles) stringify naturally.
func metadataToStringMap(m map[string]any) map[string]string {
	if len(m) == 0 {
		return nil
	}

	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = fmt.Sprint(v)
	}

	return out
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

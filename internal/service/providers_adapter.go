package service

import (
	"context"
	"encoding/json"
	stderrs "errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/gen/go/provider/v1"
	improviders "github.com/webitel/im-thread-service/infra/webitel/im-providers"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	"github.com/webitel/im-thread-service/internal/store"
)

type ProvidersAdapter interface {
	SendMessage(ctx context.Context, message *model.Message) error
	// SendTyping forwards a fire-and-forget typing indicator to the external
	// peers of a thread whose channel supports it.
	SendTyping(ctx context.Context, req *model.TypingDispatch) error
	SendReaction(ctx context.Context, reaction *model.Reaction, result *model.ReactionResult) error
}

// sendFailureRecorder records a send failure im-providers did not report itself.
type sendFailureRecorder interface {
	MarkUnreportedFailed(ctx context.Context, receipts []*model.StatusReceipt) (int64, error)
}

type baseRPCProvidersAdapter struct {
	logger          *slog.Logger
	providersClient *improviders.Client
	externalIDs     store.MessageExternalStore
	reactionCaps    ReactionCapabilities
	failures        sendFailureRecorder
}

func newBaseRPCProvidersAdapter(
	logger *slog.Logger,
	providersClient *improviders.Client,
	externalIDs store.MessageExternalStore,
	reactionCaps ReactionCapabilities,
	statuses *MessageStatusService,
) *baseRPCProvidersAdapter {
	log := logger.With("component", "base_rpc_providers_adapter")

	return &baseRPCProvidersAdapter{
		logger:          log,
		providersClient: providersClient,
		externalIDs:     externalIDs,
		reactionCaps:    reactionCaps,
		failures:        statuses,
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

	sender := message.GetSender()

	externalPairs := make([]*model.ExternalPeerPair, 0, len(allExternal))
	for _, p := range allExternal {
		if p.ContactID != sender {
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

	// Formatting spans are parsed once at the gateway/thread boundary and stored on
	// the message; im-providers-service only relays/renders them for the destination
	// channel, so they're decoded once here and reused for every external peer below.
	entities := mapEntitiesToProvider(model.DecodeEntitiesFromMetadata(message.Metadata))

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
					SenderName:        senderName(message),
					Entities:          entities,
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
					SenderName:        senderName(message),
					Entities:          entities,
				})

			case model.MessageTypeContact:
				if message.Contact == nil {
					peerLog.Warn("contact message has nil contact payload")

					return
				}

				peerLog.Debug("sending contact")

				resp, err = a.providersClient.SendContact(ctx, &provider.ProviderSendContactRequest{
					GateId:            externalPeer.Via,
					ExternalUserId:    userID,
					DomainId:          message.DomainID,
					Name:              strval(message.Contact.Name),
					PhoneNumber:       strval(message.Contact.PhoneNumber),
					Email:             message.Contact.Email,
					ReplyToExternalId: replyToExternal,
					SenderName:        senderName(message),
					MessageId:         message.ID.String(),
					ThreadId:          message.ThreadID.String(),
				})
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
					SenderName:        senderName(message),
				})
			case model.MessageTypeLocation:
				if message.Location == nil {
					peerLog.Warn("location message has nil location payload, skipping")

					return
				}

				peerLog.Debug("sending location")

				resp, err = a.providersClient.SendLocation(ctx, &provider.ProviderSendLocationRequest{
					GateId:            externalPeer.Via,
					ExternalUserId:    userID,
					DomainId:          message.DomainID,
					Latitude:          message.Location.Latitude,
					Longitude:         message.Location.Longitude,
					Name:              message.Location.Name,
					Address:           message.Location.Address,
					ReplyToExternalId: replyToExternal,
					SenderName:        senderName(message),
					MessageId:         message.ID.String(),
					ThreadId:          message.ThreadID.String(),
				})
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

				a.recordSendFailure(ctx, message, externalPeer, err, peerLog)

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

// SendReaction forwards a reaction change to the external messenger(s) the
// message was exchanged with. It is best-effort and messenger-dependent: only
// gates that carry the message externally and whose provider supports reactions
// receive it; everything else is a no-op. Internal (webitel) recipients see the
// change through the MessageReaction domain event, not through this path.
func (a *baseRPCProvidersAdapter) SendReaction(ctx context.Context, reaction *model.Reaction, result *model.ReactionResult) error {
	log := a.logger.With("operation", "send_reaction")

	if reaction == nil || result == nil {
		return nil
	}

	externalPairs := make([]*model.ExternalPeerPair, 0)

	for _, p := range model.ThreadDialogs.ExtractExternalPeers(reaction.To) {
		if p.ContactID != reaction.ReactorID {
			externalPairs = append(externalPairs, p)
		}
	}

	if len(externalPairs) == 0 {
		return nil
	}

	removed := result.Action == model.ReactionActionRemoved

	var (
		wg          sync.WaitGroup
		errorsMu    sync.Mutex
		errorsArray []error
	)

	for _, externalPeer := range externalPairs {
		// The provider reacts to a message by its platform-specific id, so a gate
		// with no external id for this message cannot carry the reaction.
		externalID, err := a.externalIDs.LookupExternalID(ctx, reaction.MessageID, externalPeer.Via)
		if err != nil || externalID == "" {
			log.Debug("skipping reaction forward: message has no external id for gate",
				slog.String("message_id", reaction.MessageID.String()),
				slog.String("gate_id", externalPeer.Via),
			)

			continue
		}

		// A removal is always forwarded (clearing must reach the far side); a set
		// is gated by the gate's reaction allow-list.
		if !removed && a.reactionCaps != nil && !a.reactionCaps.Allowed(externalPeer.Via, result.Emoji) {
			log.Debug("skipping reaction forward: emoji not allowed for gate",
				slog.String("gate_id", externalPeer.Via),
				slog.String("emoji", result.Emoji),
			)

			continue
		}

		wg.Go(func() {
			peerLog := log.With(
				slog.String("gate_id", externalPeer.Via),
				slog.String("external_message_id", externalID),
			)

			if _, err := a.providersClient.SendReaction(ctx, &provider.ProviderSendReactionRequest{
				GateId:            externalPeer.Via,
				ExternalUserId:    externalPeer.ContactID.String(),
				DomainId:          reaction.DomainID,
				ExternalMessageId: externalID,
				Emoji:             result.Emoji,
				Removed:           removed,
				MessageId:         reaction.MessageID.String(),
				ThreadId:          reaction.ThreadID.String(),
			}); err != nil {
				peerLog.Error("provider SendReaction failed", slog.String("error", err.Error()))

				errorsMu.Lock()

				errorsArray = append(errorsArray, err)
				errorsMu.Unlock()

				return
			}

			peerLog.Debug("reaction forwarded to provider")
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
	out := &provider.ProviderInteractive{
		SingleUse:       m.SingleUse,
		Placement:       mapMenuPlacement(m.Placement),
		InputFieldState: mapInputFieldState(m.InputFieldState),
	}

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

func mapMenuPlacement(in model.MenuPlacement) provider.MenuPlacement {
	switch in {
	case model.MenuPlacementInline:
		return provider.MenuPlacement_MENU_PLACEMENT_INLINE
	case model.MenuPlacementPersistent:
		return provider.MenuPlacement_MENU_PLACEMENT_PERSISTENT
	case model.MenuPlacementUnspecified:
		return provider.MenuPlacement_MENU_PLACEMENT_UNSPECIFIED
	default:
		return provider.MenuPlacement_MENU_PLACEMENT_UNSPECIFIED
	}
}

func mapInputFieldState(in model.InputFieldState) provider.InputFieldState {
	switch in {
	case model.InputFieldStateRegular:
		return provider.InputFieldState_INPUT_FIELD_STATE_REGULAR
	case model.InputFieldStateMinimized:
		return provider.InputFieldState_INPUT_FIELD_STATE_MINIMIZED
	case model.InputFieldStateHidden:
		return provider.InputFieldState_INPUT_FIELD_STATE_HIDDEN
	case model.InputFieldStateUnspecified:
		return provider.InputFieldState_INPUT_FIELD_STATE_UNSPECIFIED
	default:
		return provider.InputFieldState_INPUT_FIELD_STATE_UNSPECIFIED
	}
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

// mapEntitiesToProvider converts stored formatting spans to the provider-service wire
// type. Value is only set when non-empty since provider.Entity.Value is optional
// (unset for span kinds that don't carry extra data, e.g. BOLD/ITALIC).
func mapEntitiesToProvider(entities []shared.Entity) []*provider.Entity {
	if len(entities) == 0 {
		return nil
	}

	out := make([]*provider.Entity, 0, len(entities))
	for _, e := range entities {
		pe := &provider.Entity{
			Type:   e.Type,
			Offset: int32(e.Offset),
			Length: int32(e.Length),
		}

		if e.Value != "" {
			pe.Value = &e.Value
		}

		out = append(out, pe)
	}

	return out
}

func strval(s *string) string {
	if s == nil {
		return ""
	}

	return *s
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

func senderName(message *model.Message) string {
	if message == nil || message.From.Identity == nil {
		return ""
	}

	return message.From.Identity.ChatName
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
		Size:     first.GetSize(),
	}
}

// failureRecordTimeout bounds the fallback failure write; it runs detached from
// the caller so a canceled send request still leaves the message marked failed.
const failureRecordTimeout = 5 * time.Second

// recordSendFailure marks the message failed for the peer when the provider RPC
// itself errored. im-providers reports channel rejections before it returns, and
// that richer report wins; this covers what it never got to report (unreachable,
// gate not resolved). Only messages a client can retry are marked: a failure
// nobody can act on would only leave a dead "not sent" badge.
func (a *baseRPCProvidersAdapter) recordSendFailure(ctx context.Context, message *model.Message, peer *model.ExternalPeerPair, sendErr error, log *slog.Logger) {
	if a.failures == nil || message.ID == uuid.Nil || message.ThreadID == uuid.Nil || !message.Type.Retryable() {
		return
	}

	// A timed-out or canceled call may still have reached the channel; the
	// outcome is unknown and im-providers reports the real one itself.
	if providerOutcomeUnknown(sendErr) {
		log.Warn("provider send outcome unknown, not marking it failed", slog.String("error", sendErr.Error()))

		return
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), failureRecordTimeout)
	defer cancel()

	code, detail := classifyProviderRPCError(sendErr)

	_, err := a.failures.MarkUnreportedFailed(ctx, []*model.StatusReceipt{{
		DomainID:  message.DomainID,
		ThreadID:  message.ThreadID,
		MessageID: message.ID,
		MemberID:  peer.ContactID,
		Via:       "provider",
		Error: map[string]any{
			"code":    code,
			"message": detail,
		},
	}})
	if err != nil {
		log.Error("recording provider send failure", slog.String("error", err.Error()))
	}
}

// unreachableCodes mean im-providers could not take the send at all: the
// channel never got it.
var unreachableCodes = []codes.Code{codes.Unavailable, codes.ResourceExhausted}

// providerOutcomeUnknown reports whether the provider call ended without a
// verdict (deadline or cancellation) rather than with a rejection.
func providerOutcomeUnknown(err error) bool {
	if stderrs.Is(err, context.DeadlineExceeded) || stderrs.Is(err, context.Canceled) {
		return true
	}

	st, ok := status.FromError(err)

	return ok && (st.Code() == codes.DeadlineExceeded || st.Code() == codes.Canceled)
}

// classifyProviderRPCError maps an im-providers RPC error to a normalized
// delivery error code (see DeliveryError in the history proto). Callers skip
// errors providerOutcomeUnknown reports.
func classifyProviderRPCError(err error) (code, detail string) {
	st, ok := status.FromError(err)
	if !ok {
		return model.DeliveryErrorSendFailed, err.Error()
	}

	detail = remoteErrorDetail(st.Message())

	switch {
	case slices.Contains(unreachableCodes, st.Code()):
		return model.DeliveryErrorChannelUnavailable, detail
	case st.Code() == codes.Unimplemented:
		// The channel cannot send this kind of message (e.g. a location to a
		// messenger without location support).
		return model.DeliveryErrorUnsupportedMedia, detail
	default:
		return model.DeliveryErrorSendFailed, detail
	}
}

// remoteErrorDetail unwraps the JSON envelope webitel services put in a gRPC
// status message ({"id":...,"detail":"..."}), so the stored failure carries
// the provider's text rather than the envelope.
func remoteErrorDetail(msg string) string {
	var envelope struct {
		Detail string `json:"detail"`
	}

	if err := json.Unmarshal([]byte(msg), &envelope); err != nil || envelope.Detail == "" {
		return msg
	}

	return envelope.Detail
}

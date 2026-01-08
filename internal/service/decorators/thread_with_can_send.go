package decorators

import (
	"context"
	"log/slog"

	imcontact "github.com/webitel/im-thread-service/infra/webitel/im-contact"

	"github.com/webitel/im-thread-service/internal/service"
	"github.com/webitel/im-thread-service/internal/service/dto"
	"github.com/webitel/im-thread-service/internal/service/guards"
)

type threadWithCanSendDecorator struct {
	service.ThreadManager

	logger     *slog.Logger
	imContacts *imcontact.Client
}

// Return ThreadManager decorator with imcontact.Client gRPC client
func NewThreadWithCanSendDecorator(logger *slog.Logger, base service.ThreadManager, imContactsClient *imcontact.Client) *threadWithCanSendDecorator {
	componentLogger := logger.With("component", "thread.decorator.can_send") // ADD SCOPE CONTEXT

	return &threadWithCanSendDecorator{
		logger:        componentLogger,
		imContacts:    imContactsClient,
		ThreadManager: base,
	}
}

// EnsureDirectThread will check if the request is valid (non empty, non self-send),
// then it will send a gRPC request to im-contact service to validate
// that Peer.From can chat with Peer.To. If the validation returns an error,
// it will log the error and return the error. If the validation returns
// false (i.e. Peer.From can't chat with Peer.To), it will log a warning
// and return an error. If the validation returns true (i.e. Peer.From can
// chat with Peer.To), it will return the result of the base implementation's
// EnsureDirectThread method.
func (t *threadWithCanSendDecorator) EnsureDirectThread(ctx context.Context, req *dto.EnsureDirectThreadRequest) (*dto.EnsureDirectThreadResponse, error) {
	var err error

	// INPUT VALIDATION FIRST!
	if err = guards.EnsureDirectThreadValidationGuard(req); err != nil {
		t.logger.Warn("direct thread validation violation!", "err", err)
		return nil, err
	}

	canSendRequest := dto.NewCanSendRequestDtoFromPeers(*req.PeerFrom, *req.PeerTo)

	//SEND RPC CALL TO IM-CONTACT SERVICE TO VALIDATE THAT Peer.From CAN CHAT WITH Peer.To!
	//TODO: add can send result cacheability!
	//? better to cache result centralized on RPC ContactsService implementation or in this wrapper?
	resp, err := t.imContacts.ContactsService().CanSend(ctx, canSendRequest)
	if err != nil {
		t.logger.Error("error in can send rights validation gRPC request!", "err", err)
		return nil, err
	}

	// EARLY RETURN WITH VIOLATION LOG IF CAN`T CHAT!
	if err = guards.CanSendRightsViolationGuard(resp.CanSend); err != nil {
		t.logger.Warn("send rights violation", "from", req.PeerFrom.ID, "to", req.PeerTo.ID)
		return nil, err
	}

	// RETURN BASE IMPLEMENTATION RESULT!
	return t.ThreadManager.EnsureDirectThread(ctx, req)
}

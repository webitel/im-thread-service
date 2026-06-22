package service

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

type ThreadCreatorsFactoryProvider interface {
	Create(ctx context.Context, createThreadRequest *CreateThreadRequest) (*model.Thread, error)
}

type ThreadCreatorsFactory struct {
	creators map[model.ThreadKind]ThreadCreator
}

func NewThreadCreatorsFactory(creators []ThreadCreator) *ThreadCreatorsFactory {
	factoryMap := make(map[model.ThreadKind]ThreadCreator)

	for _, creator := range creators {
		if k, ok := creator.(interface{ Kind() model.ThreadKind }); ok {
			factoryMap[k.Kind()] = creator
		}
	}

	return &ThreadCreatorsFactory{creators: factoryMap}
}

func (f *ThreadCreatorsFactory) Create(ctx context.Context, createThreadRequest *CreateThreadRequest) (*model.Thread, error) {
	creator, exists := f.creators[createThreadRequest.Kind()]
	if !exists {
		return nil, errors.NotFound(
			"creator for given thread kind not exists",
			errors.WithID("service.thread_factory.thread_creator_not_found"),
			errors.WithValue("thread_kind", createThreadRequest.Kind()),
		)
	}

	return creator.Create(ctx, createThreadRequest)
}

type ThreadCreator interface {
	Create(ctx context.Context, createThreadRequest *CreateThreadRequest) (*model.Thread, error)
}

func WithKind(kind model.ThreadKind) func(c *CreateThreadRequest) {
	return func(c *CreateThreadRequest) {
		c.kind = kind
	}
}

func WithDirectConfig(member shared.Peer) func(c *CreateThreadRequest) {
	return func(c *CreateThreadRequest) {
		c.Payload = &DirectConfig{
			Member: member,
		}
	}
}

type CreateThreadRequest struct {
	DomainID  int64
	kind      model.ThreadKind
	Initiator shared.Peer
	Payload   any
}

func NewCreateThreadRequest(domainID int64, initiator shared.Peer, options ...func(*CreateThreadRequest)) *CreateThreadRequest {
	r := &CreateThreadRequest{
		DomainID:  domainID,
		kind:      model.ThreadUnspecified,
		Initiator: initiator,
	}

	for _, o := range options {
		o(r)
	}

	return r
}

func (cr *CreateThreadRequest) Kind() model.ThreadKind {
	if cr == nil {
		return model.ThreadUnspecified
	}

	return cr.kind
}

func (cr *CreateThreadRequest) Validate() error {
	if cr == nil {
		return errors.InvalidArgument("received nil pointer call for create thread request", errors.WithID("service.thread_factory.create_thread_request.validate"))
	}

	if cr.DomainID <= 0 {
		return errors.InvalidArgument("domain id is required", errors.WithID("service.thread_factory.create_thread_request.validate"))
	}

	if cr.Initiator.ID == uuid.Nil {
		return errors.InvalidArgument("initiator is required", errors.WithID("service.thread_factory.create_thread_request.validate"))
	}

	if cr.Initiator.Type != shared.PeerContact {
		return errors.InvalidArgument("initiator must be a contact", errors.WithID("service.thread_factory.create_thread_request.validate"))
	}

	return nil
}

type DirectConfig struct{ Member shared.Peer }

func (dc *DirectConfig) Validate() error {
	if dc == nil {
		return errors.InvalidArgument("received nil pointer direct config call", errors.WithID("service.thread_factory.direct_config.validate"))
	}

	if dc.Member.ID == uuid.Nil {
		return errors.InvalidArgument("received nil uuid for direct member", errors.WithID("service.thread_factory.direct_config.validate"))
	}

	if dc.Member.Type != shared.PeerContact {
		return errors.InvalidArgument("member type must be contact", errors.WithID("service.thread_factory.direct_config.validate"))
	}

	if dc.Member.Identity == nil {
		return errors.InvalidArgument("member identity is required", errors.WithID("service.thread_factory.direct_config.validate"))
	}

	return nil
}

type DirectThreadCreatorGuarder interface {
	CheckPreconditions(_ context.Context, createThreadRequest *CreateThreadRequest) error
}

type directThreadCreatorGuard struct{}

func NewDirectThreadCreatorGuard() *directThreadCreatorGuard { return &directThreadCreatorGuard{} }

func (g *directThreadCreatorGuard) CheckPreconditions(_ context.Context, createThreadRequest *CreateThreadRequest) error {
	return g.checkRequestPreconditions(createThreadRequest)
}

func (g *directThreadCreatorGuard) checkRequestPreconditions(createThreadRequest *CreateThreadRequest) error {
	if err := createThreadRequest.Validate(); err != nil {
		return err
	}

	cfg, ok := createThreadRequest.Payload.(*DirectConfig)
	if !ok || cfg == nil {
		return errors.InvalidArgument("invalid payload type, expected direct config", errors.WithID("service.direct_guard.invalid_payload"))
	}

	if err := cfg.Validate(); err != nil {
		return err
	}

	if createThreadRequest.Initiator.ID == cfg.Member.ID {
		return errors.InvalidArgument("initiator and peer cannot be the same contact", errors.WithID("service.direct_guard.self_direct_forbidden"))
	}

	return nil
}

type DirectThreadCreator struct {
	manager ThreadManager
	guard   DirectThreadCreatorGuarder
	logger  *slog.Logger
}

func (c *DirectThreadCreator) Kind() model.ThreadKind { return model.ThreadDirect }

func NewDirectThreadCreator(manager ThreadManager, logger *slog.Logger, guard DirectThreadCreatorGuarder) *DirectThreadCreator {
	return &DirectThreadCreator{
		manager: manager,
		logger:  logger,
		guard:   guard,
	}
}

func (c *DirectThreadCreator) Create(ctx context.Context, createThreadRequest *CreateThreadRequest) (*model.Thread, error) {
	if err := c.guard.CheckPreconditions(ctx, createThreadRequest); err != nil {
		return nil, err
	}

	c.logger.Info(
		"creating direct thread initiated by",
		slog.Group(
			"initiator",
			slog.String("id", createThreadRequest.Initiator.ID.String()),
			slog.String("iss", createThreadRequest.Initiator.Identity.Issuer),
			slog.String("name", createThreadRequest.Initiator.Identity.Name),
		),
	)

	directConfig, ok := createThreadRequest.Payload.(*DirectConfig)
	if !ok {
		return nil, errors.InvalidArgument("invalid payload type", errors.WithID("service.direct_creator.invalid_payload"))
	}

	ensureDirectThreadRequest := &dto.EnsureDirectThreadRequest{
		DomainID: int(createThreadRequest.DomainID),
		From:     &createThreadRequest.Initiator,
		To:       &directConfig.Member,
	}

	thread, err := c.manager.EnsureDirectThread(ctx, ensureDirectThreadRequest)
	if err != nil {
		return nil, err
	}

	return thread, nil
}

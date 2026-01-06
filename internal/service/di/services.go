package di

import (
	"log/slog"

	imcontact "github.com/webitel/im-thread-service/infra/webitel/im-contact"
	"github.com/webitel/im-thread-service/internal/service"
	"github.com/webitel/im-thread-service/internal/service/decorators"
	"go.uber.org/fx"
)

var Module = fx.Module(
	"service",
	fx.Provide(
		fx.Annotate(
			service.NewThreadService,
			fx.As(new(service.ThreadManager)),
		),
	),
	fx.Decorate(func(logger *slog.Logger, base service.ThreadManager, imContactsClient *imcontact.Client) service.ThreadManager {
		return decorators.NewThreadWithCanSendDecorator(logger, base, imContactsClient)
	}),
)

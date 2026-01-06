package grpc

import (
	"context"

	"go.uber.org/fx"

	impb "github.com/webitel/im-thread-service/gen/go/api/v1"
	grpcsrv "github.com/webitel/im-thread-service/infra/server/grpc"
)

var Module = fx.Module("grpc",
	fx.Provide(
		NewMessageService,
	),
	fx.Invoke(
		RegisterMessageService,
	),
)

func RegisterMessageService(server *grpcsrv.Server, service *MessageService, lc fx.Lifecycle) error {
	lc.Append(
		fx.Hook{
			OnStart: func(ctx context.Context) error {
				impb.RegisterMessageServer(server.Server, service)
				return nil
			},
		})

	return nil
}

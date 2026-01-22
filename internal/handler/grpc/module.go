// internal/handler/grpc/module.go
package grpc

import (
	"go.uber.org/fx"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	grpcsrv "github.com/webitel/im-thread-service/infra/server/grpc"
)

var Module = fx.Module("message_grpc",
	fx.Provide(
		NewMessageService,
	),
	fx.Invoke(RegisterMessageService),
)

func RegisterMessageService(
	server *grpcsrv.Server,
	service *MessageService,
) {
	impb.RegisterMessageServer(server.Server, service)
}

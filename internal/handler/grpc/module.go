// internal/handler/grpc/module.go
package grpc

import (
	"go.uber.org/fx"

	impbv "github.com/webitel/im-thread-service/gen/go/api/thread/v1"
	grpcsrv "github.com/webitel/im-thread-service/infra/server/grpc"
)

var Module = fx.Module("message_grpc",
	fx.Provide(
		NewMessageService,
		NewMessageHistoryService,
		NewThreadService,
	),
	fx.Invoke(RegisterMessageService),
	fx.Invoke(RegisterMessageHistoryService),
	fx.Invoke(RegisterThreadService),
)

func RegisterMessageService(
	server *grpcsrv.Server,
	service *MessageService,
) {
	impbv.RegisterMessageServer(server.Server, service)
}

func RegisterMessageHistoryService(srv *grpcsrv.Server, svc *MessageHistoryService) {
	impbv.RegisterMessageHistoryServer(srv.Server, svc)
}

func RegisterThreadService(srv *grpcsrv.Server, svc *ThreadService) {
	impbv.RegisterThreadServer(srv.Server, svc)
}

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
		fx.Annotate(
			NewMessageHistoryService,
			fx.ParamTags(`name:"enriched_message_history"`),
		),
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
	impb.RegisterThreadManagementServer(srv.Server, svc)
}

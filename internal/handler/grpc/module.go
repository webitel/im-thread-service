// internal/handler/grpc/module.go
package grpc

import (
	"go.uber.org/fx"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	grpcsrv "github.com/webitel/im-thread-service/infra/server/grpc"
	"github.com/webitel/im-thread-service/internal/service"
)

var Module = fx.Module("message_grpc",
	fx.Provide(
		NewMessageService,
		NewMessageHistoryServer,
		NewThreadService,
		NewThreadPermissionServer,

		fx.Annotate(
			service.NewThreadVariables,
			fx.As(new(ThreadVariablesOperator)),
		),
	),
	fx.Invoke(RegisterMessageServer),
	fx.Invoke(RegisterMessageHistoryServer),
	fx.Invoke(RegisterThreadServer),
	fx.Invoke(RegisterThreadPermissionServer),
)

func RegisterMessageServer(
	server *grpcsrv.Server,
	service *MessageServer,
) {
	impb.RegisterMessageServer(server.Server, service)
}

func RegisterMessageHistoryServer(srv *grpcsrv.Server, svc *MessageHistoryServer) {
	impb.RegisterMessageHistoryServer(srv.Server, svc)
}

func RegisterThreadServer(srv *grpcsrv.Server, svc *ThreadManagementServer) {
	impb.RegisterThreadManagementServer(srv.Server, svc)
}

func RegisterThreadPermissionServer(srv *grpcsrv.Server, svc *ThreadPermissionManagementServer) {
	impb.RegisterThreadPermissionManagementServer(srv.Server, svc)
}

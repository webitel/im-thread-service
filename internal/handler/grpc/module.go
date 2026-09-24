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
		NewMessageStatusServer,
		NewMessageHistoryServer,
		NewThreadService,
		NewThreadPermissionServer,
		NewThreadTagServer,

		fx.Annotate(
			service.NewThreadVariables,
			fx.As(new(ThreadVariablesOperator)),
		),
	),
	fx.Invoke(RegisterMessageServer),
	fx.Invoke(RegisterMessageStatusServer),
	fx.Invoke(RegisterMessageHistoryServer),
	fx.Invoke(RegisterThreadServer),
	fx.Invoke(RegisterThreadPermissionServer),
	fx.Invoke(RegisterThreadTagServer),
)

func RegisterMessageServer(
	server *grpcsrv.Server,
	service *MessageServer,
) {
	impb.RegisterMessageServer(server.Server, service)
}

func RegisterMessageStatusServer(srv *grpcsrv.Server, svc *MessageStatusServer) {
	impb.RegisterMessageStatusServer(srv.Server, svc)
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

func RegisterThreadTagServer(srv *grpcsrv.Server, svc *ThreadTagManagementServer) {
	impb.RegisterThreadTagManagementServer(srv.Server, svc)
}

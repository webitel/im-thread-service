package webiteldi

import (
	"context"

	imcontact "github.com/webitel/im-thread-service/infra/webitel/im-contact"
	storageclient "github.com/webitel/im-thread-service/infra/webitel/storage"
	"go.uber.org/fx"
)

var Module = fx.Module(
	"webitel_clients",

	// [CONSTRUCTOR] Provides the resilient contact client
	fx.Provide(imcontact.New),
	fx.Provide(storageclient.New),

	// [LIFECYCLE] Ensures the gRPC connection pool is closed gracefully on app shutdown
	fx.Invoke(func(lc fx.Lifecycle, client *imcontact.Client) {
		lc.Append(fx.Hook{
			OnStop: func(ctx context.Context) error {
				return client.Close()
			},
		})
	}),
)

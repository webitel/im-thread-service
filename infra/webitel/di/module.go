package webiteldi

import (
	"context"

	"go.uber.org/fx"

	imcontact "github.com/webitel/im-thread-service/infra/webitel/im-contact"
	improviders "github.com/webitel/im-thread-service/infra/webitel/im-providers"
	storageclient "github.com/webitel/im-thread-service/infra/webitel/storage"
)

var Module = fx.Module(
	"webitel_clients",

	// [CONSTRUCTOR] Provides the resilient contact client
	fx.Provide(imcontact.New),
	fx.Provide(storageclient.New),
	fx.Provide(improviders.NewProvidersClient),

	// [LIFECYCLE] Ensures the gRPC connection pool is closed gracefully on app shutdown
	fx.Invoke(func(lc fx.Lifecycle, client *imcontact.Client) {
		lc.Append(fx.Hook{
			OnStop: func(_ context.Context) error {
				return client.Close()
			},
		})
	}),

	fx.Invoke(func(lc fx.Lifecycle, client *improviders.Client) {
		lc.Append(fx.Hook{
			OnStop: func(_ context.Context) error {
				return client.Close()
			},
		})
	}),
)

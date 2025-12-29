package cmd

import (
	"github.com/webitel/im-thread-service/config"
	"go.uber.org/fx"
)

func NewApp(cfg *config.Config) *fx.App {
	return fx.New(
		fx.Provide(
			func() *config.Config { return cfg },
			ProvideLogger,
			ProvideGrpcServer,
			ProvideSD,
			ProvidePubSub,
		),
		fx.Invoke(
			StartGrpcServer,
		),
	)
}

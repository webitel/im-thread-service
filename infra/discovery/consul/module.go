package leader

import (
	"github.com/webitel/im-thread-service/internal/adapter/pubsub"
	"go.uber.org/fx"
)

var Module = fx.Module(
	"leader",
	fx.Provide(
		fx.Annotate(
			ProvideLeaderElector,
			fx.As(new(pubsub.LeadershipElector)),
		),
	),
)

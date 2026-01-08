package leader

import (
	"go.uber.org/fx"
)

var Module = fx.Module(
	"leader",
	fx.Provide(
		fx.Annotate(
			ProvideLeaderElector,
			fx.As(new(LeadershipElector)),
		),
	),
)

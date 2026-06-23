package postgres

import (
	"context"

	"go.uber.org/fx"

	"github.com/webitel/im-thread-service/config"
	"github.com/webitel/im-thread-service/infra/db/pg"
	"github.com/webitel/im-thread-service/internal/store"
	"github.com/webitel/webitel-go-kit/infra/pgw"
)

var Module = fx.Module("store",
	fx.Provide(
		ProvideNewDBConnection,
		fx.Annotate(
			func(p *pgw.PoolManager) (Querier, error) {
				return p.Primary()
			},
			fx.As(new(Querier)),
		),

		fx.Annotate(
			NewMessageStore,
			fx.As(new(store.MessageStore)),
		),

		fx.Annotate(
			NewOutboxStore,
			fx.As(new(store.OutboxStore)),
		),

		fx.Annotate(
			NewStore,
			fx.As(new(store.Store)),
		),

		fx.Annotate(
			NewMessageHistoryStore,
			fx.As(new(store.MessageHistory)),
		),
		fx.Annotate(
			NewDirectThreadDialogOrchestration,
			fx.As(new(store.DirectThreadDialogOrchestration)),
		),
		fx.Annotate(
			NewThreadStore,
			fx.As(new(store.ThreadStore)),
		),
		fx.Annotate(
			NewThreadPermissionStore,
			fx.As(new(store.ThreadPermissionStore)),
		),
		fx.Annotate(
			NewPgxUnitOfWork,
			fx.As(new(store.UnitOfWork)),
		),

		fx.Annotate(
			NewThreadVariablesStore,
			fx.As(new(store.ThreadVariablesStore)),
		),

		fx.Annotate(
			NewBotControlStore,
			fx.As(new(store.BotControlStore)),
		),
	),
)

func ProvideNewDBConnection(cfg *config.Config, lc fx.Lifecycle) (*pgw.PoolManager, error) {
	db, err := pg.New(context.Background(), cfg)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			db.Close()
			return nil
		},
	})

	return db, err
}

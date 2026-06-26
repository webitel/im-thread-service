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
		ProvidePoolManager,
		fx.Annotate(
			NewMessageStoreFactory,
			fx.As(new(store.MessageStoreFactory)),
		),
		fx.Annotate(
			NewMessageHistoryStoreFactory,
			fx.As(new(store.MessageHistoryStoreFactory)),
		),
		fx.Annotate(
			NewThreadVariablesStoreFactory,
			fx.As(new(store.ThreadVariablesStoreFactory)),
		),
		fx.Annotate(
			NewThreadDialogStoreFactory,
			fx.As(new(store.ThreadDialogStoreFactory)),
		),
		fx.Annotate(
			NewThreadStoreFactory,
			fx.As(new(store.ThreadStoreFactory)),
		),
		fx.Annotate(
			NewThreadPermissionStoreFactory,
			fx.As(new(store.ThreadPermissionStoreFactory)),
		),
		fx.Annotate(
			NewBotControlStoreFactory,
			fx.As(new(store.BotControlStoreFactory)),
		),
		fx.Annotate(
			NewOutboxStoreFactory,
			fx.As(new(store.OutboxStoreFactory)),
		),
		fx.Annotate(
			NewUnitOfWorkFactory,
			fx.As(new(store.UnitOfWorkFactory)),
		),
	),
)

func ProvidePoolManager(cfg *config.Config, lc fx.Lifecycle) (*pgw.PoolManager, error) {
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

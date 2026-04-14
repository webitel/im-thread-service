package postgres

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/webitel/im-thread-service/config"
	"github.com/webitel/im-thread-service/infra/db/pg"
	"github.com/webitel/im-thread-service/internal/store"
	"go.uber.org/fx"
)

var Module = fx.Module("store",
	fx.Provide(
		ProvideNewDBConnection,
		pg.ProvidePgxPool,
		fx.Annotate(
			func(p *pgxpool.Pool) Querier {
				return p
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
	),
)

func ProvideNewDBConnection(cfg *config.Config, l *slog.Logger, lc fx.Lifecycle) (*pg.PgxDB, error) {
	db, err := pg.New(context.Background(), l, cfg.Postgres.DSN)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			db.Master().Close()
			return nil
		},
	})

	return db, err
}

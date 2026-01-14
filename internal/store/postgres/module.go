package postgres

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/webitel/im-thread-service/internal/store"
	"go.uber.org/fx"
)

var Module = fx.Module("store",
	fx.Provide(

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
			NewPgxUnitOfWork,
			fx.As(new(store.UnitOfWork)),
		),
	),
)

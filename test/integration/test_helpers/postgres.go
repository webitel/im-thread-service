package testhelpers

import (
	"context"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

const PostgreSQLTestVersion string = "postgres:18.1-alpine"

type PostgresContainer struct {
	*postgres.PostgresContainer
	ConnectionString string
}

func NewPostgresContainer(ctx context.Context) (*PostgresContainer, error) {
	postgresContainer, err := postgres.Run(
		ctx,
		PostgreSQLTestVersion,
		postgres.WithDatabase("webitel"),
		postgres.WithUsername("opensips"),
		postgres.WithPassword("webitel"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(time.Second*5),
		),
	)

	if err != nil {
		return nil, err
	}

	connStr, err := postgresContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return nil, err
	}

	var container = new(PostgresContainer)
	{
		container.ConnectionString = connStr
		container.PostgresContainer = postgresContainer
	}

	return container, nil
}

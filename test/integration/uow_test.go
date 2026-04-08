package integration

import (
	"context"
	"log"
	"log/slog"
	"testing"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/suite"
	"github.com/webitel/im-thread-service/cmd/migrate"
	"github.com/webitel/im-thread-service/config"
	"github.com/webitel/im-thread-service/internal/store"
	"github.com/webitel/im-thread-service/internal/store/postgres"
	testhelpers "github.com/webitel/im-thread-service/test/integration/test_helpers"
)

type UoWTestSuite struct {
	suite.Suite
	container *testhelpers.PostgresContainer
	pool      *pgxpool.Pool
	uow       store.UnitOfWork
}

func (s *UoWTestSuite) SetupSuite() {
	ctx := context.Background()
	container, err := testhelpers.NewPostgresContainer(ctx)
	s.Require().NoError(err)
	s.container = container

	pool, err := pgxpool.New(ctx, container.ConnectionString)
	s.Require().NoError(err)
	s.pool = pool

	mig := migrate.NewMigrator(&config.Config{Postgres: config.PostgresConfig{DSN: container.ConnectionString}}, slog.Default())
	if err := mig.Run(context.Background()); err != nil {
		log.Fatal(err)
	}

	s.uow = postgres.NewPgxUnitOfWork(pool, watermill.NewSlogLogger(slog.Default()))
}

func (suite *UoWTestSuite) SetupTest() {
	ctx := context.Background()
	_, err := suite.pool.Exec(ctx, "TRUNCATE TABLE im_thread.thread CASCADE;")
	suite.Require().NoError(err, "failed to truncate tables")
}

func (s *UoWTestSuite) TearDownSuite() {
	if s.container != nil {
		s.container.Terminate(context.Background())
	}
}

func TestUoWSuite(t *testing.T) {
	suite.Run(t, new(UoWTestSuite))
}

// func (s *UoWTestSuite) TestUnitOfWork_WithinTransaction_Rollback() {
// 	ctx := context.Background()
// 	domainID := 100

// 	err := s.uow.WithinTransaction(ctx, func(ctx context.Context, txUow store.UnitOfWork) error {
// 		thread := &model.Thread{
// 			BaseModel: shared.BaseModel{DomainID: domainID},
// 			Kind:      model.ThreadDirect,
// 		}
// 		createdThread, err := txUow.ThreadStore().Create(ctx, thread)
// 		s.Require().NoError(err)
// 		s.Require().NotEqual(uuid.Nil, createdThread.ID)

// 		return assert.AnError
// 	})

// 	s.ErrorIs(err, assert.AnError)
// 	var exists bool
// 	query := "SELECT EXISTS(SELECT 1 FROM im_thread.thread WHERE domain_id = $1)"
// 	err = s.pool.QueryRow(ctx, query, domainID).Scan(&exists)

// 	s.NoError(err)
// 	s.False(exists, "Data should be rolled back after transaction error")
// }

// func (s *UoWTestSuite) TestUnitOfWork_WithinTransaction_Commit() {
// 	ctx := context.Background()
// 	domainID := 200

// 	err := s.uow.WithinTransaction(ctx, func(ctx context.Context, txUow store.UnitOfWork) error {
// 		thread, _ := txUow.ThreadStore().Create(ctx, &model.Thread{
// 			BaseModel: shared.BaseModel{DomainID: domainID},
// 			Kind:      model.ThreadDirect,
// 		})

// 		directTo := uuid.New()

// 		_, err := txUow.ThreadDialogStore().CreateDirectPair(ctx, &model.ThreadDialogExtended{
// 			BaseModel: shared.BaseModel{DomainID: domainID},
// 			ThreadID:  thread.ID,
// 			MemberID:  uuid.New(),
// 			DirectTo:  &directTo,
// 		})
// 		return err
// 	})

// 	s.NoError(err)

// 	var count int
// 	s.pool.QueryRow(ctx, "SELECT count(*) FROM im_thread.thread_dialog WHERE domain_id = $1", domainID).Scan(&count)
// 	s.Equal(2, count, "Should have created two dialog records")
// }

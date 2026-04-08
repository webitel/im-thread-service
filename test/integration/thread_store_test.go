package integration

import (
	"context"
	"log"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/suite"
	"github.com/webitel/im-thread-service/cmd/migrate"
	"github.com/webitel/im-thread-service/config"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/store"
	"github.com/webitel/im-thread-service/internal/store/postgres"
	testhelpers "github.com/webitel/im-thread-service/test/integration/test_helpers"
)

type ThreadStoreTestSuite struct {
	suite.Suite
	container *testhelpers.PostgresContainer
	pool      *pgxpool.Pool
	repo      store.ThreadStore
}

func (s *ThreadStoreTestSuite) SetupSuite() {
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
}

func (s *ThreadStoreTestSuite) SetupTest() {
	_, err := s.pool.Exec(context.Background(), "TRUNCATE TABLE im_thread.thread CASCADE")
	s.Require().NoError(err)

	s.repo = postgres.NewThreadStore(s.pool)
}

func (s *ThreadStoreTestSuite) TearDownSuite() {
	if s.pool != nil {
		s.pool.Close()
	}
	if s.container != nil {
		s.container.Terminate(context.Background())
	}
}

func TestThreadStoreSuite(t *testing.T) {
	suite.Run(t, new(ThreadStoreTestSuite))
}

// func (s *ThreadStoreTestSuite) TestCreate_Success() {
// 	ctx := context.Background()
// 	now := time.Now().UTC().Truncate(time.Microsecond)

// 	req := &model.Thread{
// 		BaseModel: shared.BaseModel{
// 			DomainID:  1,
// 			CreatedAt: now,
// 			UpdatedAt: now,
// 		},
// 		Kind:        model.ThreadGroup,
// 		Subject:     "Community Chat",
// 		Description: "Test description",
// 	}

// 	result, err := s.repo.Create(ctx, req)

// 	s.NoError(err)
// 	s.NotEqual(uuid.Nil, result.ID)
// 	s.Equal(req.DomainID, result.DomainID)
// 	s.Equal(req.Subject, result.Subject)

// 	var dbSubject string
// 	err = s.pool.QueryRow(ctx, "SELECT subject FROM im_thread.thread WHERE id = $1", result.ID).Scan(&dbSubject)
// 	s.NoError(err)
// 	s.Equal("Community Chat", dbSubject)
// }

func (s *ThreadStoreTestSuite) TestCreate_EmptyRequiredFields() {
	ctx := context.Background()
	req := &model.Thread{
		Kind: model.ThreadDirect,
	}

	result, err := s.repo.Create(ctx, req)

	s.Error(err)
	s.Nil(result)
}

// func (s *ThreadStoreTestSuite) TestCreate_SpecialCharactersAndLongText() {
// 	ctx := context.Background()
// 	longDescription := "Very long long text with special characters 🤡 and emoji: \n \t ' \" ; --"

// 	req := &model.Thread{
// 		BaseModel: shared.BaseModel{
// 			DomainID:  1,
// 			CreatedAt: time.Now().UTC(),
// 			UpdatedAt: time.Now().UTC(),
// 		},
// 		Kind:        model.ThreadGroup,
// 		Subject:     "Test thread 🤡",
// 		Description: longDescription,
// 	}

// 	result, err := s.repo.Create(ctx, req)

// 	s.NoError(err)
// 	s.Equal(longDescription, result.Description)
// 	s.Equal("Test thread 🤡", result.Subject)
// }

// func (s *ThreadStoreTestSuite) TestCreate_Concurrent() {
// 	ctx := context.Background()
// 	concurrency := 10
// 	errChan := make(chan error, concurrency)

// 	for i := range concurrency {
// 		go func(idx int) {
// 			req := &model.Thread{
// 				BaseModel: shared.BaseModel{
// 					DomainID:  1,
// 					CreatedAt: time.Now().UTC(),
// 					UpdatedAt: time.Now().UTC(),
// 				},
// 				Kind: model.ThreadDirect,
// 			}
// 			_, err := s.repo.Create(ctx, req)
// 			errChan <- err
// 		}(i)
// 	}

// 	for range concurrency {
// 		s.NoError(<-errChan)
// 	}
// }

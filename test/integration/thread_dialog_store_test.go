package integration

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/suite"
	"github.com/webitel/im-thread-service/cmd/migrate"
	"github.com/webitel/im-thread-service/config"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
	"github.com/webitel/im-thread-service/internal/store"
	"github.com/webitel/im-thread-service/internal/store/postgres"
	testhelpers "github.com/webitel/im-thread-service/test/integration/test_helpers"
)

type ThreadDialogStoreTestSuite struct {
	suite.Suite
	container *testhelpers.PostgresContainer
	pool      *pgxpool.Pool
	repo      store.ThreadDialogStore
}

func (s *ThreadDialogStoreTestSuite) SetupSuite() {
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

func (s *ThreadDialogStoreTestSuite) SetupTest() {
	_, err := s.pool.Exec(context.Background(), "TRUNCATE TABLE im_thread.thread CASCADE")
	s.Require().NoError(err)

	s.repo = postgres.NewThreadDialogStore(s.pool)
}

func (s *ThreadDialogStoreTestSuite) TearDownSuite() {
	if s.pool != nil {
		s.pool.Close()
	}
	if s.container != nil {
		s.container.Terminate(context.Background())
	}
}

func TestThreadDialogStoreSuite(t *testing.T) {
	suite.Run(t, new(ThreadDialogStoreTestSuite))
}

func (s *ThreadDialogStoreTestSuite) createTestThread(ctx context.Context, domainID int) uuid.UUID {
	var id uuid.UUID
	query := `INSERT INTO im_thread.thread (domain_id, kind) VALUES ($1, 1) RETURNING id`
	err := s.pool.QueryRow(ctx, query, domainID).Scan(&id)
	s.Require().NoError(err)
	return id
}

func (s *ThreadDialogStoreTestSuite) TestCreateDirectPair_Success() {
	ctx := context.Background()

	threadID := s.createTestThread(ctx, 1)

	member1 := uuid.New()
	member2 := uuid.New()
	now := time.Now().UTC().Truncate(time.Microsecond)

	dialog := &model.ThreadDialog{
		BaseModel: model.BaseModel{
			DomainID:  1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		MemberID: member1,
		ThreadID: threadID,
		DirectTo: &member2,
	}

	result, err := s.repo.CreateDirectPair(ctx, dialog)

	s.NoError(err)
	s.Len(result, 2, "Should create two records for a direct pair")

	s.Equal(member1, result[0].MemberID)
	s.Equal(member2, *result[0].DirectTo)

	s.Equal(member2, result[1].MemberID)
	s.Equal(member1, *result[1].DirectTo)
}

func (s *ThreadDialogStoreTestSuite) TestResolve_Success() {
	ctx := context.Background()
	domainID := 1
	threadID := s.createTestThread(ctx, domainID)
	memberA := uuid.New()
	memberB := uuid.New()

	_, err := s.repo.CreateDirectPair(ctx, &model.ThreadDialog{
		BaseModel: model.BaseModel{DomainID: domainID},
		MemberID:  memberA,
		ThreadID:  threadID,
		DirectTo:  &memberB,
	})
	s.Require().NoError(err)

	search := &dto.SearchThreadDialogRequest{
		DomainID: domainID,
		Kind:     model.ThreadDirect,
		From:     &model.Peer{ID: memberA},
		To:       &model.Peer{ID: memberB},
	}

	foundID, err := s.repo.Resolve(ctx, search)

	s.NoError(err)
	s.Equal(threadID, foundID)
}

func (s *ThreadDialogStoreTestSuite) TestCreateDirectPair_UniqueViolation() {
	ctx := context.Background()
	threadID := s.createTestThread(ctx, 1)
	memberA := uuid.New()
	memberB := uuid.New()

	dialog := &model.ThreadDialog{
		BaseModel: model.BaseModel{DomainID: 1},
		MemberID:  memberA,
		ThreadID:  threadID,
		DirectTo:  &memberB,
	}

	result, err := s.repo.CreateDirectPair(ctx, dialog)
	fmt.Print(result)
	s.NoError(err)

	result, err = s.repo.CreateDirectPair(ctx, dialog)
	s.Error(err)
	s.Contains(err.Error(), "thread_dialog_member_direct_unique")
}

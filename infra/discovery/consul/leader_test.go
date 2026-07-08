package leader

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/webitel/im-thread-service/config"
)

func TestLeaderElector_Run_LeakCheck(t *testing.T) {
	defer goleak.VerifyNone(t)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	cfg := config.LeaderElectionConfig{
		TTL:           10 * time.Second,
		RenewInterval: 5 * time.Second,
		ErrCooldown:   5 * time.Second,
		BlockingWait:  2 * time.Minute,
		LockDelay:     time.Second,
	}

	le, err := NewLeaderElector("localhost:9999", "test-node", cfg, logger)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	go le.Run(ctx,
		func(ctx context.Context) error { return nil },
		func() {},
	)

	time.Sleep(100 * time.Millisecond)

	cancel()

	time.Sleep(200 * time.Millisecond)
}

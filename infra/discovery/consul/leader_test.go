package leader

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestLeaderElector_Run_LeakCheck(t *testing.T) {
	defer goleak.VerifyNone(t)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	le, err := NewLeaderElector("localhost:9999", "test-node", logger)
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

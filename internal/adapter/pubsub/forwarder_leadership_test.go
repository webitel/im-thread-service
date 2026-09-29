package pubsub

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/pubsub/gochannel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"

	"github.com/webitel/im-thread-service/internal/adapter/journal"
	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/model"
)

type safePublisher struct {
	mu     sync.Mutex
	topics []string
}

func (p *safePublisher) Publish(topic string, _ ...*message.Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.topics = append(p.topics, topic)

	return nil
}

func (p *safePublisher) Close() error { return nil }

func (p *safePublisher) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return len(p.topics)
}

// termElector grants leadership `terms` times in a row, feeding one outbox
// message per term and demoting once it is relayed.
type termElector struct {
	terms   int
	subs    chan *gochannel.GoChannel
	relayed func() int
	done    chan struct{}
}

func (e *termElector) Run(ctx context.Context, onStart func(context.Context) error, onStop func()) {
	defer close(e.done)

	for term := 1; term <= e.terms; term++ {
		termCtx, cancel := context.WithCancel(ctx)
		if err := onStart(termCtx); err != nil {
			cancel()

			return
		}

		sub := <-e.subs
		if err := sub.Publish(OutboxTopic, message.NewMessage(watermill.NewUUID(), []byte(`{}`))); err != nil {
			cancel()

			return
		}

		deadline := time.Now().Add(5 * time.Second)
		for e.relayed() < term && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}

		cancel()
		onStop()
	}
}

type noopOutbox struct{}

func (noopOutbox) Publish(context.Context, string, event.Outboxer) error { return nil }
func (noopOutbox) Cleanup(context.Context, *model.OutboxCleanupOptions) (int64, error) {
	return 0, nil
}

type noopDB struct{}

func (noopDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (noopDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("not used")
}
func (noopDB) QueryRow(context.Context, string, ...any) pgx.Row { return noopRow{} }

type noopRow struct{}

func (noopRow) Scan(...any) error { return errors.New("not used") }

// A node demoted and re-promoted in the same process must relay again: each
// term needs its own router and subscriber (a watermill router runs once).
func TestForwarder_RelaysAcrossLeadershipTerms(t *testing.T) {
	const terms = 2

	pub := &safePublisher{}
	subs := make(chan *gochannel.GoChannel, terms)

	var built atomic.Int32

	factory := func() (OutboxSubscriber, error) {
		built.Add(1)

		sub := gochannel.NewGoChannel(gochannel.Config{Persistent: true}, watermill.NopLogger{})
		subs <- sub

		return sub, nil
	}

	elector := &termElector{terms: terms, subs: subs, relayed: pub.count, done: make(chan struct{})}
	lc := fxtest.NewLifecycle(t)

	require.NoError(t, RegisterOutboxForwarder(lc, factory, journal.New(noopDB{}), pub,
		watermill.NopLogger{}, elector, noopOutbox{}, discardLog()))

	lc.RequireStart()

	select {
	case <-elector.done:
	case <-time.After(15 * time.Second):
		t.Fatal("leadership terms did not complete")
	}

	lc.RequireStop()

	require.Equal(t, int32(terms), built.Load(), "one subscriber per term")
	require.Equal(t, terms, pub.count(), "every term must relay its message")
}

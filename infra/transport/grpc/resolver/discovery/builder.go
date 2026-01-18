package discovery

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/webitel/webitel-go-kit/infra/discovery"
	"google.golang.org/grpc/resolver"
)

const name = "discovery"

var ErrWatcherCreateTimeout = errors.New("discovery create watcher overtime")

type (
	builder struct {
		discoverer discovery.Discovery
		timeout    time.Duration
		insecure   bool
		subsetSize int
		debugLog   bool
	}
	Option func(b *builder)
)

// WithTimeout with timeout option.
func WithTimeout(timeout time.Duration) Option {
	return func(b *builder) {
		b.timeout = timeout
	}
}

// WithInsecure with isSecure option.
func WithInsecure(insecure bool) Option {
	return func(b *builder) {
		b.insecure = insecure
	}
}

// WithSubset with subset size.
func WithSubset(size int) Option {
	return func(b *builder) {
		b.subsetSize = size
	}
}

// PrintDebugLog print grpc resolver watch service log
func PrintDebugLog(p bool) Option {
	return func(b *builder) {
		b.debugLog = p
	}
}

func NewBuilder(d discovery.Discovery, options ...Option) resolver.Builder {
	b := new(builder)
	{
		b.discoverer = d
		b.timeout = time.Second * 10
		b.debugLog = true
		b.insecure = false
		b.subsetSize = 25
	}

	for _, o := range options {
		o(b)
	}

	return b
}

// Build implements [resolver.Builder].
func (b *builder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	var (
		watchRes = &struct {
			err error
			w   discovery.Watcher
		}{}
		done        = make(chan struct{}, 1)
		ctx, cancel = context.WithCancel(context.Background())
		err         error
	)

	go func() {
		watchRes.w, watchRes.err = b.discoverer.GetWatcher(ctx, strings.TrimPrefix(target.URL.Path, "/"))
		close(done)
	}()

	if b.timeout > 0 {
		select {
		case <-done:
			err = watchRes.err
		case <-time.After(b.timeout):
			err = ErrWatcherCreateTimeout
		}
	} else {
		<-done
		err = watchRes.err
	}

	if err != nil {
		cancel()
		return nil, err
	}

	r := &discoveryResolver{
		w:           watchRes.w,
		cc:          cc,
		ctx:         ctx,
		cancel:      cancel,
		insecure:    b.insecure,
		debugLog:    b.debugLog,
		subsetSize:  b.subsetSize,
		selectorKey: uuid.New().String(),
	}

	go r.watch()

	return r, nil
}

// Scheme implements [resolver.Builder].
func (b *builder) Scheme() string {
	return name
}

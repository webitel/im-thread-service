package webitel

import (
	"context"
	"fmt"
	"log/slog"

	infratls "github.com/webitel/im-thread-service/infra/tls"
	"github.com/webitel/im-thread-service/infra/webitel/interceptors"
	ds "github.com/webitel/webitel-go-kit/infra/discovery"
	rpc "github.com/webitel/webitel-go-kit/infra/transport/gRPC"
	"github.com/webitel/webitel-go-kit/infra/transport/gRPC/resolver/discovery"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// New initializes a go-kit RPC client with embedded Circuit Breaker and Discovery
func New[T any](log *slog.Logger, dp ds.DiscoveryProvider, target string, tlsConf *infratls.Config, factory rpc.ClientFactory[T]) (*rpc.Client[T], error) {
	// [STABILITY] Create a method-aware circuit breaker for this specific connection
	cb := interceptors.NewBreakerInterceptor()

	tlsOpt := grpc.WithTransportCredentials(insecure.NewCredentials())
	if tlsConf != nil && tlsConf.Client != nil {
		tlsOpt = grpc.WithTransportCredentials(credentials.NewTLS(tlsConf.Client))
	}

	options := []grpc.DialOption{
		tlsOpt,
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithResolvers(discovery.NewBuilder(dp, discovery.WithInsecure(true))),
		// [INTERCEPTOR] Circuit breaker wraps all unary calls
		grpc.WithChainUnaryInterceptor(
			cb.UnaryClientInterceptor(),
		),
	}

	client, err := rpc.NewClient(
		context.Background(),
		factory,
		rpc.WithTarget(fmt.Sprintf("discovery:///%s", target)),
		rpc.WithDialOptions(options...),
		// [RETRY] Built-in transport-level retries
		rpc.WithRetry(rpc.DefaultRetryConfig()),
	)
	if err != nil {
		return nil, err
	}

	return client, nil
}

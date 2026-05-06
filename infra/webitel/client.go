package webitel

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	infratls "github.com/webitel/im-thread-service/infra/tls"
	ds "github.com/webitel/webitel-go-kit/infra/discovery"
	rpc "github.com/webitel/webitel-go-kit/infra/transport/gRPC"
	"github.com/webitel/webitel-go-kit/infra/transport/gRPC/resolver/discovery"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

func New[T any](log *slog.Logger, dp ds.DiscoveryProvider, target string, tlsConf *infratls.Config, factory rpc.ClientFactory[T]) (*rpc.Client[T], error) {
	tlsOpt := grpc.WithTransportCredentials(insecure.NewCredentials())
	if tlsConf != nil && tlsConf.Client != nil {
		tlsOpt = grpc.WithTransportCredentials(credentials.NewTLS(tlsConf.Client))
	}

	options := []grpc.DialOption{
		tlsOpt,
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithResolvers(discovery.NewBuilder(dp, discovery.WithInsecure(true))),
		grpc.WithChainUnaryInterceptor(),
	}

	client, err := rpc.NewClient(
		context.Background(),
		factory,
		rpc.WithTarget(fmt.Sprintf("discovery:///%s", target)),
		rpc.WithDialOptions(options...),
		rpc.WithRetry(rpc.DefaultRetryConfig()),
		rpc.WithKeepalive(
			keepalive.ClientParameters{
				Time:                10 * time.Minute,
				Timeout:             20 * time.Second,
				PermitWithoutStream: false,
			},
		),
	)
	if err != nil {
		return nil, err
	}

	return client, nil
}

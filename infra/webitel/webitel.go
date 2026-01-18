package webitel

import (
	"log/slog"

	"github.com/webitel/im-thread-service/infra/transport/grpc/resolver/discovery"
	ds "github.com/webitel/webitel-go-kit/infra/discovery"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	// see https://github.com/grpc/grpc/blob/master/doc/service_config.md to know more about service config
	retryPolicy string = ` {
		"loadBalancingConfig": [ { "round_robin": {} } ],
		"methodConfig": [
			{
				"timeout": "5.000000001s",
				"waitForReady": true,
				"retryPolicy": {
					"MaxAttempts": 4,
					"InitialBackoff": ".01s",
					"MaxBackoff": ".01s",
					"BackoffMultiplier": 1.0,
    				"RetryableStatusCodes": [ "UNAVAILABLE" ]
				}
			}
		]
	}`
)

func New(log *slog.Logger, dp ds.DiscoveryProvider, target string) (*grpc.ClientConn, error) {
	log.Info("connecting to service", slog.String("target", target))

	options := []grpc.DialOption{
		grpc.WithResolvers(discovery.NewBuilder(dp, discovery.WithInsecure(true))),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(retryPolicy),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	}

	client, err := grpc.NewClient("discovery:///"+target, options...)
	if err != nil {
		return nil, err
	}

	return client, nil
}

package webitel

import (
	"log/slog"

	"github.com/webitel/webitel-go-kit/infra/discovery"
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

func New(log *slog.Logger, discovery discovery.DiscoveryProvider, target string) (*grpc.ClientConn, error) {
	log.Info("connecting to service", slog.String("target", target))

	options := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(retryPolicy),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	}

	var remoteAddr string
	switch target {
	case "storage":
		remoteAddr = "172.22.22.22:10039"
	case "contacts":
		remoteAddr = "127.0.0.1:21501"
	default:
		remoteAddr = "consul:///" + target
	}

	log.Debug("resolved service address", slog.String("addr", remoteAddr))

	client, err := grpc.NewClient(remoteAddr, options...)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// func New(log *slog.Logger, discovery discovery.DiscoveryProvider, target string) (*grpc.ClientConn, error) {
// 	log.Info("connecting to service", slog.String("target", target))
// 	options := []grpc.DialOption{
// 		grpc.WithTransportCredentials(insecure.NewCredentials()),
// 		grpc.WithDefaultServiceConfig(retryPolicy),
// 		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
// 	}

// 	// client, err := grpc.NewClient("consul:///"+target, options...)
// 	client, err := grpc.NewClient("172.20.10.2:21501", options...)
// 	if err != nil {
// 		return nil, err
// 	}

// 	return client, nil
// }

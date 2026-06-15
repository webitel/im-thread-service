package server

import (
	"context"
	"log/slog"
	"net/url"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.38.0"
	"go.uber.org/fx"

	"github.com/webitel/webitel-go-kit/infra/discovery"
	otelsdk "github.com/webitel/webitel-go-kit/infra/otel/sdk"
	"github.com/webitel/webitel-go-kit/infra/profiler"
	"github.com/webitel/webitel-go-kit/pkg/depenlog"
	"github.com/webitel/webitel-go-kit/pkg/logger"

	"github.com/webitel/im-thread-service/config"
	"github.com/webitel/im-thread-service/internal/domain/model"

	_ "github.com/webitel/webitel-go-kit/infra/discovery/consul" // register consul discovery driver
	_ "github.com/webitel/webitel-go-kit/infra/otel/sdk/log/otlp"
	_ "github.com/webitel/webitel-go-kit/infra/otel/sdk/log/stdout"
	_ "github.com/webitel/webitel-go-kit/infra/otel/sdk/metric/otlp"
	_ "github.com/webitel/webitel-go-kit/infra/otel/sdk/metric/stdout"
	_ "github.com/webitel/webitel-go-kit/infra/otel/sdk/trace/otlp"
	_ "github.com/webitel/webitel-go-kit/infra/otel/sdk/trace/stdout"
)

func ProvideWatermillLogger(l *slog.Logger) watermill.LoggerAdapter {
	return watermill.NewSlogLogger(l)
}

// ProvideLogger builds the process-wide logger on top of depenlog. It returns
// both the kit logger.Logger (consumed by the fx/profiler adapters) and the
// *slog.Logger (consumed by the many components that inject slog directly).
// depenlog.New installs the logger as slog's default and wires grpc-go's global
// logger, so slog.Default() returns the same configured logger after New.
func ProvideLogger(cfg *config.Config, lc fx.Lifecycle) (logger.Logger, *slog.Logger, error) {
	logSettings := cfg.Log

	dcfg := depenlog.Config{
		Level:   logSettings.Level,
		JSON:    logSettings.JSON,
		File:    logSettings.File,
		Console: logSettings.Console,
	}

	// Default to console when no sink is selected, matching prior behaviour.
	if !logSettings.Console && !logSettings.Otel && logSettings.File == "" {
		dcfg.Console = true
	}

	var opts []depenlog.Option

	if logSettings.Otel {
		service := resource.NewSchemaless(
			semconv.ServiceName(model.ServiceName),
			semconv.ServiceVersion(model.Version),
			semconv.ServiceInstanceID(discovery.GenerateInstanceID(model.ServiceName)),
			semconv.ServiceNamespace(model.ServiceNamespace),
		)
		otelHandler := otelslog.NewHandler("slog")

		// WithLogBridge's callback runs synchronously inside Configure, and only
		// when an OTel log exporter is actually active.
		bridged := false
		shutdown, err := otelsdk.Configure(context.Background(), otelsdk.WithResource(service),
			otelsdk.WithLogBridge(
				func() {
					bridged = true
				},
			),
		)
		if err != nil {
			return nil, nil, err
		}

		lc.Append(fx.Hook{
			OnStop: func(ctx context.Context) error {
				return shutdown(ctx)
			},
		})

		// When the bridge is active, let the OTel LoggerProvider own the handler
		// so it controls schema and trace correlation. Console/file are bypassed
		// in this mode. Otherwise fall back to the Config-built handler.
		if bridged {
			opts = append(opts, depenlog.WithHandler(otelHandler))
		}
	}

	l, err := depenlog.New(dcfg, opts...)
	if err != nil {
		return nil, nil, err
	}

	return l, slog.Default(), nil
}

func ProvideSD(cfg *config.Config, log *slog.Logger, lc fx.Lifecycle) (discovery.DiscoveryProvider, error) {
	provider, err := discovery.DefaultFactory.CreateProvider(
		discovery.ProviderConsul,
		log,
		cfg.Consul.Addr,
		discovery.WithHeartbeat[discovery.DiscoveryProvider](true),
		discovery.WithTimeout[discovery.DiscoveryProvider](time.Second*30),
	)
	if err != nil {
		return nil, err
	}

	si := new(discovery.ServiceInstance)
	{
		si.Id = discovery.GenerateInstanceID(model.ServiceName)
		si.Name = model.ServiceName
		si.Version = model.Version
		si.Metadata = map[string]string{
			"commit":         model.Commit,
			"commitDate":     model.CommitDate,
			"branch":         model.Branch,
			"buildTimestamp": model.BuildTimestamp,
		}
		si.Endpoints = []string{(&url.URL{Scheme: "grpc", Host: cfg.Service.Addr}).String()}
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := provider.Register(ctx, si); err != nil {
				return err
			}

			return nil
		},
		OnStop: func(ctx context.Context) error {
			if err := provider.Deregister(ctx, si); err != nil {
				return err
			}

			return nil
		},
	})

	return provider, nil
}

// ProvideProfiler provides only the profiler config; the profiler module
// consumes the logger.Logger provided by ProvideLogger.
func ProvideProfiler(cfg *config.Config) profiler.Config {
	return profiler.Config{
		Addr:                 cfg.Profiler.Addr,
		MutexProfileFraction: cfg.Profiler.MutexFraction,
		BlockProfileRate:     cfg.Profiler.BlockRate,
	}
}

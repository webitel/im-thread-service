module github.com/webitel/im-thread-service

go 1.26

require (
	buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go v1.36.11-20260415201107-50325440f8f2.1
	buf.build/go/protovalidate v1.2.0
	github.com/Masterminds/squirrel v1.5.4
	github.com/ThreeDotsLabs/watermill v1.5.2
	github.com/ThreeDotsLabs/watermill-amqp/v3 v3.1.0
	github.com/ThreeDotsLabs/watermill-sql/v4 v4.1.5
	github.com/fsnotify/fsnotify v1.10.1
	github.com/georgysavva/scany/v2 v2.1.4
	github.com/google/uuid v1.6.0
	github.com/grpc-ecosystem/go-grpc-middleware/v2 v2.3.3
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.29.0
	github.com/huandu/go-sqlbuilder v1.42.1
	github.com/jackc/pgx/v5 v5.10.0
	github.com/sony/gobreaker v1.0.0
	github.com/spf13/pflag v1.0.10
	github.com/spf13/viper v1.21.0 // indirect
	github.com/stretchr/testify v1.11.1
	github.com/urfave/cli/v2 v2.27.7
	github.com/webitel/storage v0.0.0-20260625115836-a6cac81e0a4c
	github.com/webitel/webitel-go-kit/infra/discovery v0.0.0-20260623172444-1a81b7322f37
	github.com/webitel/webitel-go-kit/infra/otel v0.1.0
	github.com/webitel/webitel-go-kit/infra/profiler v0.1.0
	github.com/webitel/webitel-go-kit/infra/transport v0.0.0-20260623172444-1a81b7322f37
	github.com/webitel/webitel-go-kit/pkg/errors v0.1.1-0.20260424081817-15dec057ee90
	github.com/webitel/webitel-go-kit/pkg/interceptors v0.1.2-0.20260424081817-15dec057ee90
	github.com/webitel/webitel-go-kit/pkg/logger v0.1.1
	go.opentelemetry.io/contrib/bridges/otelslog v0.19.0
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.69.0
	go.opentelemetry.io/otel v1.44.0
	go.opentelemetry.io/otel/sdk v1.44.0
	go.uber.org/fx v1.24.0
	go.uber.org/goleak v1.3.0
	golang.org/x/sync v0.21.0
	google.golang.org/genproto/googleapis/api v0.0.0-20260622175928-b703f567277d
	google.golang.org/grpc v1.81.1
)

require (
	cel.dev/expr v0.25.2 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dgraph-io/ristretto/v2 v2.2.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/cel-go v0.28.1 // indirect
	github.com/hashicorp/go-metrics v0.6.0 // indirect
	github.com/huandu/go-clone v1.7.3 // indirect
	github.com/huandu/xstrings v1.5.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/lann/builder v0.0.0-20180802200727-47ae307949d0 // indirect
	github.com/lann/ps v0.0.0-20150810152359-62de8c46ede0 // indirect
	github.com/lib/pq v1.12.3 // indirect
	github.com/mfridman/interpolate v0.0.2 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/sethvargo/go-retry v0.3.0 // indirect
	github.com/webitel/wlog v0.0.0-20250325101442-de4f125c1ec7 // indirect
	go.opentelemetry.io/contrib/bridges/otelzap v0.19.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc v0.20.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp v0.20.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.44.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v1.44.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.44.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.44.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.44.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdoutmetric v1.44.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.44.0 // indirect
	go.opentelemetry.io/proto/otlp v1.10.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

require (
	github.com/armon/go-metrics v0.4.1 // indirect
	github.com/cenkalti/backoff/v3 v3.2.2 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.7 // indirect
	github.com/fatih/color v1.19.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/hashicorp/consul/api v1.34.3
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-hclog v1.6.3 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/golang-lru v1.0.2 // indirect
	github.com/hashicorp/serf v0.10.2 // indirect
	github.com/lithammer/shortuuid/v3 v3.0.7 // indirect
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.22 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/pelletier/go-toml/v2 v2.4.2 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pressly/goose/v3 v3.27.1
	github.com/rabbitmq/amqp091-go v1.12.0
	github.com/rivo/uniseg v0.4.7
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sagikazarmark/locafero v0.12.0 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/xrash/smetrics v0.0.0-20250705151800-55b8f293f342 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel/log v0.20.0 // indirect
	go.opentelemetry.io/otel/metric v1.44.0 // indirect
	go.opentelemetry.io/otel/sdk/log v0.20.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.44.0 // indirect
	go.opentelemetry.io/otel/trace v1.44.0 // indirect
	go.uber.org/dig v1.19.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.28.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/exp v0.0.0-20260611194520-c48552f49976 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	golang.org/x/text v0.38.0
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260622175928-b703f567277d // indirect
	google.golang.org/protobuf v1.36.11
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
)

require (
	github.com/forPelevin/gomoji v1.4.1
	github.com/redis/go-redis/v9 v9.18.0
	github.com/webitel/webitel-go-kit/appconfig v0.0.0-20260623172444-1a81b7322f37
	github.com/webitel/webitel-go-kit/pkg/cache v0.0.0-20260706110827-c434ee012806
)

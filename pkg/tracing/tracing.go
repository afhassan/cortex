package tracing

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	"github.com/sercand/kuberesolver/v5"
	"github.com/weaveworks/common/tracing"
	"go.opentelemetry.io/contrib/propagators/aws/xray"
	"google.golang.org/grpc/credentials"

	"github.com/opentracing/opentracing-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"

	"github.com/cortexproject/cortex/pkg/tracing/migration"
	util_log "github.com/cortexproject/cortex/pkg/util/log"
	"github.com/cortexproject/cortex/pkg/util/tls"
)

const (
	JaegerType = "jaeger"
	OtelType   = "otel"
)

type Config struct {
	Type string `yaml:"type" json:"type"`
	Otel Otel   `yaml:"otel" json:"otel"`
}

type Otel struct {
	OtlpEndpoint   string              `yaml:"otlp_endpoint" json:"otlp_endpoint"`
	ExporterType   string              `yaml:"exporter_type" json:"exporter_type"`
	SampleRatio    float64             `yaml:"sample_ratio" json:"sample_ratio"`
	RoundRobin     bool                `yaml:"round_robin" json:"round_robin"`
	TLSEnabled     bool                `yaml:"tls_enabled"`
	TLS            tls.ClientConfig    `yaml:"tls"`
	ExtraDetectors []resource.Detector `yaml:"-"`
}

// RegisterFlags registers flag.
func (c *Config) RegisterFlags(f *flag.FlagSet) {
	p := "tracing"
	f.StringVar(&c.Type, p+".type", JaegerType, "Tracing type. OTEL and JAEGER are currently supported. For jaeger `JAEGER_AGENT_HOST` environment variable should also be set. See: https://cortexmetrics.io/docs/guides/tracing .")
	f.Float64Var(&c.Otel.SampleRatio, p+".otel.sample-ratio", 0.001, "Fraction of traces to be sampled. Fractions >= 1 means sampling if off and everything is traced.")
	f.StringVar(&c.Otel.OtlpEndpoint, p+".otel.otlp-endpoint", "", "otl collector endpoint that the driver will use to send spans.")
	f.StringVar(&c.Otel.ExporterType, p+".otel.exporter-type", "", "enhance/modify traces/propagators for specific exporter. If empty, OTEL defaults will apply. Supported values are: `awsxray.`")
	f.BoolVar(&c.Otel.TLSEnabled, p+".otel.tls-enabled", c.Otel.TLSEnabled, "Enable TLS in the GRPC client. This flag needs to be enabled when any other TLS flag is set. If set to false, insecure connection to gRPC server will be used.")
	f.BoolVar(&c.Otel.RoundRobin, p+".otel.round-robin", false, "If enabled, use round_robin gRPC load balancing policy. By default, use pick_first policy. For more details, please refer to https://github.com/grpc/grpc/blob/master/doc/load-balancing.md#load-balancing-policies.")
	c.Otel.TLS.RegisterFlagsWithPrefix(p+".otel.tls", f)
}

func (c *Config) Validate() error {
	switch strings.ToLower(c.Type) {
	case OtelType:
		if c.Otel.OtlpEndpoint == "" {
			return errors.New("otlp-endpoint must be defined when using otel exporter")
		}
	}

	return nil
}

func SetupTracing(ctx context.Context, name string, c Config) (func(context.Context) error, error) {
	switch strings.ToLower(c.Type) {
	case JaegerType:
		// Setting the environment variable JAEGER_AGENT_HOST enables tracing.
		if trace, err := tracing.NewFromEnv(name); err != nil {
			level.Error(util_log.Logger).Log("msg", "Failed to setup tracing", "err", err.Error())
		} else {
			return func(ctx context.Context) error {
				trace.Close()
				return nil
			}, nil
		}
	case OtelType:
		util_log.Logger.Log("msg", "creating otel exporter")

		endpoint := c.Otel.OtlpEndpoint
		options := []otlptracegrpc.Option{
			otlptracegrpc.WithEndpoint(endpoint),
		}
		// Following https://github.com/sercand/kuberesolver/blob/master/builder.go#L96.
		if strings.HasPrefix(endpoint, "kubernetes://") {
			// Registers the kuberesolver which resolves endpoint with prefix kubernetes://
			// as kubernetes service endpoint addresses.
			kuberesolver.RegisterInCluster()
		}

		if c.Otel.RoundRobin {
			options = append(options, otlptracegrpc.WithServiceConfig(`{"loadBalancingPolicy": "round_robin"}`))
		}

		if c.Otel.TLSEnabled {
			tlsConfig, err := c.Otel.TLS.GetTLSConfig()
			if err != nil {
				return nil, errors.Wrap(err, "error creating grpc dial options")
			}
			options = append(options, otlptracegrpc.WithTLSCredentials(credentials.NewTLS(tlsConfig)))
		} else {
			options = append(options, otlptracegrpc.WithInsecure())
		}

		exporter, err := otlptracegrpc.New(ctx, options...)
		if err != nil {
			return nil, fmt.Errorf("creating OTLP trace exporter: %w", err)
		}

		r, err := newResource(ctx, name, c.Otel.ExtraDetectors)
		if err != nil {
			return nil, fmt.Errorf("creating tracing resource: %w", err)
		}

		propagator, tracerProvider := newTraceProvider(r, c, exporter)

		bridge, wrappedProvider := migration.NewCortexBridgeTracerWrapper(tracerProvider.Tracer("github.com/cortexproject/cortex/cmd/cortex"))
		bridge.SetTextMapPropagator(propagator)
		opentracing.SetGlobalTracer(bridge)
		otel.SetTracerProvider(wrappedProvider)

		return tracerProvider.Shutdown, nil
	}

	return func(ctx context.Context) error {
		return nil
	}, nil
}

func newTraceProvider(r *resource.Resource, c Config, exporter *otlptrace.Exporter) (propagation.TextMapPropagator, *sdktrace.TracerProvider) {
	options := []sdktrace.TracerProviderOption{
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(r),
	}
	var propagator propagation.TextMapPropagator = propagation.TraceContext{}
	switch strings.ToLower(c.Otel.ExporterType) {
	case "awsxray":
		options = append(options, sdktrace.WithIDGenerator(xray.NewIDGenerator()))
		propagator = xray.Propagator{}
	default:
	}

	options = append(options, sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(c.Otel.SampleRatio))))

	return propagator, sdktrace.NewTracerProvider(options...)
}

func newResource(ctx context.Context, target string, detectors []resource.Detector) (*resource.Resource, error) {
	opts := []resource.Option{
		resource.WithHost(),
		resource.WithDetectors(detectors...),
		resource.WithAttributes(semconv.ServiceNameKey.String(target)),
	}
	return resource.New(ctx, opts...)
}

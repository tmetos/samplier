// Package telemetry wires up structured logging, OpenTelemetry tracing, and
// metrics for the Samplier API.
//
// Logs are structured JSON on stdout and are the source of truth; when a request
// is traced, each log line carries trace_id and span_id for correlation.
// Traces and metrics are exported over OTLP/gRPC when OTEL_EXPORTER_OTLP_ENDPOINT
// is set; otherwise the global no-op providers are used.
package telemetry

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const serviceName = "samplier-api"

// Init configures the OTel SDK. It returns a shutdown function that flushes and
// closes exporters. With no OTEL_EXPORTER_OTLP_ENDPOINT set, providers stay no-op
// and the API still runs normally.
func Init(ctx context.Context) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))

	noop := func(context.Context) error { return nil }
	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if endpoint == "" {
		return noop, nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(attribute.String("service.name", serviceName)),
	)
	if err != nil {
		return nil, err
	}

	traceExp, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
		// 10% head sampling; child spans follow their parent's decision.
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(0.1))),
	)
	otel.SetTracerProvider(tp)

	metricExp, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(endpoint),
		otlpmetricgrpc.WithInsecure(),
	)
	if err != nil {
		_ = traceExp.Shutdown(ctx)
		return nil, err
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp,
			sdkmetric.WithInterval(30*time.Second))),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	return func(c context.Context) error {
		_ = tp.Shutdown(c)
		_ = mp.Shutdown(c)
		return nil
	}, nil
}

// NewLogger returns the structured JSON logger. When a log is emitted inside a
// traced request, trace_id and span_id are added automatically.
func NewLogger() *slog.Logger {
	level := slog.LevelInfo
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		_ = level.UnmarshalText([]byte(v))
	}
	base := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
	})
	return slog.New(&otelLogHandler{next: base})
}

// otelLogHandler injects trace_id/span_id from the OTel span context.
type otelLogHandler struct {
	next slog.Handler
}

func (h *otelLogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *otelLogHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.next.Handle(ctx, r)
}

func (h *otelLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &otelLogHandler{next: h.next.WithAttrs(attrs)}
}

func (h *otelLogHandler) WithGroup(name string) slog.Handler {
	return &otelLogHandler{next: h.next.WithGroup(name)}
}

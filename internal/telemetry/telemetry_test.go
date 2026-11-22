package telemetry

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

func TestInitWithoutEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	shutdown, err := Init(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("expected shutdown function")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestNewLoggerJSON(t *testing.T) {
	t.Setenv("LOG_LEVEL", "debug")
	logger := NewLogger()
	if logger == nil {
		t.Fatal("expected logger")
	}
	logger.Info("test message", "key", "value")
}

func TestLogLevelParsing(t *testing.T) {
	t.Setenv("LOG_LEVEL", "warn")
	logger := NewLogger()
	if logger == nil {
		t.Fatal("expected logger")
	}
	// Warn-level logging must be enabled, and debug disabled.
	if logger.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("debug should be disabled at warn level")
	}
	if !logger.Enabled(context.Background(), slog.LevelWarn) {
		t.Error("warn should be enabled")
	}
}

func TestOtelHandlerInjectTraceIDs(t *testing.T) {
	// Build a span context with valid trace/span ids.
	var tid trace.TraceID
	var sid trace.SpanID
	for i := range tid {
		tid[i] = byte(i + 1)
	}
	for i := range sid {
		sid[i] = byte(i + 11)
	}
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: tid, SpanID: sid, TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)

	// Capture output by swapping the underlying handler's writer via a temp file.
	f, err := os.CreateTemp(t.TempDir(), "log")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer f.Close()

	h := &otelLogHandler{next: slog.NewJSONHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug})}
	logger := slog.New(h)

	// Force the handler to emit and inspect the record.
	r := slog.NewRecord(nowForTest(), slog.LevelInfo, "hello", 0)
	if err := h.Handle(ctx, r); err != nil {
		t.Fatalf("handle: %v", err)
	}
	_ = logger

	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var entry map[string]any
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("parse log line %q: %v", data, err)
	}
	if entry["trace_id"] != tid.String() {
		t.Errorf("trace_id = %v, want %s", entry["trace_id"], tid.String())
	}
	if entry["span_id"] != sid.String() {
		t.Errorf("span_id = %v, want %s", entry["span_id"], sid.String())
	}
}

func TestOtelHandlerWithoutSpan(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "log")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer f.Close()

	h := &otelLogHandler{next: slog.NewJSONHandler(f, nil)}
	r := slog.NewRecord(nowForTest(), slog.LevelInfo, "no span", 0)
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("handle: %v", err)
	}

	data, _ := os.ReadFile(f.Name())
	var entry map[string]any
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, ok := entry["trace_id"]; ok {
		t.Error("trace_id must be absent without a span")
	}
}

func TestOtelHandlerWithAttrsAndGroups(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "log")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer f.Close()

	h := &otelLogHandler{next: slog.NewJSONHandler(f, nil)}
	h2 := h.WithAttrs([]slog.Attr{slog.String("a", "b")})
	h3 := h2.WithGroup("grp")

	logger := slog.New(h3)
	logger.Info("with group", "x", 1)

	data, _ := os.ReadFile(f.Name())
	if len(data) == 0 {
		t.Fatal("expected log output")
	}
	var entry map[string]any
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("parse: %v", err)
	}
	grp, ok := entry["grp"].(map[string]any)
	if !ok {
		t.Fatalf("expected grouped keys: %v", entry)
	}
	if grp["x"] != float64(1) {
		t.Errorf("grouped value = %v", grp["x"])
	}
	if entry["a"] != "b" {
		t.Errorf("attribute = %v", entry["a"])
	}
}

// nowForTest returns a fixed timestamp for deterministic log records.
func nowForTest() time.Time {
	return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
}

func TestInitWithEndpoint(t *testing.T) {
	// gRPC exporters dial lazily, so a bogus endpoint still initialises.
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "127.0.0.1:4317")
	shutdown, err := Init(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("expected shutdown function")
	}
	// Tracer provider should now be a real SDK provider.
	tp := otel.GetTracerProvider()
	if tp == nil {
		t.Fatal("expected tracer provider")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

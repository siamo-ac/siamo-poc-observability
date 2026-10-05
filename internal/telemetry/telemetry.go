// Package telemetry wires OpenTelemetry tracing (stdout exporter) and a
// structured JSON logger whose records carry trace_id / span_id so logs
// correlate with traces.
package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// Init installs a tracer provider that prints finished spans to stdout and
// returns a shutdown function. It uses the synchronous exporter (no batch
// delay) so spans appear the moment a request finishes — handy for a demo.
//
// NOTE: otel-go ships with a NO-OP global propagator, so Init explicitly
// installs W3C TraceContext (+Baggage). Without this, trace context would
// never cross the HTTP boundary and each service would start its own trace.
func Init(serviceName string) (func(context.Context), error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	exp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		return nil, fmt.Errorf("stdout trace exporter: %w", err)
	}
	res, err := resource.New(context.Background(),
		resource.WithAttributes(semconv.ServiceNameKey.String(serviceName)),
	)
	if err != nil {
		return nil, fmt.Errorf("resource: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	return func(ctx context.Context) {
		_ = tp.Shutdown(ctx)
	}, nil
}

// Logger writes one JSON object per line. Every record carries the trace_id
// and span_id pulled from the request context, which is what lets a log line
// be matched to a span in a trace.
type Logger struct {
	service string
	out     *json.Encoder
}

func NewLogger(service string) *Logger {
	return &Logger{service: service, out: json.NewEncoder(os.Stdout)}
}

// Log emits a single JSON line. fields is free-form extra context.
func (l *Logger) Log(ctx context.Context, level, msg string, fields map[string]any) {
	rec := map[string]any{
		"ts":      time.Now().UTC().Format(time.RFC3339Nano),
		"level":   level,
		"service": l.service,
		"msg":     msg,
	}
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		rec["trace_id"] = sc.TraceID().String()
		rec["span_id"] = sc.SpanID().String()
	}
	for k, v := range fields {
		rec[k] = v
	}
	_ = l.out.Encode(rec)
}

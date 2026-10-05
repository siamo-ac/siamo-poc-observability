// Backend service: simulates order processing. Exposes POST /process with
// child spans under the incoming trace context, structured JSON logs, and
// Prometheus-format /metrics.
package main

import (
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"

	"github.com/siamosystems/siamo-poc-observability/internal/metrics"
	"github.com/siamosystems/siamo-poc-observability/internal/telemetry"
)

const service = "siamo-orders-backend"

var (
	tracer = otel.Tracer("backend")
	reg    = metrics.NewRegistry(service)
)

func main() {
	shutdown, err := telemetry.Init(service)
	if err != nil {
		panic(err)
	}
	log := telemetry.NewLogger(service)

	mux := http.NewServeMux()
	mux.Handle("/process", otelhttp.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		start := time.Now()
		status := "200"

		// Child span: a sub-step of processing, nested under /process.
		_, child := tracer.Start(ctx, "validateOrder")
		time.Sleep(time.Duration(20+rand.Intn(40)) * time.Millisecond)
		child.AddEvent("order.validated")
		child.End()

		// Main simulated work.
		time.Sleep(time.Duration(40+rand.Intn(60)) * time.Millisecond)
		log.Log(ctx, "info", "order processed", map[string]any{
			"work_ms": time.Since(start).Milliseconds(),
		})

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":        true,
			"processed": true,
			"service":   service,
		})
		reg.Observe(r.Method, "/process", status, time.Since(start).Seconds())
	}), "process"))

	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte(reg.Handler()))
		reg.Observe(r.Method, "/metrics", "200", time.Since(start).Seconds())
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok\n"))
	})

	srv := &http.Server{Addr: "127.0.0.1:8082", Handler: mux}
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		<-c
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		shutdown(shutdownCtx)
	}()
	log.Log(context.Background(), "info", "backend listening", map[string]any{"addr": srv.Addr})
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		panic(err)
	}
}

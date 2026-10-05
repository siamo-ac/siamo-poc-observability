// Frontend service: receives POST /order, starts the trace, and calls the
// backend over HTTP with trace context propagation (otelhttp transport).
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"

	"github.com/siamosystems/siamo-poc-observability/internal/metrics"
	"github.com/siamosystems/siamo-poc-observability/internal/telemetry"
)

const (
	service     = "siamo-orders-frontend"
	backendAddr = "http://127.0.0.1:8082"
)

var (
	tracer = otel.Tracer("frontend")
	reg    = metrics.NewRegistry(service)
	// Client whose Transport injects the current trace context into outgoing
	// requests — this is what connects the two services into one trace.
	client = &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}
)

func main() {
	shutdown, err := telemetry.Init(service)
	if err != nil {
		panic(err)
	}
	log := telemetry.NewLogger(service)

	mux := http.NewServeMux()
	mux.Handle("/order", otelhttp.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		start := time.Now()
		status := "200"

		// Child span: fake auth check, nested under /order.
		_, auth := tracer.Start(ctx, "checkAuth")
		time.Sleep(10 * time.Millisecond)
		auth.End()

		log.Log(ctx, "info", "calling backend", map[string]any{"backend": backendAddr + "/process"})
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, backendAddr+"/process", nil)
		resp, err := client.Do(req)
		if err != nil {
			status = "502"
			reg.Observe(r.Method, "/order", status, time.Since(start).Seconds())
			log.Log(ctx, "error", "backend call failed", map[string]any{"error": err.Error()})
			http.Error(w, "backend unavailable", http.StatusBadGateway)
			return
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"ok":true,"frontend":%q,"backend_response":%s}`, service, body)
		reg.Observe(r.Method, "/order", status, time.Since(start).Seconds())
		log.Log(ctx, "info", "order served", map[string]any{
			"backend_status": resp.StatusCode,
			"latency_ms":     time.Since(start).Milliseconds(),
		})
	}), "order"))

	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte(reg.Handler()))
		reg.Observe(r.Method, "/metrics", "200", time.Since(start).Seconds())
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok\n"))
	})

	srv := &http.Server{Addr: "127.0.0.1:8081", Handler: mux}
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		<-c
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		shutdown(shutdownCtx)
	}()
	log.Log(context.Background(), "info", "frontend listening", map[string]any{"addr": srv.Addr})
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		panic(err)
	}
}

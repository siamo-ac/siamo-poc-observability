# siamo-poc-observability

**Siamo Workflow Atlas — backend-architecture POC #7: Observability.**

## The concept, in plain language

When software runs as several small services, three questions matter:

1. **Logs** — *"What happened?"* A running diary of events. Here every line is
   one JSON object, so machines (and people) can search and filter it.
2. **Metrics** — *"How much / how fast?"* Numbers over time: request counts,
   error rates, latencies. Exposed here in the Prometheus format that most
   monitoring tools scrape.
3. **Traces** — *"Where did one request go?"* A request often crosses service
   boundaries. Tracing gives the whole journey one ID and records a **span**
   (a timed step) per service, so you can see the frontend calling the backend
   as one connected timeline.

The glue: the **trace ID travels with the request** (in an HTTP header) and is
also stamped into every log line, so logs, metrics, and traces all refer to the
same event.

## What it is

Two tiny Go services:

- `frontend` (127.0.0.1:**8081**) — `POST /order` starts a trace, makes a
  sub-span (`checkAuth`), then calls the backend.
- `backend` (127.0.0.1:**8082**) — `POST /process` extracts the trace context
  from the incoming request (becoming a **child span** of the frontend's) and
  adds its own sub-span (`validateOrder`).

Shared internals (hexagonal-light layout):

- `internal/telemetry` — OpenTelemetry SDK with the **stdout exporter**
  (spans print to the terminal) + structured JSON logger with `trace_id` /
  `span_id` correlation.
- `internal/metrics` — hand-rolled Prometheus exposition endpoint:
  `http_requests_total` (counter) and `http_request_duration_seconds`
  (histogram) on `/metrics`.

## Run it

```bash
# from this directory
./run.sh
```

`run.sh` builds both services, starts the backend then the frontend, and fires
one `POST /order`. It then prints what to look at.

Or by hand:

```bash
go build -o bin/backend ./cmd/backend
go build -o bin/frontend ./cmd/frontend
./bin/backend &          # :8082
./bin/frontend &         # :8081
curl -s -X POST http://127.0.0.1:8081/order | head -c 300; echo
curl -s http://127.0.0.1:8081/metrics
curl -s http://127.0.0.1:8082/metrics
kill %1 %2
```

Requires Go ≥ 1.24 (module deps download on first build).

## What to observe

1. **One trace, both services.** In the terminal you will see spans print as
   they finish. A single `TraceID` (e.g. `"TraceID": "4f1c…"`) appears on
   the frontend's `order` span, the backend's `process` span, and the
   `validateOrder`/`checkAuth` sub-spans — one request, one connected timeline
   across the HTTP boundary.
2. **Log correlation.** Every JSON log line carries the same `trace_id`, e.g.
   `"msg":"calling backend","trace_id":"4f1c…"`. Filter logs by that ID and you
   get the request's story in plain words.
3. **Metrics.** `/metrics` on either port shows
   `http_requests_total{service=…,method=…,route=…,status=…}` incrementing and
   `http_request_duration_seconds_bucket{…}` histogram rows. It is exactly the
   text format Prometheus scrapes.
4. **Centralized logging (ELK).** The JSON lines are already in the shape an
   ELK pipeline wants — see [docs/elk-pipeline.md](docs/elk-pipeline.md) for
   the Filebeat → Elasticsearch → Kibana path, a ready-to-adapt Filebeat
   config, and the KQL query that finds one request's logs by `trace_id`.

Sample trace (abridged, real output looks like this):

```json
{"Name":"validateOrder","TraceID":"9b3e2a…","Parent":{"SpanID":"c41d…"}, ...}
{"Name":"process","TraceID":"9b3e2a…", ...}
{"Name":"order","TraceID":"9b3e2a…", ...}
```

## Honest limits

- **Stdout exporter only** — spans print to the terminal; there is no
  collector, no Jaeger/Tempo, no retention. That is deliberate for a POC.
- **Hand-rolled metrics** — counters/histogram are in-memory, reset on restart,
  and only cover the demo routes. Not a metrics library.
- **No sampling, no cardinality controls, no TLS, no auth.** Demo traffic on
  localhost.
- POC — not production hardening.

## Repo layout

```
cmd/frontend/main.go        # :8081  POST /order, starts trace, calls backend
cmd/backend/main.go         # :8082  POST /process, child span, simulated work
internal/telemetry/         # OTel SDK (stdout) + JSON logger w/ trace correlation
internal/metrics/           # hand-rolled Prometheus exposition registry
docs/elk-pipeline.md        # centralized logging: Filebeat -> Elasticsearch -> Kibana
run.sh                      # build + run both + fire one request
```

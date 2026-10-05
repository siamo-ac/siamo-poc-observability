# Centralized logging: shipping these logs to ELK

**ELK** = Elasticsearch (search + store), Logstash (optional transform),
Kibana (dashboards). The third leg of this POC's logging story: the demo
prints JSON to the terminal, but the *format* is already what a centralized
pipeline wants — one JSON object per line, stable field names, and the
`trace_id` on every line.

## Why these logs are ELK-shippable as-is

- **Line-delimited JSON**: each log line parses on its own; no multiline
  stitching, no regex grokking.
- **Stable schema**: `ts`, `level`, `msg`, `service`, `trace_id`, `span_id`
  (see `internal/telemetry`). Elasticsearch can map these to fields once
  and index every line the same way.
- **`trace_id` correlation**: the same ID the traces use, so a Kibana search
  for one request finds its logs *and* you can pivot to the trace timeline.

## The pipeline (what runs where in production)

```
app (JSON to stdout/file) -> Filebeat -> Elasticsearch -> Kibana
                                  ^
                     (Logstash optional: enrich/filter between shipper and store)
```

Filebeat tails the log file and ships each line; Elasticsearch indexes the
JSON fields; Kibana queries them.

## Filebeat config sample

Point this at wherever the services' stdout lands (a file, journald, a
container log driver — the app side doesn't change):

```yaml
filebeat.inputs:
  - type: filestream
    paths:
      - /var/log/siamo/frontend.log
      - /var/log/siamo/backend.log
    parsers:
      - ndjson:                 # each line is already JSON — no grok needed
          target: ""
          overwrite_keys: true

output.elasticsearch:
  hosts: ["https://es.example:9200"]
  index: "siamo-logs-%{+yyyy.MM.dd}"
```

With `ndjson` parsing, the line `{"msg":"calling backend","trace_id":"4f1c…"}`
arrives in Elasticsearch as the searchable fields `msg` and `trace_id` —
no Logstash required for this shape.

## When you'd add Logstash

Filebeat → Elasticsearch covers this POC. Logstash earns its place when you
need to *transform* in flight: drop noisy health-check lines, enrich with
GeoIP, normalize a legacy non-JSON app's format, or fan out to two clusters.
Rule of thumb: parse at the edge (Filebeat) if the format is already clean;
centralize parsing (Logstash) only when sources disagree.

## Finding one request in Kibana

Index pattern `siamo-logs-*`, then KQL:

```
trace_id : "4f1c*"
```

gives every log line for that request across both services, in time order —
the same `TraceID` you'd see in the trace view. Logs answer *"what
happened"*, traces answer *"where did it go"*, and the shared ID joins them.

## Honest scope

This POC ships **no ELK stack** — no Elasticsearch to run, no Kibana to
click. The demo proves the *producer* side (structured, correlated JSON);
this doc shows the *consumer* side is a standard, boring pipeline, which is
exactly the point: keep the log format clean and centralized logging becomes
configuration, not a rewrite.

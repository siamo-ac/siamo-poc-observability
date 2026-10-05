// Package metrics is a tiny hand-rolled Prometheus metrics registry: a
// counter (http_requests_total) and a histogram (http_request_duration_seconds),
// rendered in the Prometheus text exposition format on /metrics.
package metrics

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
)

// Buckets are the fixed histogram upper bounds, in seconds.
var Buckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, math.Inf(1)}

// key identifies one label set: service|method|route|status.
type key struct {
	service, method, route, status string
}

func (k key) labelString() string {
	return fmt.Sprintf(`service=%q,method=%q,route=%q,status=%q`, k.service, k.method, k.route, k.status)
}

// Registry is safe for concurrent use by HTTP handlers.
type Registry struct {
	mu      sync.Mutex
	counts  map[key]uint64
	hist    map[key][]uint64 // len == len(Buckets), cumulative per bucket
	sums    map[key]float64
	service string
}

func NewRegistry(service string) *Registry {
	return &Registry{
		counts:  map[key]uint64{},
		hist:    map[key][]uint64{},
		sums:    map[key]float64{},
		service: service,
	}
}

// Observe records one finished request.
func (r *Registry) Observe(method, route, status string, seconds float64) {
	k := key{service: r.service, method: method, route: route, status: status}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counts[k]++
	if r.hist[k] == nil {
		r.hist[k] = make([]uint64, len(Buckets))
	}
	for i, b := range Buckets {
		if seconds <= b {
			r.hist[k][i]++
		}
	}
	r.sums[k] += seconds
}

// Handler returns the exposition-format body for /metrics.
func (r *Registry) Handler() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var sb strings.Builder
	sb.WriteString("# HELP http_requests_total Total number of HTTP requests.\n")
	sb.WriteString("# TYPE http_requests_total counter\n")
	keys := r.sortedKeys()
	for _, k := range keys {
		fmt.Fprintf(&sb, "http_requests_total{%s} %d\n", k.labelString(), r.counts[k])
	}
	sb.WriteString("# HELP http_request_duration_seconds HTTP request latency in seconds.\n")
	sb.WriteString("# TYPE http_request_duration_seconds histogram\n")
	for _, k := range keys {
		for i, b := range Buckets {
			le := "+Inf"
			if !math.IsInf(b, 1) {
				le = fmt.Sprintf("%g", b)
			}
			fmt.Fprintf(&sb, "http_request_duration_seconds_bucket{%s,le=%q} %d\n",
				k.labelString(), le, r.hist[k][i])
		}
		fmt.Fprintf(&sb, "http_request_duration_seconds_sum{%s} %g\n", k.labelString(), r.sums[k])
		fmt.Fprintf(&sb, "http_request_duration_seconds_count{%s} %d\n", k.labelString(), r.counts[k])
	}
	return sb.String()
}

func (r *Registry) sortedKeys() []key {
	ks := make([]key, 0, len(r.counts))
	for k := range r.counts {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool {
		a, b := ks[i], ks[j]
		if a.method != b.method {
			return a.method < b.method
		}
		if a.route != b.route {
			return a.route < b.route
		}
		return a.status < b.status
	})
	return ks
}

// Package metrics provides a small, dependency-free metrics registry with
// Prometheus text exposition and HTTP middleware.
//
// A Registry is safe for concurrent use. The middleware records request
// counts, in-flight requests, and a duration histogram keyed by method,
// matched route pattern, and status code. Route labels use the matched
// pattern (for example "/users/{id}") rather than the raw path so metric
// cardinality stays bounded; unmatched requests are labelled "unmatched".
package metrics

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/brandonbert8/grove/packages/router"
)

const (
	requestsName = "http_requests_total"
	durationName = "http_request_duration_seconds"
	inflightName = "http_requests_in_flight"
)

var defaultBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// Option customizes a Registry.
type Option func(*Registry)

// WithBuckets replaces the default duration histogram buckets (seconds).
// Values are sorted and de-duplicated; non-positive values are ignored.
func WithBuckets(buckets []float64) Option {
	return func(r *Registry) {
		clean := make([]float64, 0, len(buckets))
		for _, b := range buckets {
			if b > 0 {
				clean = append(clean, b)
			}
		}
		if len(clean) > 0 {
			r.buckets = clean
		}
	}
}

// Registry stores HTTP metrics and exposes them in Prometheus text format.
// The zero value is ready to use with default buckets.
type Registry struct {
	mu        sync.Mutex
	buckets   []float64
	requests  map[requestKey]uint64
	durations map[routeKey]*durationSeries
	inflight  atomic.Int64
}

type requestKey struct {
	method string
	route  string
	status int
}

type routeKey struct {
	method string
	route  string
}

type durationSeries struct {
	counts []uint64
	sum    float64
	count  uint64
}

// New builds a Registry. Use WithBuckets to customize the histogram.
func New(opts ...Option) *Registry {
	r := &Registry{
		buckets:   append([]float64(nil), defaultBuckets...),
		requests:  make(map[requestKey]uint64),
		durations: make(map[routeKey]*durationSeries),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(r)
		}
	}
	r.sortBucketsLocked()
	return r
}

// Middleware records HTTP metrics around the next handler.
func (r *Registry) Middleware() router.Middleware {
	return func(next router.HandlerFunc) router.HandlerFunc {
		return func(c router.Context) error {
			if r == nil {
				return next(c)
			}
			start := time.Now()
			r.inflight.Add(1)
			defer r.inflight.Add(-1)

			rec := &statusWriter{ResponseWriter: c.ResponseWriter(), status: http.StatusOK}
			err := next(&writerContext{Context: c, writer: rec})
			status := rec.status
			if err != nil && !rec.wrote {
				status = statusOf(err)
			}
			route := router.RoutePattern(c)
			if route == "" {
				route = "unmatched"
			}
			r.observe(c.Request().Method, route, status, time.Since(start).Seconds())
			return err
		}
	}
}

// Handler returns an http.Handler that renders the Prometheus text format.
func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_ = r.WriteText(w)
	})
}

// HandlerFunc adapts the exposition endpoint to router.HandlerFunc.
func (r *Registry) HandlerFunc() router.HandlerFunc {
	h := r.Handler()
	return func(c router.Context) error {
		h.ServeHTTP(c.ResponseWriter(), c.Request())
		return nil
	}
}

// WriteText writes the current metrics in Prometheus text format.
func (r *Registry) WriteText(w io.Writer) error {
	if r == nil {
		return nil
	}

	r.mu.Lock()
	r.ensureLocked()
	requests := make(map[requestKey]uint64, len(r.requests))
	for k, v := range r.requests {
		requests[k] = v
	}
	durations := make(map[routeKey]durationSeries, len(r.durations))
	for k, v := range r.durations {
		durations[k] = durationSeries{
			counts: append([]uint64(nil), v.counts...),
			sum:    v.sum,
			count:  v.count,
		}
	}
	inflight := r.inflight.Load()
	buckets := append([]float64(nil), r.buckets...)
	r.mu.Unlock()

	bw := bufio.NewWriter(w)

	fmt.Fprintf(bw, "# HELP %s Total number of HTTP requests.\n", requestsName)
	fmt.Fprintf(bw, "# TYPE %s counter\n", requestsName)
	reqKeys := make([]requestKey, 0, len(requests))
	for k := range requests {
		reqKeys = append(reqKeys, k)
	}
	sort.Slice(reqKeys, func(i, j int) bool {
		if reqKeys[i].method != reqKeys[j].method {
			return reqKeys[i].method < reqKeys[j].method
		}
		if reqKeys[i].route != reqKeys[j].route {
			return reqKeys[i].route < reqKeys[j].route
		}
		return reqKeys[i].status < reqKeys[j].status
	})
	for _, k := range reqKeys {
		fmt.Fprintf(bw, "%s{method=\"%s\",route=\"%s\",status=\"%s\"} %d\n",
			requestsName, escapeLabel(k.method), escapeLabel(k.route),
			strconv.Itoa(k.status), requests[k])
	}

	fmt.Fprintf(bw, "# HELP %s Current number of in-flight HTTP requests.\n", inflightName)
	fmt.Fprintf(bw, "# TYPE %s gauge\n", inflightName)
	fmt.Fprintf(bw, "%s %d\n", inflightName, inflight)

	fmt.Fprintf(bw, "# HELP %s HTTP request duration in seconds.\n", durationName)
	fmt.Fprintf(bw, "# TYPE %s histogram\n", durationName)
	durKeys := make([]routeKey, 0, len(durations))
	for k := range durations {
		durKeys = append(durKeys, k)
	}
	sort.Slice(durKeys, func(i, j int) bool {
		if durKeys[i].method != durKeys[j].method {
			return durKeys[i].method < durKeys[j].method
		}
		return durKeys[i].route < durKeys[j].route
	})
	for _, k := range durKeys {
		d := durations[k]
		for i, b := range buckets {
			fmt.Fprintf(bw, "%s_bucket{method=\"%s\",route=\"%s\",le=\"%s\"} %d\n",
				durationName, escapeLabel(k.method), escapeLabel(k.route),
				strconv.FormatFloat(b, 'f', -1, 64), d.counts[i])
		}
		fmt.Fprintf(bw, "%s_bucket{method=\"%s\",route=\"%s\",le=\"+Inf\"} %d\n",
			durationName, escapeLabel(k.method), escapeLabel(k.route), d.count)
		fmt.Fprintf(bw, "%s_sum{method=\"%s\",route=\"%s\"} %s\n",
			durationName, escapeLabel(k.method), escapeLabel(k.route),
			strconv.FormatFloat(d.sum, 'f', -1, 64))
		fmt.Fprintf(bw, "%s_count{method=\"%s\",route=\"%s\"} %d\n",
			durationName, escapeLabel(k.method), escapeLabel(k.route), d.count)
	}

	return bw.Flush()
}

// Reset clears all recorded metrics. In-flight requests are not reset.
func (r *Registry) Reset() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.requests = make(map[requestKey]uint64)
	r.durations = make(map[routeKey]*durationSeries)
	r.mu.Unlock()
}

func (r *Registry) observe(method, route string, status int, seconds float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureLocked()

	r.requests[requestKey{method: method, route: route, status: status}]++

	k := routeKey{method: method, route: route}
	d := r.durations[k]
	if d == nil {
		d = &durationSeries{counts: make([]uint64, len(r.buckets))}
		r.durations[k] = d
	}
	for i, b := range r.buckets {
		if seconds <= b {
			d.counts[i]++
		}
	}
	d.sum += seconds
	d.count++
}

func (r *Registry) ensureLocked() {
	if r.requests == nil {
		r.requests = make(map[requestKey]uint64)
	}
	if r.durations == nil {
		r.durations = make(map[routeKey]*durationSeries)
	}
	if len(r.buckets) == 0 {
		r.buckets = append([]float64(nil), defaultBuckets...)
	}
}

func (r *Registry) sortBucketsLocked() {
	sort.Float64s(r.buckets)
	out := r.buckets[:0]
	for i, b := range r.buckets {
		if i == 0 || b != r.buckets[i-1] {
			out = append(out, b)
		}
	}
	r.buckets = out
}

func statusOf(err error) int {
	var se router.Statuser
	if errors.As(err, &se) {
		if code := se.StatusCode(); code >= 100 && code <= 599 {
			return code
		}
	}
	return http.StatusInternalServerError
}

func escapeLabel(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *statusWriter) WriteHeader(code int) {
	if w.wrote {
		return
	}
	w.status = code
	w.wrote = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) Flush() {
	f, ok := w.ResponseWriter.(http.Flusher)
	if !ok {
		return
	}
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	f.Flush()
}

func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("metrics: underlying writer does not support hijacking")
	}
	return h.Hijack()
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

type writerContext struct {
	router.Context
	writer http.ResponseWriter
}

func (c *writerContext) ResponseWriter() http.ResponseWriter { return c.writer }

func (c *writerContext) SetRequest(r *http.Request) {
	if sr, ok := c.Context.(interface{ SetRequest(*http.Request) }); ok {
		sr.SetRequest(r)
		return
	}
	*c.Context.Request() = *r
}

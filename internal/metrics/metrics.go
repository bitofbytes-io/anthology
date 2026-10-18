// Package metrics exposes Anthology's Prometheus metrics: HTTP RED metrics for
// the public router, database pool stats, and a few domain counters. Metrics
// live in their own registry and are served on a separate internal listener
// (see Start), never on the public router.
package metrics

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// UnmatchedRoute is the route label for requests that matched no route, so
// unknown paths cannot grow the label set.
const UnmatchedRoute = "unmatched"

// OAuth providers, used as the provider label.
const ProviderGoogle = "google"

// Upload results, used as the result label of anthology_uploads_total.
const (
	UploadSuccess  = "success"
	UploadInvalid  = "invalid"
	UploadTooLarge = "too_large"
	UploadError    = "error"
)

// Background jobs and their results, used by anthology_jobs_total.
const (
	JobSessionCleanup = "session_cleanup"
	JobSuccess        = "success"
	JobError          = "error"
)

// skippedPaths are probe and scrape endpoints left out of the RED metrics.
var skippedPaths = map[string]struct{}{
	"/health":     {},
	"/ready":      {},
	"/metrics":    {},
	"/api/health": {},
	"/api/ready":  {},
}

// knownMethods bounds the method label; anything else is recorded as OTHER.
var knownMethods = map[string]struct{}{
	http.MethodGet:     {},
	http.MethodHead:    {},
	http.MethodPost:    {},
	http.MethodPut:     {},
	http.MethodPatch:   {},
	http.MethodDelete:  {},
	http.MethodOptions: {},
}

// Metrics holds Anthology's collectors and the registry they are exposed
// from. A nil *Metrics is valid and records nothing.
type Metrics struct {
	registry        *prometheus.Registry
	requests        *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	oauthFailures   *prometheus.CounterVec
	uploads         *prometheus.CounterVec
	jobs            *prometheus.CounterVec
}

// New returns Metrics backed by a fresh registry that also carries the Go
// runtime and process collectors.
func New() *Metrics {
	m := &Metrics{
		registry: prometheus.NewRegistry(),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "HTTP requests handled, by method, route pattern and status code.",
		}, []string{"method", "route", "status"}),
		requestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency, by method, route pattern and status code.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route", "status"}),
		oauthFailures: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "anthology_oauth_failures_total",
			Help: "OAuth sign-in attempts that did not end in a session, by provider.",
		}, []string{"provider"}),
		uploads: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "anthology_uploads_total",
			Help: "CSV import uploads, by result.",
		}, []string{"result"}),
		jobs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "anthology_jobs_total",
			Help: "Background job runs, by job and result.",
		}, []string{"job", "result"}),
	}
	m.registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.requests,
		m.requestDuration,
		m.oauthFailures,
		m.uploads,
		m.jobs,
	)
	return m
}

// RegisterDB exposes db's connection pool stats as go_sql_* metrics labelled
// with dbName.
func (m *Metrics) RegisterDB(db *sql.DB, dbName string) {
	if m == nil || db == nil {
		return
	}
	m.registry.MustRegister(collectors.NewDBStatsCollector(db, dbName))
}

// Handler serves the registry in the Prometheus exposition format.
func (m *Metrics) Handler() http.Handler {
	if m == nil {
		return http.NotFoundHandler()
	}
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{Registry: m.registry})
}

// Middleware records http_requests_total and http_request_duration_seconds
// for every request except health, readiness and metrics probes. Routes are
// labelled by chi route pattern, so it must be installed on routes, the chi
// router whose patterns it reports.
func (m *Metrics) Middleware(routes chi.Routes) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if m == nil {
			return next
		}
		return m.instrument(routes, next)
	}
}

func (m *Metrics) instrument(routes chi.Routes, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, skip := skippedPaths[r.URL.Path]; skip {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		defer func() {
			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}
			labels := prometheus.Labels{
				"method": methodLabel(r.Method),
				"route":  routeLabel(routes, r),
				"status": strconv.Itoa(status),
			}
			m.requests.With(labels).Inc()
			m.requestDuration.With(labels).Observe(time.Since(start).Seconds())
		}()
		next.ServeHTTP(ww, r)
	})
}

// OAuthFailure counts a sign-in attempt through provider that failed.
func (m *Metrics) OAuthFailure(provider string) {
	if m == nil {
		return
	}
	m.oauthFailures.WithLabelValues(provider).Inc()
}

// Upload counts a CSV upload with the given result.
func (m *Metrics) Upload(result string) {
	if m == nil {
		return
	}
	m.uploads.WithLabelValues(result).Inc()
}

// Job counts a run of a background job with the given result.
func (m *Metrics) Job(job, result string) {
	if m == nil {
		return
	}
	m.jobs.WithLabelValues(job, result).Inc()
}

func methodLabel(method string) string {
	if _, ok := knownMethods[method]; ok {
		return method
	}
	return "OTHER"
}

// routeLabel returns the chi route pattern that served r (for a 405, the
// pattern whose path matched). When routing stopped early, at a mounted
// subrouter's "/*" (for example, auth middleware rejected the request before
// the subrouter routed it), the full pattern is looked up in routes. A request
// matching no endpoint is labelled UnmatchedRoute; the router defines no
// wildcard endpoints of its own, so a "/*" pattern is never a real route.
func routeLabel(routes chi.Routes, r *http.Request) string {
	pattern := chi.RouteContext(r.Context()).RoutePattern()
	if (pattern == "" || strings.HasSuffix(pattern, "/*")) && routes != nil {
		path := r.URL.RawPath
		if path == "" {
			path = r.URL.Path
		}
		pattern = routes.Find(chi.NewRouteContext(), r.Method, path)
		if pattern != "/" {
			pattern = strings.TrimSuffix(pattern, "/")
		}
	}
	if pattern == "" || strings.HasSuffix(pattern, "/*") {
		return UnmatchedRoute
	}
	return pattern
}

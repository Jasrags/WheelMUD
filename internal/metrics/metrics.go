// Package metrics exposes Prometheus metrics, healthz, and pprof on a
// private HTTP listener (Phase J slice J5 / #54).
//
// Construction:
//
//	m := metrics.New(metrics.Config{
//	    DB:          conn,               // *sql.DB for db_open_conns + ping
//	    Sessions:    sessions.Snapshot,  // func() map[int64]*Session
//	    BuildInfo:   metrics.BuildInfo{Version: "..."},
//	})
//	srv := &http.Server{Addr: addr, Handler: m.Handler()}
//	go srv.ListenAndServe()
//
// Caller is responsible for the http.Server lifecycle so shutdown
// drains can hook the same context.
package metrics

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/pprof"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// BuildInfo carries the version/commit/date stamped at build time
// (Phase J slice J7 will populate these via ldflags). All fields zero
// is acceptable — the build_info metric will report empty strings.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// SessionSnapshotFn returns the currently bound session map. Used by
// the sessions_active gauge collector. nil disables the gauge.
type SessionSnapshotFn func() map[int64]any

// Config wires the Metrics instance to its data sources.
type Config struct {
	// DB is the live database handle. Used for the db_open_conns
	// gauge and healthz ping. nil disables both.
	DB *sql.DB

	// SessionCount returns the current active session count. Used by
	// the sessions_active gauge. nil disables the gauge.
	SessionCount func() int

	// BuildInfo populates the build_info metric labels.
	BuildInfo BuildInfo

	// HealthPingTimeout caps the DB ping issued by /healthz. Default
	// 500ms.
	HealthPingTimeout time.Duration
}

// Metrics is the registered Prometheus metric set + HTTP handler
// wiring. Construct via New.
type Metrics struct {
	cfg Config
	reg *prometheus.Registry

	// Counters.
	CommandsTotal *prometheus.CounterVec

	// Gauges.
	sessionsActive prometheus.GaugeFunc
	dbOpenConns    prometheus.GaugeFunc
	buildInfo      *prometheus.GaugeVec

	// Lifecycle flag for healthz. True once SetReady(true) is called
	// (after the listener binds); flipped false on shutdown drain.
	ready atomic.Bool
}

// New registers the metric set against a fresh Prometheus registry
// and returns the Metrics handle. Safe to call once at startup.
func New(cfg Config) *Metrics {
	if cfg.HealthPingTimeout <= 0 {
		cfg.HealthPingTimeout = 500 * time.Millisecond
	}
	m := &Metrics{
		cfg: cfg,
		reg: prometheus.NewRegistry(),
	}

	m.CommandsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "wheelmud_commands_total",
			Help: "Count of in-game commands dispatched, labeled by verb and result (ok|error).",
		},
		[]string{"verb", "result"},
	)
	m.reg.MustRegister(m.CommandsTotal)

	m.buildInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "wheelmud_build_info",
			Help: "Build metadata; value is always 1. Labels carry version, commit, date, and go_version.",
		},
		[]string{"version", "commit", "date", "go_version"},
	)
	m.buildInfo.WithLabelValues(
		cfg.BuildInfo.Version, cfg.BuildInfo.Commit, cfg.BuildInfo.Date, runtime.Version(),
	).Set(1)
	m.reg.MustRegister(m.buildInfo)

	if cfg.SessionCount != nil {
		m.sessionsActive = prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{
				Name: "wheelmud_sessions_active",
				Help: "Number of currently bound player sessions.",
			},
			func() float64 { return float64(cfg.SessionCount()) },
		)
		m.reg.MustRegister(m.sessionsActive)
	}
	if cfg.DB != nil {
		m.dbOpenConns = prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{
				Name: "wheelmud_db_open_conns",
				Help: "Number of currently open database connections (sql.DB.Stats().OpenConnections).",
			},
			func() float64 { return float64(cfg.DB.Stats().OpenConnections) },
		)
		m.reg.MustRegister(m.dbOpenConns)
	}

	// Process collector (memory, FDs, CPU, uptime) is useful enough
	// to justify the dep cost. Go collector ships goroutines, GC, and
	// alloc histograms.
	m.reg.MustRegister(prometheus.NewGoCollector())
	m.reg.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))

	return m
}

// SetReady toggles the healthz response. Call SetReady(true) once the
// telnet listener binds, SetReady(false) at the start of shutdown
// drain.
func (m *Metrics) SetReady(ready bool) { m.ready.Store(ready) }

// ObserveCommand bumps the commands_total counter. verb is the
// lowercased verb name; ok is true for successful dispatch, false
// for refusals / unknown / privilege denials. Called by the
// mode/game.go audit hook.
func (m *Metrics) ObserveCommand(verb string, ok bool) {
	if verb == "" {
		return
	}
	result := "ok"
	if !ok {
		result = "error"
	}
	m.CommandsTotal.WithLabelValues(verb, result).Inc()
}

// Handler returns an http.Handler with /metrics, /healthz, and the
// stdlib pprof endpoints mounted. Wrap in your own http.Server.
func (m *Metrics) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(m.reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", m.handleHealthz)
	// pprof — the stdlib package registers its handlers on
	// http.DefaultServeMux by default; mount them explicitly here so
	// the metrics handler is self-contained.
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	return mux
}

func (m *Metrics) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if !m.ready.Load() {
		writeHealth(w, http.StatusServiceUnavailable, "not ready")
		return
	}
	if m.cfg.DB != nil {
		ctx, cancel := context.WithTimeout(r.Context(), m.cfg.HealthPingTimeout)
		defer cancel()
		if err := m.cfg.DB.PingContext(ctx); err != nil {
			writeHealth(w, http.StatusServiceUnavailable, "db ping failed: "+err.Error())
			return
		}
	}
	writeHealth(w, http.StatusOK, "ok")
}

func writeHealth(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(strings.TrimRight(body, "\n") + "\n"))
}

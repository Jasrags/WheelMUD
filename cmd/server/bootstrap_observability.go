package main

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/Jasrags/WheelMUD/internal/backup"
	"github.com/Jasrags/WheelMUD/internal/config"
	"github.com/Jasrags/WheelMUD/internal/metrics"
	"github.com/Jasrags/WheelMUD/internal/mode"
	"github.com/Jasrags/WheelMUD/internal/safego"
	"github.com/Jasrags/WheelMUD/internal/session"
)

// setupMetrics stands up the Prometheus + pprof + healthz HTTP server
// when cfg.Server.MetricsAddr is non-empty. Mutates srv.metrics and
// srv.metricsHTTP; wires the per-command metric hook onto gameMode.
// Empty MetricsAddr disables the HTTP server entirely. Bound to
// loopback by default so an unprotected listener doesn't leak pprof
// to the public internet. Phase J slice J5 / #54.
func setupMetrics(srv *server, gameMode *mode.Game, cfg config.Config, conn *sql.DB, sessions *session.Registry) {
	if cfg.Server.MetricsAddr == "" {
		return
	}
	srv.metrics = metrics.New(metrics.Config{
		DB:           conn,
		SessionCount: func() int { return len(sessions.Snapshot()) },
		BuildInfo: metrics.BuildInfo{
			Version: buildVersion,
			Commit:  buildCommit,
			Date:    buildDate,
		},
	})
	gameMode.SetMetricHook(buildCommandMetricFn(srv.metrics))
	srv.metricsHTTP = &http.Server{
		Addr:              cfg.Server.MetricsAddr,
		Handler:           srv.metrics.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		// WriteTimeout bounds streaming endpoints like
		// /debug/pprof/profile (default 30s duration) and
		// /debug/pprof/trace. Set generously so a profile
		// completes, but tight enough that a slow-reader
		// attacker can't accumulate goroutines indefinitely.
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	safego.Go("metrics-http", func() {
		slog.Info("Metrics server listening", "addr", cfg.Server.MetricsAddr)
		if err := srv.metricsHTTP.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Metrics server failed", "error", err)
		}
	})
}

// setupBackup starts the VACUUM-INTO backup manager when both
// db.backup_dir and a positive interval are configured. The Manager
// takes its own background goroutine and observes the server-lifetime
// ctx so a SIGTERM / admin shutdown drains cleanly. Init failure is
// fatal: a misconfigured backup_dir (e.g. a symlink) is a deploy
// mistake the operator must fix before boot continues. Phase J slice
// J4 / #56.
func setupBackup(ctx context.Context, cfg config.Config, conn *sql.DB) {
	if cfg.DB.BackupDir == "" || cfg.DB.BackupIntervalHours <= 0 {
		return
	}
	mgr, berr := backup.New(conn, backup.Config{
		Dir:           cfg.DB.BackupDir,
		IntervalHours: cfg.DB.BackupIntervalHours,
		Retention:     cfg.DB.BackupRetention,
	})
	if berr != nil {
		slog.Error("Backup manager init failed", "error", berr)
		os.Exit(1)
	}
	slog.Info("Backup manager enabled",
		"dir", cfg.DB.BackupDir,
		"interval_h", cfg.DB.BackupIntervalHours,
		"retention", cfg.DB.BackupRetention)
	safego.Go("backup-manager", func() { mgr.Run(ctx) })
}

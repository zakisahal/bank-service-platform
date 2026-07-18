package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/zsahal/bank-service/internal/config"
	"github.com/zsahal/bank-service/internal/db"
	"github.com/zsahal/bank-service/internal/httpserver"
	"github.com/zsahal/bank-service/internal/tracing"
)

func main() {
	// Distroless has no shell, no curl, no wget - so Docker's HEALTHCHECK
	// can't shell out to anything. Instead the binary itself does the
	// probe: `bank-service -healthcheck` hits its own /healthz over
	// loopback and exits 0/1, which is all HEALTHCHECK needs.
	if len(os.Args) > 1 && os.Args[1] == "-healthcheck" {
		os.Exit(runHealthcheck())
	}

	// preStop hook target. Distroless has no `sleep` binary either, so the
	// delay-before-SIGTERM trick (give kube-proxy/ALB time to stop routing
	// new connections here before we start shutting down) also has to live
	// in the binary. See deploy/helm .../deployment.yaml lifecycle.preStop
	// and DECISIONS.md "Zero-downtime rollout".
	if len(os.Args) > 1 && os.Args[1] == "-preStop" {
		time.Sleep(preStopSleepDuration())
		os.Exit(0)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(logger); err != nil {
		logger.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func runHealthcheck() int {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

func preStopSleepDuration() time.Duration {
	seconds := 5
	if v := os.Getenv("PRESTOP_SLEEP_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			seconds = n
		}
	}
	return time.Duration(seconds) * time.Second
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdownTracing, err := tracing.Init(ctx, "bank-service", cfg.OTLPEndpoint)
	if err != nil {
		return fmt.Errorf("init tracing: %w", err)
	}
	defer func() {
		// Independent timeout: tracer flush shouldn't be able to block
		// process exit past the app's own shutdown budget.
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTracing(c); err != nil {
			logger.Warn("tracer shutdown failed", "error", err)
		}
	}()

	store, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()

	srv := httpserver.New(store, cfg.DBPingTimeout, logger)

	// otelhttp gives every request its own span - the HTTP-server half of
	// the one trace path this service exposes (see internal/db for the
	// child span around the Postgres call each request makes).
	instrumentedHandler := otelhttp.NewHandler(srv.Handler(), "bank-service")

	httpSrv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           instrumentedHandler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("starting server", "port", cfg.Port)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("listen error", "error", err)
		}
	}()

	// Give the process a moment to bind before advertising readiness.
	srv.MarkReady()

	<-ctx.Done()
	logger.Info("shutdown signal received, draining")

	// Fail /readyz immediately so the load balancer stops sending new
	// traffic before we start tearing the process down.
	srv.MarkDraining()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}

	logger.Info("shutdown complete")
	return nil
}

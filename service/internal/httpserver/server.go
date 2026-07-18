// Package httpserver implements the bank-service HTTP API: health checks,
// metrics, and the accounts read endpoint.
package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/zsahal/bank-service/internal/db"
)

// Store is the subset of db.Store the HTTP layer depends on, kept as an
// interface so handlers can be unit tested without a real Postgres.
type Store interface {
	Ping(ctx context.Context) error
	GetAccount(ctx context.Context, id string) (db.Account, error)
}

type Server struct {
	store         Store
	dbPingTimeout time.Duration
	ready         atomic.Bool
	logger        *slog.Logger
	mux           *http.ServeMux
}

func New(store Store, dbPingTimeout time.Duration, logger *slog.Logger) *Server {
	s := &Server{
		store:         store,
		dbPingTimeout: dbPingTimeout,
		logger:        logger,
		mux:           http.NewServeMux(),
	}
	// Not ready until main() calls MarkReady() once startup checks pass.
	s.ready.Store(false)
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

// MarkReady flips readiness on after successful startup, and is flipped
// off again during shutdown so /readyz starts failing before the process
// stops accepting connections (fast load-balancer deregistration).
func (s *Server) MarkReady()    { s.ready.Store(true) }
func (s *Server) MarkDraining() { s.ready.Store(false) }

func (s *Server) routes() {
	s.mux.Handle("GET /healthz", s.instrument("/healthz", http.HandlerFunc(s.handleHealthz)))
	s.mux.Handle("GET /readyz", s.instrument("/readyz", http.HandlerFunc(s.handleReadyz)))
	s.mux.Handle("GET /api/accounts/{id}", s.instrument("/api/accounts/{id}", http.HandlerFunc(s.handleGetAccount)))
	s.mux.Handle("GET /metrics", promhttp.Handler())
}

// instrument wraps a handler with Prometheus request metrics.
func (s *Server) instrument(route string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestsInFlight.Inc()
		defer requestsInFlight.Dec()

		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		requestDuration.WithLabelValues(route, r.Method).Observe(time.Since(start).Seconds())
		requestsTotal.WithLabelValues(route, r.Method, http.StatusText(rec.status)).Inc()
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if !s.ready.Load() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "draining"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.dbPingTimeout)
	defer cancel()

	if err := s.store.Ping(ctx); err != nil {
		dbUp.Set(0)
		s.logger.Warn("readyz: db ping failed", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "db_unreachable"})
		return
	}

	dbUp.Set(1)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleGetAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	account, err := s.store.GetAccount(ctx, id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "account not found"})
			return
		}
		s.logger.Error("get account failed", "id", id, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	writeJSON(w, http.StatusOK, account)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

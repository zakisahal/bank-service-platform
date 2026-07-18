package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zsahal/bank-service/internal/db"
)

type fakeStore struct {
	pingErr  error
	accounts map[string]db.Account
}

func (f *fakeStore) Ping(ctx context.Context) error { return f.pingErr }

func (f *fakeStore) GetAccount(ctx context.Context, id string) (db.Account, error) {
	a, ok := f.accounts[id]
	if !ok {
		return db.Account{}, db.ErrNotFound
	}
	return a, nil
}

func newTestServer(store *fakeStore) *Server {
	return New(store, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestHealthzAlwaysOK(t *testing.T) {
	s := newTestServer(&fakeStore{})
	// healthz must be OK even before MarkReady, unlike readyz.
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestReadyzNotReadyBeforeMarkReady(t *testing.T) {
	s := newTestServer(&fakeStore{})
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 before MarkReady, got %d", rec.Code)
	}
}

func TestReadyzOKWhenDBHealthy(t *testing.T) {
	s := newTestServer(&fakeStore{})
	s.MarkReady()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestReadyzFailsWhenDBUnreachable(t *testing.T) {
	s := newTestServer(&fakeStore{pingErr: errors.New("connection refused")})
	s.MarkReady()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when db unreachable, got %d", rec.Code)
	}
}

func TestReadyzFailsWhileDraining(t *testing.T) {
	s := newTestServer(&fakeStore{})
	s.MarkReady()
	s.MarkDraining()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 while draining, got %d", rec.Code)
	}
}

func TestGetAccountFound(t *testing.T) {
	want := db.Account{ID: "acc_1001", Owner: "Ada Lovelace", Balance: 542300, Currency: "USD"}
	s := newTestServer(&fakeStore{accounts: map[string]db.Account{want.ID: want}})
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/accounts/acc_1001", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got db.Account
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != want.ID || got.Owner != want.Owner || got.Balance != want.Balance {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestGetAccountNotFound(t *testing.T) {
	s := newTestServer(&fakeStore{accounts: map[string]db.Account{}})
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/accounts/does-not-exist", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestMetricsEndpointExposesPrometheusFormat(t *testing.T) {
	s := newTestServer(&fakeStore{})
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct == "" {
		t.Fatalf("expected a content-type header on /metrics")
	}
}

// Package db wraps the Postgres connection pool and account queries.
package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// The child span half of this service's one trace path: every request span
// from otelhttp (see cmd/api/main.go) gets a nested "db.get_account" span
// covering the actual Postgres round-trip.
var tracer = otel.Tracer("bank-service/db")

var ErrNotFound = errors.New("account not found")

type Account struct {
	ID        string    `json:"id"`
	Owner     string    `json:"owner"`
	Balance   int64     `json:"balance_cents"`
	Currency  string    `json:"currency"`
	CreatedAt time.Time `json:"created_at"`
}

type Store struct {
	pool *pgxpool.Pool
}

func Connect(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() {
	s.pool.Close()
}

// Ping is used by /readyz to check Postgres reachability.
func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *Store) GetAccount(ctx context.Context, id string) (Account, error) {
	ctx, span := tracer.Start(ctx, "db.get_account",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation", "SELECT"),
			attribute.String("db.sql.table", "accounts"),
			// account_id is an opaque identifier, not owner-identifying by
			// itself - see DECISIONS.md "PII scrubbing" for what's
			// deliberately kept out of spans (owner name, balance).
			attribute.String("bank.account_id", id),
		),
	)
	defer span.End()

	var a Account
	row := s.pool.QueryRow(ctx,
		`SELECT id, owner, balance_cents, currency, created_at FROM accounts WHERE id = $1`,
		id,
	)
	if err := row.Scan(&a.ID, &a.Owner, &a.Balance, &a.Currency, &a.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			span.SetStatus(codes.Ok, "not found")
			return Account{}, ErrNotFound
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return Account{}, fmt.Errorf("query account: %w", err)
	}
	return a, nil
}

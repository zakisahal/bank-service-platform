package events

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestHandleIsIdempotent proves the actual claim this package makes: the
// same event_id delivered twice produces exactly one audit_log row. It
// needs a real Postgres with migrations 0001-0003 applied (TEST_DATABASE_URL),
// so it's skipped rather than faked - the property being tested is a
// transaction-level guarantee that a mock DB can't meaningfully exercise.
//
// c.handle deliberately runs as bank_service_consumer, the same
// least-privilege role the real consumer uses (see migrations/0003_roles.sql)
// - that role can INSERT but not SELECT on audit_log or DELETE on either
// table, so assertions connect separately as ADMIN_TEST_DATABASE_URL to
// read back the result, and each run uses a fresh event_id rather than a
// fixed one that would need cleanup. (An earlier version of this test tried
// to both assert and clean up over the consumer connection and hit
// "permission denied" - a good sign the grant is real, but the fix is
// narrower test credentials, not a broader grant on the role itself.)
func TestHandleIsIdempotent(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	adminDSN := os.Getenv("ADMIN_TEST_DATABASE_URL")
	if dsn == "" || adminDSN == "" {
		t.Skip("TEST_DATABASE_URL/ADMIN_TEST_DATABASE_URL not set; skipping DB-backed idempotency test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect as bank_service_consumer: %v", err)
	}
	defer pool.Close()

	adminPool, err := pgxpool.New(ctx, adminDSN)
	if err != nil {
		t.Fatalf("connect as admin: %v", err)
	}
	defer adminPool.Close()

	c := &Consumer{pool: pool, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	ev := Event{
		ID:        fmt.Sprintf("evt-test-idempotent-%d", time.Now().UnixNano()),
		AccountID: "acc_1001",
		Type:      "test.event",
		Payload:   json.RawMessage(`{"x":1}`),
	}

	if err := c.handle(ctx, ev); err != nil {
		t.Fatalf("first handle: %v", err)
	}
	if err := c.handle(ctx, ev); err != nil {
		t.Fatalf("second handle (simulated redelivery): %v", err)
	}

	var count int
	if err := adminPool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE event_id = $1`, ev.ID).Scan(&count); err != nil {
		t.Fatalf("count audit_log rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 audit_log row after 2 deliveries of the same event_id, got %d", count)
	}

	var processedCount int
	if err := adminPool.QueryRow(ctx, `SELECT count(*) FROM processed_events WHERE event_id = $1`, ev.ID).Scan(&processedCount); err != nil {
		t.Fatalf("count processed_events rows: %v", err)
	}
	if processedCount != 1 {
		t.Fatalf("expected exactly 1 processed_events row, got %d", processedCount)
	}
}

func TestParseEventRejectsMalformed(t *testing.T) {
	cases := map[string]map[string]interface{}{
		"missing data field":  {"other": "x"},
		"data not a string":   {"data": 123},
		"data not valid json": {"data": "not-json"},
		"missing id":          {"data": `{"account_id":"acc_1"}`},
	}
	for name, fields := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseEvent(fields); err == nil {
				t.Fatalf("expected an error for case %q", name)
			}
		})
	}
}

func TestParseEventAcceptsValid(t *testing.T) {
	fields := map[string]interface{}{
		"data": `{"id":"evt-1","account_id":"acc_1001","type":"account.updated","payload":{"balance_cents":100}}`,
	}
	ev, err := ParseEvent(fields)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.ID != "evt-1" || ev.AccountID != "acc_1001" || ev.Type != "account.updated" {
		t.Fatalf("unexpected parsed event: %+v", ev)
	}
}

package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type Consumer struct {
	rdb           *redis.Client
	pool          *pgxpool.Pool
	logger        *slog.Logger
	stream        string
	dlqStream     string
	group         string
	consumerName  string
	maxDeliveries int64
	minIdle       time.Duration
}

func New(rdb *redis.Client, pool *pgxpool.Pool, logger *slog.Logger, stream, group, consumerName string, maxDeliveries int64) *Consumer {
	return &Consumer{
		rdb:           rdb,
		pool:          pool,
		logger:        logger,
		stream:        stream,
		dlqStream:     stream + ".dlq",
		group:         group,
		consumerName:  consumerName,
		maxDeliveries: maxDeliveries,
		minIdle:       30 * time.Second,
	}
}

func (c *Consumer) ensureGroup(ctx context.Context) error {
	err := c.rdb.XGroupCreateMkStream(ctx, c.stream, c.group, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return err
	}
	return nil
}

// Run reads from the stream until ctx is cancelled. Every iteration also
// gives stale pending messages (crashed-consumer redelivery, or a handler
// that errored out and left its message unacked) a chance to be reclaimed
// or, past maxDeliveries, dead-lettered - see reclaimStale.
func (c *Consumer) Run(ctx context.Context) error {
	if err := c.ensureGroup(ctx); err != nil {
		return fmt.Errorf("ensure consumer group: %w", err)
	}
	c.logger.Info("consumer started", "stream", c.stream, "group", c.group, "consumer", c.consumerName, "max_deliveries", c.maxDeliveries)

	reclaimTick := time.NewTicker(10 * time.Second)
	defer reclaimTick.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-reclaimTick.C:
			c.reclaimStale(ctx)
		default:
		}

		streams, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    c.group,
			Consumer: c.consumerName,
			Streams:  []string{c.stream, ">"},
			Count:    10,
			Block:    2 * time.Second,
		}).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) || errors.Is(err, context.Canceled) {
				continue
			}
			c.logger.Warn("XREADGROUP failed, backing off", "error", err)
			time.Sleep(time.Second)
			continue
		}

		for _, s := range streams {
			for _, msg := range s.Messages {
				c.process(ctx, msg)
			}
		}
	}
}

func (c *Consumer) process(ctx context.Context, msg redis.XMessage) {
	ev, err := ParseEvent(msg.Values)
	if err != nil {
		// A malformed message can never be processed no matter how many
		// times we retry it - dead-letter it immediately rather than let
		// it occupy a retry slot that a fixable message could use.
		c.logger.Warn("unparseable event, dead-lettering without retry", "id", msg.ID, "error", err)
		c.deadLetter(ctx, msg, "unparseable: "+err.Error())
		return
	}

	if err := c.handle(ctx, ev); err != nil {
		// Left un-ACKed on purpose: it stays in the consumer group's
		// pending entries list, and reclaimStale retries it (or parks it
		// in the DLQ once maxDeliveries is exceeded) rather than us
		// retrying inline and blocking the read loop on a struggling DB.
		c.logger.Warn("processing failed, leaving for retry", "event_id", ev.ID, "redis_id", msg.ID, "error", err)
		return
	}

	if err := c.rdb.XAck(ctx, c.stream, c.group, msg.ID).Err(); err != nil {
		c.logger.Error("ack failed", "id", msg.ID, "error", err)
	}
}

// handle is where idempotency actually lives: the dedupe check and the
// audit write happen in one transaction, so a crash between them is
// impossible and a redelivered event_id can never produce a second audit
// row.
func (c *Consumer) handle(ctx context.Context, ev Event) error {
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx,
		`INSERT INTO processed_events (event_id) VALUES ($1) ON CONFLICT DO NOTHING`,
		ev.ID,
	)
	if err != nil {
		return fmt.Errorf("mark processed: %w", err)
	}

	if tag.RowsAffected() == 1 {
		// First time this event_id has been seen - do the side effect. A
		// redelivery of the same ID hits ON CONFLICT DO NOTHING above
		// instead, so this INSERT (and the effect it represents) runs
		// exactly once regardless of how many times Redis redelivers.
		if _, err := tx.Exec(ctx,
			`INSERT INTO audit_log (event_id, account_id, event_type, payload) VALUES ($1, $2, $3, $4)`,
			ev.ID, ev.AccountID, ev.Type, ev.Payload,
		); err != nil {
			return fmt.Errorf("write audit record: %w", err)
		}
	} else {
		c.logger.Info("duplicate delivery, dedupe hit - skipping side effect", "event_id", ev.ID)
	}

	return tx.Commit(ctx)
}

// reclaimStale looks at everything still pending (delivered, not yet
// ACKed) for this group. A message idle past minIdle has either had its
// consumer crash mid-processing or failed and been left for retry - either
// way it's eligible to be claimed again here. Once a message's delivery
// count exceeds maxDeliveries, retrying it further is assumed futile (a
// poison message) and it's parked in the DLQ instead of retried forever.
func (c *Consumer) reclaimStale(ctx context.Context) {
	pending, err := c.rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: c.stream, Group: c.group, Start: "-", End: "+", Count: 50,
	}).Result()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			c.logger.Warn("XPENDING failed", "error", err)
		}
		return
	}

	for _, p := range pending {
		if p.Idle < c.minIdle {
			continue // still within its normal processing window, leave it alone
		}

		if p.RetryCount > c.maxDeliveries {
			c.logger.Warn("message exceeded max deliveries, dead-lettering", "id", p.ID, "deliveries", p.RetryCount)
			c.deadLetterByID(ctx, p.ID, fmt.Sprintf("exceeded %d delivery attempts", c.maxDeliveries))
			continue
		}

		claimed, err := c.rdb.XClaim(ctx, &redis.XClaimArgs{
			Stream: c.stream, Group: c.group, Consumer: c.consumerName,
			MinIdle: c.minIdle, Messages: []string{p.ID},
		}).Result()
		if err != nil {
			c.logger.Warn("XCLAIM failed", "id", p.ID, "error", err)
			continue
		}
		for _, msg := range claimed {
			c.process(ctx, msg)
		}
	}
}

func (c *Consumer) deadLetterByID(ctx context.Context, id string, reason string) {
	entries, err := c.rdb.XRange(ctx, c.stream, id, id).Result()
	if err != nil || len(entries) == 0 {
		c.logger.Error("could not read original message for dead-lettering", "id", id, "error", err)
		return
	}
	c.deadLetter(ctx, entries[0], reason)
}

func (c *Consumer) deadLetter(ctx context.Context, msg redis.XMessage, reason string) {
	values := make(map[string]interface{}, len(msg.Values)+2)
	for k, v := range msg.Values {
		values[k] = v
	}
	values["dlq_reason"] = reason
	values["dlq_original_id"] = msg.ID

	if err := c.rdb.XAdd(ctx, &redis.XAddArgs{Stream: c.dlqStream, Values: values}).Err(); err != nil {
		// Leave it un-ACKed rather than lose it: better to retry the DLQ
		// write later than silently drop a message we couldn't process.
		c.logger.Error("failed to write to DLQ, leaving original message pending", "id", msg.ID, "error", err)
		return
	}
	if err := c.rdb.XAck(ctx, c.stream, c.group, msg.ID).Err(); err != nil {
		c.logger.Error("failed to ack after dead-lettering", "id", msg.ID, "error", err)
	}
}

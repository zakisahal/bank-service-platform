package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/zsahal/bank-service/internal/events"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	redisAddr := getEnv("REDIS_ADDR", "localhost:6379")
	stream := getEnv("STREAM_NAME", "bank.events")
	group := getEnv("CONSUMER_GROUP", "audit-log")

	maxDeliveries := int64(5)
	if v := os.Getenv("MAX_DELIVERIES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			maxDeliveries = n
		}
	}

	// Kubernetes sets HOSTNAME to the pod name, which gives each replica a
	// distinct consumer identity within the group automatically - required
	// for XPENDING/XCLAIM to attribute in-flight messages correctly when
	// more than one replica is running.
	consumerName := getEnv("HOSTNAME", "consumer-local")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer pool.Close()

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	defer rdb.Close()

	c := events.New(rdb, pool, logger, stream, group, consumerName, maxDeliveries)
	return c.Run(ctx)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

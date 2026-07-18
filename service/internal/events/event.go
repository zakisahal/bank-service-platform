// Package events implements the audit-log consumer: an idempotent reader of
// the bank.events Redis Stream that turns each event into exactly one
// audit_log row, no matter how many times Redis redelivers it.
package events

import (
	"encoding/json"
	"fmt"
	"time"
)

// Event is the domain payload carried in each stream entry's "data" field.
// ID is the producer-assigned idempotency key - stable across redeliveries,
// unlike the Redis Stream entry ID (msg.ID), which is a new value every
// time a message is claimed/redelivered and can't be used for dedup.
type Event struct {
	ID         string          `json:"id"`
	AccountID  string          `json:"account_id"`
	Type       string          `json:"type"`
	Payload    json.RawMessage `json:"payload"`
	OccurredAt time.Time       `json:"occurred_at"`
}

// ParseEvent reads the stream entry's "data" field, expected to be a JSON
// object matching Event. Anything else - missing field, non-JSON, missing
// ID - is treated as permanently unparseable: no retry will ever fix it, so
// callers should dead-letter it immediately rather than leave it pending.
func ParseEvent(fields map[string]interface{}) (Event, error) {
	raw, ok := fields["data"]
	if !ok {
		return Event{}, fmt.Errorf("message has no \"data\" field")
	}
	s, ok := raw.(string)
	if !ok {
		return Event{}, fmt.Errorf("\"data\" field is not a string")
	}

	var ev Event
	if err := json.Unmarshal([]byte(s), &ev); err != nil {
		return Event{}, fmt.Errorf("invalid event JSON: %w", err)
	}
	if ev.ID == "" {
		return Event{}, fmt.Errorf("event is missing \"id\"")
	}
	return ev, nil
}

package comms

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/redis/go-redis/v9"
)

// MessageEvent is the payload published on a message create/edit/reaction change,
// broadcast to every API instance subscribed to the org's channel and forwarded to
// connected SSE clients that are authorized to see it.
type MessageEvent struct {
	Type        string      `json:"type"` // message.created | message.updated | message.reaction_added | message.reaction_removed
	HouseholdID string      `json:"household_id"`
	SwimmerID   string      `json:"swimmer_id"`
	Message     MessageView `json:"message"`
}

// MessageBroadcaster publishes/subscribes message events over Redis pub/sub so
// that multiple API instances (and every browser tab with an open SSE stream)
// observe new messages without polling. One channel per org keeps subscription
// management simple; the SSE handler filters events per-caller before forwarding.
type MessageBroadcaster struct {
	client *redis.Client
	logger *slog.Logger
}

// NewMessageBroadcaster connects to Redis using the same URL format Asynq uses
// (see server/internal/workers/redis_backend.go) and verifies connectivity.
func NewMessageBroadcaster(redisURL string, logger *slog.Logger) (*MessageBroadcaster, error) {
	if strings.TrimSpace(redisURL) == "" {
		return nil, fmt.Errorf("realtime: redis url is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	opt, err := redis.ParseURL(strings.TrimSpace(redisURL))
	if err != nil {
		return nil, fmt.Errorf("realtime: parse redis url: %w", err)
	}
	client := redis.NewClient(opt)
	if err := client.Ping(context.Background()).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("realtime: ping: %w", err)
	}
	return &MessageBroadcaster{client: client, logger: logger}, nil
}

func messageChannel(orgID string) string {
	return "msg:" + orgID
}

// Publish marshals and publishes an event to the org's channel. Errors are
// logged, not returned — a failed realtime push must never fail the underlying
// write (send/edit/react), since realtime is a best-effort enhancement.
func (b *MessageBroadcaster) Publish(ctx context.Context, orgID string, event MessageEvent) {
	if b == nil {
		return
	}
	payload, err := json.Marshal(event)
	if err != nil {
		b.logger.Error("realtime: marshal event failed", "err", err)
		return
	}
	if err := b.client.Publish(ctx, messageChannel(orgID), payload).Err(); err != nil {
		b.logger.Error("realtime: publish failed", "err", err)
	}
}

// Subscribe returns a Redis pub/sub subscription for the org's channel. Callers
// must call Close on the returned PubSub when done.
func (b *MessageBroadcaster) Subscribe(ctx context.Context, orgID string) *redis.PubSub {
	if b == nil {
		return nil
	}
	return b.client.Subscribe(ctx, messageChannel(orgID))
}

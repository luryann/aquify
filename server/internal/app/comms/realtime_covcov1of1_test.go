package comms

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// cov1xDeadRedis returns a client that can never connect (nothing listens on port 1).
func cov1xDeadRedis() *redis.Client {
	return redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1, DialTimeout: time.Second})
}

func cov1xLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestCov1xNewMessageBroadcasterRejectsBadConfig(t *testing.T) {
	cases := []struct {
		name, url, wantErr string
	}{
		{"empty", "", "redis url is required"},
		{"blank", "   ", "redis url is required"},
		{"bad scheme", "http://localhost:6379", "parse redis url"},
		{"unreachable", "redis://127.0.0.1:1/0?max_retries=0&dial_timeout=1s", "ping"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := NewMessageBroadcaster(tc.url, nil)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
			}
			if b != nil {
				t.Fatalf("expected nil broadcaster on error, got %+v", b)
			}
		})
	}
}

func TestCov1xNilBroadcasterIsNoop(t *testing.T) {
	var b *MessageBroadcaster
	b.Publish(context.Background(), "org", MessageEvent{Type: "message.created"}) // must not panic
	if sub := b.Subscribe(context.Background(), "org"); sub != nil {
		t.Fatalf("nil broadcaster Subscribe = %v, want nil", sub)
	}
	// A service with no broadcaster still persists and returns messages.
	s := NewService()
	if s.Broadcaster() != nil {
		t.Fatal("default broadcaster should be nil")
	}
	if _, err := s.SendMessage(CreateMessageInput{OrgID: "o", HouseholdID: "h", SwimmerID: "s", Body: "x", SenderUserID: "u", SenderRole: "coach"}); err != nil {
		t.Fatal(err)
	}
}

func TestCov1xPublishLogsMarshalFailure(t *testing.T) {
	var buf bytes.Buffer
	client := cov1xDeadRedis()
	defer client.Close()
	b := &MessageBroadcaster{client: client, logger: cov1xLogger(&buf)}
	// time.Time cannot marshal to JSON outside years 0..9999.
	bad := MessageEvent{Type: "message.created", Message: MessageView{Message: Message{CreatedAt: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}}}
	b.Publish(context.Background(), "org", bad)
	if !strings.Contains(buf.String(), "marshal event failed") {
		t.Fatalf("log = %q, want marshal failure", buf.String())
	}
	if strings.Contains(buf.String(), "publish failed") {
		t.Fatal("publish must not be attempted after a marshal failure")
	}
}

func TestCov1xPublishLogsPublishFailure(t *testing.T) {
	var buf bytes.Buffer
	client := cov1xDeadRedis()
	defer client.Close()
	b := &MessageBroadcaster{client: client, logger: cov1xLogger(&buf)}
	b.Publish(context.Background(), "org", MessageEvent{Type: "message.created"})
	if !strings.Contains(buf.String(), "publish failed") {
		t.Fatalf("log = %q, want publish failure", buf.String())
	}
}

func TestCov1xBroadcasterRoundTripOverRedis(t *testing.T) {
	url := os.Getenv("REDIS_URL")
	if url == "" {
		t.Skip("REDIS_URL is not configured")
	}
	b, err := NewMessageBroadcaster(url, nil)
	if err != nil {
		t.Fatalf("NewMessageBroadcaster: %v", err)
	}
	t.Cleanup(func() { _ = b.client.Close() })
	if b.logger != slog.Default() {
		t.Fatal("nil logger should fall back to slog.Default()")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	org := uuid.NewString() // unique channel: no cross-test interference
	sub := b.Subscribe(ctx, org)
	t.Cleanup(func() { _ = sub.Close() })
	if _, err := sub.Receive(ctx); err != nil { // subscription confirmation
		t.Fatalf("subscribe confirm: %v", err)
	}
	if got := sub.Channel(); got == nil {
		t.Fatal("no channel")
	}

	// A different org's channel must not see this event.
	other := b.Subscribe(ctx, uuid.NewString())
	t.Cleanup(func() { _ = other.Close() })
	if _, err := other.Receive(ctx); err != nil {
		t.Fatal(err)
	}

	want := MessageEvent{Type: "message.created", HouseholdID: "hh", SwimmerID: "sw", Message: MessageView{Message: Message{ID: "m1", Body: "hello"}}}
	b.Publish(ctx, org, want)

	select {
	case msg := <-sub.Channel():
		if msg.Channel != messageChannel(org) {
			t.Fatalf("channel = %q", msg.Channel)
		}
		var got MessageEvent
		if err := json.Unmarshal([]byte(msg.Payload), &got); err != nil {
			t.Fatal(err)
		}
		if got.Type != want.Type || got.HouseholdID != "hh" || got.SwimmerID != "sw" || got.Message.ID != "m1" || got.Message.Body != "hello" {
			t.Fatalf("event = %+v", got)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for the published event")
	}
	select {
	case msg := <-other.Channel():
		t.Fatalf("event leaked to another org: %+v", msg)
	default:
	}
}

func TestCov1xServicePublishesMessageEventsToBroadcaster(t *testing.T) {
	url := os.Getenv("REDIS_URL")
	if url == "" {
		t.Skip("REDIS_URL is not configured")
	}
	b, err := NewMessageBroadcaster(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.client.Close() })
	s := NewService()
	s.SetBroadcaster(b)
	if s.Broadcaster() != b {
		t.Fatal("SetBroadcaster did not attach the broadcaster")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	org := uuid.NewString()
	sub := b.Subscribe(ctx, org)
	t.Cleanup(func() { _ = sub.Close() })
	if _, err := sub.Receive(ctx); err != nil {
		t.Fatal(err)
	}

	m, err := s.SendMessage(CreateMessageInput{OrgID: org, HouseholdID: "h", SwimmerID: "s", Body: "hi", SenderUserID: "u1", SenderRole: "coach"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddReaction(org, m.ID, "u2", "👍"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.EditMessage(org, m.ID, "u1", "hi!"); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveReaction(org, m.ID, "u2", "👍"); err != nil {
		t.Fatal(err)
	}
	var types []string
	for len(types) < 4 {
		select {
		case msg := <-sub.Channel():
			var ev MessageEvent
			if err := json.Unmarshal([]byte(msg.Payload), &ev); err != nil {
				t.Fatal(err)
			}
			types = append(types, ev.Type)
		case <-ctx.Done():
			t.Fatalf("timed out; got %v", types)
		}
	}
	want := []string{"message.created", "message.reaction_added", "message.updated", "message.reaction_removed"}
	for i := range want {
		if types[i] != want[i] {
			t.Fatalf("events = %v, want %v", types, want)
		}
	}
}

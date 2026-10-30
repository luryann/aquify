package usaswimming

import (
	"context"
	"sync"
	"time"
)

// StubClient is an in-memory Client for development and tests. It is STUBBED
// data, never used in production wiring.
type StubClient struct {
	mu      sync.Mutex
	Times   map[string][]OfficialTime // by USA Swimming ID
	PingErr error
}

// NewStubClient returns a stub with a small canned dataset for ID "STUB0001".
func NewStubClient() *StubClient {
	d := func(s string) time.Time { t, _ := time.Parse("2006-01-02", s); return t }
	return &StubClient{Times: map[string][]OfficialTime{
		"STUB0001": {
			{Distance: 100, Stroke: "free", Course: "SCY", TimeMs: 58120, SwimDate: d("2026-01-17"), MeetName: "Stub Winter Invite"},
			{Distance: 50, Stroke: "fly", Course: "SCY", TimeMs: 29480, SwimDate: d("2026-02-21"), MeetName: "Stub Winter Invite"},
		},
	}}
}

// TestConnection returns PingErr.
func (s *StubClient) TestConnection(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.PingErr
}

// OfficialTimes returns canned times or ErrSwimmerNotFound.
func (s *StubClient) OfficialTimes(_ context.Context, id string) ([]OfficialTime, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.Times[id]
	if !ok {
		return nil, ErrSwimmerNotFound
	}
	return append([]OfficialTime(nil), t...), nil
}

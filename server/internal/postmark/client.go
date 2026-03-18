// Package postmark provides a Postmark email send client interface and a
// stub implementation for development/testing.
//
// Production note: wire the real HTTP client via NewHTTPClient. The stub
// records all sent messages in memory so tests can inspect them.
package postmark

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ─── Errors ───────────────────────────────────────────────────────────────────

var (
	ErrSendFailed    = errors.New("postmark: send failed")
	ErrInvalidConfig = errors.New("postmark: missing server token or from address")
)

// ─── Types ────────────────────────────────────────────────────────────────────

// SendEmailRequest represents a single outbound email.
type SendEmailRequest struct {
	// To is the recipient address (single address per call).
	To      string
	Subject string
	// HTMLBody is the HTML version of the email. At least one of HTMLBody or
	// TextBody must be non-empty.
	HTMLBody string
	// TextBody is the plain-text version of the email.
	TextBody string
	// FromName overrides the default sender name for this message.
	FromName string
	// FromEmail overrides the default sender address for this message.
	FromEmail string
	// Tag is an optional Postmark message stream / tag.
	Tag string
	// MessageStreamID is the Postmark message stream (default: "outbound").
	MessageStreamID string
}

// SendEmailResponse captures the provider-assigned message ID and delivery state.
type SendEmailResponse struct {
	// MessageID is the Postmark-assigned unique message identifier.
	MessageID string
	// SubmittedAt is the time reported by the provider.
	SubmittedAt time.Time
}

// ─── Interface ────────────────────────────────────────────────────────────────

// Client is the minimal Postmark interface required by the comms service.
type Client interface {
	SendEmail(ctx context.Context, req SendEmailRequest) (SendEmailResponse, error)
}

// ─── Stub client ──────────────────────────────────────────────────────────────

// StubClient is an in-memory Client used in tests and local development.
// It records all sent messages and generates deterministic fake message IDs.
type StubClient struct {
	mu       sync.Mutex
	messages []SentMessage
	counter  int
	// FailNext causes the next SendEmail call to return ErrSendFailed.
	FailNext bool
}

// SentMessage captures a single send request handled by the stub.
type SentMessage struct {
	MessageID string
	Req       SendEmailRequest
	SentAt    time.Time
}

// NewStubClient returns a new StubClient ready for use.
func NewStubClient() *StubClient {
	return &StubClient{}
}

// SendEmail records the request and returns a deterministic message ID.
func (s *StubClient) SendEmail(_ context.Context, req SendEmailRequest) (SendEmailResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.FailNext {
		s.FailNext = false
		return SendEmailResponse{}, ErrSendFailed
	}

	s.counter++
	msgID := fmt.Sprintf("stub-msg-%04d", s.counter)
	now := time.Now().UTC()
	s.messages = append(s.messages, SentMessage{
		MessageID: msgID,
		Req:       req,
		SentAt:    now,
	})
	return SendEmailResponse{MessageID: msgID, SubmittedAt: now}, nil
}

// Sent returns a snapshot of all messages sent so far (for test assertions).
func (s *StubClient) Sent() []SentMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SentMessage, len(s.messages))
	copy(out, s.messages)
	return out
}

// Reset clears the recorded messages and counter.
func (s *StubClient) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = nil
	s.counter = 0
}

// ─── Real HTTP client ─────────────────────────────────────────────────────────

// HTTPClient sends email via the Postmark REST API.
type HTTPClient struct {
	serverToken string
	fromName    string
	fromEmail   string
	httpClient  *http.Client
}

// Config holds the configuration required to build a real HTTPClient.
type Config struct {
	ServerToken string
	FromName    string
	FromEmail   string
	// HTTPTimeout overrides the default client timeout (10 s).
	HTTPTimeout time.Duration
}

const postmarkAPIBase = "https://api.postmarkapp.com"

// NewHTTPClient returns a production-ready Postmark client.
// Returns ErrInvalidConfig if the server token or from address is empty.
func NewHTTPClient(cfg Config) (*HTTPClient, error) {
	if strings.TrimSpace(cfg.ServerToken) == "" || strings.TrimSpace(cfg.FromEmail) == "" {
		return nil, ErrInvalidConfig
	}
	timeout := cfg.HTTPTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &HTTPClient{
		serverToken: cfg.ServerToken,
		fromName:    cfg.FromName,
		fromEmail:   cfg.FromEmail,
		httpClient:  &http.Client{Timeout: timeout},
	}, nil
}

// postmarkSendRequest is the Postmark REST API payload shape.
type postmarkSendRequest struct {
	From            string `json:"From"`
	To              string `json:"To"`
	Subject         string `json:"Subject"`
	HTMLBody        string `json:"HtmlBody,omitempty"`
	TextBody        string `json:"TextBody,omitempty"`
	Tag             string `json:"Tag,omitempty"`
	MessageStreamID string `json:"MessageStream,omitempty"`
}

type postmarkSendResponse struct {
	MessageID   string `json:"MessageID"`
	SubmittedAt string `json:"SubmittedAt"`
	ErrorCode   int    `json:"ErrorCode"`
	Message     string `json:"Message"`
}

// SendEmail sends a single email via the Postmark API.
func (c *HTTPClient) SendEmail(ctx context.Context, req SendEmailRequest) (SendEmailResponse, error) {
	fromEmail := req.FromEmail
	if fromEmail == "" {
		fromEmail = c.fromEmail
	}
	fromName := req.FromName
	if fromName == "" {
		fromName = c.fromName
	}

	from := fromEmail
	if fromName != "" {
		from = fmt.Sprintf("%s <%s>", fromName, fromEmail)
	}

	streamID := req.MessageStreamID
	if streamID == "" {
		streamID = "outbound"
	}

	payload := postmarkSendRequest{
		From:            from,
		To:              req.To,
		Subject:         req.Subject,
		HTMLBody:        req.HTMLBody,
		TextBody:        req.TextBody,
		Tag:             req.Tag,
		MessageStreamID: streamID,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return SendEmailResponse{}, fmt.Errorf("%w: marshal: %v", ErrSendFailed, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, postmarkAPIBase+"/email", bytes.NewReader(body))
	if err != nil {
		return SendEmailResponse{}, fmt.Errorf("%w: build request: %v", ErrSendFailed, err)
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Postmark-Server-Token", c.serverToken)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return SendEmailResponse{}, fmt.Errorf("%w: http: %v", ErrSendFailed, err)
	}
	defer resp.Body.Close()

	var pmResp postmarkSendResponse
	if err := json.NewDecoder(resp.Body).Decode(&pmResp); err != nil {
		return SendEmailResponse{}, fmt.Errorf("%w: decode response: %v", ErrSendFailed, err)
	}
	if pmResp.ErrorCode != 0 {
		return SendEmailResponse{}, fmt.Errorf("%w: provider error %d: %s", ErrSendFailed, pmResp.ErrorCode, pmResp.Message)
	}

	submittedAt := time.Now().UTC()
	if t, err := time.Parse(time.RFC3339, pmResp.SubmittedAt); err == nil {
		submittedAt = t
	}

	return SendEmailResponse{
		MessageID:   pmResp.MessageID,
		SubmittedAt: submittedAt,
	}, nil
}

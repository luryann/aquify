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
	// Metadata is attached to the Postmark message for webhook correlation.
	Metadata map[string]string
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
	Ready(ctx context.Context) error
	SendEmail(ctx context.Context, req SendEmailRequest) (SendEmailResponse, error)
	// SendBatch sends up to MaxBatchSize messages in one call. The returned
	// slice has one result per request, in order. A non-nil error means the
	// whole batch failed (transport/auth); per-message failures are in Err.
	SendBatch(ctx context.Context, reqs []SendEmailRequest) ([]BatchResult, error)
}

// MaxBatchSize is Postmark's per-call limit for /email/batch.
const MaxBatchSize = 500

// callTimeout bounds every Postmark HTTP call (Ready, SendEmail, SendBatch). It
// is kept well under the API server WriteTimeout (60s) so a hung provider
// surfaces as an error response instead of a dropped client connection.
const callTimeout = 20 * time.Second

// BatchResult is the outcome of one message within a SendBatch call.
type BatchResult struct {
	MessageID   string
	SubmittedAt time.Time
	// Err is non-nil (wrapping ErrSendFailed) when this message was rejected.
	Err error
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

// Ready reports the stub as available for local/test environments.
func (s *StubClient) Ready(_ context.Context) error {
	return nil
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

// SendBatch records each request like SendEmail. FailNext fails the first one.
func (s *StubClient) SendBatch(ctx context.Context, reqs []SendEmailRequest) ([]BatchResult, error) {
	if len(reqs) > MaxBatchSize {
		return nil, fmt.Errorf("%w: batch of %d exceeds %d", ErrSendFailed, len(reqs), MaxBatchSize)
	}
	out := make([]BatchResult, len(reqs))
	for i, r := range reqs {
		resp, err := s.SendEmail(ctx, r)
		out[i] = BatchResult{MessageID: resp.MessageID, SubmittedAt: resp.SubmittedAt, Err: err}
	}
	return out, nil
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
	if timeout > callTimeout {
		timeout = callTimeout
	}
	return &HTTPClient{
		serverToken: cfg.ServerToken,
		fromName:    cfg.FromName,
		fromEmail:   cfg.FromEmail,
		httpClient:  &http.Client{Timeout: timeout},
	}, nil
}

// Ready performs a lightweight authenticated probe against the Postmark API.
func (c *HTTPClient) Ready(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, postmarkAPIBase+"/server", nil)
	if err != nil {
		return fmt.Errorf("postmark: build readiness request: %w", err)
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("X-Postmark-Server-Token", c.serverToken)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("postmark: readiness http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return nil
	}

	var apiErr struct {
		ErrorCode int    `json:"ErrorCode"`
		Message   string `json:"Message"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&apiErr)

	if apiErr.Message != "" {
		return fmt.Errorf("postmark: readiness status %d: %s", resp.StatusCode, apiErr.Message)
	}
	return fmt.Errorf("postmark: readiness status %d", resp.StatusCode)
}

// postmarkSendRequest is the Postmark REST API payload shape.
type postmarkSendRequest struct {
	From            string            `json:"From"`
	To              string            `json:"To"`
	Subject         string            `json:"Subject"`
	HTMLBody        string            `json:"HtmlBody,omitempty"`
	TextBody        string            `json:"TextBody,omitempty"`
	Tag             string            `json:"Tag,omitempty"`
	MessageStreamID string            `json:"MessageStream,omitempty"`
	Metadata        map[string]string `json:"Metadata,omitempty"`
}

type postmarkSendResponse struct {
	MessageID   string `json:"MessageID"`
	SubmittedAt string `json:"SubmittedAt"`
	ErrorCode   int    `json:"ErrorCode"`
	Message     string `json:"Message"`
}

// buildPayload converts a SendEmailRequest into the Postmark wire shape.
func (c *HTTPClient) buildPayload(req SendEmailRequest) postmarkSendRequest {
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

	return postmarkSendRequest{
		From:            from,
		To:              req.To,
		Subject:         req.Subject,
		HTMLBody:        req.HTMLBody,
		TextBody:        req.TextBody,
		Tag:             req.Tag,
		MessageStreamID: streamID,
		Metadata:        req.Metadata,
	}
}

// SendEmail sends a single email via the Postmark API.
func (c *HTTPClient) SendEmail(ctx context.Context, req SendEmailRequest) (SendEmailResponse, error) {
	payload := c.buildPayload(req)
	body, err := json.Marshal(payload)
	if err != nil {
		return SendEmailResponse{}, fmt.Errorf("%w: marshal: %v", ErrSendFailed, err)
	}

	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
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
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_ = json.NewDecoder(resp.Body).Decode(&pmResp)
		return SendEmailResponse{}, fmt.Errorf("%w: http status %d: %s", ErrSendFailed, resp.StatusCode, pmResp.Message)
	}
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

// SendBatch sends up to MaxBatchSize messages via POST /email/batch.
func (c *HTTPClient) SendBatch(ctx context.Context, reqs []SendEmailRequest) ([]BatchResult, error) {
	if len(reqs) == 0 {
		return nil, nil
	}
	if len(reqs) > MaxBatchSize {
		return nil, fmt.Errorf("%w: batch of %d exceeds %d", ErrSendFailed, len(reqs), MaxBatchSize)
	}
	payload := make([]postmarkSendRequest, len(reqs))
	for i, r := range reqs {
		payload[i] = c.buildPayload(r)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal: %v", ErrSendFailed, err)
	}
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, postmarkAPIBase+"/email/batch", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %v", ErrSendFailed, err)
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Postmark-Server-Token", c.serverToken)

	// Batches carry up to 500 messages, so allow the full call budget rather
	// than the shorter default (shallow copy keeps the configured transport).
	hc := *c.httpClient
	hc.Timeout = callTimeout
	resp, err := hc.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: http: %v", ErrSendFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		var apiErr postmarkSendResponse
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		return nil, fmt.Errorf("%w: http status %d: %s", ErrSendFailed, resp.StatusCode, apiErr.Message)
	}
	var items []postmarkSendResponse
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("%w: decode response: %v", ErrSendFailed, err)
	}
	if len(items) != len(reqs) {
		return nil, fmt.Errorf("%w: batch response has %d results for %d messages", ErrSendFailed, len(items), len(reqs))
	}
	out := make([]BatchResult, len(items))
	for i, it := range items {
		if it.ErrorCode != 0 {
			out[i].Err = fmt.Errorf("%w: provider error %d: %s", ErrSendFailed, it.ErrorCode, it.Message)
			continue
		}
		submittedAt := time.Now().UTC()
		if t, err := time.Parse(time.RFC3339, it.SubmittedAt); err == nil {
			submittedAt = t
		}
		out[i] = BatchResult{MessageID: it.MessageID, SubmittedAt: submittedAt}
	}
	return out, nil
}

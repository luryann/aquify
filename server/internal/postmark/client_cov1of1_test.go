package postmark

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func cov1xClient(t *testing.T, cfg Config, fn roundTripFunc) *HTTPClient {
	t.Helper()
	c, err := NewHTTPClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	c.httpClient.Transport = fn
	return c
}

func cov1xResp(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}
}

func TestCov1xStubClient(t *testing.T) {
	s := NewStubClient()
	ctx := context.Background()
	if err := s.Ready(ctx); err != nil {
		t.Fatal(err)
	}
	r1, err := s.SendEmail(ctx, SendEmailRequest{To: "a@example.com"})
	if err != nil || r1.MessageID != "stub-msg-0001" {
		t.Fatalf("r1=%+v err=%v", r1, err)
	}
	s.FailNext = true
	if _, err := s.SendEmail(ctx, SendEmailRequest{To: "b@example.com"}); !errors.Is(err, ErrSendFailed) {
		t.Fatalf("err=%v", err)
	}
	if s.FailNext {
		t.Fatal("FailNext should reset")
	}
	s.FailNext = true
	res, err := s.SendBatch(ctx, []SendEmailRequest{{To: "c@example.com"}, {To: "d@example.com"}})
	if err != nil || len(res) != 2 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if !errors.Is(res[0].Err, ErrSendFailed) || res[1].Err != nil || res[1].MessageID != "stub-msg-0002" {
		t.Fatalf("res=%+v", res)
	}
	sent := s.Sent()
	if len(sent) != 2 || sent[1].Req.To != "d@example.com" {
		t.Fatalf("sent=%+v", sent)
	}
	sent[0].Req.To = "mutated"
	if s.Sent()[0].Req.To != "a@example.com" {
		t.Fatal("Sent must return a copy")
	}
	if _, err := s.SendBatch(ctx, make([]SendEmailRequest, MaxBatchSize+1)); !errors.Is(err, ErrSendFailed) {
		t.Fatalf("err=%v", err)
	}
	s.Reset()
	if len(s.Sent()) != 0 {
		t.Fatal("Reset should clear messages")
	}
	r, _ := s.SendEmail(ctx, SendEmailRequest{})
	if r.MessageID != "stub-msg-0001" {
		t.Fatalf("counter not reset: %s", r.MessageID)
	}
}

func TestCov1xNewHTTPClientTimeouts(t *testing.T) {
	if _, err := NewHTTPClient(Config{ServerToken: " ", FromEmail: "a@example.com"}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("err=%v", err)
	}
	if _, err := NewHTTPClient(Config{ServerToken: "t", FromEmail: ""}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("err=%v", err)
	}
	for _, tc := range []struct{ in, want time.Duration }{
		{0, 10 * time.Second},
		{5 * time.Second, 5 * time.Second},
		{time.Hour, callTimeout},
	} {
		c, err := NewHTTPClient(Config{ServerToken: "t", FromEmail: "a@example.com", HTTPTimeout: tc.in})
		if err != nil || c.httpClient.Timeout != tc.want {
			t.Fatalf("in=%v got=%v err=%v", tc.in, c.httpClient.Timeout, err)
		}
	}
}

func TestCov1xReady(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		status           int
	}{
		{"ok", `{}`, "", 200},
		{"message", `{"Message":"bad token"}`, "readiness status 401: bad token", 401},
		{"no message", `oops`, "readiness status 500", 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := cov1xClient(t, Config{ServerToken: "tok", FromEmail: "a@example.com"}, func(r *http.Request) (*http.Response, error) {
				if r.Method != http.MethodGet || r.URL.String() != postmarkAPIBase+"/server" || r.Header.Get("X-Postmark-Server-Token") != "tok" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL)
				}
				return cov1xResp(tc.status, tc.body), nil
			})
			err := c.Ready(context.Background())
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || err.Error() != "postmark: "+tc.want {
				t.Fatalf("err=%v", err)
			}
		})
	}
	c := cov1xClient(t, Config{ServerToken: "tok", FromEmail: "a@example.com"}, func(*http.Request) (*http.Response, error) {
		return nil, errors.New("boom")
	})
	if err := c.Ready(context.Background()); err == nil || !strings.Contains(err.Error(), "readiness http") {
		t.Fatalf("err=%v", err)
	}
}

func TestCov1xBuildPayload(t *testing.T) {
	c, _ := NewHTTPClient(Config{ServerToken: "t", FromEmail: "default@example.com", FromName: "Club"})
	p := c.buildPayload(SendEmailRequest{To: "x@example.com"})
	if p.From != "Club <default@example.com>" || p.MessageStreamID != "outbound" {
		t.Fatalf("%+v", p)
	}
	p = c.buildPayload(SendEmailRequest{FromEmail: "o@example.com", FromName: "Other", MessageStreamID: "broadcast"})
	if p.From != "Other <o@example.com>" || p.MessageStreamID != "broadcast" {
		t.Fatalf("%+v", p)
	}
	c2, _ := NewHTTPClient(Config{ServerToken: "t", FromEmail: "default@example.com"})
	if p := c2.buildPayload(SendEmailRequest{}); p.From != "default@example.com" {
		t.Fatalf("%+v", p)
	}
}

func TestCov1xSendEmailPaths(t *testing.T) {
	var gotBody postmarkSendRequest
	c := cov1xClient(t, Config{ServerToken: "tok", FromEmail: "a@example.com"}, func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != postmarkAPIBase+"/email" || r.Header.Get("X-Postmark-Server-Token") != "tok" {
			t.Errorf("bad request %s", r.URL)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		return cov1xResp(200, `{"MessageID":"m1","SubmittedAt":"2026-01-02T03:04:05Z"}`), nil
	})
	got, err := c.SendEmail(context.Background(), SendEmailRequest{To: "r@example.com", Subject: "Hi"})
	if err != nil || got.MessageID != "m1" || !got.SubmittedAt.Equal(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if gotBody.To != "r@example.com" || gotBody.Subject != "Hi" {
		t.Fatalf("%+v", gotBody)
	}

	c.httpClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return cov1xResp(200, `{"MessageID":"m2","SubmittedAt":"not-a-time"}`), nil
	})
	before := time.Now().UTC().Add(-time.Second)
	got, err = c.SendEmail(context.Background(), SendEmailRequest{})
	if err != nil || got.MessageID != "m2" || got.SubmittedAt.Before(before) {
		t.Fatalf("got=%+v err=%v", got, err)
	}

	c.httpClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return cov1xResp(200, `not json`), nil
	})
	if _, err = c.SendEmail(context.Background(), SendEmailRequest{}); !errors.Is(err, ErrSendFailed) || !strings.Contains(err.Error(), "decode response") {
		t.Fatalf("err=%v", err)
	}

	c.httpClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network down")
	})
	if _, err = c.SendEmail(context.Background(), SendEmailRequest{}); !errors.Is(err, ErrSendFailed) || !strings.Contains(err.Error(), "network down") {
		t.Fatalf("err=%v", err)
	}
}

func TestCov1xSendBatchPaths(t *testing.T) {
	c := cov1xClient(t, Config{ServerToken: "tok", FromEmail: "a@example.com"}, func(*http.Request) (*http.Response, error) {
		t.Error("no request expected")
		return nil, errors.New("unexpected")
	})
	if res, err := c.SendBatch(context.Background(), nil); res != nil || err != nil {
		t.Fatalf("res=%v err=%v", res, err)
	}
	if _, err := c.SendBatch(context.Background(), make([]SendEmailRequest, MaxBatchSize+1)); !errors.Is(err, ErrSendFailed) || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("err=%v", err)
	}

	reqs := []SendEmailRequest{{To: "a@example.com"}, {To: "b@example.com"}}
	run := func(fn roundTripFunc) ([]BatchResult, error) {
		c.httpClient.Transport = fn
		return c.SendBatch(context.Background(), reqs)
	}

	if _, err := run(func(*http.Request) (*http.Response, error) { return nil, errors.New("down") }); !errors.Is(err, ErrSendFailed) || !strings.Contains(err.Error(), "down") {
		t.Fatalf("err=%v", err)
	}
	if _, err := run(func(*http.Request) (*http.Response, error) {
		return cov1xResp(401, `{"Message":"bad token"}`), nil
	}); !errors.Is(err, ErrSendFailed) || !strings.Contains(err.Error(), "http status 401: bad token") {
		t.Fatalf("err=%v", err)
	}
	if _, err := run(func(*http.Request) (*http.Response, error) { return cov1xResp(200, `nope`), nil }); !errors.Is(err, ErrSendFailed) || !strings.Contains(err.Error(), "decode response") {
		t.Fatalf("err=%v", err)
	}
	if _, err := run(func(*http.Request) (*http.Response, error) { return cov1xResp(200, `[{"MessageID":"x"}]`), nil }); !errors.Is(err, ErrSendFailed) || !strings.Contains(err.Error(), "1 results for 2 messages") {
		t.Fatalf("err=%v", err)
	}

	res, err := run(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != postmarkAPIBase+"/email/batch" {
			t.Errorf("url=%s", r.URL)
		}
		return cov1xResp(200, `[{"MessageID":"m1","SubmittedAt":"2026-01-02T03:04:05Z"},{"ErrorCode":406,"Message":"inactive"}]`), nil
	})
	if err != nil || len(res) != 2 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if res[0].MessageID != "m1" || res[0].Err != nil || res[0].SubmittedAt.Year() != 2026 {
		t.Fatalf("res[0]=%+v", res[0])
	}
	if !errors.Is(res[1].Err, ErrSendFailed) || !strings.Contains(res[1].Err.Error(), "provider error 406: inactive") {
		t.Fatalf("res[1]=%+v", res[1])
	}

	res, err = run(func(*http.Request) (*http.Response, error) {
		return cov1xResp(200, `[{"MessageID":"m1","SubmittedAt":"bad"},{"MessageID":"m2"}]`), nil
	})
	if err != nil || res[0].SubmittedAt.IsZero() || res[1].MessageID != "m2" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

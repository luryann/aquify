package postmark

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSendEmailHTTPStatus(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		body    string
		wantErr bool
	}{
		{"success", 200, `{"MessageID":"msg-1","ErrorCode":0}`, false},
		{"server failure without provider code", 503, `{"Message":"unavailable"}`, true},
		{"server failure with empty JSON", 500, `{}`, true},
		{"plain text failure", 502, `bad gateway`, true},
		{"provider failure", 200, `{"ErrorCode":300,"Message":"invalid email"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := NewHTTPClient(Config{ServerToken: "test", FromEmail: "sender@example.com"})
			if err != nil {
				t.Fatal(err)
			}
			c.httpClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			got, err := c.SendEmail(context.Background(), SendEmailRequest{To: "recipient@example.com", TextBody: "hello"})
			if tc.wantErr {
				if !errors.Is(err, ErrSendFailed) {
					t.Fatalf("expected send failure, got response=%+v err=%v", got, err)
				}
			} else if err != nil || got.MessageID != "msg-1" {
				t.Fatalf("response=%+v err=%v", got, err)
			}
		})
	}
}

func TestSendBatchMapsPerMessageResults(t *testing.T) {
	c, _ := NewHTTPClient(Config{ServerToken: "test", FromEmail: "sender@example.com"})
	var gotPath string
	c.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotPath = r.URL.Path
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(
			`[{"To":"a@example.com","MessageID":"m1","ErrorCode":0,"Message":"OK"},{"To":"b@@x","ErrorCode":300,"Message":"Illegal email address"}]`))}, nil
	})
	res, err := c.SendBatch(context.Background(), []SendEmailRequest{{To: "a@example.com", TextBody: "x"}, {To: "b@@x", TextBody: "x"}})
	if err != nil || len(res) != 2 {
		t.Fatalf("res=%v err=%v", res, err)
	}
	if gotPath != "/email/batch" || res[0].MessageID != "m1" || res[0].Err != nil || !errors.Is(res[1].Err, ErrSendFailed) {
		t.Fatalf("path=%s res=%+v", gotPath, res)
	}
}

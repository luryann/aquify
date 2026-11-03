package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
)

type storageTransport func(*http.Request) (*http.Response, error)

func (f storageTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestS3ReadyRequiresExistingBucket(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusNotFound, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			client, err := minio.New("storage.example.com", &minio.Options{Region: "us-east-1", Transport: storageTransport(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: status, Status: http.StatusText(status), Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
			})})
			if err != nil {
				t.Fatal(err)
			}
			s := &S3Storage{client: client, bucket: "test-bucket"}
			err = s.Ready(context.Background())
			if (err == nil) != (status == http.StatusOK) {
				t.Fatalf("status=%d readiness error=%v", status, err)
			}
		})
	}
}

func TestS3PresignRejectsTTLOverSevenDays(t *testing.T) {
	client, err := minio.New("storage.example.com", &minio.Options{Region: "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}
	s := &S3Storage{client: client, bucket: "bucket"}
	if _, err := s.PresignedGetURL("k", MaxPresignTTL+time.Second); !errors.Is(err, ErrPresignTTLTooLong) {
		t.Fatalf("get err = %v, want ErrPresignTTLTooLong", err)
	}
	if _, err := s.PresignedPutURL("k", 30*24*time.Hour); !errors.Is(err, ErrPresignTTLTooLong) {
		t.Fatalf("put err = %v, want ErrPresignTTLTooLong", err)
	}
	if _, err := s.PresignedGetURL("k", MaxPresignTTL); err != nil {
		t.Fatalf("7d ttl should be allowed: %v", err)
	}
}

func TestIsUnavailable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"missing", ErrObjectNotFound, false},
		{"deadline", fmt.Errorf("stat: %w", context.DeadlineExceeded), true},
		{"net op", fmt.Errorf("stat: %w", &net.OpError{Op: "dial", Err: errors.New("refused")}), true},
		{"503", fmt.Errorf("stat: %w", minio.ErrorResponse{StatusCode: 503, Code: "SlowDown"}), true},
		{"403", fmt.Errorf("stat: %w", minio.ErrorResponse{StatusCode: 403, Code: "AccessDenied"}), false},
		{"other", errors.New("boom"), false},
	}
	for _, c := range cases {
		if got := IsUnavailable(c.err); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

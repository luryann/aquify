package storage

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

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

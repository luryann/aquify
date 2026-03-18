// Package storage provides an object-storage abstraction over S3-compatible backends
// (MinIO in local dev; AWS S3 or equivalent in production).
package storage

import (
	"fmt"
	"time"
)

// ObjectStorage is the interface satisfied by both the real MinIO/S3 client and the
// in-memory stub used in tests and early development.
type ObjectStorage interface {
	// PresignedPutURL returns a presigned URL that a client may use to PUT an object
	// directly into the bucket.  The URL expires after ttl.
	PresignedPutURL(key string, ttl time.Duration) (string, error)

	// PresignedGetURL returns a presigned URL that a client may use to GET an object
	// from the bucket.  The URL expires after ttl.
	PresignedGetURL(key string, ttl time.Duration) (string, error)

	// Delete removes an object from the bucket.  Idempotent — no error if the
	// object does not exist.
	Delete(key string) error
}

// ─── In-memory stub ──────────────────────────────────────────────────────────

// StubStorage is a no-op implementation of ObjectStorage for local development
// and tests.  It does not perform real network I/O.  Presigned URLs are
// deterministic fake strings that embed the object key.
type StubStorage struct {
	baseURL string // e.g. "http://localhost:9000/aquify"
}

// NewStubStorage returns a StubStorage whose presigned URLs are rooted at baseURL.
// Pass an empty string to use the default local MinIO URL.
func NewStubStorage(baseURL string) *StubStorage {
	if baseURL == "" {
		baseURL = "http://localhost:9000/aquify"
	}
	return &StubStorage{baseURL: baseURL}
}

// PresignedPutURL returns a fake upload URL for the given key.
func (s *StubStorage) PresignedPutURL(key string, ttl time.Duration) (string, error) {
	expires := time.Now().Add(ttl).Unix()
	return fmt.Sprintf("%s/%s?X-Stub-Method=PUT&X-Stub-Expires=%d", s.baseURL, key, expires), nil
}

// PresignedGetURL returns a fake download URL for the given key.
func (s *StubStorage) PresignedGetURL(key string, ttl time.Duration) (string, error) {
	expires := time.Now().Add(ttl).Unix()
	return fmt.Sprintf("%s/%s?X-Stub-Method=GET&X-Stub-Expires=%d", s.baseURL, key, expires), nil
}

// Delete is a no-op in the stub.
func (s *StubStorage) Delete(_ string) error {
	return nil
}

// ─── Config helpers ───────────────────────────────────────────────────────────

// Config holds the environment-sourced settings for the object storage client.
type Config struct {
	Endpoint  string // e.g. "localhost:9000"
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

// BaseURL returns the base HTTP(S) URL for the stub.
func (c Config) BaseURL() string {
	scheme := "http"
	if c.UseSSL {
		scheme = "https"
	}
	if c.Endpoint == "" {
		return "http://localhost:9000/aquify"
	}
	return fmt.Sprintf("%s://%s/%s", scheme, c.Endpoint, c.Bucket)
}

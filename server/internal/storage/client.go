// Package storage provides an object-storage abstraction over S3-compatible
// backends (MinIO in local dev; AWS S3 or equivalent in production).
package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var ErrObjectNotFound = errors.New("object not found")

// maxGetObjectBytes bounds GetObject, which loads the whole object into memory
// (reports and exports are small; larger downloads should use a presigned URL).
const maxGetObjectBytes = 64 << 20

// ObjectStorage is the interface satisfied by both the real MinIO/S3 client and
// the in-memory stub used in tests and early development.
type ObjectStorage interface {
	PresignedPutURL(key string, ttl time.Duration) (string, error)
	PresignedGetURL(key string, ttl time.Duration) (string, error)
	PutObject(ctx context.Context, key string, contentType string, data []byte) error
	GetObject(ctx context.Context, key string) ([]byte, string, error)
	Exists(ctx context.Context, key string) (bool, error)
	// ObjectSize returns the actual stored byte size of key, independent of
	// any caller-declared metadata, so callers can enforce size caps before
	// reading the full object into memory.
	ObjectSize(ctx context.Context, key string) (int64, error)
	Delete(key string) error
}

// Config holds the environment-sourced settings for the object storage client.
type Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
	Region    string
}

func (c Config) BaseURL() string {
	scheme := "http"
	if c.UseSSL {
		scheme = "https"
	}

	endpoint := strings.TrimSpace(c.Endpoint)
	if endpoint == "" {
		endpoint = "localhost:9000"
	}
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")

	bucket := strings.TrimSpace(c.Bucket)
	if bucket == "" {
		bucket = "aquify"
	}
	return fmt.Sprintf("%s://%s/%s", scheme, endpoint, bucket)
}

func (c Config) Enabled() bool {
	return strings.TrimSpace(c.Endpoint) != "" &&
		strings.TrimSpace(c.AccessKey) != "" &&
		strings.TrimSpace(c.SecretKey) != "" &&
		strings.TrimSpace(c.Bucket) != ""
}

func NewClient(cfg Config) (ObjectStorage, error) {
	if !cfg.Enabled() {
		return NewStubStorage(cfg.BaseURL()), nil
	}
	return NewS3Storage(cfg)
}

// StubStorage is an in-memory implementation of ObjectStorage for tests and
// development flows that do not have S3 credentials available.
type StubStorage struct {
	baseURL string
	mu      sync.RWMutex
	objects map[string]storedObject
}

type storedObject struct {
	contentType string
	data        []byte
}

func NewStubStorage(baseURL string) *StubStorage {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "http://localhost:9000/aquify"
	}
	return &StubStorage{
		baseURL: strings.TrimRight(baseURL, "/"),
		objects: make(map[string]storedObject),
	}
}

func (s *StubStorage) PresignedPutURL(key string, ttl time.Duration) (string, error) {
	expires := time.Now().Add(ttl).Unix()
	return fmt.Sprintf("%s/%s?X-Stub-Method=PUT&X-Stub-Expires=%d", s.baseURL, url.PathEscape(strings.TrimLeft(key, "/")), expires), nil
}

func (s *StubStorage) PresignedGetURL(key string, ttl time.Duration) (string, error) {
	expires := time.Now().Add(ttl).Unix()
	return fmt.Sprintf("%s/%s?X-Stub-Method=GET&X-Stub-Expires=%d", s.baseURL, url.PathEscape(strings.TrimLeft(key, "/")), expires), nil
}

func (s *StubStorage) PutObject(_ context.Context, key string, contentType string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = storedObject{
		contentType: strings.TrimSpace(contentType),
		data:        append([]byte{}, data...),
	}
	return nil
}

func (s *StubStorage) GetObject(_ context.Context, key string) ([]byte, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	obj, ok := s.objects[key]
	if !ok {
		return nil, "", ErrObjectNotFound
	}
	return append([]byte{}, obj.data...), obj.contentType, nil
}

func (s *StubStorage) Exists(_ context.Context, key string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.objects[key]
	return ok, nil
}

func (s *StubStorage) ObjectSize(_ context.Context, key string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	obj, ok := s.objects[key]
	if !ok {
		return 0, ErrObjectNotFound
	}
	return int64(len(obj.data)), nil
}

func (s *StubStorage) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, key)
	return nil
}

func (s *StubStorage) Ready(_ context.Context) error {
	return nil
}

type S3Storage struct {
	client *minio.Client
	bucket string
}

func NewS3Storage(cfg Config) (*S3Storage, error) {
	endpoint := strings.TrimSpace(cfg.Endpoint)
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: strings.TrimSpace(cfg.Region),
	})
	if err != nil {
		return nil, fmt.Errorf("create s3 client: %w", err)
	}

	store := &S3Storage{
		client: client,
		bucket: strings.TrimSpace(cfg.Bucket),
	}
	if err := store.ensureBucket(context.Background(), strings.TrimSpace(cfg.Region)); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *S3Storage) ensureBucket(ctx context.Context, region string) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("check bucket %s: %w", s.bucket, err)
	}
	if exists {
		return nil
	}
	if err := s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{Region: region}); err != nil {
		exists, existsErr := s.client.BucketExists(ctx, s.bucket)
		if existsErr == nil && exists {
			return nil
		}
		return fmt.Errorf("create bucket %s: %w", s.bucket, err)
	}
	return nil
}

func (s *S3Storage) PresignedPutURL(key string, ttl time.Duration) (string, error) {
	u, err := s.client.PresignedPutObject(context.Background(), s.bucket, key, ttl)
	if err != nil {
		return "", fmt.Errorf("presign put %s: %w", key, err)
	}
	return u.String(), nil
}

func (s *S3Storage) PresignedGetURL(key string, ttl time.Duration) (string, error) {
	u, err := s.client.PresignedGetObject(context.Background(), s.bucket, key, ttl, nil)
	if err != nil {
		return "", fmt.Errorf("presign get %s: %w", key, err)
	}
	return u.String(), nil
}

func (s *S3Storage) PutObject(ctx context.Context, key string, contentType string, data []byte) error {
	reader := bytes.NewReader(data)
	_, err := s.client.PutObject(ctx, s.bucket, key, reader, int64(len(data)), minio.PutObjectOptions{
		ContentType: strings.TrimSpace(contentType),
	})
	if err != nil {
		return fmt.Errorf("put object %s: %w", key, err)
	}
	return nil
}

func (s *S3Storage) GetObject(ctx context.Context, key string) ([]byte, string, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, "", fmt.Errorf("get object %s: %w", key, err)
	}
	defer obj.Close()

	info, err := obj.Stat()
	if err != nil {
		if isMinIOObjectMissing(err) {
			return nil, "", ErrObjectNotFound
		}
		return nil, "", fmt.Errorf("stat object %s: %w", key, err)
	}

	if info.Size > maxGetObjectBytes {
		return nil, "", fmt.Errorf("object %s is %d bytes, over the %d byte in-memory read limit", key, info.Size, int64(maxGetObjectBytes))
	}
	data, err := io.ReadAll(io.LimitReader(obj, maxGetObjectBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("read object %s: %w", key, err)
	}
	// Stat's size can be stale (object replaced mid-read); the extra byte read
	// above exposes an overflow that must not be returned silently truncated.
	if int64(len(data)) > maxGetObjectBytes {
		return nil, "", fmt.Errorf("object %s exceeds the %d byte in-memory read limit", key, int64(maxGetObjectBytes))
	}
	return data, info.ContentType, nil
}

func (s *S3Storage) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err == nil {
		return true, nil
	}
	if isMinIOObjectMissing(err) {
		return false, nil
	}
	return false, fmt.Errorf("stat object %s: %w", key, err)
}

func (s *S3Storage) ObjectSize(ctx context.Context, key string) (int64, error) {
	info, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		if isMinIOObjectMissing(err) {
			return 0, ErrObjectNotFound
		}
		return 0, fmt.Errorf("stat object %s: %w", key, err)
	}
	return info.Size, nil
}

func (s *S3Storage) Delete(key string) error {
	err := s.client.RemoveObject(context.Background(), s.bucket, key, minio.RemoveObjectOptions{})
	if err != nil && !isMinIOObjectMissing(err) {
		return fmt.Errorf("delete object %s: %w", key, err)
	}
	return nil
}

func (s *S3Storage) Ready(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("bucket probe failed: %w", err)
	}
	if !exists {
		return fmt.Errorf("bucket %s does not exist", s.bucket)
	}
	return nil
}

func isMinIOObjectMissing(err error) bool {
	resp := minio.ToErrorResponse(err)
	switch resp.Code {
	case "NoSuchKey", "NoSuchObject", "NotFound":
		return true
	default:
		return false
	}
}

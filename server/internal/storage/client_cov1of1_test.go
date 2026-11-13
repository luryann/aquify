package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
)

// cov1xFakeS3 is a minimal path-style S3 server (the real HTTP boundary of the
// minio client) used to drive S3Storage end to end.
type cov1xFakeS3 struct {
	mu      sync.Mutex
	bucket  string
	objects map[string][]byte
	types   map[string]string
}

func cov1xS3Error(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>%s</Code><Message>%s</Message></Error>`, code, code)
}

func (f *cov1xFakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	path := strings.TrimPrefix(r.URL.Path, "/")
	bucket, key, _ := strings.Cut(path, "/")
	if bucket != f.bucket {
		cov1xS3Error(w, 404, "NoSuchBucket")
		return
	}
	if key == "" {
		switch r.Method {
		case http.MethodHead:
			w.WriteHeader(200)
		case http.MethodGet:
			prefix := r.URL.Query().Get("prefix")
			var keys []string
			for k := range f.objects {
				if strings.HasPrefix(k, prefix) {
					keys = append(keys, k)
				}
			}
			sort.Strings(keys)
			w.Header().Set("Content-Type", "application/xml")
			var b strings.Builder
			b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><ListBucketResult><Name>` + f.bucket + `</Name><IsTruncated>false</IsTruncated>`)
			for _, k := range keys {
				fmt.Fprintf(&b, `<Contents><Key>%s</Key><Size>%d</Size></Contents>`, k, len(f.objects[k]))
			}
			b.WriteString(`</ListBucketResult>`)
			io.WriteString(w, b.String())
		}
		return
	}
	switch r.Method {
	case http.MethodPut:
		data, _ := io.ReadAll(r.Body)
		data = cov1xDecodeChunked(data)
		f.objects[key] = data
		f.types[key] = r.Header.Get("Content-Type")
		w.Header().Set("ETag", `"abc"`)
		w.WriteHeader(200)
	case http.MethodGet, http.MethodHead:
		data, ok := f.objects[key]
		if !ok {
			cov1xS3Error(w, 404, "NoSuchKey")
			return
		}
		w.Header().Set("Content-Type", f.types[key])
		w.Header().Set("ETag", `"abc"`)
		w.Header().Set("Last-Modified", time.Unix(1700000000, 0).UTC().Format(http.TimeFormat))
		w.Header().Set("Content-Length", fmt.Sprint(len(data)))
		w.WriteHeader(200)
		if r.Method == http.MethodGet {
			w.Write(data)
		}
	case http.MethodDelete:
		delete(f.objects, key)
		w.WriteHeader(204)
	}
}

// cov1xDecodeChunked unwraps the aws-chunked streaming-signature framing the
// minio client uses for plain-HTTP uploads.
func cov1xDecodeChunked(b []byte) []byte {
	if !strings.Contains(string(b), ";chunk-signature=") {
		return b
	}
	var out []byte
	rest := string(b)
	for rest != "" {
		head, tail, ok := strings.Cut(rest, "\r\n")
		if !ok {
			break
		}
		var n int
		fmt.Sscanf(head, "%x;", &n)
		if n == 0 {
			break
		}
		out = append(out, tail[:n]...)
		rest = tail[n+2:]
	}
	return out
}

func cov1xNewFake(t *testing.T) (*S3Storage, *cov1xFakeS3) {
	t.Helper()
	fake := &cov1xFakeS3{bucket: "club-bucket", objects: map[string][]byte{}, types: map[string]string{}}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	s, err := NewS3Storage(Config{Endpoint: "http://" + srv.Listener.Addr().String(), AccessKey: "ak", SecretKey: "sk", Bucket: fake.bucket, Region: "us-east-1"})
	if err != nil {
		t.Fatalf("NewS3Storage: %v", err)
	}
	return s, fake
}

// cov1xCustom starts a server with a custom handler and builds an S3Storage
// against it with the given bucket, without ensuring the bucket.
func cov1xCustom(t *testing.T, bucket string, h http.HandlerFunc) *S3Storage {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	cfg := Config{Endpoint: srv.Listener.Addr().String(), AccessKey: "ak", SecretKey: "sk", Bucket: bucket, Region: "us-east-1"}
	s, err := NewS3Storage(cfg)
	if err == nil {
		return s
	}
	t.Fatalf("setup: %v", err)
	return nil
}

func cov1xAllowAll(extra http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead && strings.Count(strings.Trim(r.URL.Path, "/"), "/") == 0 {
			w.WriteHeader(200)
			return
		}
		extra(w, r)
	}
}

type cov1xNetErr struct{}

func (cov1xNetErr) Error() string   { return "net failure" }
func (cov1xNetErr) Timeout() bool   { return true }
func (cov1xNetErr) Temporary() bool { return true }

var _ net.Error = cov1xNetErr{}

func TestCov1xIsUnavailableMoreCases(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"canceled", fmt.Errorf("x: %w", context.Canceled), true},
		{"net.Error", fmt.Errorf("x: %w", cov1xNetErr{}), true},
		{"unexpected eof", fmt.Errorf("x: %w", io.ErrUnexpectedEOF), true},
		{"slowdown 400", fmt.Errorf("x: %w", cov1xErrResp(400, "SlowDown")), true},
		{"request timeout", fmt.Errorf("x: %w", cov1xErrResp(400, "RequestTimeout")), true},
		{"internal error", fmt.Errorf("x: %w", cov1xErrResp(400, "InternalError")), true},
		{"service unavailable", fmt.Errorf("x: %w", cov1xErrResp(400, "ServiceUnavailable")), true},
		{"no such key", fmt.Errorf("x: %w", cov1xErrResp(404, "NoSuchKey")), false},
	}
	for _, c := range cases {
		if got := IsUnavailable(c.err); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestCov1xConfigBaseURL(t *testing.T) {
	if got := (Config{}).BaseURL(); got != "http://localhost:9000/aquify" {
		t.Fatalf("defaults = %q", got)
	}
	got := Config{UseSSL: true, Endpoint: " https://s3.example.com ", Bucket: " b1 "}.BaseURL()
	if got != "https://s3.example.com/b1" {
		t.Fatalf("ssl url = %q", got)
	}
}

func TestCov1xNewClientBuildsS3WhenConfigured(t *testing.T) {
	fake := &cov1xFakeS3{bucket: "club-bucket", objects: map[string][]byte{}, types: map[string]string{}}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	c, err := NewClient(Config{Endpoint: srv.Listener.Addr().String(), AccessKey: "ak", SecretKey: "sk", Bucket: "club-bucket", Region: "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.(*S3Storage); !ok {
		t.Fatalf("enabled config should give S3 storage, got %T", c)
	}
}

func TestCov1xNewClientSelectsBackend(t *testing.T) {
	c, err := NewClient(Config{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.(*StubStorage); !ok {
		t.Fatalf("disabled config should give stub, got %T", c)
	}
}

func TestCov1xStubObjectSizeAndReady(t *testing.T) {
	s := NewStubStorage("")
	ctx := context.Background()
	if _, err := s.ObjectSize(ctx, "nope"); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("missing size err = %v", err)
	}
	if err := s.PutObject(ctx, "k", "text/plain", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if n, err := s.ObjectSize(ctx, "k"); err != nil || n != 5 {
		t.Fatalf("size = %d, %v", n, err)
	}
	if err := s.Ready(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestCov1xS3RoundTrip(t *testing.T) {
	s, fake := cov1xNewFake(t)
	ctx := context.Background()

	if err := s.Ready(ctx); err != nil {
		t.Fatalf("ready: %v", err)
	}
	if ok, err := s.Exists(ctx, "a/one.txt"); err != nil || ok {
		t.Fatalf("exists before put = %v, %v", ok, err)
	}
	if err := s.PutObject(ctx, "a/one.txt", " text/plain ", []byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := s.PutObject(ctx, "a/two.txt", "text/plain", []byte("twotwo")); err != nil {
		t.Fatal(err)
	}
	if err := s.PutObject(ctx, "b/three.txt", "text/plain", []byte("3")); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	ct := fake.types["a/one.txt"]
	fake.mu.Unlock()
	if ct != "text/plain" {
		t.Fatalf("content type stored = %q", ct)
	}
	if ok, err := s.Exists(ctx, "a/one.txt"); err != nil || !ok {
		t.Fatalf("exists = %v, %v", ok, err)
	}
	data, gotCT, err := s.GetObject(ctx, "a/one.txt")
	if err != nil || string(data) != "one" || gotCT != "text/plain" {
		t.Fatalf("get = %q %q %v", data, gotCT, err)
	}
	if _, _, err := s.GetObject(ctx, "missing"); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("get missing err = %v", err)
	}
	if n, err := s.ObjectSize(ctx, "a/two.txt"); err != nil || n != 6 {
		t.Fatalf("size = %d, %v", n, err)
	}
	if _, err := s.ObjectSize(ctx, "missing"); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("size missing err = %v", err)
	}

	keys, err := s.ListPrefix(ctx, "a/")
	if err != nil || strings.Join(keys, ",") != "a/one.txt,a/two.txt" {
		t.Fatalf("list = %v, %v", keys, err)
	}

	if err := s.Delete("b/three.txt"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("b/never-existed"); err != nil {
		t.Fatalf("deleting a missing key must be a no-op: %v", err)
	}
	if ok, _ := s.Exists(ctx, "b/three.txt"); ok {
		t.Fatal("object should be deleted")
	}

	n, err := s.DeletePrefix(ctx, "a/")
	if err != nil || n != 2 {
		t.Fatalf("delete prefix = %d, %v", n, err)
	}
	fake.mu.Lock()
	left := len(fake.objects)
	fake.mu.Unlock()
	if left != 0 {
		t.Fatalf("objects left: %d", left)
	}
}

func TestCov1xS3PresignedURLs(t *testing.T) {
	s, _ := cov1xNewFake(t)
	put, err := s.PresignedPutURL("k/file.bin", time.Minute)
	if err != nil || !strings.Contains(put, "/club-bucket/k/file.bin") || !strings.Contains(put, "X-Amz-Signature=") {
		t.Fatalf("put url = %q, %v", put, err)
	}
	get, err := s.PresignedGetURL("k/file.bin", time.Minute)
	if err != nil || !strings.Contains(get, "/club-bucket/k/file.bin") || !strings.Contains(get, "X-Amz-Expires=60") {
		t.Fatalf("get url = %q, %v", get, err)
	}
	// An invalid bucket name makes the client refuse to presign.
	bad := &S3Storage{client: s.client, bucket: "x"}
	if _, err := bad.PresignedPutURL("k", time.Minute); err == nil || !strings.Contains(err.Error(), "presign put k") {
		t.Fatalf("put err = %v", err)
	}
	if _, err := bad.PresignedGetURL("k", time.Minute); err == nil || !strings.Contains(err.Error(), "presign get k") {
		t.Fatalf("get err = %v", err)
	}
}

func TestCov1xNewS3StorageErrors(t *testing.T) {
	if _, err := NewS3Storage(Config{Endpoint: "host/with/path", AccessKey: "a", SecretKey: "b", Bucket: "bucket"}); err == nil || !strings.Contains(err.Error(), "create s3 client") {
		t.Fatalf("bad endpoint err = %v", err)
	}

	newCfg := func(h http.HandlerFunc) Config {
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)
		return Config{Endpoint: srv.Listener.Addr().String(), AccessKey: "a", SecretKey: "b", Bucket: "club-bucket", Region: "us-east-1"}
	}

	t.Run("probe denied", func(t *testing.T) {
		_, err := NewS3Storage(newCfg(func(w http.ResponseWriter, r *http.Request) { cov1xS3Error(w, 403, "AccessDenied") }))
		if err == nil || !strings.Contains(err.Error(), "check bucket club-bucket") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("creates missing bucket", func(t *testing.T) {
		var mu sync.Mutex
		created := false
		s, err := NewS3Storage(newCfg(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			switch {
			case r.Method == http.MethodPut:
				created = true
				w.WriteHeader(200)
			case created:
				w.WriteHeader(200)
			default:
				cov1xS3Error(w, 404, "NoSuchBucket")
			}
		}))
		if err != nil || s == nil || !created {
			t.Fatalf("s=%v created=%v err=%v", s, created, err)
		}
	})

	t.Run("create races with another creator", func(t *testing.T) {
		var mu sync.Mutex
		heads := 0
		s, err := NewS3Storage(newCfg(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			if r.Method == http.MethodPut {
				cov1xS3Error(w, 409, "BucketAlreadyOwnedByYou")
				return
			}
			heads++
			if heads == 1 {
				cov1xS3Error(w, 404, "NoSuchBucket")
				return
			}
			w.WriteHeader(200)
		}))
		if err != nil || s == nil {
			t.Fatalf("expected success when bucket exists after failed create: %v", err)
		}
	})

	t.Run("create fails", func(t *testing.T) {
		_, err := NewS3Storage(newCfg(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPut {
				cov1xS3Error(w, 403, "AccessDenied")
				return
			}
			cov1xS3Error(w, 404, "NoSuchBucket")
		}))
		if err == nil || !strings.Contains(err.Error(), "create bucket club-bucket") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestCov1xS3ErrorPaths(t *testing.T) {
	ctx := context.Background()
	denied := cov1xCustom(t, "club-bucket", cov1xAllowAll(func(w http.ResponseWriter, r *http.Request) {
		cov1xS3Error(w, 403, "AccessDenied")
	}))

	if err := denied.PutObject(ctx, "k", "t", []byte("x")); err == nil || !strings.Contains(err.Error(), "put object k") {
		t.Fatalf("put err = %v", err)
	}
	if _, _, err := denied.GetObject(ctx, "k"); err == nil || !strings.Contains(err.Error(), "stat object k") || errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("get err = %v", err)
	}
	if _, _, err := denied.GetObject(ctx, ""); err == nil || !strings.Contains(err.Error(), "get object ") {
		t.Fatalf("get empty key err = %v", err)
	}
	if ok, err := denied.Exists(ctx, "k"); err == nil || ok || !strings.Contains(err.Error(), "stat object k") {
		t.Fatalf("exists = %v, %v", ok, err)
	}
	if _, err := denied.ObjectSize(ctx, "k"); err == nil || errors.Is(err, ErrObjectNotFound) || !strings.Contains(err.Error(), "stat object k") {
		t.Fatalf("size err = %v", err)
	}
	if err := denied.Delete("k"); err == nil || !strings.Contains(err.Error(), "delete object k") {
		t.Fatalf("delete err = %v", err)
	}
	if _, err := denied.ListPrefix(ctx, "p/"); err == nil || !strings.Contains(err.Error(), "list p/") {
		t.Fatalf("list err = %v", err)
	}
	if n, err := denied.DeletePrefix(ctx, "p/"); err == nil || n != 0 {
		t.Fatalf("delete prefix = %d, %v", n, err)
	}
}

func TestCov1xS3DeletePrefixRemoveFailure(t *testing.T) {
	var mu sync.Mutex
	deleted := 0
	s := cov1xCustom(t, "club-bucket", cov1xAllowAll(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/xml")
			io.WriteString(w, `<ListBucketResult><IsTruncated>false</IsTruncated><Contents><Key>p/1</Key></Contents><Contents><Key>p/2</Key></Contents><Contents><Key>p/3</Key></Contents></ListBucketResult>`)
		case http.MethodDelete:
			mu.Lock()
			defer mu.Unlock()
			deleted++
			switch deleted {
			case 1:
				w.WriteHeader(204)
			case 2:
				cov1xS3Error(w, 404, "NoSuchKey") // already gone: counted, not an error
			default:
				cov1xS3Error(w, 403, "AccessDenied")
			}
		}
	}))
	n, err := s.DeletePrefix(context.Background(), "p/")
	if err == nil || !strings.Contains(err.Error(), "delete object p/3") {
		t.Fatalf("err = %v", err)
	}
	if n != 2 {
		t.Fatalf("n = %d, want 2 removed before the failure", n)
	}
}

func TestCov1xS3ReadyBucketMissing(t *testing.T) {
	var mu sync.Mutex
	gone := false
	s := cov1xCustom(t, "club-bucket", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if gone {
			cov1xS3Error(w, 404, "NoSuchBucket")
			return
		}
		w.WriteHeader(200)
	})
	mu.Lock()
	gone = true
	mu.Unlock()
	if err := s.Ready(context.Background()); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("err = %v", err)
	}
}

func TestCov1xS3GetObjectSizeAndReadFailures(t *testing.T) {
	ctx := context.Background()
	t.Run("over limit", func(t *testing.T) {
		s := cov1xCustom(t, "club-bucket", cov1xAllowAll(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", fmt.Sprint(int64(maxGetObjectBytes)+1))
			w.Header().Set("Last-Modified", time.Unix(1700000000, 0).UTC().Format(http.TimeFormat))
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
		}))
		_, _, err := s.GetObject(ctx, "big")
		if err == nil || !strings.Contains(err.Error(), "in-memory read limit") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("truncated body", func(t *testing.T) {
		s := cov1xCustom(t, "club-bucket", cov1xAllowAll(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "100")
			w.Header().Set("Last-Modified", time.Unix(1700000000, 0).UTC().Format(http.TimeFormat))
			w.WriteHeader(200)
			io.WriteString(w, "short")
		}))
		_, _, err := s.GetObject(ctx, "trunc")
		if err == nil || !strings.Contains(err.Error(), "read object trunc") {
			t.Fatalf("err = %v", err)
		}
	})
}

func cov1xErrResp(status int, code string) error {
	return minio.ErrorResponse{StatusCode: status, Code: code}
}

func TestCov1xStubFullBehavior(t *testing.T) {
	s := NewStubStorage("http://files.test/bucket/")
	ctx := context.Background()

	put, err := s.PresignedPutURL("/a b/k.txt", time.Minute)
	if err != nil || !strings.HasPrefix(put, "http://files.test/bucket/a%20b%2Fk.txt?X-Stub-Method=PUT&X-Stub-Expires=") {
		t.Fatalf("put url = %q, %v", put, err)
	}
	get, err := s.PresignedGetURL("a/k.txt", time.Minute)
	if err != nil || !strings.Contains(get, "X-Stub-Method=GET") {
		t.Fatalf("get url = %q, %v", get, err)
	}

	if ok, _ := s.Exists(ctx, "a/k.txt"); ok {
		t.Fatal("should not exist yet")
	}
	if _, _, err := s.GetObject(ctx, "a/k.txt"); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("get missing err = %v", err)
	}
	src := []byte("abc")
	for _, k := range []string{"a/k.txt", "a/j.txt", "b/z.txt"} {
		if err := s.PutObject(ctx, k, " text/csv ", src); err != nil {
			t.Fatal(err)
		}
	}
	src[0] = 'X' // stored copy must be independent of the caller's slice
	data, ct, err := s.GetObject(ctx, "a/k.txt")
	if err != nil || string(data) != "abc" || ct != "text/csv" {
		t.Fatalf("get = %q %q %v", data, ct, err)
	}
	data[0] = 'Y'
	if again, _, _ := s.GetObject(ctx, "a/k.txt"); string(again) != "abc" {
		t.Fatalf("returned slice aliases storage: %q", again)
	}
	if ok, err := s.Exists(ctx, "a/k.txt"); err != nil || !ok {
		t.Fatalf("exists = %v, %v", ok, err)
	}
	keys, err := s.ListPrefix(ctx, "a/")
	if err != nil || strings.Join(keys, ",") != "a/j.txt,a/k.txt" {
		t.Fatalf("list = %v, %v", keys, err)
	}
	if err := s.Delete("b/z.txt"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.Exists(ctx, "b/z.txt"); ok {
		t.Fatal("should be deleted")
	}
	if n, err := s.DeletePrefix(ctx, "a/"); err != nil || n != 2 {
		t.Fatalf("delete prefix = %d, %v", n, err)
	}
	if keys, _ := s.ListPrefix(ctx, ""); len(keys) != 0 {
		t.Fatalf("leftover keys %v", keys)
	}
}

// A backend that streams without a Content-Length reports no usable size at
// Stat time, so only the extra-byte read can reveal an oversized object.
func TestCov1xS3GetObjectRejectsOversizedStreamWithoutLength(t *testing.T) {
	s := cov1xCustom(t, "club-bucket", cov1xAllowAll(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Last-Modified", time.Unix(1700000000, 0).UTC().Format(http.TimeFormat))
		w.WriteHeader(200)
		chunk := make([]byte, 1<<20)
		for i := 0; i < 65; i++ {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	_, _, err := s.GetObject(context.Background(), "streamed")
	if err == nil || !strings.Contains(err.Error(), "exceeds the") {
		t.Fatalf("err = %v", err)
	}
}

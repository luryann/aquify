package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/compliance"
	"github.com/luryann/aquify/server/internal/app/documents"
	"github.com/luryann/aquify/server/internal/storage"
	"github.com/luryann/aquify/server/internal/workers"
	"github.com/redis/go-redis/v9"
)

func TestSanitizeDocumentFilenameRejectsControlAndLong(t *testing.T) {
	for _, bad := range []string{"evil\x00.pdf", "a\nb.pdf", "tab\t.pdf", "x\x7f.pdf", strings.Repeat("a", 256) + ".pdf"} {
		if _, err := sanitizeDocumentFilename(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
	if _, err := sanitizeDocumentFilename(strings.Repeat("a", 251) + ".pdf"); err != nil {
		t.Errorf("255-byte name should pass: %v", err)
	}
	if _, err := sanitizeDocumentFilename("résumé.pdf"); err != nil {
		t.Errorf("unicode name should pass: %v", err)
	}
}

type flakyStore struct {
	*storage.StubStorage
	existsErr error
	sizeOver  bool
	deleted   []string
}

func (f *flakyStore) Exists(ctx context.Context, key string) (bool, error) {
	if f.existsErr != nil {
		return false, f.existsErr
	}
	return true, nil
}

func (f *flakyStore) ObjectSize(ctx context.Context, key string) (int64, error) {
	if f.sizeOver {
		return maxDocumentUploadBytes + 1, nil
	}
	return 10, nil
}

func (f *flakyStore) Delete(key string) error {
	f.deleted = append(f.deleted, key)
	return f.StubStorage.Delete(key)
}

func attachFixture(t *testing.T, store storage.ObjectStorage) (http.Handler, *documents.DocumentSubmission) {
	t.Helper()
	svc := documents.NewService()
	dt, err := svc.CreateDocumentType(engOrg, "Waiver", "legal", "", "swimmer", true, true, nil, "u")
	if err != nil {
		t.Fatal(err)
	}
	sub, err := svc.CreateSubmission(engOrg, dt.ID, "hh-1", "", "u-admin")
	if err != nil {
		t.Fatal(err)
	}
	h := NewDocumentHandlers(svc, audit.NewService(), store)
	return g2Router(t, g2Tokens, h.RegisterRoutes), sub
}

var attachKeys = map[string]string{}

func attachKey(sub *documents.DocumentSubmission) string {
	if k, ok := attachKeys[sub.ID]; ok {
		return k
	}
	k := documentObjectKey(engOrg, sub.ID, "a.pdf")
	attachKeys[sub.ID] = k
	return k
}

func attachBody(sub *documents.DocumentSubmission) string {
	return fmt.Sprintf(`{"file_object_key":%q,"file_name":"a.pdf","file_mime":"application/pdf","file_size_bytes":10}`, attachKey(sub))
}

func TestAttachOversizedObjectIsDeleted(t *testing.T) {
	store := &flakyStore{StubStorage: storage.NewStubStorage(""), sizeOver: true}
	r, sub := attachFixture(t, store)
	rr := g2Do(r, http.MethodPost, "/documents/"+sub.ID+"/attach", "admin", attachBody(sub))
	if rr.Code != http.StatusUnprocessableEntity || !strings.Contains(rr.Body.String(), "file_too_large") {
		t.Fatalf("got %d %s", rr.Code, rr.Body.String())
	}
	if len(store.deleted) != 1 || store.deleted[0] != attachKey(sub) {
		t.Fatalf("oversized object not deleted: %v", store.deleted)
	}
}

func TestAttachStorageOutageReturns503(t *testing.T) {
	store := &flakyStore{StubStorage: storage.NewStubStorage(""), existsErr: fmt.Errorf("stat: %w", &net.OpError{Op: "dial", Err: errors.New("refused")})}
	r, sub := attachFixture(t, store)
	rr := g2Do(r, http.MethodPost, "/documents/"+sub.ID+"/attach", "admin", attachBody(sub))
	if rr.Code != http.StatusServiceUnavailable || !strings.Contains(rr.Body.String(), "storage_unavailable") {
		t.Fatalf("got %d %s", rr.Code, rr.Body.String())
	}
	store.existsErr = errors.New("some other failure")
	rr = g2Do(r, http.MethodPost, "/documents/"+sub.ID+"/attach", "admin", attachBody(sub))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("non-connectivity error: got %d", rr.Code)
	}
}

type failingQueue struct{ workers.Queue }

func (failingQueue) Enqueue(context.Context, *workers.Task, ...workers.Option) (*workers.TaskInfo, error) {
	return nil, errors.New("dial tcp: connection refused")
}

func TestExportEnqueueFailureMarksRequestFailed(t *testing.T) {
	svc := compliance.NewService()
	h := NewComplianceHandlers(svc, audit.NewService())
	h.SetJobQueue(failingQueue{})
	r := g2Router(t, g2Tokens, h.RegisterRoutes)
	rr := g2Do(r, http.MethodPost, "/compliance/data-export-requests", "admin", `{"household_id":"hh-1"}`)
	if rr.Code != http.StatusServiceUnavailable || !strings.Contains(rr.Body.String(), "export_queue_unavailable") {
		t.Fatalf("got %d %s", rr.Code, rr.Body.String())
	}
	list, _ := svc.ListDataExportRequests(engOrg, "")
	if len(list) != 1 || list[0].Status != "failed" {
		t.Fatalf("expected one failed request, got %+v", list)
	}
}

func TestComplianceReadMintsFreshPresignedURL(t *testing.T) {
	svc := compliance.NewService()
	store := storage.NewStubStorage("")
	h := NewComplianceHandlers(svc, audit.NewService())
	h.SetStorage(store)
	req, _ := svc.CreateDataExportRequest(engOrg, "hh-1", "u-admin")
	if _, err := svc.MarkDataExportProcessing(engOrg, req.ID); err != nil {
		t.Fatal(err)
	}
	key := "compliance/" + engOrg + "/exports/" + req.ID + ".json"
	exp := req.RequestedAt.AddDate(0, 0, 30)
	if _, err := svc.CompleteDataExport(engOrg, req.ID, key, exp); err != nil {
		t.Fatal(err)
	}
	r := g2Router(t, g2Tokens, h.RegisterRoutes)
	rr := g2Do(r, http.MethodGet, "/compliance/data-export-requests/"+req.ID, "admin", "")
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "X-Stub-Method=GET") {
		t.Fatalf("got %d %s", rr.Code, rr.Body.String())
	}
}

func TestJobsQueueUnavailableClassification(t *testing.T) {
	if !isQueueUnavailable(&net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}) {
		t.Error("connection refused should be unavailable")
	}
	if !isQueueUnavailable(fmt.Errorf("x: %w", &net.OpError{Op: "dial"})) {
		t.Error("net.OpError should be unavailable")
	}
	if isQueueUnavailable(errors.New("task not found")) || isQueueUnavailable(nil) {
		t.Error("app errors are not unavailability")
	}
	for name, err := range map[string]error{
		"unexpected EOF":        fmt.Errorf("decode payload: %w", io.ErrUnexpectedEOF),
		"json eof":              fmt.Errorf("decode payload: %w: %w", &json.SyntaxError{}, io.EOF),
		"text only dial":        errors.New("dial tcp 127.0.0.1:6379: connection refused"),
		"text only eof":         errors.New("unexpected EOF"),
		"non-timeout net.Error": fmt.Errorf("x: %w", staticNetErr{}),
		"wrapped plain app err": fmt.Errorf("retry: %w", errors.New("broken pipe in report")),
	} {
		if isQueueUnavailable(err) {
			t.Errorf("%s misclassified as queue unavailable", name)
		}
	}
	for name, err := range map[string]error{
		"bare EOF":       io.EOF,
		"wrapped EOF":    fmt.Errorf("inspect: %w", io.EOF),
		"ECONNRESET":     fmt.Errorf("read: %w", syscall.ECONNRESET),
		"pool timeout":   redis.ErrPoolTimeout,
		"client closed":  redis.ErrClosed,
		"deadline":       context.DeadlineExceeded,
		"timeout neterr": fmt.Errorf("x: %w", staticNetErr{timeout: true}),
	} {
		if !isQueueUnavailable(err) {
			t.Errorf("%s should be queue unavailable", name)
		}
	}
}

type staticNetErr struct{ timeout bool }

func (e staticNetErr) Error() string   { return "static net error" }
func (e staticNetErr) Timeout() bool   { return e.timeout }
func (e staticNetErr) Temporary() bool { return false }

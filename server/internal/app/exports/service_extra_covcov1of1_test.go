package exports

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/luryann/aquify/server/internal/storage"
)

func TestCov1xNewServiceWithPoolNilIsMemoryMode(t *testing.T) {
	t.Parallel()
	svc := NewServiceWithPool(nil, storage.NewStubStorage(""))
	if svc.repo != nil {
		t.Fatal("nil pool must keep memory mode")
	}
	job := cov1xGenerate(t, svc, "org-1")
	if !strings.HasPrefix(job.ID, "export_") {
		t.Fatalf("memory-mode id = %q", job.ID)
	}
}

func TestCov1xGenerateExportSurfacesProcessingFailure(t *testing.T) {
	t.Parallel()
	store := &cov1xStore{ObjectStorage: storage.NewStubStorage(""), putErr: errors.New("bucket full")}
	svc := NewServiceWithStorage(store)
	_, audit, err := svc.GenerateExport(cov1xAdmin, "org-1", "m1", sampleReportInput(ReportTypeEvent, ""))
	if err == nil || !strings.Contains(err.Error(), "store export artifact") || audit != nil {
		t.Fatalf("err=%v audit=%v", err, audit)
	}
	list := svc.ListExportsForAuthorizedMeet("org-1", "m1")
	if len(list) != 1 || list[0].Status != "failed" || list[0].LastError != "bucket full" {
		t.Fatalf("job should be recorded as failed: %+v", list)
	}
}

// putHook runs a callback in PutObject, simulating a concurrent change mid-generation.
type cov1xPutHook struct {
	storage.ObjectStorage
	onPut func()
}

func (h *cov1xPutHook) PutObject(ctx context.Context, key, ct string, data []byte) error {
	h.onPut()
	return h.ObjectStorage.PutObject(ctx, key, ct, data)
}

func TestCov1xProcessExportJobRemovedDuringGeneration(t *testing.T) {
	t.Parallel()
	svc := NewService()
	in := sampleReportInput(ReportTypeEvent, "")
	svc.exports["j"] = &FileExport{ID: "j", OrgID: "o", Status: "pending", StorageKey: "k", pendingInput: &in}
	svc.store = &cov1xPutHook{ObjectStorage: svc.store, onPut: func() {
		svc.mu.Lock()
		delete(svc.exports, "j")
		svc.mu.Unlock()
	}}
	if err := svc.ProcessExport(context.Background(), "j"); !errors.Is(err, ErrExportNotFound) {
		t.Fatalf("err = %v, want ErrExportNotFound", err)
	}
}

func TestCov1xSectionHeadingWithoutRightLabel(t *testing.T) {
	t.Parallel()
	pdf, _ := cov1xDoc()
	y0 := pdf.GetY()
	sectionHeading(pdf, "Only Title", "")
	y1 := pdf.GetY()
	sectionHeading(pdf, "Title", "3 entered")
	y2 := pdf.GetY()
	if y1 <= y0 || y2 <= y1 {
		t.Fatalf("headings must advance the cursor: %v %v %v", y0, y1, y2)
	}
	if pdf.Error() != nil {
		t.Fatalf("pdf error: %v", pdf.Error())
	}
}

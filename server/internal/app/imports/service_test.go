package imports

import (
	"context"
	"errors"
	"testing"
	"time"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/storage"
)

func TestRequestUploadURLValidationAndExpiry(t *testing.T) {
	t.Parallel()

	svc := NewServiceWithStorage(storage.NewStubStorage("http://storage.test/aquify"))
	admin := &appmiddleware.Principal{UserID: "admin-1", Roles: []string{"admin"}}

	if _, _, err := svc.RequestUploadURL(&appmiddleware.Principal{UserID: "parent", Roles: []string{"parent"}}, "org-a", RequestUploadURLInput{
		Filename: "results.cl2", SizeBytes: 10, MIMEType: "text/plain",
	}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("RequestUploadURL() forbidden error = %v, want %v", err, ErrForbidden)
	}
	if _, _, err := svc.RequestUploadURL(admin, "org-a", RequestUploadURLInput{
		Filename: "../results.cl2", SizeBytes: 10, MIMEType: "text/plain",
	}); !errors.Is(err, ErrInvalidFilename) {
		t.Fatalf("RequestUploadURL() invalid filename error = %v, want %v", err, ErrInvalidFilename)
	}
	if _, _, err := svc.RequestUploadURL(admin, "org-a", RequestUploadURLInput{
		Filename: "results.cl2", FileType: "sd3", SizeBytes: 10, MIMEType: "text/plain",
	}); !errors.Is(err, ErrInvalidFileType) {
		t.Fatalf("RequestUploadURL() mismatched type error = %v, want %v", err, ErrInvalidFileType)
	}

	imp, audit, err := svc.RequestUploadURL(admin, "org-a", RequestUploadURLInput{
		Filename: "results.cl2", SizeBytes: 10, MIMEType: "text/plain",
	})
	if err != nil {
		t.Fatalf("RequestUploadURL() error = %v", err)
	}
	if imp.Status != "uploaded" || imp.FileType != "cl2" || imp.UploadURL == "" || imp.UploadURLExpiresAt == nil {
		t.Fatalf("RequestUploadURL() returned unexpected import: %+v", imp)
	}
	if audit == nil || audit.Action != "import.upload_url_requested" {
		t.Fatalf("RequestUploadURL() audit = %+v, want upload audit", audit)
	}

	expired := svc.ExpirePendingUploads(imp.UploadURLExpiresAt.Add(time.Second))
	if len(expired) != 1 || expired[0].ImportID != imp.ID {
		t.Fatalf("ExpirePendingUploads() = %+v, want one expired artifact", expired)
	}
	current, err := svc.GetImport(admin, "org-a", imp.ID)
	if err != nil {
		t.Fatalf("GetImport() error = %v", err)
	}
	if current.Status != "expired" || current.UploadURL != "" || current.UploadURLExpiresAt != nil {
		t.Fatalf("expired import = %+v, want cleared upload URL", current)
	}

	if err := svc.DeleteArtifact(""); err != nil {
		t.Fatalf("DeleteArtifact(empty) error = %v", err)
	}
	if err := svc.DeleteArtifact("missing-key"); err != nil {
		t.Fatalf("DeleteArtifact(missing) error = %v", err)
	}
	_ = context.Background()
}

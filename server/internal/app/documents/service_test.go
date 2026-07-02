package documents

import (
	"errors"
	"testing"
)

func TestDocumentSubmissionLifecycleAndOrgScope(t *testing.T) {
	t.Parallel()

	svc := NewService()
	expiryMonths := 12
	docType, err := svc.CreateDocumentType("org-a", "Medical Release", "medical", "Annual form", "swimmer", true, true, &expiryMonths, "admin")
	if err != nil {
		t.Fatalf("CreateDocumentType() error = %v", err)
	}
	if docType.Name != "Medical Release" || docType.Category != "medical" || docType.ExpiryMonths == nil {
		t.Fatalf("CreateDocumentType() returned unexpected type: %+v", docType)
	}

	if _, err := svc.CreateDocumentType("org-a", "medical release", "medical", "", "swimmer", true, true, nil, "admin"); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("CreateDocumentType() duplicate error = %v, want %v", err, ErrDuplicateName)
	}

	submission, err := svc.CreateSubmission("org-a", docType.ID, "household-1", "swimmer-1", "parent")
	if err != nil {
		t.Fatalf("CreateSubmission() error = %v", err)
	}
	if submission.Status != "pending" || submission.ExpiresAt == nil {
		t.Fatalf("CreateSubmission() returned unexpected submission: %+v", submission)
	}

	attached, err := svc.AttachFile("org-a", submission.ID, "documents/org-a/sub/file.pdf", "file.pdf", "application/pdf", 1234, "parent")
	if err != nil {
		t.Fatalf("AttachFile() error = %v", err)
	}
	if attached.Status != "submitted" || attached.SubmittedAt == nil || attached.FileName != "file.pdf" {
		t.Fatalf("AttachFile() returned unexpected submission: %+v", attached)
	}

	verified, err := svc.VerifySubmission("org-a", submission.ID, "admin", "Looks good")
	if err != nil {
		t.Fatalf("VerifySubmission() error = %v", err)
	}
	if verified.Status != "verified" || verified.VerifiedAt == nil || verified.ReviewNotes != "Looks good" {
		t.Fatalf("VerifySubmission() returned unexpected submission: %+v", verified)
	}

	if _, err := svc.AttachFile("org-a", submission.ID, "documents/org-a/sub/again.pdf", "again.pdf", "application/pdf", 99, "parent"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("AttachFile() after verify error = %v, want %v", err, ErrInvalidTransition)
	}
	if _, err := svc.GetSubmission("org-b", submission.ID); !errors.Is(err, ErrSubmissionNotFound) {
		t.Fatalf("GetSubmission() cross-org error = %v, want %v", err, ErrSubmissionNotFound)
	}
}

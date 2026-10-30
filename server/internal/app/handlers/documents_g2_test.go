package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/documents"
	"github.com/go-chi/chi/v5"
)

func TestValidateDocumentObjectKeyRejectsTraversal(t *testing.T) {
	good := "documents/o/s/123-file.pdf"
	if err := validateDocumentObjectKey("o", "s", good); err != nil {
		t.Fatalf("valid key rejected: %v", err)
	}
	for _, bad := range []string{
		"documents/o/s/../other/file.pdf",
		"documents/o/s/..",
		"documents/o/s/./x",
		"documents/o/s//x",
		"documents/o/s/a\\b",
		"documents/o/s/",
		"documents/o/s/x\x00.pdf",
		"documents/o/x/file.pdf",
	} {
		if err := validateDocumentObjectKey("o", "s", bad); err == nil {
			t.Errorf("key %q should be rejected", bad)
		}
	}
}

func TestUpdateDocumentTypeCanClearExpiryMonths(t *testing.T) {
	svc := documents.NewService()
	months := 12
	dt, err := svc.CreateDocumentType(engOrg, "Waiver", "legal", "", "swimmer", true, true, &months, "u")
	if err != nil {
		t.Fatal(err)
	}
	h := NewDocumentHandlers(svc, audit.NewService(), nil)
	r := g2Router(t, g2Tokens, h.RegisterRoutes)
	path := "/document-types/" + dt.ID

	// Absent field leaves expiry unchanged.
	expectStatus(t, g2Do(r, http.MethodPut, path, "admin", `{"name":"Waiver 2"}`), http.StatusOK, "rename")
	got, _ := svc.ListDocumentTypes(engOrg, false)
	if got[0].ExpiryMonths == nil || *got[0].ExpiryMonths != 12 {
		t.Fatalf("expiry changed by unrelated update: %+v", got[0].ExpiryMonths)
	}
	// A number sets it.
	rr := g2Do(r, http.MethodPut, path, "admin", `{"expiry_months":6}`)
	expectStatus(t, rr, http.StatusOK, "set")
	// Explicit null clears it.
	rr = g2Do(r, http.MethodPut, path, "admin", `{"expiry_months":null}`)
	expectStatus(t, rr, http.StatusOK, "clear")
	var out struct {
		DT documents.DocumentType `json:"document_type"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	if out.DT.ExpiryMonths != nil {
		t.Fatalf("expiry_months not cleared: %d", *out.DT.ExpiryMonths)
	}
}

var _ = chi.NewRouter

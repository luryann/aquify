package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/documents"
	"github.com/luryann/aquify/server/internal/storage"
	"github.com/luryann/aquify/server/internal/testutil"
)

// cov3xStore wraps the in-memory stub and lets a test force individual
// object-storage operations to misbehave.
type cov3xStore struct {
	*storage.StubStorage
	presignErr   error
	getErr       error
	putErr       error
	sizeErr      error
	sizeOverride *int64
	existsAlways bool
	deleteErrFor map[string]error
	deleted      []string
}

func newCov3xStore() *cov3xStore { return &cov3xStore{StubStorage: storage.NewStubStorage("")} }

func (s *cov3xStore) PresignedPutURL(key string, ttl time.Duration) (string, error) {
	if s.presignErr != nil {
		return "", s.presignErr
	}
	return s.StubStorage.PresignedPutURL(key, ttl)
}

func (s *cov3xStore) GetObject(ctx context.Context, key string) ([]byte, string, error) {
	if s.getErr != nil {
		return nil, "", s.getErr
	}
	return s.StubStorage.GetObject(ctx, key)
}

func (s *cov3xStore) PutObject(ctx context.Context, key, ct string, data []byte) error {
	if s.putErr != nil {
		return s.putErr
	}
	return s.StubStorage.PutObject(ctx, key, ct, data)
}

func (s *cov3xStore) Exists(ctx context.Context, key string) (bool, error) {
	if s.existsAlways {
		return true, nil
	}
	return s.StubStorage.Exists(ctx, key)
}

func (s *cov3xStore) ObjectSize(ctx context.Context, key string) (int64, error) {
	if s.sizeErr != nil {
		return 0, s.sizeErr
	}
	if s.sizeOverride != nil {
		return *s.sizeOverride, nil
	}
	return s.StubStorage.ObjectSize(ctx, key)
}

func (s *cov3xStore) Delete(key string) error {
	s.deleted = append(s.deleted, key)
	if err := s.deleteErrFor[key]; err != nil {
		return err
	}
	return s.StubStorage.Delete(key)
}

type cov3xDocFixture struct {
	svc    *documents.Service
	audit  *audit.Service
	h      *DocumentHandlers
	router http.Handler
	store  *cov3xStore
	typeID string
}

func newCov3xDocFixture(t *testing.T) cov3xDocFixture {
	t.Helper()
	svc := documents.NewService()
	dt, err := svc.CreateDocumentType(cov3xOrg, "Medical Form", "medical", "", "swimmer", true, true, nil, cov3xAdminID)
	if err != nil {
		t.Fatal(err)
	}
	store := newCov3xStore()
	auditSvc := audit.NewService()
	h := NewDocumentHandlers(svc, auditSvc, store)
	h.SetSwimmerHouseholdResolver(func(orgID, swimmerID string) (string, bool) {
		if orgID == cov3xOrg && swimmerID == cov3xSw1 {
			return cov3xHH1, true
		}
		return "", false
	})
	return cov3xDocFixture{svc: svc, audit: auditSvc, h: h, router: cov3xRouter(h.RegisterRoutes), store: store, typeID: dt.ID}
}

func (f cov3xDocFixture) newSubmission(t *testing.T, household string) *documents.DocumentSubmission {
	t.Helper()
	sub, err := f.svc.CreateSubmission(cov3xOrg, f.typeID, household, "", cov3xAdminID)
	if err != nil {
		t.Fatal(err)
	}
	return sub
}

func (f cov3xDocFixture) putPDF(t *testing.T, sub *documents.DocumentSubmission, name string, data []byte) string {
	t.Helper()
	key := documentObjectKey(cov3xOrg, sub.ID, name)
	if err := f.store.StubStorage.PutObject(context.Background(), key, "application/pdf", data); err != nil {
		t.Fatal(err)
	}
	return key
}

func cov3xAttachBody(key, name, mime string) string {
	return fmt.Sprintf(`{"file_object_key":%q,"file_name":%q,"file_mime":%q,"file_size_bytes":4}`, key, name, mime)
}

func TestCov3xDocumentsScopeGuards(t *testing.T) {
	f := newCov3xDocFixture(t)
	cov3xAssertScopeGuards(t, f.h.RegisterRoutes, []cov3xRoute{
		{http.MethodGet, "/document-types", ""},
		{http.MethodPost, "/document-types", `{}`},
		{http.MethodPut, "/document-types/x", `{}`},
		{http.MethodDelete, "/document-types/x", ""},
		{http.MethodGet, "/documents", ""},
		{http.MethodPost, "/documents", `{}`},
		{http.MethodGet, "/documents/x", ""},
		{http.MethodPost, "/documents/x/upload-url", `{}`},
		{http.MethodPost, "/documents/x/attach", `{}`},
		{http.MethodPost, "/documents/x/verify", ""},
		{http.MethodPost, "/documents/x/reject", ""},
	})
}

func TestCov3xDocumentTypeCRUDAndRoles(t *testing.T) {
	f := newCov3xDocFixture(t)
	r := f.router

	for _, tok := range []string{"swimmer", "stranger"} {
		cov3xWant(t, cov3xDo(r, http.MethodGet, "/document-types", tok, ""), http.StatusForbidden, "Insufficient role")
	}
	cov3xWant(t, cov3xDo(r, http.MethodPost, "/document-types", "coach", `{"name":"x","category":"c"}`), http.StatusForbidden, "Only admin may create")
	cov3xWant(t, cov3xDo(r, http.MethodPost, "/document-types", "admin", `{"name":"x"}`), http.StatusUnprocessableEntity, "validation_error")
	cov3xWant(t, cov3xDo(r, http.MethodPost, "/document-types", "admin", `{not json`), http.StatusBadRequest, "bad_request")

	// applies_to defaults to swimmer; inactive types are hidden by default.
	rr := cov3xDo(r, http.MethodPost, "/document-types", "admin", `{"name":"Photo Release","category":"legal"}`)
	cov3xWant(t, rr, http.StatusCreated, "")
	dt := cov3xJSON(t, rr)["document_type"].(map[string]any)
	if dt["applies_to"] != "swimmer" || dt["is_active"] != false {
		t.Fatalf("unexpected defaults: %v", dt)
	}
	inactiveID := dt["id"].(string)
	cov3xWant(t, cov3xDo(r, http.MethodPost, "/document-types", "admin", `{"name":"photo release","category":"legal"}`), http.StatusConflict, "already exists")

	list := func(q string) int {
		rr := cov3xDo(r, http.MethodGet, "/document-types"+q, "parent", "")
		cov3xWant(t, rr, http.StatusOK, "")
		return len(cov3xJSON(t, rr)["document_types"].([]any))
	}
	if got := list(""); got != 1 {
		t.Fatalf("active-only list: want 1, got %d", got)
	}
	if got := list("?active_only=false"); got != 2 {
		t.Fatalf("full list: want 2, got %d", got)
	}

	path := "/document-types/" + inactiveID
	cov3xWant(t, cov3xDo(r, http.MethodPut, path, "coach", `{}`), http.StatusForbidden, "Only admin may update")
	cov3xWant(t, cov3xDo(r, http.MethodPut, path, "admin", `{bad`), http.StatusBadRequest, "bad_request")
	cov3xWant(t, cov3xDo(r, http.MethodPut, path, "admin", `{"expiry_months":"soon"}`), http.StatusBadRequest, "bad_request")
	cov3xWant(t, cov3xDo(r, http.MethodPut, "/document-types/missing", "admin", `{"name":"n"}`), http.StatusNotFound, "not_found")
	cov3xWant(t, cov3xDo(r, http.MethodPut, path, "admin", `{"name":"Medical Form"}`), http.StatusConflict, "already exists")

	rr = cov3xDo(r, http.MethodPut, path, "admin",
		`{"name":"Renamed","category":"medical","description":"Annual","is_required":true,"applies_to":"household","expiry_months":3,"is_active":true}`)
	cov3xWant(t, rr, http.StatusOK, "")
	upd := cov3xJSON(t, rr)["document_type"].(map[string]any)
	if upd["name"] != "Renamed" || upd["category"] != "medical" || upd["description"] != "Annual" ||
		upd["is_required"] != true || upd["applies_to"] != "household" || upd["expiry_months"] != float64(3) || upd["is_active"] != true {
		t.Fatalf("update not applied: %v", upd)
	}
	if got := list(""); got != 2 {
		t.Fatalf("activated type should now be listed, got %d", got)
	}

	cov3xWant(t, cov3xDo(r, http.MethodDelete, path, "coach", ""), http.StatusForbidden, "Only admin may delete")
	cov3xWant(t, cov3xDo(r, http.MethodDelete, "/document-types/missing", "admin", ""), http.StatusNotFound, "not_found")
	rr = cov3xDo(r, http.MethodDelete, path, "admin", "")
	cov3xWant(t, rr, http.StatusOK, "")
	if cov3xJSON(t, rr)["deleted"] != true {
		t.Fatalf("delete flag missing: %s", rr.Body.String())
	}
	if got := list("?active_only=false"); got != 1 {
		t.Fatalf("deleted type must disappear, got %d", got)
	}
	events, _ := f.audit.Query(audit.Filter{OrgID: cov3xOrg})
	actions := map[string]bool{}
	for _, e := range events {
		actions[e.Action] = true
	}
	for _, want := range []string{"document_type.create", "document_type.updated", "document_type.deleted"} {
		if !actions[want] {
			t.Errorf("missing audit action %s (have %v)", want, actions)
		}
	}
}

func TestCov3xDocumentSubmissionReadWriteScoping(t *testing.T) {
	f := newCov3xDocFixture(t)
	r := f.router
	own := f.newSubmission(t, cov3xHH1)
	other := f.newSubmission(t, cov3xHH2)

	// list
	cov3xWant(t, cov3xDo(r, http.MethodGet, "/documents", "swimmer", ""), http.StatusForbidden, "Insufficient role")
	cov3xWant(t, cov3xDo(r, http.MethodGet, "/documents", "parent0", ""), http.StatusForbidden, "no household")
	cov3xWant(t, cov3xDo(r, http.MethodGet, "/documents?household_id="+cov3xHH2, "parent", ""), http.StatusForbidden, "own household")
	rr := cov3xDo(r, http.MethodGet, "/documents", "parent", "")
	cov3xWant(t, rr, http.StatusOK, "")
	docs := cov3xJSON(t, rr)["documents"].([]any)
	if len(docs) != 1 || docs[0].(map[string]any)["id"] != own.ID {
		t.Fatalf("parent must see only own household: %v", docs)
	}
	rr = cov3xDo(r, http.MethodGet, "/documents", "admin", "")
	if n := len(cov3xJSON(t, rr)["documents"].([]any)); n != 2 {
		t.Fatalf("admin sees all, got %d", n)
	}
	rr = cov3xDo(r, http.MethodGet, "/documents?household_id="+cov3xHH2, "coach", "")
	if docs := cov3xJSON(t, rr)["documents"].([]any); len(docs) != 1 || docs[0].(map[string]any)["id"] != other.ID {
		t.Fatalf("household filter ignored: %v", docs)
	}

	// create
	body := func(hh, sw string) string {
		return fmt.Sprintf(`{"document_type_id":%q,"household_id":%q,"swimmer_id":%q}`, f.typeID, hh, sw)
	}
	cov3xWant(t, cov3xDo(r, http.MethodPost, "/documents", "coach", body(cov3xHH1, "")), http.StatusForbidden, "Insufficient role")
	cov3xWant(t, cov3xDo(r, http.MethodPost, "/documents", "admin", `{}`), http.StatusUnprocessableEntity, "validation_error")
	cov3xWant(t, cov3xDo(r, http.MethodPost, "/documents", "parent", body(cov3xHH2, "")), http.StatusForbidden, "own household")
	cov3xWant(t, cov3xDo(r, http.MethodPost, "/documents", "admin", body(cov3xHH2, cov3xSw1)), http.StatusBadRequest, "does not belong")
	cov3xWant(t, cov3xDo(r, http.MethodPost, "/documents", "admin", body(cov3xHH1, cov3xSw2)), http.StatusBadRequest, "does not belong")
	cov3xWant(t, cov3xDo(r, http.MethodPost, "/documents", "admin", fmt.Sprintf(`{"document_type_id":"nope","household_id":%q}`, cov3xHH1)), http.StatusBadRequest, "Document type not found")
	rr = cov3xDo(r, http.MethodPost, "/documents", "parent", body(cov3xHH1, cov3xSw1))
	cov3xWant(t, rr, http.StatusCreated, "")
	created := cov3xJSON(t, rr)["document"].(map[string]any)
	if created["status"] != "pending" || created["swimmer_id"] != cov3xSw1 || created["household_id"] != cov3xHH1 {
		t.Fatalf("unexpected submission: %v", created)
	}

	// get
	cov3xWant(t, cov3xDo(r, http.MethodGet, "/documents/"+own.ID, "swimmer", ""), http.StatusForbidden, "Insufficient role")
	cov3xWant(t, cov3xDo(r, http.MethodGet, "/documents/missing", "admin", ""), http.StatusNotFound, "not_found")
	cov3xWant(t, cov3xDo(r, http.MethodGet, "/documents/"+own.ID, "parent2", ""), http.StatusNotFound, "not_found")
	cov3xWant(t, cov3xDo(r, http.MethodGet, "/documents/"+own.ID, "parent", ""), http.StatusOK, own.ID)
	cov3xWant(t, cov3xDo(r, http.MethodGet, "/documents/"+own.ID, "coach", ""), http.StatusOK, own.ID)
}

func TestCov3xSanitizeDocumentFilename(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		errPart string
	}{
		{"  scan.pdf  ", "scan.pdf", ""},
		{"   ", "", "required"},
		{".", "", "safe basename"},
		{"dir/file.pdf", "", "safe basename"},
		{"a..b.pdf", "", "traversal"},
		{"..", "", "traversal"},
	}
	for _, c := range cases {
		got, err := sanitizeDocumentFilename(c.in)
		if c.errPart == "" {
			if err != nil || got != c.want {
				t.Errorf("%q: got %q, %v", c.in, got, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), c.errPart) {
			t.Errorf("%q: want error containing %q, got %v", c.in, c.errPart, err)
		}
	}
}

func TestCov3xSealUploadedObject(t *testing.T) {
	ctx := context.Background()
	prefix := "documents/o/s/"

	t.Run("seals and removes the source", func(t *testing.T) {
		st := newCov3xStore()
		_ = st.StubStorage.PutObject(ctx, prefix+"src", "application/pdf", []byte("pdf!"))
		key, n, err := sealUploadedObject(ctx, st, prefix+"src", prefix, "a.pdf", "application/pdf", 100)
		if err != nil || n != 4 || !strings.HasPrefix(key, prefix+"sealed/") || !strings.HasSuffix(key, "-a.pdf") {
			t.Fatalf("got %q %d %v", key, n, err)
		}
		if ok, _ := st.StubStorage.Exists(ctx, prefix+"src"); ok {
			t.Fatal("source object must be deleted")
		}
		if data, _, err := st.StubStorage.GetObject(ctx, key); err != nil || string(data) != "pdf!" {
			t.Fatalf("sealed copy wrong: %q %v", data, err)
		}
	})
	t.Run("read failure", func(t *testing.T) {
		st := newCov3xStore()
		_, _, err := sealUploadedObject(ctx, st, prefix+"missing", prefix, "a.pdf", "application/pdf", 100)
		if !errors.Is(err, storage.ErrObjectNotFound) {
			t.Fatalf("want ErrObjectNotFound, got %v", err)
		}
	})
	t.Run("oversize object is deleted", func(t *testing.T) {
		st := newCov3xStore()
		_ = st.StubStorage.PutObject(ctx, prefix+"big", "application/pdf", []byte("0123456789"))
		_, n, err := sealUploadedObject(ctx, st, prefix+"big", prefix, "a.pdf", "application/pdf", 5)
		if !errors.Is(err, errUploadTooLarge) || n != 10 {
			t.Fatalf("got %d %v", n, err)
		}
		if ok, _ := st.StubStorage.Exists(ctx, prefix+"big"); ok {
			t.Fatal("oversized source must be deleted")
		}
	})
	t.Run("write failure keeps the source", func(t *testing.T) {
		st := newCov3xStore()
		_ = st.StubStorage.PutObject(ctx, prefix+"src", "application/pdf", []byte("pdf!"))
		st.putErr = errors.New("disk full")
		_, _, err := sealUploadedObject(ctx, st, prefix+"src", prefix, "a.pdf", "application/pdf", 100)
		if err == nil || !strings.Contains(err.Error(), "disk full") {
			t.Fatalf("want disk full, got %v", err)
		}
		if ok, _ := st.StubStorage.Exists(ctx, prefix+"src"); !ok {
			t.Fatal("source must survive a failed seal")
		}
	})
	t.Run("source delete failure is tolerated", func(t *testing.T) {
		st := newCov3xStore()
		_ = st.StubStorage.PutObject(ctx, prefix+"src", "application/pdf", []byte("pdf!"))
		st.deleteErrFor = map[string]error{prefix + "src": errors.New("locked")}
		key, _, err := sealUploadedObject(ctx, st, prefix+"src", prefix, "a.pdf", "application/pdf", 100)
		if err != nil || key == "" {
			t.Fatalf("seal should still succeed: %q %v", key, err)
		}
	})
}

func TestCov3xRequestUploadURL(t *testing.T) {
	f := newCov3xDocFixture(t)
	r := f.router
	sub := f.newSubmission(t, cov3xHH1)
	path := "/documents/" + sub.ID + "/upload-url"
	good := `{"file_name":"form.pdf","file_mime":"application/pdf","file_size_bytes":1024}`

	cov3xWant(t, cov3xDo(r, http.MethodPost, path, "coach", good), http.StatusForbidden, "Insufficient role")
	cov3xWant(t, cov3xDo(r, http.MethodPost, "/documents/missing/upload-url", "admin", good), http.StatusNotFound, "not_found")
	cov3xWant(t, cov3xDo(r, http.MethodPost, path, "parent2", good), http.StatusNotFound, "not_found")
	cov3xWant(t, cov3xDo(r, http.MethodPost, path, "admin", `{}`), http.StatusUnprocessableEntity, "validation_error")
	cov3xWant(t, cov3xDo(r, http.MethodPost, path, "admin", `{"file_name":"../x.pdf","file_mime":"application/pdf","file_size_bytes":5}`), http.StatusBadRequest, "invalid_filename")
	cov3xWant(t, cov3xDo(r, http.MethodPost, path, "admin", `{"file_name":"x.exe","file_mime":"application/x-msdownload","file_size_bytes":5}`), http.StatusUnprocessableEntity, "invalid_mime_type")
	cov3xWant(t, cov3xDo(r, http.MethodPost, path, "admin", fmt.Sprintf(`{"file_name":"x.pdf","file_mime":"application/pdf","file_size_bytes":%d}`, maxDocumentUploadBytes+1)), http.StatusUnprocessableEntity, "file_too_large")

	rr := cov3xDo(r, http.MethodPost, path, "parent", good)
	cov3xWant(t, rr, http.StatusCreated, "")
	out := cov3xJSON(t, rr)
	key, _ := out["object_key"].(string)
	if !strings.HasPrefix(key, "documents/"+cov3xOrg+"/"+sub.ID+"/") || !strings.HasSuffix(key, "-form.pdf") ||
		!strings.Contains(out["upload_url"].(string), "X-Stub-Method=PUT") || out["expected_file_name"] != "form.pdf" {
		t.Fatalf("unexpected upload response: %v", out)
	}

	f.store.presignErr = errors.New("signer down")
	cov3xWant(t, cov3xDo(r, http.MethodPost, path, "admin", good), http.StatusInternalServerError, "Failed to create upload URL")

	// No object storage configured.
	noStore := cov3xRouter(NewDocumentHandlers(f.svc, f.audit, nil).RegisterRoutes)
	cov3xWant(t, cov3xDo(noStore, http.MethodPost, path, "admin", good), http.StatusServiceUnavailable, "not configured")
}

func TestCov3xAttachFileValidationAndStorageFailures(t *testing.T) {
	f := newCov3xDocFixture(t)
	r := f.router
	sub := f.newSubmission(t, cov3xHH1)
	path := "/documents/" + sub.ID + "/attach"
	key := f.putPDF(t, sub, "form.pdf", []byte("%PDF"))
	good := cov3xAttachBody(key, "form.pdf", "application/pdf")

	cov3xWant(t, cov3xDo(r, http.MethodPost, path, "coach", good), http.StatusForbidden, "Insufficient role")
	cov3xWant(t, cov3xDo(r, http.MethodPost, "/documents/missing/attach", "admin", good), http.StatusNotFound, "not_found")
	cov3xWant(t, cov3xDo(r, http.MethodPost, path, "parent2", good), http.StatusNotFound, "not_found")
	cov3xWant(t, cov3xDo(r, http.MethodPost, path, "admin", `{}`), http.StatusUnprocessableEntity, "validation_error")
	cov3xWant(t, cov3xDo(r, http.MethodPost, path, "admin", cov3xAttachBody(key, "a/b.pdf", "application/pdf")), http.StatusBadRequest, "invalid_filename")
	cov3xWant(t, cov3xDo(r, http.MethodPost, path, "admin", cov3xAttachBody("documents/other/x/f.pdf", "f.pdf", "application/pdf")), http.StatusBadRequest, "invalid_object_key")
	cov3xWant(t, cov3xDo(r, http.MethodPost, path, "admin", cov3xAttachBody(key, "form.pdf", "text/html")), http.StatusUnprocessableEntity, "invalid_mime_type")

	noStore := cov3xRouter(NewDocumentHandlers(f.svc, f.audit, nil).RegisterRoutes)
	cov3xWant(t, cov3xDo(noStore, http.MethodPost, path, "admin", good), http.StatusServiceUnavailable, "not configured")

	// Object never uploaded.
	missingKey := documentObjectKey(cov3xOrg, sub.ID, "ghost.pdf")
	cov3xWant(t, cov3xDo(r, http.MethodPost, path, "admin", cov3xAttachBody(missingKey, "ghost.pdf", "application/pdf")), http.StatusConflict, "missing_uploaded_file")

	// Exists says yes but the object vanishes before it is measured.
	f.store.existsAlways = true
	cov3xWant(t, cov3xDo(r, http.MethodPost, path, "admin", cov3xAttachBody(missingKey, "ghost.pdf", "application/pdf")), http.StatusConflict, "missing_uploaded_file")

	// Size probe fails with an outage vs another error.
	f.store.sizeErr = fmt.Errorf("stat: %w", context.DeadlineExceeded)
	cov3xWant(t, cov3xDo(r, http.MethodPost, path, "admin", good), http.StatusServiceUnavailable, "storage_unavailable")
	f.store.sizeErr = errors.New("bad gateway body")
	cov3xWant(t, cov3xDo(r, http.MethodPost, path, "admin", good), http.StatusInternalServerError, "internal_error")
	f.store.sizeErr = nil

	// Seal: object grows after the size probe -> too large; read fails -> 500.
	huge := make([]byte, maxDocumentUploadBytes+1)
	hugeKey := f.putPDF(t, sub, "huge.pdf", huge)
	small := int64(10)
	f.store.sizeOverride = &small
	rr := cov3xDo(r, http.MethodPost, path, "admin", cov3xAttachBody(hugeKey, "huge.pdf", "application/pdf"))
	cov3xWant(t, rr, http.StatusUnprocessableEntity, "file_too_large")
	f.store.sizeOverride = nil
	f.store.getErr = errors.New("read failed")
	cov3xWant(t, cov3xDo(r, http.MethodPost, path, "admin", good), http.StatusInternalServerError, "Failed to store uploaded file")
	f.store.getErr = nil

	// The submission stayed pending through every failure.
	got, _ := f.svc.GetSubmission(cov3xOrg, sub.ID)
	if got.Status != "pending" || got.FileObjectKey != "" {
		t.Fatalf("failed attaches must not mutate the submission: %+v", got)
	}
}

func TestCov3xAttachRejectReattachAndReview(t *testing.T) {
	f := newCov3xDocFixture(t)
	r := f.router
	sub := f.newSubmission(t, cov3xHH1)
	path := "/documents/" + sub.ID + "/attach"

	key1 := f.putPDF(t, sub, "one.pdf", []byte("1111"))
	rr := cov3xDo(r, http.MethodPost, path, "parent", cov3xAttachBody(key1, "one.pdf", "application/pdf"))
	cov3xWant(t, rr, http.StatusOK, "")
	first := cov3xJSON(t, rr)["document"].(map[string]any)
	sealed1 := first["file_object_key"].(string)
	if first["status"] != "submitted" || first["file_size_bytes"] != float64(4) || !strings.Contains(sealed1, "/sealed/") {
		t.Fatalf("unexpected attach result: %v", first)
	}

	// A second attach on a submitted document is refused.
	key2 := f.putPDF(t, sub, "two.pdf", []byte("22222"))
	cov3xWant(t, cov3xDo(r, http.MethodPost, path, "admin", cov3xAttachBody(key2, "two.pdf", "application/pdf")), http.StatusConflict, "invalid_transition")
	if ok, _ := f.store.StubStorage.Exists(context.Background(), key2); ok {
		t.Fatal("the unsealed second upload should have been consumed by sealing")
	}

	// Review: roles, bad body, unknown id, wrong state, then reject/verify.
	cov3xWant(t, cov3xDo(r, http.MethodPost, "/documents/"+sub.ID+"/verify", "coach", ""), http.StatusForbidden, "Only admin may verify")
	cov3xWant(t, cov3xDo(r, http.MethodPost, "/documents/"+sub.ID+"/reject", "parent", ""), http.StatusForbidden, "Only admin may reject")
	cov3xWant(t, cov3xDo(r, http.MethodPost, "/documents/"+sub.ID+"/verify", "admin", `{bad`), http.StatusBadRequest, "bad_request")
	cov3xWant(t, cov3xDo(r, http.MethodPost, "/documents/missing/verify", "admin", ""), http.StatusNotFound, "not_found")
	rr = cov3xDo(r, http.MethodPost, "/documents/"+sub.ID+"/reject", "admin", `{"review_notes":"blurry"}`)
	cov3xWant(t, rr, http.StatusOK, "")
	if d := cov3xJSON(t, rr)["document"].(map[string]any); d["status"] != "rejected" || d["review_notes"] != "blurry" {
		t.Fatalf("reject not recorded: %v", d)
	}
	cov3xWant(t, cov3xDo(r, http.MethodPost, "/documents/"+sub.ID+"/reject", "admin", ""), http.StatusConflict, "Cannot reject")

	// Re-attach after rejection replaces the key and drops the superseded
	// object; a failing delete of the old object is only logged.
	key3 := f.putPDF(t, sub, "three.pdf", []byte("333"))
	f.store.deleteErrFor = map[string]error{sealed1: errors.New("locked")}
	rr = cov3xDo(r, http.MethodPost, path, "parent", cov3xAttachBody(key3, "three.pdf", "application/pdf"))
	cov3xWant(t, rr, http.StatusOK, "")
	second := cov3xJSON(t, rr)["document"].(map[string]any)
	if second["file_object_key"] == sealed1 || second["file_size_bytes"] != float64(3) {
		t.Fatalf("re-attach did not replace the file: %v", second)
	}
	found := false
	for _, k := range f.store.deleted {
		if k == sealed1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("superseded object %s was not deleted (deleted: %v)", sealed1, f.store.deleted)
	}

	rr = cov3xDo(r, http.MethodPost, "/documents/"+sub.ID+"/verify", "admin", `{"review_notes":"ok"}`)
	cov3xWant(t, rr, http.StatusOK, "")
	if d := cov3xJSON(t, rr)["document"].(map[string]any); d["status"] != "verified" {
		t.Fatalf("verify not recorded: %v", d)
	}
}

func TestCov3xAttachQuotaExceeded(t *testing.T) {
	f := newCov3xDocFixture(t)
	f.svc.SetStorageQuota(2)
	sub := f.newSubmission(t, cov3xHH1)
	key := f.putPDF(t, sub, "q.pdf", []byte("12345"))
	rr := cov3xDo(f.router, http.MethodPost, "/documents/"+sub.ID+"/attach", "admin", cov3xAttachBody(key, "q.pdf", "application/pdf"))
	cov3xWant(t, rr, http.StatusRequestEntityTooLarge, "storage_quota_exceeded")
	// The sealed copy that was created for the rejected attach is cleaned up.
	if len(f.store.deleted) < 2 {
		t.Fatalf("sealed object must be deleted after a rejected attach, deletes: %v", f.store.deleted)
	}
}

func TestCov3xAttachOversizedObjectDeleteFailureStillRejects(t *testing.T) {
	f := newCov3xDocFixture(t)
	sub := f.newSubmission(t, cov3xHH1)
	key := f.putPDF(t, sub, "big.pdf", []byte("data"))
	over := maxDocumentUploadBytes + 1
	f.store.sizeOverride = &over
	f.store.deleteErrFor = map[string]error{key: errors.New("locked")}
	rr := cov3xDo(f.router, http.MethodPost, "/documents/"+sub.ID+"/attach", "admin", cov3xAttachBody(key, "big.pdf", "application/pdf"))
	cov3xWant(t, rr, http.StatusUnprocessableEntity, "file_too_large")
	if len(f.store.deleted) != 1 || f.store.deleted[0] != key {
		t.Fatalf("deletion of the oversized object should have been attempted once: %v", f.store.deleted)
	}
}

func TestCov3xReviewSubmissionInternalError(t *testing.T) {
	f := newCov3xDocFixture(t)
	r := cov3xRouter(func(rt chi.Router) {
		rt.Post("/boom", func(w http.ResponseWriter, req *http.Request) {
			f.h.reviewSubmission(w, req, "verify", func(orgID, id, reviewer, notes string) (*documents.DocumentSubmission, error) {
				return nil, errors.New("db exploded")
			})
		})
	})
	rr := cov3xDo(r, http.MethodPost, "/boom", "admin", "")
	cov3xWant(t, rr, http.StatusInternalServerError, "Failed to verify submission")
	if strings.Contains(rr.Body.String(), "db exploded") {
		t.Fatal("internal error text must not leak to clients")
	}
}

func TestCov3xDocumentsPersistenceFailures(t *testing.T) {
	t.Run("reads fail when the database is gone", func(t *testing.T) {
		pool := testutil.IsolatedSchemaPool(t)
		svc := documents.NewServiceWithPool(pool)
		dt, err := svc.CreateDocumentType(cov3xOrg, "Waiver", "legal", "", "swimmer", true, true, nil, cov3xAdminID)
		if err != nil {
			t.Fatal(err)
		}
		h := NewDocumentHandlers(svc, audit.NewService(), newCov3xStore())
		r := cov3xRouter(h.RegisterRoutes)
		pool.Close()

		cov3xWant(t, cov3xDo(r, http.MethodGet, "/document-types", "admin", ""), http.StatusInternalServerError, "Failed to list document types")
		cov3xWant(t, cov3xDo(r, http.MethodPost, "/document-types", "admin", `{"name":"N","category":"c"}`), http.StatusInternalServerError, "Failed to create document type")
		cov3xWant(t, cov3xDo(r, http.MethodPut, "/document-types/"+dt.ID, "admin", `{"name":"N2"}`), http.StatusInternalServerError, "Failed to update document type")
		cov3xWant(t, cov3xDo(r, http.MethodDelete, "/document-types/"+dt.ID, "admin", ""), http.StatusInternalServerError, "Failed to delete document type")
		cov3xWant(t, cov3xDo(r, http.MethodGet, "/documents", "admin", ""), http.StatusInternalServerError, "Failed to list document submissions")
		body := fmt.Sprintf(`{"document_type_id":%q,"household_id":%q}`, dt.ID, cov3xHH1)
		cov3xWant(t, cov3xDo(r, http.MethodPost, "/documents", "admin", body), http.StatusInternalServerError, "Failed to create document submission")
	})

	t.Run("writes fail while reads still work", func(t *testing.T) {
		pool := testutil.IsolatedSchemaPool(t)
		svc := documents.NewServiceWithPool(pool)
		dt, err := svc.CreateDocumentType(cov3xOrg, "Waiver", "legal", "", "swimmer", true, true, nil, cov3xAdminID)
		if err != nil {
			t.Fatal(err)
		}
		sub, err := svc.CreateSubmission(cov3xOrg, dt.ID, cov3xHH1, "", cov3xAdminID)
		if err != nil {
			t.Fatal(err)
		}
		store := newCov3xStore()
		h := NewDocumentHandlers(svc, audit.NewService(), store)
		r := cov3xRouter(h.RegisterRoutes)
		key := documentObjectKey(cov3xOrg, sub.ID, "w.pdf")
		_ = store.StubStorage.PutObject(context.Background(), key, "application/pdf", []byte("data"))
		cov3xBlockWrites(t, pool, "document_submissions")

		rr := cov3xDo(r, http.MethodPost, "/documents/"+sub.ID+"/attach", "admin", cov3xAttachBody(key, "w.pdf", "application/pdf"))
		cov3xWant(t, rr, http.StatusInternalServerError, "Failed to attach file")
		got, err := svc.GetSubmission(cov3xOrg, sub.ID)
		if err != nil || got.Status != "pending" {
			t.Fatalf("failed attach must leave the submission pending: %+v %v", got, err)
		}
	})
}

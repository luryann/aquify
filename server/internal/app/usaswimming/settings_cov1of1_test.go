package usaswimming

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/testutil"
)

func TestCov1xCipher(t *testing.T) {
	if NewCipher("") != nil {
		t.Fatal("empty secret must yield nil cipher")
	}
	var nilC *Cipher
	if _, err := nilC.Encrypt("x"); !errors.Is(err, ErrEncryptionUnavailable) {
		t.Fatalf("Encrypt nil: %v", err)
	}
	if _, err := nilC.EncryptFor("o", "x"); !errors.Is(err, ErrEncryptionUnavailable) {
		t.Fatalf("EncryptFor nil: %v", err)
	}
	if _, err := nilC.DecryptFor("o", "x"); !errors.Is(err, ErrEncryptionUnavailable) {
		t.Fatalf("DecryptFor nil: %v", err)
	}
	if _, err := nilC.Decrypt("x"); !errors.Is(err, ErrEncryptionUnavailable) {
		t.Fatalf("Decrypt nil: %v", err)
	}

	c := NewCipher("secret-a")
	enc, err := c.Encrypt("hello")
	if err != nil || strings.HasPrefix(enc, boundPrefix) {
		t.Fatalf("%q %v", enc, err)
	}
	if got, err := c.Decrypt(enc); err != nil || got != "hello" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := c.Decrypt("!!!not-base64!!!"); err == nil || !strings.Contains(err.Error(), "unreadable") {
		t.Fatalf("bad base64: %v", err)
	}
	if _, err := c.Decrypt("QQ=="); err == nil || !strings.Contains(err.Error(), "unreadable") {
		t.Fatalf("too short: %v", err)
	}
	if _, err := NewCipher("secret-b").Decrypt(enc); err == nil || !strings.Contains(err.Error(), "could not be decrypted") {
		t.Fatalf("wrong key: %v", err)
	}

	bound, err := c.EncryptFor("org", "hello")
	if err != nil || !strings.HasPrefix(bound, boundPrefix) {
		t.Fatalf("%q %v", bound, err)
	}
	if _, err := c.DecryptFor("org", boundPrefix+"!!!"); err == nil || !strings.Contains(err.Error(), "unreadable") {
		t.Fatalf("bound bad base64: %v", err)
	}
	if _, err := c.DecryptFor("org", boundPrefix+"QQ=="); err == nil || !strings.Contains(err.Error(), "unreadable") {
		t.Fatalf("bound too short: %v", err)
	}
	if _, err := c.DecryptFor("other", bound); err == nil || !strings.Contains(err.Error(), "wrong organization") {
		t.Fatalf("wrong org: %v", err)
	}
}

func TestCov1xUpdateBranches(t *testing.T) {
	stub := NewStubClient()
	svc := NewService(NewCipher("k"), stub)
	org := "org-1"

	// Stub base URL + enable without a key is allowed with a stub client.
	v, err := svc.Update(org, Updates{BaseURL: ptr("stub"), Enabled: boolPtr(true)}, "u")
	if err != nil || !v.Enabled || !v.Configured || !v.StubAvailable || v.HasAPIKey || v.UpdatedAt == nil {
		t.Fatalf("%+v %v", v, err)
	}
	// Clearing the base URL, then enabling fails.
	if v, err = svc.Update(org, Updates{BaseURL: ptr("  "), Enabled: boolPtr(false)}, "u"); err != nil || v.BaseURL != "" || v.Configured || v.Enabled {
		t.Fatalf("%+v %v", v, err)
	}
	if _, err = svc.Update(org, Updates{Enabled: boolPtr(true)}, "u"); !errors.Is(err, ErrInvalidSettings) {
		t.Fatalf("enable without base: %v", err)
	}

	// Vendor fields.
	v, err = svc.Update(org, Updates{
		BaseURL: ptr("http://127.0.0.1:9"), TokenURL: ptr(" http://127.0.0.1:9/token "),
		ClientID: ptr(" cid "), ClubID: ptr(" club "), APIKey: ptr(" sekret-1234 "),
	}, "u")
	if err != nil || v.TokenURL != "http://127.0.0.1:9/token" || v.ClientID != "cid" || v.ClubID != "club" || !v.VendorAPI || v.APIKeyHint != "1234" {
		t.Fatalf("%+v %v", v, err)
	}
	// Invalid token URL (stub service allows loopback but not a bad scheme/userinfo).
	if _, err = svc.Update(org, Updates{TokenURL: ptr("https://u:p@x.example.com")}, "u"); !errors.Is(err, ErrInvalidSettings) {
		t.Fatalf("token url: %v", err)
	}
	// Clearing token URL and key.
	v, err = svc.Update(org, Updates{TokenURL: ptr(""), APIKey: ptr("")}, "u")
	if err != nil || v.VendorAPI || v.HasAPIKey || v.APIKeyHint != "" {
		t.Fatalf("%+v %v", v, err)
	}
	// Without a stub, loopback token URLs are rejected.
	prod := NewService(NewCipher("k"), nil)
	if _, err = prod.Update(org, Updates{TokenURL: ptr("https://127.0.0.1/token")}, "u"); !errors.Is(err, ErrInvalidSettings) {
		t.Fatalf("prod token url: %v", err)
	}
}

func TestCov1xRecordAndGet(t *testing.T) {
	svc := NewService(NewCipher("k"), nil)
	svc.RecordTest("o", true, "fine")
	svc.RecordSync("o", "synced 3")
	v, err := svc.Get("o")
	if err != nil || !v.LastTestOK || v.LastTestMsg != "fine" || v.LastTestAt == nil || v.LastSyncMsg != "synced 3" || v.LastSyncAt == nil {
		t.Fatalf("%+v %v", v, err)
	}
	if v, _ = svc.Get("unknown"); v.Configured || v.BaseURL != "" || v.UpdatedAt != nil {
		t.Fatalf("unknown org: %+v", v)
	}
}

func TestCov1xClientVariants(t *testing.T) {
	srv := cov1xNewSwims(t)
	stub := NewStubClient()
	svc := NewService(NewCipher("k"), stub)
	org := "org-x"

	if _, err := svc.Client(org); !errors.Is(err, ErrDisabled) {
		t.Fatalf("disabled: %v", err)
	}
	if _, _, err := svc.Vendor(org); !errors.Is(err, ErrDisabled) {
		t.Fatalf("vendor disabled: %v", err)
	}
	if _, err := svc.TestClient(org); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("unconfigured: %v", err)
	}
	if !svc.SupportsTimes(org) {
		t.Fatal("legacy/unconfigured orgs support times")
	}

	// Stub.
	if _, err := svc.Update(org, Updates{BaseURL: ptr("stub"), Enabled: boolPtr(true)}, "u"); err != nil {
		t.Fatal(err)
	}
	if c, err := svc.Client(org); err != nil || c != Client(stub) {
		t.Fatalf("stub client: %v %v", c, err)
	}
	if _, _, err := svc.Vendor(org); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("stub is not a vendor client: %v", err)
	}

	// Legacy HTTP client (real, against a local server).
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer legacy-key" {
			w.WriteHeader(401)
		}
	}))
	defer web.Close()
	if _, err := svc.Update(org, Updates{BaseURL: &web.URL, APIKey: ptr("legacy-key")}, "u"); err != nil {
		t.Fatal(err)
	}
	c, err := svc.Client(org)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.(*HTTPClient); !ok {
		t.Fatalf("want *HTTPClient got %T", c)
	}
	if err := c.TestConnection(context.Background()); err != nil {
		t.Fatalf("decrypted key should authenticate: %v", err)
	}

	// Vendor client (real, against the fake vendor API).
	if _, err := svc.Update(org, Updates{BaseURL: &srv.srv.URL, TokenURL: ptr(srv.srv.URL + "/token"), ClientID: ptr("cid"), ClubID: ptr("club-9"), APIKey: ptr("vendor-secret")}, "u"); err != nil {
		t.Fatal(err)
	}
	if svc.SupportsTimes(org) {
		t.Fatal("vendor orgs cannot list times")
	}
	srv.apiBody = `[]`
	vc, club, err := svc.Vendor(org)
	if err != nil || club != "club-9" {
		t.Fatalf("%v %q", err, club)
	}
	if err := vc.TestConnection(context.Background()); err != nil {
		t.Fatalf("vendor test connection: %v", err)
	}

	// A non-vendor client returned by the builder is unsupported.
	svc.NewSwims = func(SwimsConfig) (Client, error) { return stub, nil }
	if _, _, err := svc.Vendor(org); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("non-vendor: %v", err)
	}
	// Builder errors propagate.
	boom := errors.New("builder failed")
	svc.NewSwims = func(SwimsConfig) (Client, error) { return nil, boom }
	if _, _, err := svc.Vendor(org); !errors.Is(err, boom) {
		t.Fatalf("builder err: %v", err)
	}
	svc.NewHTTP = func(ClientConfig) (Client, error) { return nil, boom }
	if _, err := svc.TestClient("org-legacy-missing"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("missing: %v", err)
	}

	// Vendor settings without a client id are not configured.
	if _, err := svc.Update(org, Updates{ClientID: ptr("")}, "u"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Vendor(org); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("no client id: %v", err)
	}
	if _, err := svc.Client(org); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("no client id: %v", err)
	}
}

func TestCov1xBuildClientDecryptFailure(t *testing.T) {
	good := NewService(NewCipher("key-1"), nil)
	if _, err := good.Update("o", Updates{BaseURL: ptr("https://swims.example.com"), APIKey: ptr("abcd1234")}, "u"); err != nil {
		t.Fatal(err)
	}
	row, _ := good.load("o")
	rotated := NewService(NewCipher("key-2"), nil)
	if _, err := rotated.buildClient(row); err == nil || !strings.Contains(err.Error(), "could not be decrypted") {
		t.Fatalf("rotated key: %v", err)
	}
	other := *row
	other.OrgID = "another-org"
	if _, err := good.buildClient(&other); err == nil || !strings.Contains(err.Error(), "wrong organization") {
		t.Fatalf("cross-org: %v", err)
	}
	// The default builders create real clients.
	c, err := good.buildClient(row)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.(*HTTPClient); !ok {
		t.Fatalf("%T", c)
	}
	vrow := *row
	vrow.TokenURL, vrow.ClientID = "https://token.example.com/t", "cid"
	vc, err := good.buildClient(&vrow)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := vc.(*SwimsClient); !ok {
		t.Fatalf("%T", vc)
	}
}

func TestCov1xPostgresPersistence(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	cipher := NewCipher("db-key")
	org := uuid.NewString()
	svc := NewServiceWithPool(pool, cipher, nil)

	v, err := svc.Get(org)
	if err != nil || v.Configured || v.BaseURL != "" {
		t.Fatalf("empty org: %+v %v", v, err)
	}

	v, err = svc.Update(org, Updates{BaseURL: ptr("https://swims.example.com"), APIKey: ptr("abcd1234"),
		TokenURL: ptr("https://token.example.com/t"), ClientID: ptr("cid"), ClubID: ptr("club")}, "")
	if err != nil || !v.Configured || v.APIKeyHint != "1234" {
		t.Fatalf("%+v %v", v, err)
	}
	if _, err := svc.Update(org, Updates{Enabled: boolPtr(true)}, ""); err != nil {
		t.Fatal(err)
	}
	svc.RecordTest(org, false, "nope")
	svc.RecordSync(org, "did sync")

	// A second instance (separate cache) sees the persisted state.
	other := NewServiceWithPool(pool, cipher, nil)
	got, err := other.Get(org)
	if err != nil {
		t.Fatal(err)
	}
	if got.BaseURL != "https://swims.example.com" || !got.Enabled || !got.HasAPIKey || got.TokenURL != "https://token.example.com/t" ||
		got.ClientID != "cid" || got.ClubID != "club" || got.LastTestOK || got.LastTestMsg != "nope" || got.LastSyncMsg != "did sync" ||
		got.LastTestAt == nil || got.LastSyncAt == nil || got.UpdatedAt == nil {
		t.Fatalf("persisted view: %+v", got)
	}
	vc, club, err := other.Vendor(org)
	if err != nil || club != "club" {
		t.Fatalf("%v %q", err, club)
	}
	if _, ok := vc.(*SwimsClient); !ok {
		t.Fatalf("%T", vc)
	}
	// A different org does not see it.
	if v, _ := other.Get(uuid.NewString()); v.BaseURL != "" {
		t.Fatal("tenant leak")
	}

	// nil pool falls back to in-memory.
	if mem := NewServiceWithPool(nil, cipher, nil); mem.repo != nil {
		t.Fatal("nil pool must stay in-memory")
	}
}

func TestCov1xPostgresErrors(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	org := uuid.NewString()
	svc := NewServiceWithPool(pool, NewCipher("k"), NewStubClient())
	if _, err := svc.Update(org, Updates{BaseURL: ptr("stub"), Enabled: boolPtr(true)}, ""); err != nil {
		t.Fatal(err)
	}
	pool.Close()

	if _, err := svc.Get(org); err == nil || !strings.Contains(err.Error(), "usaswimming: get") {
		t.Fatalf("Get on closed pool: %v", err)
	}
	if _, err := svc.Update(org, Updates{ClubID: ptr("c")}, ""); err == nil {
		t.Fatal("Update should surface load error")
	}
	if _, err := svc.Client(org); err == nil {
		t.Fatal("Client should surface load error")
	}
	if _, _, err := svc.Vendor(org); err == nil {
		t.Fatal("Vendor should surface load error")
	}
	if _, err := svc.TestClient(org); err == nil {
		t.Fatal("TestClient should surface load error")
	}
	if svc.SupportsTimes(org) {
		t.Fatal("SupportsTimes must be false when settings cannot be read")
	}
	svc.RecordTest(org, true, "x") // must not panic
	svc.RecordSync(org, "x")
	if err := svc.save(&Settings{OrgID: org}); err == nil {
		t.Fatal("save on closed pool should fail")
	}
}

func TestCov1xUpdateSaveFailure(t *testing.T) {
	// Load succeeds (no row), but the upsert violates a DB constraint
	// (updated_by must reference a real user), so Update returns the error.
	pool := testutil.IsolatedSchemaPool(t)
	svc := NewServiceWithPool(pool, NewCipher("k"), nil)
	_, err := svc.Update(uuid.NewString(), Updates{ClubID: ptr("c")}, uuid.NewString())
	if err == nil {
		t.Fatal("expected FK violation for unknown updated_by")
	}
}

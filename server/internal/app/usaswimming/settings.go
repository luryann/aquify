package usaswimming

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// StubBaseURL, as the base URL, selects the in-memory stub client. Only
// honoured when the service was built with a stub (development/test).
const StubBaseURL = "stub"

var (
	ErrEncryptionUnavailable = errors.New("integration encryption key is not configured")
	ErrInvalidSettings       = errors.New("invalid integration settings")
	ErrDisabled              = errors.New("usa swimming integration is disabled")
)

// Cipher encrypts API keys with AES-256-GCM. The key is derived from a
// server-side secret (INTEGRATIONS_ENCRYPTION_KEY) via SHA-256.
type Cipher struct{ aead cipher.AEAD }

// NewCipher derives a cipher from secret; empty secret returns nil.
func NewCipher(secret string) *Cipher {
	if secret == "" {
		return nil
	}
	sum := sha256.Sum256([]byte(secret))
	block, _ := aes.NewCipher(sum[:])
	aead, _ := cipher.NewGCM(block)
	return &Cipher{aead: aead}
}

func (c *Cipher) Encrypt(plain string) (string, error) {
	if c == nil {
		return "", ErrEncryptionUnavailable
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(c.aead.Seal(nonce, nonce, []byte(plain), nil)), nil
}

func (c *Cipher) Decrypt(enc string) (string, error) {
	if c == nil {
		return "", ErrEncryptionUnavailable
	}
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil || len(raw) < c.aead.NonceSize() {
		return "", errors.New("stored api key is unreadable")
	}
	n := c.aead.NonceSize()
	plain, err := c.aead.Open(nil, raw[:n], raw[n:], nil)
	if err != nil {
		return "", errors.New("stored api key could not be decrypted (encryption key changed?)")
	}
	return string(plain), nil
}

// Settings is the stored per-org record. APIKeyEnc never leaves the package.
type Settings struct {
	OrgID       string
	BaseURL     string
	APIKeyEnc   string
	APIKeyHint  string // last 4 chars, for display
	Enabled     bool
	LastTestAt  *time.Time
	LastTestOK  bool
	LastTestMsg string
	LastSyncAt  *time.Time
	LastSyncMsg string
	UpdatedAt   time.Time
	UpdatedBy   string
	// Vendor API (OAuth client credentials); APIKeyEnc holds the client secret.
	TokenURL string
	ClientID string
	ClubID   string
}

// View is the redacted API representation.
type View struct {
	BaseURL       string     `json:"base_url"`
	Enabled       bool       `json:"enabled"`
	HasAPIKey     bool       `json:"has_api_key"`
	APIKeyHint    string     `json:"api_key_hint,omitempty"`
	Configured    bool       `json:"configured"`
	StubAvailable bool       `json:"stub_available"`
	LastTestAt    *time.Time `json:"last_test_at,omitempty"`
	LastTestOK    bool       `json:"last_test_ok"`
	LastTestMsg   string     `json:"last_test_message,omitempty"`
	LastSyncAt    *time.Time `json:"last_sync_at,omitempty"`
	LastSyncMsg   string     `json:"last_sync_message,omitempty"`
	UpdatedAt     *time.Time `json:"updated_at,omitempty"`
	TokenURL      string     `json:"token_url"`
	ClientID      string     `json:"client_id"`
	ClubID        string     `json:"club_id"`
	VendorAPI     bool       `json:"vendor_api"` // OAuth vendor API (roster/verification) vs legacy bearer key
}

// Updates patches settings; nil leaves a field unchanged. An empty APIKey
// string clears the key; BaseURL must validate.
type Updates struct {
	BaseURL  *string
	APIKey   *string
	Enabled  *bool
	TokenURL *string
	ClientID *string
	ClubID   *string
}

// Service stores settings and builds clients.
type Service struct {
	mu     sync.RWMutex
	rows   map[string]*Settings
	repo   *repository
	cipher *Cipher
	stub   Client // non-nil only in development/test
	// Overridable for tests.
	NewHTTP  func(ClientConfig) (Client, error)
	NewSwims func(SwimsConfig) (Client, error)
}

// NewService returns an in-memory service. cipher may be nil (saving an API
// key then fails closed); stub may be nil (production).
func NewService(c *Cipher, stub Client) *Service {
	return &Service{rows: map[string]*Settings{}, cipher: c, stub: stub,
		NewHTTP:  func(cfg ClientConfig) (Client, error) { return NewHTTPClient(cfg) },
		NewSwims: func(cfg SwimsConfig) (Client, error) { return NewSwimsClient(cfg) }}
}

// NewServiceWithPool adds Postgres persistence (table usa_swimming_integrations).
func NewServiceWithPool(pool *pgxpool.Pool, c *Cipher, stub Client) *Service {
	s := NewService(c, stub)
	if pool != nil {
		s.repo = &repository{pool: pool}
	}
	return s
}

func (s *Service) load(orgID string) (*Settings, error) {
	if s.repo != nil {
		row, err := s.repo.get(context.Background(), orgID)
		if err != nil {
			return nil, err
		}
		if row != nil {
			return row, nil
		}
		return &Settings{OrgID: orgID}, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if r, ok := s.rows[orgID]; ok {
		cp := *r
		return &cp, nil
	}
	return &Settings{OrgID: orgID}, nil
}

func (s *Service) save(row *Settings) error {
	row.UpdatedAt = time.Now().UTC()
	if s.repo != nil {
		return s.repo.upsert(context.Background(), row)
	}
	s.mu.Lock()
	cp := *row
	s.rows[row.OrgID] = &cp
	s.mu.Unlock()
	return nil
}

func (s *Service) view(r *Settings) *View {
	v := &View{BaseURL: r.BaseURL, Enabled: r.Enabled, HasAPIKey: r.APIKeyEnc != "", APIKeyHint: r.APIKeyHint,
		LastTestAt: r.LastTestAt, LastTestOK: r.LastTestOK, LastTestMsg: r.LastTestMsg,
		LastSyncAt: r.LastSyncAt, LastSyncMsg: r.LastSyncMsg, StubAvailable: s.stub != nil,
		TokenURL: r.TokenURL, ClientID: r.ClientID, ClubID: r.ClubID, VendorAPI: r.TokenURL != ""}
	v.Configured = r.BaseURL != "" && (r.APIKeyEnc != "" || (r.BaseURL == StubBaseURL && s.stub != nil))
	if !r.UpdatedAt.IsZero() {
		t := r.UpdatedAt
		v.UpdatedAt = &t
	}
	return v
}

// Get returns the redacted settings.
func (s *Service) Get(orgID string) (*View, error) {
	r, err := s.load(orgID)
	if err != nil {
		return nil, err
	}
	return s.view(r), nil
}

// Update applies a patch and returns the redacted result.
func (s *Service) Update(orgID string, u Updates, by string) (*View, error) {
	r, err := s.load(orgID)
	if err != nil {
		return nil, err
	}
	if u.BaseURL != nil {
		b := strings.TrimSpace(*u.BaseURL)
		switch {
		case b == "":
		case b == StubBaseURL && s.stub != nil:
		default:
			if err := ValidateBaseURL(b, s.stub != nil); err != nil {
				return nil, errors.Join(ErrInvalidSettings, err)
			}
		}
		r.BaseURL = b
	}
	if u.TokenURL != nil {
		t := strings.TrimSpace(*u.TokenURL)
		if t != "" {
			if err := ValidateBaseURL(t, s.stub != nil); err != nil {
				return nil, errors.Join(ErrInvalidSettings, err)
			}
		}
		r.TokenURL = t
	}
	if u.ClientID != nil {
		r.ClientID = strings.TrimSpace(*u.ClientID)
	}
	if u.ClubID != nil {
		r.ClubID = strings.TrimSpace(*u.ClubID)
	}
	if u.APIKey != nil {
		k := strings.TrimSpace(*u.APIKey)
		if k == "" {
			r.APIKeyEnc, r.APIKeyHint = "", ""
		} else {
			enc, err := s.cipher.Encrypt(k)
			if err != nil {
				return nil, err
			}
			r.APIKeyEnc = enc
			kr := []rune(k)
			r.APIKeyHint = string(kr[max(0, len(kr)-4):])
		}
	}
	if u.Enabled != nil {
		if *u.Enabled && (r.BaseURL == "" || (r.APIKeyEnc == "" && !(r.BaseURL == StubBaseURL && s.stub != nil))) {
			return nil, errors.Join(ErrInvalidSettings, errors.New("set a base URL and API key before enabling"))
		}
		r.Enabled = *u.Enabled
	}
	r.UpdatedBy = by
	if err := s.save(r); err != nil {
		return nil, err
	}
	return s.view(r), nil
}

// Client builds the client for an org. It requires the integration to be
// enabled and configured.
func (s *Service) Client(orgID string) (Client, error) {
	r, err := s.load(orgID)
	if err != nil {
		return nil, err
	}
	if !r.Enabled {
		return nil, ErrDisabled
	}
	return s.buildClient(r)
}

// Vendor returns the org's vendor-API client and configured club ID from a
// single settings read. ErrUnsupported means the org uses the legacy bearer-key
// (or stub) client, which has no roster/verification/registration endpoints.
func (s *Service) Vendor(orgID string) (VendorClient, string, error) {
	r, err := s.load(orgID)
	if err != nil {
		return nil, "", err
	}
	if !r.Enabled {
		return nil, "", ErrDisabled
	}
	if r.TokenURL == "" {
		return nil, "", ErrUnsupported
	}
	c, err := s.buildClient(r)
	if err != nil {
		return nil, "", err
	}
	vc, ok := c.(VendorClient)
	if !ok {
		return nil, "", ErrUnsupported
	}
	return vc, r.ClubID, nil
}

// SupportsTimes reports whether the org's client can list official times, which
// the vendor API cannot.
func (s *Service) SupportsTimes(orgID string) bool {
	r, err := s.load(orgID)
	return err == nil && r.TokenURL == ""
}

// TestClient is like Client but does not require the enabled flag, so an admin
// can verify credentials before turning the integration on.
func (s *Service) TestClient(orgID string) (Client, error) {
	r, err := s.load(orgID)
	if err != nil {
		return nil, err
	}
	return s.buildClient(r)
}

func (s *Service) buildClient(r *Settings) (Client, error) {
	if r.BaseURL == StubBaseURL && s.stub != nil {
		return s.stub, nil
	}
	if r.BaseURL == "" || r.APIKeyEnc == "" {
		return nil, ErrNotConfigured
	}
	key, err := s.cipher.Decrypt(r.APIKeyEnc)
	if err != nil {
		return nil, err
	}
	if r.TokenURL != "" {
		if r.ClientID == "" {
			return nil, ErrNotConfigured
		}
		return s.NewSwims(SwimsConfig{BaseURL: r.BaseURL, TokenURL: r.TokenURL, ClientID: r.ClientID, ClientSecret: key, AllowLoopback: s.stub != nil})
	}
	return s.NewHTTP(ClientConfig{BaseURL: r.BaseURL, APIKey: key, AllowLoopback: s.stub != nil})
}

// RecordTest stores the outcome of a connection test.
func (s *Service) RecordTest(orgID string, ok bool, msg string) {
	if r, err := s.load(orgID); err == nil {
		now := time.Now().UTC()
		r.LastTestAt, r.LastTestOK, r.LastTestMsg = &now, ok, msg
		_ = s.save(r)
	}
}

// RecordSync stores the outcome of a sync.
func (s *Service) RecordSync(orgID, msg string) {
	if r, err := s.load(orgID); err == nil {
		now := time.Now().UTC()
		r.LastSyncAt, r.LastSyncMsg = &now, msg
		_ = s.save(r)
	}
}

//  PROPRIETARY AND CONFIDENTIAL
//  COPYRIGHT (C) 2026 RYAN LU. ALL RIGHTS RESERVED.
//
//  THIS SOURCE FILE IS PART OF A PROPRIETARY, CLOSED-SOURCE SOFTWARE
//  SYSTEM OWNED EXCLUSIVELY BY RYAN LU. THIS FILE AND ALL ASSOCIATED
//  SOURCE CODE, OBJECT CODE, MODULES, COMPONENTS, DOCUMENTATION, AND
//  RELATED MATERIALS ARE CONFIDENTIAL AND PROPRIETARY INTELLECTUAL
//  PROPERTY OF RYAN LU.
//
//  USE OF THIS SOFTWARE IS PERMITTED ONLY FOR PERSONS OR ENTITIES
//  EXPRESSLY AUTHORIZED BY RYAN LU AND ONLY AFTER ACCEPTANCE OF THE
//  APPLICABLE LICENSE TERMS. DARE AQUATICS AND OTHER TEAMS OR
//  ORGANIZATIONS MAY USE THIS SOFTWARE ONLY IF SEPARATELY APPROVED BY
//  RYAN LU.
//
//  NO COPYING, MODIFICATION, DISTRIBUTION, HOSTING, DISCLOSURE,
//  REVERSE ENGINEERING, DECOMPILATION, OR OTHER USE IS PERMITTED
//  EXCEPT AS EXPRESSLY AUTHORIZED IN WRITING BY RYAN LU.
//
//  ALL RIGHTS NOT EXPRESSLY GRANTED ARE RESERVED BY RYAN LU.

package authn

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrSessionNotFound      = errors.New("session not found")
	ErrSessionExpired       = errors.New("session expired")
	ErrSessionRevoked       = errors.New("session revoked")
	ErrInvalidMFAFactor     = errors.New("invalid mfa factor")
	ErrInvalidMFACode       = errors.New("invalid mfa challenge code")
	ErrUnsupportedMFAFactor = errors.New("unsupported mfa factor")
	ErrMFATooManyAttempts   = errors.New("too many failed mfa attempts; try again later")
	ErrOIDCNotConfigured    = errors.New("oidc is not configured")
	ErrOIDCStateInvalid     = errors.New("oidc state is invalid")
)

type Config struct {
	EnableDevStubTokens bool
	SeedLocalUsers      bool
	OIDC                OIDCConfig
	// Logger is used for diagnostic warnings (e.g. org_id fallback). If nil, slog.Default() is used.
	Logger *slog.Logger
}

type OIDCConfig struct {
	AuthorizeURL          string
	TokenURL              string
	JWKSURL               string
	EndSessionURL         string
	RedirectURL           string
	PostLogoutRedirectURL string
	ClientID              string
	ClientSecret          string
	ExpectedIssuer        string
	StateSigningKey       string
	HTTPTimeout           time.Duration
}

type Service struct {
	mu               sync.RWMutex
	sessionsByAccess map[string]*sessionRecord
	byRefresh        map[string]*sessionRecord
	mfaFactorsByUser map[string][]mfaFactorRecord
	assignments      []RoleAssignment
	staticPrincipals map[string]*appmiddleware.Principal
	cfg              Config
	httpClient       *http.Client
	logger           *slog.Logger
	pool             *pgxpool.Pool

	// householdLookup is an optional function injected from the members service
	// that resolves a Keycloak user ID to a household ID. It is called during
	// OIDCCallback to attach household_id to the newly-issued session.
	householdLookup func(keycloakUserID string) string

	// swimmerLookup is an optional function injected from the members service
	// that resolves a Keycloak user ID to a specific linked swimmer ID. It is
	// called during OIDCCallback to attach swimmer_id to the newly-issued
	// session so a swimmer-role principal's "self" is never ambiguous across
	// siblings in the same household.
	swimmerLookup func(keycloakUserID string) string

	// TOTP replay prevention: "factorID|code" → time the code was last accepted.
	usedTOTPCodes map[string]time.Time

	// mfaFailures counts recent failed challenges per factor so a 6-digit code
	// cannot be brute-forced; entries expire with their window.
	mfaFailures map[string]mfaAttempts

	// JWKS cache — keyed independently with its own mutex so JWKS fetches do
	// not block unrelated session operations.
	jwksMu       sync.RWMutex
	jwksCache    *oidcJWKSet
	jwksCachedAt time.Time
	jwksCacheTTL time.Duration
}

// SetHouseholdLookup wires a household-lookup function into the service.
// The function receives a Keycloak user ID and should return the corresponding
// household ID, or an empty string if no household is linked.
func (s *Service) SetHouseholdLookup(fn func(keycloakUserID string) string) {
	s.householdLookup = fn
}

// SetSwimmerLookup wires a swimmer-lookup function into the service.
// The function receives a Keycloak user ID and should return the ID of the
// swimmer record linked to it, or an empty string if none is linked.
func (s *Service) SetSwimmerLookup(fn func(keycloakUserID string) string) {
	s.swimmerLookup = fn
}

// SetPool wires a DB pool into the service and enables persisted auth/session
// state. MFA factors are loaded from the DB so restart-time state is restored
// before the service handles requests.
func (s *Service) SetPool(pool *pgxpool.Pool) {
	if pool == nil {
		return
	}
	s.pool = pool
	s.loadMFAFactorsFromDB()
	if s.cfg.SeedLocalUsers {
		s.seedKnownLocalUsers()
	}
}

func (s *Service) loadMFAFactorsFromDB() {
	ctx := context.Background()
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, user_id::text, COALESCE(org_id::text, ''), factor_type,
		       COALESCE(label, ''), is_verified,
		       COALESCE(totp_secret, ''), COALESCE(backup_codes, '[]'::jsonb),
		       COALESCE(created_at, now()), last_challenged_at
		FROM mfa_factors`)
	if err != nil {
		s.logger.Warn("authn: failed to load MFA factors from DB", "error", err)
		return
	}
	defer rows.Close()

	s.mu.Lock()
	defer s.mu.Unlock()

	for rows.Next() {
		var r mfaFactorRecord
		var backupCodesJSON []byte
		if err := rows.Scan(
			&r.Factor.ID, &r.Factor.UserID, &r.Factor.OrgID, &r.Factor.FactorType,
			&r.Factor.Label, &r.Factor.IsVerified, &r.TOTPSecret,
			&backupCodesJSON, &r.Factor.CreatedAt, &r.Factor.LastChallengedAt,
		); err != nil {
			s.logger.Warn("authn: scan MFA factor row", "error", err)
			continue
		}
		if len(backupCodesJSON) > 0 {
			_ = json.Unmarshal(backupCodesJSON, &r.BackupCodes)
		}
		s.mfaFactorsByUser[r.Factor.UserID] = append(s.mfaFactorsByUser[r.Factor.UserID], r)
	}
}

func (s *Service) seedKnownLocalUsers() {
	ctx := context.Background()
	for _, user := range LocalDevUsers() {
		if err := s.upsertAuthenticatedUser(ctx, user.UserID, user.Email, user.Name, LocalDevOrgID, false); err != nil {
			s.logger.Warn("authn: failed to seed local dev user", "user_id", user.UserID, "email", user.Email, "error", err)
		}
	}
}

func (s *Service) syncAuthenticatedUser(ctx context.Context, input SessionInput) error {
	return s.upsertAuthenticatedUser(ctx, input.UserID, input.Email, input.Name, input.OrgID, true)
}

func (s *Service) upsertAuthenticatedUser(ctx context.Context, userID, email, name, orgID string, markLastLogin bool) error {
	if s.pool == nil {
		return nil
	}

	userID = strings.TrimSpace(userID)
	orgID = strings.TrimSpace(orgID)
	if userID == "" || orgID == "" {
		return nil
	}
	if _, err := uuid.Parse(userID); err != nil {
		s.logger.Warn("authn: skipping DB user sync because user_id is not a UUID", "user_id", userID, "error", err)
		return nil
	}
	if _, err := uuid.Parse(orgID); err != nil {
		s.logger.Warn("authn: skipping DB user sync because org_id is not a UUID", "org_id", orgID, "error", err)
		return nil
	}

	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		email = fmt.Sprintf("%s@local.invalid", userID)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = email
	}

	var lastLoginAt *time.Time
	if markLastLogin {
		now := time.Now().UTC()
		lastLoginAt = &now
	}

	err := s.upsertAuthenticatedUserRow(ctx, userID, email, name, orgID, lastLoginAt, false)
	if isUniqueConstraintViolation(err, "users_email_key") {
		// Identity policy: the email belongs to a different users row. Never
		// overwrite an existing row's email with a placeholder; keep it as is.
		// Only a brand new row (email is NOT NULL) gets a surrogate address.
		s.logger.Error("authn: email already belongs to another users row; keeping existing email for this user",
			"user_id", userID,
			"email", email,
		)
		err = s.upsertAuthenticatedUserRow(ctx, userID, surrogateSyncedUserEmail(userID), name, orgID, lastLoginAt, true)
	}
	if err != nil {
		return fmt.Errorf("upsert authenticated user: %w", err)
	}
	return nil
}

func (s *Service) upsertAuthenticatedUserRow(ctx context.Context, userID, email, name, orgID string, lastLoginAt *time.Time, keepExistingEmail bool) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO users (
			id, org_id, email, oidc_subject, display_name, status, last_login_at, created_at, updated_at
		)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, 'active', $6, now(), now())
		ON CONFLICT (id) DO UPDATE SET
			org_id        = EXCLUDED.org_id,
			email         = CASE WHEN $7::boolean THEN users.email ELSE EXCLUDED.email END,
			oidc_subject  = EXCLUDED.oidc_subject,
			display_name  = EXCLUDED.display_name,
			status        = 'active',
			last_login_at = COALESCE(EXCLUDED.last_login_at, users.last_login_at),
			updated_at    = now()`,
		userID, orgID, email, userID, name, lastLoginAt, keepExistingEmail,
	)
	return err
}

func surrogateSyncedUserEmail(userID string) string {
	return fmt.Sprintf("%s@users.local.invalid", strings.ToLower(strings.TrimSpace(userID)))
}

func isUniqueConstraintViolation(err error, constraintName string) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	if pgErr.Code != "23505" {
		return false
	}
	if constraintName == "" {
		return true
	}
	return pgErr.ConstraintName == constraintName
}

// marshalBackupCodes encodes backup-code hashes for storage; nil when there are none.
func marshalBackupCodes(codes []backupCodeRecord) []byte {
	if len(codes) == 0 {
		return nil
	}
	data, _ := json.Marshal(codes)
	return data
}

func (s *Service) saveMFAFactorToDB(r mfaFactorRecord) error {
	if s.pool == nil {
		return nil
	}
	backupCodesJSON := marshalBackupCodes(r.BackupCodes)
	var totpSecret *string
	if r.TOTPSecret != "" {
		totpSecret = &r.TOTPSecret
	}
	ctx := context.Background()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO mfa_factors (id, user_id, org_id, factor_type, label, is_verified,
		                         totp_secret, backup_codes, created_at, last_challenged_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
		    is_verified        = EXCLUDED.is_verified,
		    backup_codes       = EXCLUDED.backup_codes,
		    last_challenged_at = EXCLUDED.last_challenged_at`,
		r.Factor.ID, r.Factor.UserID, r.Factor.OrgID, r.Factor.FactorType,
		r.Factor.Label, r.Factor.IsVerified,
		totpSecret, backupCodesJSON,
		r.Factor.CreatedAt, r.Factor.LastChallengedAt,
	)
	if err != nil {
		return fmt.Errorf("save MFA factor: %w", err)
	}
	return nil
}

func (s *Service) updateMFAFactorInDB(r mfaFactorRecord) error {
	if s.pool == nil {
		return nil
	}
	// backup_codes is intentionally not written here: consumption is a
	// conditional UPDATE (consumeBackupCodeInDB), and writing this process's
	// copy back would resurrect codes another instance already spent.
	ctx := context.Background()
	_, err := s.pool.Exec(ctx, `
		UPDATE mfa_factors
		SET is_verified        = $2,
		    last_challenged_at = $3
		WHERE id = $1`,
		r.Factor.ID, r.Factor.IsVerified, r.Factor.LastChallengedAt,
	)
	if err != nil {
		return fmt.Errorf("update MFA factor: %w", err)
	}
	return nil
}

// reloadMFAFactorsForUser refreshes one user's factors from Postgres so a
// factor enrolled, verified or spent on another instance is seen here. The
// query runs without s.mu; only the swap takes the lock.
func (s *Service) reloadMFAFactorsForUser(ctx context.Context, userID string) error {
	if s.pool == nil {
		return nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, user_id::text, COALESCE(org_id::text, ''), factor_type,
		       COALESCE(label, ''), is_verified,
		       COALESCE(totp_secret, ''), COALESCE(backup_codes, '[]'::jsonb),
		       COALESCE(created_at, now()), last_challenged_at
		FROM mfa_factors WHERE user_id::text = $1
		ORDER BY created_at, id`, userID)
	if err != nil {
		return fmt.Errorf("reload MFA factors: %w", err)
	}
	defer rows.Close()
	var fresh []mfaFactorRecord
	for rows.Next() {
		var r mfaFactorRecord
		var backupCodesJSON []byte
		if err := rows.Scan(
			&r.Factor.ID, &r.Factor.UserID, &r.Factor.OrgID, &r.Factor.FactorType,
			&r.Factor.Label, &r.Factor.IsVerified, &r.TOTPSecret,
			&backupCodesJSON, &r.Factor.CreatedAt, &r.Factor.LastChallengedAt,
		); err != nil {
			return fmt.Errorf("scan MFA factor: %w", err)
		}
		if len(backupCodesJSON) > 0 {
			_ = json.Unmarshal(backupCodesJSON, &r.BackupCodes)
		}
		fresh = append(fresh, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("reload MFA factors: %w", err)
	}
	s.mu.Lock()
	s.mfaFactorsByUser[userID] = fresh
	s.mu.Unlock()
	return nil
}

type SessionInput struct {
	UserID           string
	Email            string
	Name             string
	OrgID            string
	HouseholdID      string
	SwimmerID        string
	Roles            []string
	MFAAuthenticated bool
	ExpiresIn        time.Duration
	OIDCIDToken      string
}

type SessionTokens struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type OIDCCallbackResult struct {
	UserID string   `json:"user_id"`
	Email  string   `json:"email,omitempty"`
	Name   string   `json:"name,omitempty"`
	OrgID  string   `json:"org_id"`
	Roles  []string `json:"roles"`
	SessionTokens
}

type LogoutResult struct {
	LogoutURL   string `json:"logout_url,omitempty"`
	RedirectURL string `json:"redirect_url"`
}

type MFAFactor struct {
	ID               string     `json:"id"`
	UserID           string     `json:"user_id"`
	OrgID            string     `json:"org_id"`
	FactorType       string     `json:"factor_type"`
	Label            string     `json:"label"`
	IsVerified       bool       `json:"is_verified"`
	CreatedAt        time.Time  `json:"created_at"`
	LastChallengedAt *time.Time `json:"last_challenged_at,omitempty"`
	ChallengeHint    string     `json:"challenge_hint,omitempty"`
	// ProvisioningURI is the otpauth:// URI for TOTP enrollment. Only populated
	// in the enroll response — never returned from list or challenge endpoints.
	ProvisioningURI string `json:"provisioning_uri,omitempty"`
	// Codes is the one-time plaintext list of backup codes, returned only at
	// enrollment time. Never stored in plaintext; only hashes are retained.
	Codes []string `json:"codes,omitempty"`
}

type RoleAssignmentInput struct {
	OrgID        string
	TargetUserID string
	Role         string
	ScopeType    string
	ScopeID      string
	Reason       string
	ActorUserID  string
}

type RoleAssignment struct {
	OrgID        string    `json:"org_id"`
	TargetUserID string    `json:"target_user_id"`
	Role         string    `json:"role"`
	ScopeType    string    `json:"scope_type"`
	ScopeID      string    `json:"scope_id,omitempty"`
	Reason       string    `json:"reason"`
	ActorUserID  string    `json:"actor_user_id"`
	AssignedAt   time.Time `json:"assigned_at"`
}

type sessionRecord struct {
	AccessToken  string
	RefreshToken string
	IDToken      string
	Principal    appmiddleware.Principal
	ExpiresAt    time.Time
	RevokedAt    *time.Time
}

type backupCodeRecord struct {
	Hash string
	Used bool
}

type mfaFactorRecord struct {
	Factor      MFAFactor
	TOTPSecret  string
	BackupCodes []backupCodeRecord
}

type oidcTokenResponse struct {
	AccessToken string `json:"access_token"`
	IDToken     string `json:"id_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

type oidcStatePayload struct {
	Nonce string `json:"nonce"`
	Exp   int64  `json:"exp"`
	OrgID string `json:"org_id,omitempty"`
}

type idTokenClaims struct {
	Subject string
	Email   string
	Name    string
	Issuer  string
	Nonce   string
	OrgID   string
	Roles   []string
	Exp     int64
	Iat     int64
	AZP     string
	Aud     []string
}

type idTokenHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	Typ string `json:"typ"`
}

type oidcJWKSet struct {
	Keys []oidcJWK `json:"keys"`
}

type oidcJWK struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func NewService(cfg Config) *Service {
	if cfg.OIDC.HTTPTimeout <= 0 {
		cfg.OIDC.HTTPTimeout = 10 * time.Second
	}

	l := cfg.Logger
	if l == nil {
		l = slog.Default()
	}

	service := &Service{
		sessionsByAccess: make(map[string]*sessionRecord),
		byRefresh:        make(map[string]*sessionRecord),
		mfaFactorsByUser: make(map[string][]mfaFactorRecord),
		assignments:      []RoleAssignment{},
		staticPrincipals: map[string]*appmiddleware.Principal{},
		cfg:              cfg,
		httpClient:       &http.Client{Timeout: cfg.OIDC.HTTPTimeout},
		logger:           l,
		usedTOTPCodes:    make(map[string]time.Time),
		mfaFailures:      make(map[string]mfaAttempts),
		jwksCacheTTL:     5 * time.Minute,
	}
	if cfg.EnableDevStubTokens {
		service.staticPrincipals = localDevStaticPrincipals()
	}

	return service
}

func (s *Service) BeginOIDCLogin(loginHint, orgID string) (string, error) {
	if !s.oidcAuthorizeConfigured() {
		return "", ErrOIDCNotConfigured
	}

	state, nonce, err := signOIDCState(oidcStatePayload{
		Nonce: randomToken(24),
		Exp:   time.Now().UTC().Add(10 * time.Minute).Unix(),
		OrgID: strings.TrimSpace(orgID),
	}, s.cfg.OIDC.StateSigningKey)
	if err != nil {
		return "", err
	}

	q := url.Values{}
	q.Set("client_id", s.cfg.OIDC.ClientID)
	q.Set("redirect_uri", s.cfg.OIDC.RedirectURL)
	q.Set("response_type", "code")
	q.Set("scope", "openid profile email")
	q.Set("state", state)
	q.Set("nonce", nonce)
	// Force Keycloak to always show the login screen. Without this, an active
	// Keycloak SSO session from a previous login survives our app-level logout
	// and silently re-authenticates the old user, bypassing the login page.
	q.Set("prompt", "login")
	if hint := strings.TrimSpace(loginHint); hint != "" {
		q.Set("login_hint", hint)
	}

	base, err := url.Parse(s.cfg.OIDC.AuthorizeURL)
	if err != nil {
		return "", err
	}
	base.RawQuery = q.Encode()
	return base.String(), nil
}

func (s *Service) OIDCCallback(code, state, _ string) (OIDCCallbackResult, error) {
	if strings.TrimSpace(code) == "" || strings.TrimSpace(state) == "" {
		return OIDCCallbackResult{}, errors.New("code and state are required")
	}
	if !s.oidcConfigured() {
		return OIDCCallbackResult{}, ErrOIDCNotConfigured
	}

	statePayload, err := validateOIDCState(state, s.cfg.OIDC.StateSigningKey, time.Now().UTC())
	if err != nil {
		return OIDCCallbackResult{}, fmt.Errorf("%w: %v", ErrOIDCStateInvalid, err)
	}

	tokenResponse, err := s.exchangeOIDCAuthorizationCode(code)
	if err != nil {
		return OIDCCallbackResult{}, err
	}

	now := time.Now().UTC()
	claims, err := s.verifyAndParseIDToken(tokenResponse.IDToken, now)
	if err != nil {
		return OIDCCallbackResult{}, err
	}

	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(claims.Nonce)), []byte(strings.TrimSpace(statePayload.Nonce))) != 1 {
		return OIDCCallbackResult{}, errors.New("id_token nonce does not match state nonce")
	}
	if expectedIssuer := strings.TrimSpace(s.cfg.OIDC.ExpectedIssuer); expectedIssuer != "" {
		if strings.TrimSpace(claims.Issuer) != expectedIssuer {
			return OIDCCallbackResult{}, errors.New("id_token issuer mismatch")
		}
	}
	if !audienceContains(claims.Aud, s.cfg.OIDC.ClientID) {
		return OIDCCallbackResult{}, errors.New("id_token audience mismatch")
	}
	if len(claims.Aud) > 1 && strings.TrimSpace(claims.AZP) != strings.TrimSpace(s.cfg.OIDC.ClientID) {
		return OIDCCallbackResult{}, errors.New("id_token azp mismatch")
	}

	resolvedOrgID := strings.TrimSpace(claims.OrgID)
	if resolvedOrgID == "" {
		// The state org_id is caller-supplied at /authorize time and is not an
		// authentication authority: trusting it here would let an attacker with
		// any valid provider account choose their tenant. Only fall back to it
		// in explicit local-dev/test deployments where Keycloak protocol
		// mappers may not be configured; production/staging must reject.
		if !s.cfg.EnableDevStubTokens {
			return OIDCCallbackResult{}, errors.New("org_id claim missing from id token; configure Keycloak protocol mapper (User Attribute 'org_id' → ID token claim 'org_id')")
		}
		s.logger.Warn("oidc: org_id absent from ID token claims — falling back to state parameter (dev-only); verify Keycloak protocol mapper (User Attribute 'org_id' → ID token claim 'org_id') is configured",
			"user_id", claims.Subject,
			"issuer", claims.Issuer,
		)
		resolvedOrgID = strings.TrimSpace(statePayload.OrgID)
	}
	if resolvedOrgID == "" {
		return OIDCCallbackResult{}, errors.New("org_id could not be resolved from trusted oidc claims/state")
	}
	if normalizedOrgID := normalizeLocalDevOrgID(claims.Subject, claims.Email, resolvedOrgID); normalizedOrgID != resolvedOrgID {
		s.logger.Warn("oidc: normalizing legacy local dev org_id claim to UUID",
			"user_id", claims.Subject,
			"legacy_org_id", resolvedOrgID,
			"normalized_org_id", normalizedOrgID,
		)
		resolvedOrgID = normalizedOrgID
	}

	roles := claims.Roles
	if len(roles) == 0 {
		roles = []string{"parent"}
	}

	// Resolve linked household for this Keycloak user, if one exists.
	var householdID string
	if s.householdLookup != nil {
		householdID = s.householdLookup(claims.Subject)
	}

	// Resolve the specific linked swimmer for this Keycloak user, if one
	// exists, so a swimmer-role session never has to guess "self" from the
	// household roster.
	var swimmerID string
	if s.swimmerLookup != nil {
		swimmerID = s.swimmerLookup(claims.Subject)
	}

	session, err := s.IssueSession(SessionInput{
		UserID:           claims.Subject,
		Email:            claims.Email,
		Name:             claims.Name,
		OrgID:            resolvedOrgID,
		HouseholdID:      householdID,
		SwimmerID:        swimmerID,
		Roles:            roles,
		ExpiresIn:        24 * time.Hour,
		MFAAuthenticated: false,
		OIDCIDToken:      tokenResponse.IDToken,
	})
	if err != nil {
		return OIDCCallbackResult{}, err
	}

	return OIDCCallbackResult{
		UserID:        claims.Subject,
		Email:         claims.Email,
		Name:          claims.Name,
		OrgID:         resolvedOrgID,
		Roles:         roles,
		SessionTokens: session,
	}, nil
}

func (s *Service) IssueSession(input SessionInput) (SessionTokens, error) {
	input.UserID = strings.TrimSpace(input.UserID)
	input.Email = strings.TrimSpace(input.Email)
	input.Name = strings.TrimSpace(input.Name)
	input.OrgID = normalizeLocalDevOrgID(input.UserID, input.Email, input.OrgID)

	if strings.TrimSpace(input.UserID) == "" {
		return SessionTokens{}, errors.New("user_id is required")
	}
	if len(input.Roles) == 0 {
		return SessionTokens{}, errors.New("roles are required")
	}
	if input.ExpiresIn <= 0 {
		input.ExpiresIn = 24 * time.Hour
	}
	if strings.TrimSpace(input.SwimmerID) == "" && s.swimmerLookup != nil && hasRole(input.Roles, "swimmer") {
		input.SwimmerID = s.swimmerLookup(input.UserID)
	}
	if err := s.syncAuthenticatedUser(context.Background(), input); err != nil {
		return SessionTokens{}, err
	}

	return s.issueSessionRecord(context.Background(), appmiddleware.Principal{
		UserID:           input.UserID,
		Email:            strings.TrimSpace(input.Email),
		Name:             strings.TrimSpace(input.Name),
		OrgID:            input.OrgID,
		HouseholdID:      input.HouseholdID,
		SwimmerID:        input.SwimmerID,
		Roles:            append([]string{}, input.Roles...),
		MFAAuthenticated: input.MFAAuthenticated,
	}, strings.TrimSpace(input.OIDCIDToken), input.ExpiresIn)
}

func (s *Service) RefreshSession(refreshToken string) (SessionTokens, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return SessionTokens{}, ErrSessionNotFound
	}

	if s.pool != nil {
		return s.refreshPersistentSession(context.Background(), refreshToken)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.byRefresh[refreshToken]
	if !ok {
		return SessionTokens{}, ErrSessionNotFound
	}
	if record.RevokedAt != nil {
		return SessionTokens{}, ErrSessionRevoked
	}
	if time.Now().UTC().After(record.ExpiresAt) {
		return SessionTokens{}, ErrSessionExpired
	}

	now := time.Now().UTC()
	record.RevokedAt = &now

	nextAccess := "atk_" + randomToken(32)
	nextRefresh := "rtk_" + randomToken(32)
	expiresAt := refreshedExpiry(now, record)

	next := &sessionRecord{
		AccessToken:  nextAccess,
		RefreshToken: nextRefresh,
		IDToken:      record.IDToken,
		Principal:    normalizeLocalDevPrincipal(copyPrincipal(record.Principal)),
		ExpiresAt:    expiresAt,
	}

	s.sessionsByAccess[nextAccess] = next
	s.byRefresh[nextRefresh] = next
	delete(s.byRefresh, refreshToken)

	return SessionTokens{
		AccessToken:  nextAccess,
		RefreshToken: nextRefresh,
		ExpiresAt:    expiresAt,
	}, nil
}

func (s *Service) RevokeSession(refreshToken string) error {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return ErrSessionNotFound
	}

	if s.pool != nil {
		_, err := s.revokePersistentSessionByRefreshToken(context.Background(), refreshToken)
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.byRefresh[refreshToken]
	if !ok {
		return ErrSessionNotFound
	}
	if record.RevokedAt != nil {
		return nil
	}

	now := time.Now().UTC()
	record.RevokedAt = &now
	return nil
}

func (s *Service) LogoutSession(refreshToken string) LogoutResult {
	return s.LogoutSessionWithAccess(refreshToken, "")
}

func (s *Service) LogoutSessionWithAccess(refreshToken, accessToken string) LogoutResult {
	result := LogoutResult{RedirectURL: "/login"}
	refreshToken = strings.TrimSpace(refreshToken)
	accessToken = strings.TrimSpace(accessToken)
	if refreshToken == "" && accessToken == "" {
		return result
	}

	var idToken string

	if s.pool != nil {
		var record *sessionRecord
		var err error
		switch {
		case refreshToken != "":
			record, err = s.revokePersistentSessionByRefreshToken(context.Background(), refreshToken)
		case accessToken != "":
			record, err = s.revokePersistentSessionByAccessToken(context.Background(), accessToken)
		}
		if err == nil && record != nil {
			idToken = record.IDToken
		}
		if logoutURL, ok := s.buildOIDCLogoutURL(idToken); ok {
			result.LogoutURL = logoutURL
		}
		return result
	}

	s.mu.Lock()
	switch {
	case refreshToken != "":
		record, ok := s.byRefresh[refreshToken]
		if ok {
			idToken = record.IDToken
			if record.RevokedAt == nil {
				now := time.Now().UTC()
				record.RevokedAt = &now
			}
		}
	case accessToken != "":
		record, ok := s.sessionsByAccess[accessToken]
		if ok {
			idToken = record.IDToken
			if record.RevokedAt == nil {
				now := time.Now().UTC()
				record.RevokedAt = &now
			}
		}
	}
	s.mu.Unlock()

	if logoutURL, ok := s.buildOIDCLogoutURL(idToken); ok {
		result.LogoutURL = logoutURL
	}

	return result
}

func (s *Service) ResolvePrincipal(token string) (*appmiddleware.Principal, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, false
	}

	s.mu.RLock()
	if principal, ok := s.staticPrincipals[token]; ok {
		s.mu.RUnlock()
		cloned := clonePrincipal(principal)
		if cloned != nil {
			normalized := normalizeLocalDevPrincipal(*cloned)
			cloned = &normalized
			// Dev-stub sessions have no real Keycloak subject to look up a
			// linked swimmer by, so resolve it the same way a real swimmer
			// login would: by keycloak_user_id, using the stub's own fixed
			// user ID as that identifier. This keeps dev-stub testing on
			// the exact same "self" resolution path as production, instead
			// of guessing a household roster entry.
			if s.swimmerLookup != nil && hasRole(cloned.Roles, "swimmer") {
				cloned.SwimmerID = s.swimmerLookup(cloned.UserID)
			}
		}
		return cloned, true
	}

	if s.pool != nil {
		s.mu.RUnlock()
		record, err := s.loadPersistentSessionByAccessToken(context.Background(), token)
		if err != nil {
			return nil, false
		}
		principal := clonePrincipal(&record.Principal)
		if principal != nil {
			normalized := normalizeLocalDevPrincipal(*principal)
			principal = &normalized
		}
		return principal, true
	}

	record, ok := s.sessionsByAccess[token]
	if !ok {
		s.mu.RUnlock()
		return nil, false
	}
	if record.RevokedAt != nil || time.Now().UTC().After(record.ExpiresAt) {
		s.mu.RUnlock()
		return nil, false
	}

	principal := clonePrincipal(&record.Principal)
	if principal != nil {
		normalized := normalizeLocalDevPrincipal(*principal)
		principal = &normalized
	}
	s.mu.RUnlock()
	return principal, true
}

func (s *Service) ListMFAFactors(userID string) []MFAFactor {
	if err := s.reloadMFAFactorsForUser(context.Background(), userID); err != nil {
		// Signature has no error return; serve the cached view and log loudly.
		s.logger.Warn("authn: MFA factor reload failed; serving cached factors", "user_id", userID, "error", err)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	records := s.mfaFactorsByUser[userID]
	out := make([]MFAFactor, 0, len(records))
	for _, record := range records {
		out = append(out, record.Factor)
	}
	return out
}

func (s *Service) EnrollMFA(userID, email, orgID, factorType, label string) (MFAFactor, error) {
	factorType = strings.TrimSpace(strings.ToLower(factorType))
	if !slices.Contains([]string{"totp", "backup_code"}, factorType) {
		return MFAFactor{}, errors.New("unsupported factor type")
	}

	factor := MFAFactor{
		ID:         "mfa_" + randomToken(10),
		UserID:     userID,
		OrgID:      orgID,
		FactorType: factorType,
		Label:      strings.TrimSpace(label),
		CreatedAt:  time.Now().UTC(),
	}
	record := mfaFactorRecord{Factor: factor}

	switch factorType {
	case "totp":
		record.TOTPSecret = generateTOTPSecret()
		// Always return the provisioning URI on enrollment so the user can add it to
		// their authenticator app. Prefer email as the account name; fall back to userID.
		accountName := strings.TrimSpace(email)
		if accountName == "" {
			accountName = userID
		}
		if s.cfg.EnableDevStubTokens {
			factor.ChallengeHint = record.TOTPSecret
		}
		factor.ProvisioningURI = fmt.Sprintf(
			"otpauth://totp/aquify:%s?secret=%s&issuer=aquify&algorithm=SHA1&digits=6&period=30",
			url.QueryEscape(accountName), record.TOTPSecret,
		)
	case "backup_code":
		const numCodes = 8
		plainCodes := make([]string, numCodes)
		record.BackupCodes = make([]backupCodeRecord, numCodes)
		for i := range numCodes {
			code := strings.ToUpper(randomAlphaNumeric(10))
			plainCodes[i] = code
			record.BackupCodes[i] = backupCodeRecord{Hash: hashChallengeCode(code)}
		}
		factor.Codes = plainCodes
	}
	// record.Factor deliberately stays the bare factor: the returned copy carries
	// one-time secrets (backup codes, provisioning URI, dev hint) that must never
	// be retained in memory where ListMFAFactors would hand them out again.

	// Persist first: a factor the database never saw must not exist in memory.
	if err := s.saveMFAFactorToDB(record); err != nil {
		return MFAFactor{}, err
	}
	s.mu.Lock()
	s.mfaFactorsByUser[userID] = append(s.mfaFactorsByUser[userID], record)
	s.mu.Unlock()

	return factor, nil
}

const (
	mfaMaxFailures   = 5
	mfaLockoutWindow = 5 * time.Minute
)

type mfaAttempts struct {
	count int
	start time.Time
}

// mfaLockedLocked reports whether the factor has used up its failed attempts in
// the current window (callers hold s.mu).
func (s *Service) mfaLockedLocked(factorID string, now time.Time) bool {
	w, ok := s.mfaFailures[factorID]
	if !ok {
		return false
	}
	if now.Sub(w.start) > mfaLockoutWindow {
		delete(s.mfaFailures, factorID)
		return false
	}
	return w.count >= mfaMaxFailures
}

func (s *Service) recordMFAFailureLocked(factorID string, now time.Time) {
	w := s.mfaFailures[factorID]
	if w.count == 0 || now.Sub(w.start) > mfaLockoutWindow {
		w = mfaAttempts{start: now}
	}
	w.count++
	s.mfaFailures[factorID] = w
}

func (s *Service) ChallengeMFA(currentAccessToken string, principal *appmiddleware.Principal, factorID, challengeCode string) (SessionTokens, error) {
	if principal == nil {
		return SessionTokens{}, ErrInvalidMFAFactor
	}
	ctx := context.Background()
	currentAccessToken = strings.TrimSpace(currentAccessToken)

	// Pick up factors changed on other instances before deciding anything.
	if err := s.reloadMFAFactorsForUser(ctx, principal.UserID); err != nil {
		return SessionTokens{}, err
	}

	// Phase 1 (locked, no I/O): find the factor, enforce lockout and replay
	// protection, verify the code, and tentatively claim it in memory so a
	// concurrent attempt on this instance cannot reuse it.
	var (
		record      mfaFactorRecord
		replayKey   string
		totpStep    int64
		backupHash  string
		backupIndex = -1
	)
	s.mu.Lock()
	found := false
	factors := s.mfaFactorsByUser[principal.UserID]
	for i := range factors {
		if factors[i].Factor.ID != factorID {
			continue
		}
		found = true
		now := time.Now().UTC()
		if s.mfaLockedLocked(factors[i].Factor.ID, now) {
			s.mu.Unlock()
			return SessionTokens{}, ErrMFATooManyAttempts
		}
		switch factors[i].Factor.FactorType {
		case "totp":
			replayKey = factors[i].Factor.ID + "|" + challengeCode
			// Purge expired replay entries (TTL = 90 seconds: the ±30s window × 3).
			for k, t := range s.usedTOTPCodes {
				if now.Sub(t) > 90*time.Second {
					delete(s.usedTOTPCodes, k)
				}
			}
			if _, replayed := s.usedTOTPCodes[replayKey]; replayed {
				s.mu.Unlock()
				return SessionTokens{}, ErrInvalidMFACode
			}
			step, ok := verifyTOTPStep(factors[i].TOTPSecret, challengeCode, now)
			if !ok {
				s.recordMFAFailureLocked(factors[i].Factor.ID, now)
				s.mu.Unlock()
				return SessionTokens{}, ErrInvalidMFACode
			}
			totpStep = step
			s.usedTOTPCodes[replayKey] = now
		case "backup_code":
			providedHash := hashChallengeCode(strings.ToUpper(strings.TrimSpace(challengeCode)))
			for j := range factors[i].BackupCodes {
				if factors[i].BackupCodes[j].Used {
					continue
				}
				if subtle.ConstantTimeCompare([]byte(providedHash), []byte(factors[i].BackupCodes[j].Hash)) == 1 {
					backupIndex, backupHash = j, factors[i].BackupCodes[j].Hash
					factors[i].BackupCodes[j].Used = true
					break
				}
			}
			if backupIndex < 0 {
				s.recordMFAFailureLocked(factors[i].Factor.ID, now)
				s.mu.Unlock()
				return SessionTokens{}, ErrInvalidMFACode
			}
		default:
			s.mu.Unlock()
			return SessionTokens{}, ErrUnsupportedMFAFactor
		}
		record = factors[i]
		break
	}
	s.mu.Unlock()
	if !found {
		return SessionTokens{}, ErrInvalidMFAFactor
	}

	// undo releases the tentative in-memory claim when persistence fails.
	undo := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if replayKey != "" {
			delete(s.usedTOTPCodes, replayKey)
		}
		if backupIndex >= 0 {
			fs := s.mfaFactorsByUser[principal.UserID]
			for i := range fs {
				if fs[i].Factor.ID == factorID && backupIndex < len(fs[i].BackupCodes) {
					fs[i].BackupCodes[backupIndex].Used = false
				}
			}
		}
	}

	// Phase 2 (unlocked): database-atomic consumption, then the factor update,
	// all synchronous so a persistence failure is reported, never dropped.
	now := time.Now().UTC()
	if s.pool != nil {
		switch record.Factor.FactorType {
		case "totp":
			fresh, err := s.consumeTOTPStepInDB(ctx, record.Factor.ID, totpStep)
			if err != nil {
				undo()
				return SessionTokens{}, err
			}
			if !fresh {
				return SessionTokens{}, ErrInvalidMFACode // spent on another instance; keep the local claim
			}
		case "backup_code":
			consumed, err := s.consumeBackupCodeInDB(ctx, record.Factor.ID, backupHash)
			if err != nil {
				undo()
				return SessionTokens{}, err
			}
			if !consumed {
				return SessionTokens{}, ErrInvalidMFACode // another instance spent it
			}
		}
	}
	record.Factor.IsVerified = true
	record.Factor.LastChallengedAt = &now
	if err := s.updateMFAFactorInDB(record); err != nil {
		// The code is already spent in the DB (single use); do not release it.
		return SessionTokens{}, err
	}

	// Phase 3 (locked, no I/O): commit the verified state to memory.
	var idToken string
	s.mu.Lock()
	delete(s.mfaFailures, factorID)
	fs := s.mfaFactorsByUser[principal.UserID]
	for i := range fs {
		if fs[i].Factor.ID == factorID {
			fs[i].Factor.IsVerified = true
			fs[i].Factor.LastChallengedAt = &now
		}
	}
	if currentAccessToken != "" && s.pool == nil {
		if currentRecord, ok := s.sessionsByAccess[currentAccessToken]; ok {
			idToken = currentRecord.IDToken
			currentRecord.RevokedAt = &now
		}
	}
	s.mu.Unlock()

	if currentAccessToken != "" && s.pool != nil {
		currentRecord, err := s.revokePersistentSessionByAccessToken(ctx, currentAccessToken)
		if err == nil && currentRecord != nil {
			idToken = currentRecord.IDToken
		}
	}

	return s.issueSessionRecord(ctx, appmiddleware.Principal{
		UserID:           principal.UserID,
		Email:            principal.Email,
		Name:             principal.Name,
		OrgID:            principal.OrgID,
		HouseholdID:      principal.HouseholdID,
		SwimmerID:        principal.SwimmerID,
		Roles:            append([]string{}, principal.Roles...),
		MFAAuthenticated: true,
	}, idToken, 15*time.Minute)
}

func (s *Service) issueSessionRecord(ctx context.Context, principal appmiddleware.Principal, idToken string, expiresIn time.Duration) (SessionTokens, error) {
	principal = normalizeLocalDevPrincipal(principal)
	if expiresIn <= 0 {
		expiresIn = 24 * time.Hour
	}

	accessToken := "atk_" + randomToken(32)
	refreshToken := "rtk_" + randomToken(32)
	expiresAt := time.Now().UTC().Add(expiresIn)

	record := &sessionRecord{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		IDToken:      strings.TrimSpace(idToken),
		Principal:    copyPrincipal(principal),
		ExpiresAt:    expiresAt,
	}

	if s.pool != nil {
		if err := insertSessionRecord(ctx, s.pool, record); err != nil {
			return SessionTokens{}, err
		}
	} else {
		s.mu.Lock()
		s.sessionsByAccess[accessToken] = record
		s.byRefresh[refreshToken] = record
		s.mu.Unlock()
	}

	return SessionTokens{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
	}, nil
}

func (s *Service) refreshPersistentSession(ctx context.Context, refreshToken string) (SessionTokens, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return SessionTokens{}, fmt.Errorf("begin session refresh tx: %w", err)
	}
	defer tx.Rollback(ctx)

	record, err := s.loadPersistentSessionByRefreshTokenTx(ctx, tx, refreshToken, true)
	if err != nil {
		return SessionTokens{}, err
	}

	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
		UPDATE sessions
		SET revoked_at = $2, updated_at = now()
		WHERE refresh_token_hash = $1 AND revoked_at IS NULL`,
		hashToken(refreshToken), now,
	); err != nil {
		return SessionTokens{}, fmt.Errorf("revoke rotated session: %w", err)
	}

	nextAccess := "atk_" + randomToken(32)
	nextRefresh := "rtk_" + randomToken(32)
	expiresAt := refreshedExpiry(now, record)

	next := &sessionRecord{
		AccessToken:  nextAccess,
		RefreshToken: nextRefresh,
		IDToken:      record.IDToken,
		Principal:    copyPrincipal(record.Principal),
		ExpiresAt:    expiresAt,
	}
	if err := insertSessionRecord(ctx, tx, next); err != nil {
		return SessionTokens{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SessionTokens{}, fmt.Errorf("commit session refresh: %w", err)
	}

	return SessionTokens{
		AccessToken:  nextAccess,
		RefreshToken: nextRefresh,
		ExpiresAt:    expiresAt,
	}, nil
}

// refreshedExpiry returns the expiry for a rotated session. An MFA-elevated
// session is short-lived by design, so rotation must not extend it past its
// original expiry.
func refreshedExpiry(now time.Time, prev *sessionRecord) time.Time {
	expiresAt := now.Add(24 * time.Hour)
	if prev.Principal.MFAAuthenticated && prev.ExpiresAt.Before(expiresAt) {
		return prev.ExpiresAt
	}
	return expiresAt
}

// insertSessionRecord stores a session row using the pool or an open transaction.
func insertSessionRecord(ctx context.Context, q interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, record *sessionRecord) error {
	userID, err := parseRequiredUUID(record.Principal.UserID, "user_id")
	if err != nil {
		return err
	}
	orgID, err := parseOptionalUUID(record.Principal.OrgID, "org_id")
	if err != nil {
		return err
	}
	principalJSON, err := json.Marshal(record.Principal)
	if err != nil {
		return fmt.Errorf("marshal principal: %w", err)
	}

	_, err = q.Exec(ctx, `
		INSERT INTO sessions (
			org_id, user_id, refresh_token_hash, access_token_hash, principal, id_token, expires_at, last_seen_at
		)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5::jsonb, $6, $7, now())`,
		orgID, userID, hashToken(record.RefreshToken), hashToken(record.AccessToken), principalJSON, nullableString(record.IDToken), record.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("persist session: %w", err)
	}
	return nil
}

func (s *Service) revokePersistentSessionByRefreshToken(ctx context.Context, refreshToken string) (*sessionRecord, error) {
	return s.revokePersistentSessionByHash(ctx, "refresh_token_hash", refreshToken)
}

func (s *Service) revokePersistentSessionByAccessToken(ctx context.Context, accessToken string) (*sessionRecord, error) {
	return s.revokePersistentSessionByHash(ctx, "access_token_hash", accessToken)
}

// revokePersistentSessionByHash revokes the session whose hashColumn (a fixed
// column name chosen by the callers above, never user input) matches the token.
func (s *Service) revokePersistentSessionByHash(ctx context.Context, hashColumn, token string) (*sessionRecord, error) {
	record, err := s.loadPersistentSessionAny(ctx, `
		SELECT principal, COALESCE(id_token, ''), expires_at, revoked_at
		FROM sessions
		WHERE `+hashColumn+` = $1`,
		hashToken(token),
	)
	if err != nil {
		return nil, err
	}
	if record.RevokedAt != nil {
		return record, nil
	}
	now := time.Now().UTC()
	tag, err := s.pool.Exec(ctx, `
		UPDATE sessions
		SET revoked_at = $2, updated_at = now()
		WHERE `+hashColumn+` = $1 AND revoked_at IS NULL`,
		hashToken(token), now,
	)
	if err != nil {
		return nil, fmt.Errorf("revoke session: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return record, nil
	}
	record.RevokedAt = &now
	return record, nil
}

func (s *Service) loadPersistentSessionByAccessToken(ctx context.Context, accessToken string) (*sessionRecord, error) {
	return s.loadPersistentValidatedSession(ctx, `
		SELECT principal, COALESCE(id_token, ''), expires_at, revoked_at
		FROM sessions
		WHERE access_token_hash = $1`,
		hashToken(accessToken),
	)
}

func (s *Service) loadPersistentSessionByRefreshTokenTx(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, refreshToken string, forUpdate bool) (*sessionRecord, error) {
	query := `
		SELECT principal, COALESCE(id_token, ''), expires_at, revoked_at
		FROM sessions
		WHERE refresh_token_hash = $1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	record, err := scanSessionRecord(q.QueryRow(ctx, query, hashToken(refreshToken)))
	if err != nil {
		return nil, err
	}
	return record, validateSessionRecord(record)
}

// validateSessionRecord rejects revoked and expired sessions.
func validateSessionRecord(record *sessionRecord) error {
	if record.RevokedAt != nil {
		return ErrSessionRevoked
	}
	if time.Now().UTC().After(record.ExpiresAt) {
		return ErrSessionExpired
	}
	return nil
}

func (s *Service) loadPersistentValidatedSession(ctx context.Context, query string, arg any) (*sessionRecord, error) {
	record, err := s.loadPersistentSessionAny(ctx, query, arg)
	if err != nil {
		return nil, err
	}
	return record, validateSessionRecord(record)
}

func (s *Service) loadPersistentSessionAny(ctx context.Context, query string, arg any) (*sessionRecord, error) {
	return scanSessionRecord(s.pool.QueryRow(ctx, query, arg))
}

// scanSessionRecord reads one (principal, id_token, expires_at, revoked_at) row.
func scanSessionRecord(row pgx.Row) (*sessionRecord, error) {
	var principalJSON []byte
	record := &sessionRecord{}
	if err := row.Scan(&principalJSON, &record.IDToken, &record.ExpiresAt, &record.RevokedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("load session: %w", err)
	}
	if err := json.Unmarshal(principalJSON, &record.Principal); err != nil {
		return nil, fmt.Errorf("unmarshal session principal: %w", err)
	}
	record.Principal = normalizeLocalDevPrincipal(record.Principal)
	return record, nil
}

func parseRequiredUUID(raw, field string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("%s is required when persistent sessions are enabled", field)
	}
	if _, err := uuid.Parse(value); err != nil {
		return "", fmt.Errorf("%s must be a UUID when persistent sessions are enabled: %w", field, err)
	}
	return value, nil
}

func parseOptionalUUID(raw, field string) (*string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, nil
	}
	if _, err := uuid.Parse(value); err != nil {
		return nil, fmt.Errorf("%s must be a UUID when persistent sessions are enabled: %w", field, err)
	}
	return &value, nil
}

func nullableString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

func (s *Service) AssignRole(input RoleAssignmentInput) (RoleAssignment, error) {
	scopeType := strings.TrimSpace(strings.ToLower(input.ScopeType))
	if scopeType == "" {
		scopeType = "org"
	}

	assignment := RoleAssignment{
		OrgID:        strings.TrimSpace(input.OrgID),
		TargetUserID: strings.TrimSpace(input.TargetUserID),
		Role:         strings.TrimSpace(input.Role),
		ScopeType:    scopeType,
		ScopeID:      strings.TrimSpace(input.ScopeID),
		Reason:       strings.TrimSpace(input.Reason),
		ActorUserID:  strings.TrimSpace(input.ActorUserID),
		AssignedAt:   time.Now().UTC(),
	}

	if assignment.OrgID == "" || assignment.TargetUserID == "" || assignment.Role == "" || assignment.Reason == "" || assignment.ActorUserID == "" {
		return RoleAssignment{}, errors.New("org_id, target_user_id, role, reason, and actor_user_id are required")
	}

	s.mu.Lock()
	s.assignments = append(s.assignments, assignment)
	s.mu.Unlock()

	return assignment, nil
}

func (s *Service) ListRoleAssignments(orgID string) []RoleAssignment {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]RoleAssignment, 0, len(s.assignments))
	for _, assignment := range s.assignments {
		if strings.TrimSpace(orgID) != "" && !strings.EqualFold(strings.TrimSpace(assignment.OrgID), strings.TrimSpace(orgID)) {
			continue
		}
		out = append(out, assignment)
	}
	slices.SortFunc(out, func(a, b RoleAssignment) int {
		return b.AssignedAt.Compare(a.AssignedAt)
	})
	return out
}

func (s *Service) oidcConfigured() bool {
	oidc := s.cfg.OIDC
	return allNonBlank(oidc.TokenURL, oidc.JWKSURL, oidc.RedirectURL, oidc.ClientID, oidc.ClientSecret, oidc.StateSigningKey)
}

func (s *Service) oidcAuthorizeConfigured() bool {
	oidc := s.cfg.OIDC
	return allNonBlank(oidc.AuthorizeURL, oidc.RedirectURL, oidc.ClientID, oidc.StateSigningKey)
}

func allNonBlank(values ...string) bool {
	for _, v := range values {
		if strings.TrimSpace(v) == "" {
			return false
		}
	}
	return true
}

func (s *Service) buildOIDCLogoutURL(idToken string) (string, bool) {
	endSessionURL := strings.TrimSpace(s.cfg.OIDC.EndSessionURL)
	idToken = strings.TrimSpace(idToken)
	if endSessionURL == "" || idToken == "" {
		return "", false
	}

	base, err := url.Parse(endSessionURL)
	if err != nil {
		return "", false
	}

	query := base.Query()
	query.Set("id_token_hint", idToken)
	if postLogoutRedirectURL := strings.TrimSpace(s.cfg.OIDC.PostLogoutRedirectURL); postLogoutRedirectURL != "" {
		query.Set("post_logout_redirect_uri", postLogoutRedirectURL)
	}
	base.RawQuery = query.Encode()

	return base.String(), true
}

func (s *Service) exchangeOIDCAuthorizationCode(code string) (oidcTokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", strings.TrimSpace(code))
	form.Set("redirect_uri", s.cfg.OIDC.RedirectURL)
	form.Set("client_id", s.cfg.OIDC.ClientID)
	form.Set("client_secret", s.cfg.OIDC.ClientSecret)

	req, err := http.NewRequest(http.MethodPost, s.cfg.OIDC.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return oidcTokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return oidcTokenResponse{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return oidcTokenResponse{}, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return oidcTokenResponse{}, fmt.Errorf("oidc token exchange failed with status %d", resp.StatusCode)
	}

	var parsed oidcTokenResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return oidcTokenResponse{}, err
	}

	if strings.TrimSpace(parsed.IDToken) == "" {
		return oidcTokenResponse{}, errors.New("oidc token response did not include id_token")
	}
	return parsed, nil
}

func validateOIDCState(state, signingKey string, now time.Time) (oidcStatePayload, error) {
	parts := strings.Split(strings.TrimSpace(state), ".")
	if len(parts) != 2 {
		return oidcStatePayload{}, errors.New("state format invalid")
	}

	payloadRaw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return oidcStatePayload{}, errors.New("state payload decode failed")
	}
	sigRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return oidcStatePayload{}, errors.New("state signature decode failed")
	}

	h := hmac.New(sha256.New, []byte(signingKey))
	_, _ = h.Write([]byte(parts[0]))
	expectedSig := h.Sum(nil)
	if subtle.ConstantTimeCompare(sigRaw, expectedSig) != 1 {
		return oidcStatePayload{}, errors.New("state signature mismatch")
	}

	var payload oidcStatePayload
	if err := json.Unmarshal(payloadRaw, &payload); err != nil {
		return oidcStatePayload{}, errors.New("state payload invalid")
	}
	if strings.TrimSpace(payload.Nonce) == "" {
		return oidcStatePayload{}, errors.New("state nonce missing")
	}
	if payload.Exp <= now.Unix() {
		return oidcStatePayload{}, errors.New("state expired")
	}

	return payload, nil
}

func signOIDCState(payload oidcStatePayload, signingKey string) (string, string, error) {
	payload.Nonce = strings.TrimSpace(payload.Nonce)
	if payload.Nonce == "" {
		payload.Nonce = randomToken(24)
	}
	if payload.Exp == 0 {
		payload.Exp = time.Now().UTC().Add(10 * time.Minute).Unix()
	}

	payloadRaw, err := json.Marshal(payload)
	if err != nil {
		return "", "", err
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payloadRaw)

	h := hmac.New(sha256.New, []byte(signingKey))
	_, _ = h.Write([]byte(encodedPayload))
	signature := base64.RawURLEncoding.EncodeToString(h.Sum(nil))

	return encodedPayload + "." + signature, payload.Nonce, nil
}

func (s *Service) verifyAndParseIDToken(idToken string, now time.Time) (idTokenClaims, error) {
	parts := strings.Split(strings.TrimSpace(idToken), ".")
	if len(parts) != 3 {
		return idTokenClaims{}, errors.New("id_token format invalid")
	}

	headerRaw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return idTokenClaims{}, errors.New("id_token header decode failed")
	}
	claimsRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return idTokenClaims{}, errors.New("id_token payload decode failed")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return idTokenClaims{}, errors.New("id_token signature decode failed")
	}

	var header idTokenHeader
	if err := json.Unmarshal(headerRaw, &header); err != nil {
		return idTokenClaims{}, errors.New("id_token header invalid")
	}
	if strings.TrimSpace(header.Alg) != "RS256" {
		return idTokenClaims{}, errors.New("id_token uses unsupported signing algorithm")
	}
	if strings.TrimSpace(header.Kid) == "" {
		return idTokenClaims{}, errors.New("id_token header missing kid")
	}

	var payload map[string]any
	if err := json.Unmarshal(claimsRaw, &payload); err != nil {
		return idTokenClaims{}, errors.New("id_token payload invalid")
	}

	claims := idTokenClaims{
		Subject: strings.TrimSpace(toString(payload["sub"])),
		Email:   firstNonEmptyString(payload["email"], payload["preferred_username"]),
		Name:    firstNonEmptyString(payload["name"], payload["given_name"]),
		Issuer:  strings.TrimSpace(toString(payload["iss"])),
		Nonce:   strings.TrimSpace(toString(payload["nonce"])),
		OrgID:   strings.TrimSpace(toString(payload["org_id"])),
		Exp:     toInt64(payload["exp"]),
		Iat:     toInt64(payload["iat"]),
		AZP:     strings.TrimSpace(toString(payload["azp"])),
		Aud:     toStringSlice(payload["aud"]),
	}

	if roles := toStringSlice(payload["roles"]); len(roles) > 0 {
		claims.Roles = roles
	} else if realmAccess, ok := payload["realm_access"].(map[string]any); ok {
		claims.Roles = toStringSlice(realmAccess["roles"])
	}

	if claims.Subject == "" {
		return idTokenClaims{}, errors.New("id_token missing sub")
	}
	if claims.Issuer == "" {
		return idTokenClaims{}, errors.New("id_token missing iss")
	}
	if claims.Exp == 0 {
		return idTokenClaims{}, errors.New("id_token missing exp")
	}
	if claims.Iat == 0 {
		return idTokenClaims{}, errors.New("id_token missing iat")
	}

	if claims.Exp <= now.Unix() {
		return idTokenClaims{}, errors.New("id_token is expired")
	}

	// Allow modest provider clock skew while rejecting clearly future-issued tokens.
	if claims.Iat > now.Add(2*time.Minute).Unix() {
		return idTokenClaims{}, errors.New("id_token iat is in the future")
	}

	jwks, err := s.fetchOIDCJWKS()
	if err != nil {
		return idTokenClaims{}, err
	}

	key, err := jwks.findSigningKeyByID(header.Kid, "RS256")
	if err != nil {
		// kid not found in cached JWKS — keys may have rotated; force refresh once.
		jwks, err = s.fetchOIDCJWKSInternal(true)
		if err != nil {
			return idTokenClaims{}, err
		}
		key, err = jwks.findSigningKeyByID(header.Kid, "RS256")
		if err != nil {
			return idTokenClaims{}, err
		}
	}

	publicKey, err := key.rsaPublicKey()
	if err != nil {
		return idTokenClaims{}, err
	}

	signingInput := []byte(parts[0] + "." + parts[1])
	digest := sha256.Sum256(signingInput)
	if err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, digest[:], signature); err != nil {
		return idTokenClaims{}, errors.New("id_token signature verification failed")
	}

	return claims, nil
}

// fetchOIDCJWKS returns the provider's JWKS. Keys are cached for jwksCacheTTL
// (default 5 minutes) so that a token with a known kid does not trigger a
// network call. Pass forceRefresh=true to bypass the cache (used when a token
// references an unknown kid that may not yet be in the cache).
func (s *Service) fetchOIDCJWKS() (oidcJWKSet, error) {
	return s.fetchOIDCJWKSInternal(false)
}

func (s *Service) fetchOIDCJWKSInternal(forceRefresh bool) (oidcJWKSet, error) {
	if !forceRefresh {
		s.jwksMu.RLock()
		if s.jwksCache != nil && time.Since(s.jwksCachedAt) < s.jwksCacheTTL {
			cached := *s.jwksCache
			s.jwksMu.RUnlock()
			return cached, nil
		}
		s.jwksMu.RUnlock()
	}

	req, err := http.NewRequest(http.MethodGet, s.cfg.OIDC.JWKSURL, nil)
	if err != nil {
		return oidcJWKSet{}, err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return oidcJWKSet{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return oidcJWKSet{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return oidcJWKSet{}, fmt.Errorf("oidc jwks fetch failed with status %d", resp.StatusCode)
	}

	var set oidcJWKSet
	if err := json.Unmarshal(body, &set); err != nil {
		return oidcJWKSet{}, errors.New("oidc jwks payload invalid")
	}
	if len(set.Keys) == 0 {
		return oidcJWKSet{}, errors.New("oidc jwks has no keys")
	}

	s.jwksMu.Lock()
	s.jwksCache = &set
	s.jwksCachedAt = time.Now().UTC()
	s.jwksMu.Unlock()

	return set, nil
}

func (s oidcJWKSet) findSigningKeyByID(kid, alg string) (oidcJWK, error) {
	for _, key := range s.Keys {
		if strings.TrimSpace(key.Kid) != strings.TrimSpace(kid) {
			continue
		}
		if strings.TrimSpace(strings.ToUpper(key.Kty)) != "RSA" {
			return oidcJWK{}, errors.New("oidc jwk key type is unsupported")
		}
		if use := strings.TrimSpace(strings.ToLower(key.Use)); use != "" && use != "sig" {
			return oidcJWK{}, errors.New("oidc jwk key use is not signing")
		}
		if keyAlg := strings.TrimSpace(key.Alg); keyAlg != "" && keyAlg != alg {
			return oidcJWK{}, errors.New("oidc jwk algorithm does not match id_token")
		}
		return key, nil
	}

	return oidcJWK{}, errors.New("oidc jwk for token kid was not found")
}

func (k oidcJWK) rsaPublicKey() (*rsa.PublicKey, error) {
	nRaw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(k.N))
	if err != nil {
		return nil, errors.New("oidc jwk modulus decode failed")
	}
	eRaw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(k.E))
	if err != nil {
		return nil, errors.New("oidc jwk exponent decode failed")
	}
	if len(nRaw) == 0 || len(eRaw) == 0 {
		return nil, errors.New("oidc jwk modulus/exponent missing")
	}

	n := new(big.Int).SetBytes(nRaw)
	eBig := new(big.Int).SetBytes(eRaw)
	if !eBig.IsInt64() {
		return nil, errors.New("oidc jwk exponent too large")
	}
	e := int(eBig.Int64())
	if e <= 0 {
		return nil, errors.New("oidc jwk exponent invalid")
	}

	return &rsa.PublicKey{N: n, E: e}, nil
}

func audienceContains(aud []string, expected string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return false
	}
	for _, value := range aud {
		if strings.TrimSpace(value) == expected {
			return true
		}
	}
	return false
}

func toString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	default:
		return ""
	}
}

func toInt64(value any) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case int64:
		return typed
	case int:
		return int64(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0
		}
		return parsed
	case string:
		parsed, err := strconv.ParseInt(typed, 10, 64)
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		if s := strings.TrimSpace(toString(value)); s != "" {
			return s
		}
	}
	return ""
}

func toStringSlice(value any) []string {
	switch typed := value.(type) {
	case string:
		if trimmed := strings.TrimSpace(typed); trimmed != "" {
			return []string{trimmed}
		}
		return nil
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if str := strings.TrimSpace(toString(item)); str != "" {
				out = append(out, str)
			}
		}
		return out
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if str := strings.TrimSpace(item); str != "" {
				out = append(out, str)
			}
		}
		return out
	default:
		return nil
	}
}

// verifyTOTPStep returns the 30-second time-step counter the code matched.
func verifyTOTPStep(secret, challengeCode string, now time.Time) (int64, bool) {
	secret = strings.TrimSpace(secret)
	challengeCode = strings.TrimSpace(challengeCode)
	if secret == "" || len(challengeCode) != 6 {
		return 0, false
	}

	for _, offset := range []int64{-1, 0, 1} {
		at := now.Unix() + offset*30
		if subtle.ConstantTimeCompare([]byte(generateTOTP(secret, at)), []byte(challengeCode)) == 1 {
			return at / 30, true
		}
	}
	return 0, false
}

// consumeTOTPStepInDB atomically records step as used for the factor. It
// reports false when this or a later step was already consumed (a replay),
// including by another API instance.
func (s *Service) consumeTOTPStepInDB(ctx context.Context, factorID string, step int64) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE mfa_factors
		SET totp_last_counter = $2
		WHERE id = $1 AND (totp_last_counter IS NULL OR totp_last_counter < $2)`,
		factorID, step)
	if err != nil {
		return false, fmt.Errorf("consume totp step: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// consumeBackupCodeInDB atomically flips a single unused backup code to used.
// It reports false when the code is absent or already consumed.
func (s *Service) consumeBackupCodeInDB(ctx context.Context, factorID, hash string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE mfa_factors
		SET backup_codes = (
			SELECT jsonb_agg(CASE WHEN e->>'Hash' = $2 THEN jsonb_set(e, '{Used}', 'true'::jsonb) ELSE e END)
			FROM jsonb_array_elements(backup_codes) e)
		WHERE id = $1
		  AND EXISTS (
			SELECT 1 FROM jsonb_array_elements(backup_codes) e
			WHERE e->>'Hash' = $2 AND COALESCE((e->>'Used')::boolean, false) = false)`,
		factorID, hash)
	if err != nil {
		return false, fmt.Errorf("consume backup code: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func generateTOTP(secret string, unixTime int64) string {
	counter := uint64(unixTime / 30)
	counterBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(counterBytes, counter)

	secretBytes, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	if err != nil {
		return ""
	}

	h := hmac.New(sha1.New, secretBytes)
	_, _ = h.Write(counterBytes)
	hash := h.Sum(nil)
	offset := hash[len(hash)-1] & 0x0f
	binaryCode := (int(hash[offset])&0x7f)<<24 |
		(int(hash[offset+1])&0xff)<<16 |
		(int(hash[offset+2])&0xff)<<8 |
		(int(hash[offset+3]) & 0xff)

	code := binaryCode % 1000000
	return fmt.Sprintf("%06d", code)
}

func hashChallengeCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

func generateTOTPSecret() string {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		panic(fmt.Errorf("secure random unavailable for TOTP secret: %w", err))
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
}

func hasRole(roles []string, role string) bool {
	for _, r := range roles {
		if strings.EqualFold(r, role) {
			return true
		}
	}
	return false
}

// copyPrincipal returns a value copy of p with its own Roles slice.
func copyPrincipal(p appmiddleware.Principal) appmiddleware.Principal {
	p.Roles = append([]string{}, p.Roles...)
	return p
}

func clonePrincipal(in *appmiddleware.Principal) *appmiddleware.Principal {
	if in == nil {
		return nil
	}
	cp := copyPrincipal(*in)
	return &cp
}

func randomToken(byteLen int) string {
	raw := make([]byte, byteLen)
	if _, err := rand.Read(raw); err != nil {
		panic(fmt.Errorf("secure random unavailable for token generation: %w", err))
	}
	return hex.EncodeToString(raw)
}

func randomAlphaNumeric(length int) string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	if length <= 0 {
		return ""
	}
	// Use rejection sampling via crypto/rand.Int to avoid modulo bias.
	charsetLen := big.NewInt(int64(len(charset)))
	out := make([]byte, length)
	for i := range out {
		n, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			panic(fmt.Errorf("secure random unavailable for alphanumeric token generation: %w", err))
		}
		out[i] = charset[n.Int64()]
	}
	return string(out)
}

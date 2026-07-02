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

	// TOTP replay prevention: "factorID|code" → time the code was last accepted.
	usedTOTPCodes map[string]time.Time

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

	err := s.upsertAuthenticatedUserRow(ctx, userID, email, name, orgID, lastLoginAt)
	if isUniqueConstraintViolation(err, "users_email_key") {
		surrogateEmail := surrogateSyncedUserEmail(userID)
		s.logger.Warn("authn: email already belongs to another users row; using surrogate email for DB user sync",
			"user_id", userID,
			"email", email,
			"surrogate_email", surrogateEmail,
		)
		err = s.upsertAuthenticatedUserRow(ctx, userID, surrogateEmail, name, orgID, lastLoginAt)
	}
	if err != nil {
		return fmt.Errorf("upsert authenticated user: %w", err)
	}
	return nil
}

func (s *Service) upsertAuthenticatedUserRow(ctx context.Context, userID, email, name, orgID string, lastLoginAt *time.Time) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO users (
			id, org_id, email, oidc_subject, display_name, status, last_login_at, created_at, updated_at
		)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, 'active', $6, now(), now())
		ON CONFLICT (id) DO UPDATE SET
			org_id        = EXCLUDED.org_id,
			email         = EXCLUDED.email,
			oidc_subject  = EXCLUDED.oidc_subject,
			display_name  = EXCLUDED.display_name,
			status        = 'active',
			last_login_at = COALESCE(EXCLUDED.last_login_at, users.last_login_at),
			updated_at    = now()`,
		userID, orgID, email, userID, name, lastLoginAt,
	)
	if err != nil {
		return err
	}
	return nil
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

func (s *Service) saveMFAFactorToDB(r mfaFactorRecord) {
	if s.pool == nil {
		return
	}
	var backupCodesJSON []byte
	if len(r.BackupCodes) > 0 {
		backupCodesJSON, _ = json.Marshal(r.BackupCodes)
	}
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
		s.logger.Warn("authn: save MFA factor to DB", "error", err)
	}
}

func (s *Service) updateMFAFactorInDB(r mfaFactorRecord) {
	if s.pool == nil {
		return
	}
	var backupCodesJSON []byte
	if len(r.BackupCodes) > 0 {
		backupCodesJSON, _ = json.Marshal(r.BackupCodes)
	}
	ctx := context.Background()
	_, err := s.pool.Exec(ctx, `
		UPDATE mfa_factors
		SET is_verified        = $2,
		    backup_codes       = $3,
		    last_challenged_at = $4
		WHERE id = $1`,
		r.Factor.ID, r.Factor.IsVerified, backupCodesJSON, r.Factor.LastChallengedAt,
	)
	if err != nil {
		s.logger.Warn("authn: update MFA factor in DB", "error", err)
	}
}

type SessionInput struct {
	UserID           string
	Email            string
	Name             string
	OrgID            string
	HouseholdID      string
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
		s.logger.Warn("oidc: org_id absent from ID token claims — falling back to state parameter; verify Keycloak protocol mapper (User Attribute 'org_id' → ID token claim 'org_id') is configured",
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

	session, err := s.IssueSession(SessionInput{
		UserID:           claims.Subject,
		Email:            claims.Email,
		Name:             claims.Name,
		OrgID:            resolvedOrgID,
		HouseholdID:      householdID,
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
	if err := s.syncAuthenticatedUser(context.Background(), input); err != nil {
		return SessionTokens{}, err
	}

	return s.issueSessionRecord(context.Background(), appmiddleware.Principal{
		UserID:           input.UserID,
		Email:            strings.TrimSpace(input.Email),
		Name:             strings.TrimSpace(input.Name),
		OrgID:            input.OrgID,
		HouseholdID:      input.HouseholdID,
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
	expiresAt := now.Add(24 * time.Hour)

	next := &sessionRecord{
		AccessToken:  nextAccess,
		RefreshToken: nextRefresh,
		IDToken:      record.IDToken,
		Principal: normalizeLocalDevPrincipal(appmiddleware.Principal{
			UserID:           record.Principal.UserID,
			Email:            record.Principal.Email,
			Name:             record.Principal.Name,
			OrgID:            record.Principal.OrgID,
			HouseholdID:      record.Principal.HouseholdID,
			Roles:            append([]string{}, record.Principal.Roles...),
			MFAAuthenticated: record.Principal.MFAAuthenticated,
		}),
		ExpiresAt: expiresAt,
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
	record.Factor = factor

	s.mu.Lock()
	s.mfaFactorsByUser[userID] = append(s.mfaFactorsByUser[userID], record)
	s.mu.Unlock()

	s.saveMFAFactorToDB(record)

	return factor, nil
}

func (s *Service) ChallengeMFA(currentAccessToken string, principal *appmiddleware.Principal, factorID, challengeCode string) (SessionTokens, error) {
	if principal == nil {
		return SessionTokens{}, ErrInvalidMFAFactor
	}

	currentAccessToken = strings.TrimSpace(currentAccessToken)

	s.mu.Lock()
	unlocked := false
	defer func() {
		if !unlocked {
			s.mu.Unlock()
		}
	}()

	factors := s.mfaFactorsByUser[principal.UserID]
	for i := range factors {
		if factors[i].Factor.ID != factorID {
			continue
		}

		switch factors[i].Factor.FactorType {
		case "totp":
			now := time.Now().UTC()
			replayKey := factors[i].Factor.ID + "|" + challengeCode
			// Purge expired replay entries (TTL = 90 seconds: the ±30s window × 3).
			for k, t := range s.usedTOTPCodes {
				if now.Sub(t) > 90*time.Second {
					delete(s.usedTOTPCodes, k)
				}
			}
			// Reject replayed codes within the acceptance window.
			if _, replayed := s.usedTOTPCodes[replayKey]; replayed {
				return SessionTokens{}, ErrInvalidMFACode
			}
			if !verifyTOTP(factors[i].TOTPSecret, challengeCode, now) {
				return SessionTokens{}, ErrInvalidMFACode
			}
			s.usedTOTPCodes[replayKey] = now
		case "backup_code":
			providedHash := hashChallengeCode(strings.ToUpper(strings.TrimSpace(challengeCode)))
			matched := false
			for j := range factors[i].BackupCodes {
				if factors[i].BackupCodes[j].Used {
					continue
				}
				if subtle.ConstantTimeCompare([]byte(providedHash), []byte(factors[i].BackupCodes[j].Hash)) == 1 {
					factors[i].BackupCodes[j].Used = true
					matched = true
					break
				}
			}
			if !matched {
				return SessionTokens{}, ErrInvalidMFACode
			}
		default:
			return SessionTokens{}, ErrUnsupportedMFAFactor
		}

		now := time.Now().UTC()
		factors[i].Factor.IsVerified = true
		factors[i].Factor.LastChallengedAt = &now
		s.mfaFactorsByUser[principal.UserID] = factors
		updated := factors[i]
		go s.updateMFAFactorInDB(updated)

		var idToken string
		if currentAccessToken != "" && s.pool == nil {
			if currentRecord, ok := s.sessionsByAccess[currentAccessToken]; ok {
				idToken = currentRecord.IDToken
				currentRecord.RevokedAt = &now
			}
		}
		s.mu.Unlock()
		unlocked = true

		if currentAccessToken != "" && s.pool != nil {
			currentRecord, err := s.revokePersistentSessionByAccessToken(context.Background(), currentAccessToken)
			if err == nil && currentRecord != nil {
				idToken = currentRecord.IDToken
			}
		}

		return s.issueSessionRecord(context.Background(), appmiddleware.Principal{
			UserID:           principal.UserID,
			Email:            principal.Email,
			Name:             principal.Name,
			OrgID:            principal.OrgID,
			HouseholdID:      principal.HouseholdID,
			Roles:            append([]string{}, principal.Roles...),
			MFAAuthenticated: true,
		}, idToken, 15*time.Minute)
	}

	return SessionTokens{}, ErrInvalidMFAFactor
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
		Principal: appmiddleware.Principal{
			UserID:           principal.UserID,
			Email:            principal.Email,
			Name:             principal.Name,
			OrgID:            principal.OrgID,
			HouseholdID:      principal.HouseholdID,
			Roles:            append([]string{}, principal.Roles...),
			MFAAuthenticated: principal.MFAAuthenticated,
		},
		ExpiresAt: expiresAt,
	}

	if s.pool != nil {
		if err := s.persistSessionRecord(ctx, record); err != nil {
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

func (s *Service) persistSessionRecord(ctx context.Context, record *sessionRecord) error {
	if s.pool == nil {
		return nil
	}

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

	_, err = s.pool.Exec(ctx, `
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
	expiresAt := now.Add(24 * time.Hour)

	next := &sessionRecord{
		AccessToken:  nextAccess,
		RefreshToken: nextRefresh,
		IDToken:      record.IDToken,
		Principal: appmiddleware.Principal{
			UserID:           record.Principal.UserID,
			Email:            record.Principal.Email,
			Name:             record.Principal.Name,
			OrgID:            record.Principal.OrgID,
			HouseholdID:      record.Principal.HouseholdID,
			Roles:            append([]string{}, record.Principal.Roles...),
			MFAAuthenticated: record.Principal.MFAAuthenticated,
		},
		ExpiresAt: expiresAt,
	}
	if err := s.persistSessionRecordTx(ctx, tx, next); err != nil {
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

func (s *Service) persistSessionRecordTx(ctx context.Context, tx pgx.Tx, record *sessionRecord) error {
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

	_, err = tx.Exec(ctx, `
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
	record, err := s.loadPersistentSessionAny(ctx, `
		SELECT principal, COALESCE(id_token, ''), expires_at, revoked_at
		FROM sessions
		WHERE refresh_token_hash = $1`,
		hashToken(refreshToken),
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
		WHERE refresh_token_hash = $1 AND revoked_at IS NULL`,
		hashToken(refreshToken), now,
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

func (s *Service) revokePersistentSessionByAccessToken(ctx context.Context, accessToken string) (*sessionRecord, error) {
	record, err := s.loadPersistentSessionAny(ctx, `
		SELECT principal, COALESCE(id_token, ''), expires_at, revoked_at
		FROM sessions
		WHERE access_token_hash = $1`,
		hashToken(accessToken),
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
		WHERE access_token_hash = $1 AND revoked_at IS NULL`,
		hashToken(accessToken), now,
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

func (s *Service) loadPersistentSessionByRefreshToken(ctx context.Context, refreshToken string) (*sessionRecord, error) {
	return s.loadPersistentSessionByRefreshTokenTx(ctx, s.pool, refreshToken, false)
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
	var principalJSON []byte
	record := &sessionRecord{}
	err := q.QueryRow(ctx, query, hashToken(refreshToken)).Scan(&principalJSON, &record.IDToken, &record.ExpiresAt, &record.RevokedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("load session: %w", err)
	}
	if err := json.Unmarshal(principalJSON, &record.Principal); err != nil {
		return nil, fmt.Errorf("unmarshal session principal: %w", err)
	}
	record.Principal = normalizeLocalDevPrincipal(record.Principal)
	now := time.Now().UTC()
	if record.RevokedAt != nil {
		return nil, ErrSessionRevoked
	}
	if now.After(record.ExpiresAt) {
		return nil, ErrSessionExpired
	}
	return record, nil
}

func (s *Service) loadPersistentValidatedSession(ctx context.Context, query string, arg any) (*sessionRecord, error) {
	record, err := s.loadPersistentSessionAny(ctx, query, arg)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if record.RevokedAt != nil {
		return nil, ErrSessionRevoked
	}
	if now.After(record.ExpiresAt) {
		return nil, ErrSessionExpired
	}
	return record, nil
}

func (s *Service) loadPersistentSessionAny(ctx context.Context, query string, arg any) (*sessionRecord, error) {
	var principalJSON []byte
	record := &sessionRecord{}
	err := s.pool.QueryRow(ctx, query, arg).Scan(&principalJSON, &record.IDToken, &record.ExpiresAt, &record.RevokedAt)
	if err != nil {
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
	return strings.TrimSpace(oidc.TokenURL) != "" &&
		strings.TrimSpace(oidc.JWKSURL) != "" &&
		strings.TrimSpace(oidc.RedirectURL) != "" &&
		strings.TrimSpace(oidc.ClientID) != "" &&
		strings.TrimSpace(oidc.ClientSecret) != "" &&
		strings.TrimSpace(oidc.StateSigningKey) != ""
}

func (s *Service) oidcAuthorizeConfigured() bool {
	oidc := s.cfg.OIDC
	return strings.TrimSpace(oidc.AuthorizeURL) != "" &&
		strings.TrimSpace(oidc.RedirectURL) != "" &&
		strings.TrimSpace(oidc.ClientID) != "" &&
		strings.TrimSpace(oidc.StateSigningKey) != ""
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

func verifyTOTP(secret, challengeCode string, now time.Time) bool {
	secret = strings.TrimSpace(secret)
	challengeCode = strings.TrimSpace(challengeCode)
	if secret == "" || len(challengeCode) != 6 {
		return false
	}

	for _, offset := range []int64{-1, 0, 1} {
		if generateTOTP(secret, now.Unix()+offset*30) == challengeCode {
			return true
		}
	}
	return false
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

func clonePrincipal(in *appmiddleware.Principal) *appmiddleware.Principal {
	if in == nil {
		return nil
	}
	return &appmiddleware.Principal{
		UserID:           in.UserID,
		Email:            in.Email,
		Name:             in.Name,
		OrgID:            in.OrgID,
		HouseholdID:      in.HouseholdID,
		Roles:            append([]string{}, in.Roles...),
		MFAAuthenticated: in.MFAAuthenticated,
	}
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

func randomBase32(length int) string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
	if length <= 0 {
		return ""
	}

	buffer := make([]byte, length)
	if _, err := rand.Read(buffer); err != nil {
		panic(fmt.Errorf("secure random unavailable for base32 token generation: %w", err))
	}

	out := make([]byte, length)
	for i, value := range buffer {
		out[i] = charset[int(value)%len(charset)]
	}
	return string(out)
}

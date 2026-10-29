// Package keycloak provides a Keycloak Admin REST API client interface,
// an in-memory stub for testing, and a production HTTP client implementation.
package keycloak

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ─── Errors ───────────────────────────────────────────────────────────────────

var (
	ErrInvalidConfig = errors.New("keycloak: missing required configuration")
	ErrUserNotFound  = errors.New("keycloak: user not found")
	ErrRoleNotFound  = errors.New("keycloak: role not found")
	ErrUnauthorized  = errors.New("keycloak: unauthorized — check admin credentials")
	ErrConflict      = errors.New("keycloak: user or role already exists")
	ErrLastAdmin     = errors.New("keycloak: cannot remove last admin role assignment")
)

// ─── Types ────────────────────────────────────────────────────────────────────

// User represents a Keycloak user record from the Admin REST API.
type User struct {
	// ID is the Keycloak user UUID.
	ID string `json:"id"`
	// Username is the Keycloak username (typically the email address).
	Username string `json:"username"`
	// Email is the user's email address.
	Email string `json:"email"`
	// FirstName is the user's first name.
	FirstName string `json:"firstName"`
	// LastName is the user's last name.
	LastName string `json:"lastName"`
	// Enabled indicates whether the user account is active.
	Enabled bool `json:"enabled"`
	// EmailVerified indicates whether the user's email has been verified.
	EmailVerified bool `json:"emailVerified"`
	// Attributes holds custom user attributes (e.g. org_id).
	Attributes map[string][]string `json:"attributes,omitempty"`
}

// OrgID returns the org_id attribute value, or an empty string if not set.
func (u *User) OrgID() string {
	if u.Attributes == nil {
		return ""
	}
	vals := u.Attributes["org_id"]
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

// Role represents a Keycloak realm role.
type Role struct {
	// ID is the Keycloak role UUID.
	ID string `json:"id"`
	// Name is the role name (e.g. "admin", "coach").
	Name string `json:"name"`
	// Description is an optional human-readable description.
	Description string `json:"description,omitempty"`
}

// UpdateUserFields holds the subset of user fields that may be patched.
// Zero values are not sent to Keycloak; only explicitly set fields are updated.
type UpdateUserFields struct {
	// FirstName, if non-empty, updates the display first name.
	FirstName string
	// LastName, if non-empty, updates the display last name.
	LastName string
	// Email, if non-empty, updates the user's email address.
	Email string
	// Enabled, if non-nil, sets the account enabled/disabled state.
	Enabled *bool
	// Attributes, if non-nil, replaces the user's custom attributes map.
	// Pass the full desired attributes map (including any keys you want to preserve).
	Attributes map[string][]string
}

// ─── Client interface ─────────────────────────────────────────────────────────

// Client is the Keycloak Admin REST API surface used by this application.
// Production deployments use *AdminClient. Tests inject *StubClient.
type Client interface {
	// ListUsers returns users in the realm that belong to the given orgID.
	// search filters by username, email, first/last name (Keycloak full-text search).
	// limit and offset control pagination (limit ≤ 0 defaults to 50, max 500).
	ListUsers(ctx context.Context, orgID, search string, limit, offset int) ([]User, error)

	// GetUser fetches a single user by their Keycloak UUID.
	// Returns ErrUserNotFound if the ID does not exist in the realm.
	GetUser(ctx context.Context, userID string) (*User, error)

	// CreateUser provisions a new Keycloak user with the given attributes.
	// The user is created with emailVerified=false and enabled=true.
	// Returns the new Keycloak user UUID. Returns ErrConflict if the email
	// address is already registered in the realm.
	CreateUser(ctx context.Context, email, firstName, lastName, orgID string) (string, error)

	// SendVerificationEmail triggers a Keycloak VERIFY_EMAIL action email for the user.
	SendVerificationEmail(ctx context.Context, userID string) error

	// UpdateUser patches the mutable fields on an existing Keycloak user.
	// Only non-zero fields in UpdateUserFields are applied.
	UpdateUser(ctx context.Context, userID string, fields UpdateUserFields) error

	// DeleteUser permanently removes a user from the Keycloak realm.
	// Returns ErrUserNotFound if the user does not exist.
	DeleteUser(ctx context.Context, userID string) error

	// DisableUser sets the Keycloak user's enabled flag to false.
	DisableUser(ctx context.Context, userID string) error

	// EnableUser sets the Keycloak user's enabled flag to true.
	EnableUser(ctx context.Context, userID string) error

	// GetUserRealmRoles returns the list of realm roles assigned to the user.
	GetUserRealmRoles(ctx context.Context, userID string) ([]Role, error)

	// ListUsersWithRealmRole returns every user in orgID that holds the named
	// realm role, paging the role's membership to exhaustion. One role query
	// replaces a per-user role lookup (the N+1 used to count org admins). An
	// unknown role yields an empty list.
	ListUsersWithRealmRole(ctx context.Context, orgID, roleName string) ([]User, error)

	// AssignRealmRole adds the named realm role to the user.
	// Returns ErrRoleNotFound if the role does not exist in the realm.
	AssignRealmRole(ctx context.Context, userID, roleName string) error

	// RemoveRealmRole removes the named realm role from the user.
	// Returns ErrRoleNotFound if the role does not exist in the realm.
	RemoveRealmRole(ctx context.Context, userID, roleName string) error

	// EnsureRealmRolesExist idempotently creates each role name in the realm
	// if it does not already exist. Safe to call on startup.
	EnsureRealmRolesExist(ctx context.Context, roleNames []string) error

	// SendPasswordResetEmail triggers a Keycloak UPDATE_PASSWORD action email.
	SendPasswordResetEmail(ctx context.Context, userID string) error

	// SetUserPassword sets a direct (non-temporary) password for the given user.
	// Use this for admin-created accounts where the admin sets the initial password.
	SetUserPassword(ctx context.Context, userID, password string, temporary bool) error
}

// ─── Default realm roles ──────────────────────────────────────────────────────

// DefaultRealmRoles is the canonical set of DARE platform realm roles that
// must exist in the Keycloak realm. Pass this to EnsureRealmRolesExist on
// startup.
var DefaultRealmRoles = []string{
	"admin",
	"coach",
	"parent",
	"swimmer",
}

// ─── Production Admin client ──────────────────────────────────────────────────

// AdminConfig holds the credentials and settings for the Keycloak Admin REST API.
type AdminConfig struct {
	// AdminURL is the base URL of the Keycloak instance (e.g. "http://localhost:8180").
	AdminURL string
	// Realm is the target Keycloak realm name (e.g. "dare").
	Realm string
	// ClientID is the service-account OAuth2 client ID.
	ClientID string
	// ClientSecret is the service-account OAuth2 client secret.
	ClientSecret string
	// HTTPTimeout is the per-request timeout. Defaults to 10 s.
	HTTPTimeout time.Duration
}

// AdminClient is a production Keycloak Admin REST API client backed by
// plain net/http. It fetches and auto-refreshes a service-account token
// via the client_credentials grant.
type AdminClient struct {
	cfg    AdminConfig
	http   *http.Client
	mu     sync.Mutex
	token  string
	expiry time.Time
}

// UnavailableClient is a non-functional Keycloak client used when the app is
// intentionally running without a working admin client configuration. All
// methods fail with the configured error instead of silently mutating a stub.
type UnavailableClient struct {
	err error
}

// NewAdminClient creates a new production AdminClient.
// Returns ErrInvalidConfig if required fields are missing.
func NewAdminClient(cfg AdminConfig) (*AdminClient, error) {
	cfg.AdminURL = strings.TrimRight(strings.TrimSpace(cfg.AdminURL), "/")
	cfg.Realm = strings.TrimSpace(cfg.Realm)
	cfg.ClientID = strings.TrimSpace(cfg.ClientID)
	cfg.ClientSecret = strings.TrimSpace(cfg.ClientSecret)

	if cfg.AdminURL == "" || cfg.Realm == "" || cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, ErrInvalidConfig
	}
	if cfg.HTTPTimeout <= 0 {
		cfg.HTTPTimeout = 10 * time.Second
	}
	return &AdminClient{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.HTTPTimeout},
	}, nil
}

// NewUnavailableClient returns a Keycloak client that always fails with err.
// When err is nil, ErrInvalidConfig is used.
func NewUnavailableClient(err error) *UnavailableClient {
	if err == nil {
		err = ErrInvalidConfig
	}
	return &UnavailableClient{err: err}
}

func (c *UnavailableClient) failure() error {
	if c == nil || c.err == nil {
		return ErrInvalidConfig
	}
	return c.err
}

// Ready reports the underlying unavailability reason for readiness probes.
func (c *UnavailableClient) Ready(_ context.Context) error {
	return c.failure()
}

func (c *UnavailableClient) ListUsers(_ context.Context, _, _ string, _, _ int) ([]User, error) {
	return nil, c.failure()
}

func (c *UnavailableClient) GetUser(_ context.Context, _ string) (*User, error) {
	return nil, c.failure()
}

func (c *UnavailableClient) CreateUser(_ context.Context, _, _, _, _ string) (string, error) {
	return "", c.failure()
}

func (c *UnavailableClient) SendVerificationEmail(_ context.Context, _ string) error {
	return c.failure()
}

func (c *UnavailableClient) UpdateUser(_ context.Context, _ string, _ UpdateUserFields) error {
	return c.failure()
}

func (c *UnavailableClient) DeleteUser(_ context.Context, _ string) error {
	return c.failure()
}

func (c *UnavailableClient) DisableUser(_ context.Context, _ string) error {
	return c.failure()
}

func (c *UnavailableClient) EnableUser(_ context.Context, _ string) error {
	return c.failure()
}

func (c *UnavailableClient) GetUserRealmRoles(_ context.Context, _ string) ([]Role, error) {
	return nil, c.failure()
}

func (c *UnavailableClient) ListUsersWithRealmRole(_ context.Context, _, _ string) ([]User, error) {
	return nil, c.failure()
}

func (c *UnavailableClient) AssignRealmRole(_ context.Context, _, _ string) error {
	return c.failure()
}

func (c *UnavailableClient) RemoveRealmRole(_ context.Context, _, _ string) error {
	return c.failure()
}

func (c *UnavailableClient) EnsureRealmRolesExist(_ context.Context, _ []string) error {
	return c.failure()
}

func (c *UnavailableClient) SendPasswordResetEmail(_ context.Context, _ string) error {
	return c.failure()
}

func (c *UnavailableClient) SetUserPassword(_ context.Context, _, _ string, _ bool) error {
	return c.failure()
}

// Ready verifies that the admin client can obtain a service-account token.
func (c *AdminClient) Ready(ctx context.Context) error {
	_, err := c.getToken(ctx)
	return err
}

// ─── Token management ─────────────────────────────────────────────────────────

// getToken returns a valid admin Bearer token, refreshing if necessary.
func (c *AdminClient) getToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && time.Now().Before(c.expiry.Add(-30*time.Second)) {
		return c.token, nil
	}
	return c.refreshToken(ctx)
}

// refreshToken fetches a new admin token via client_credentials grant.
// Must be called with c.mu held.
func (c *AdminClient) refreshToken(ctx context.Context) (string, error) {
	tokenURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token",
		c.cfg.AdminURL, c.cfg.Realm)

	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.cfg.ClientID},
		"client_secret": {c.cfg.ClientSecret},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("keycloak: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("keycloak: token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return "", ErrUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("keycloak: token request returned %d: %s", resp.StatusCode, body)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("keycloak: decode token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("keycloak: empty access_token in response")
	}

	expiry := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	c.token = tokenResp.AccessToken
	c.expiry = expiry
	return c.token, nil
}

// ─── HTTP helpers ─────────────────────────────────────────────────────────────

func (c *AdminClient) adminURL(path string) string {
	return fmt.Sprintf("%s/admin/realms/%s%s", c.cfg.AdminURL, c.cfg.Realm, path)
}

// userURL builds an admin URL for one user. Keycloak user IDs are UUIDs; an ID
// with anything outside [A-Za-z0-9_.@-], or containing "..", (a "/", an encoded
// slash, a traversal segment) is treated
// as not found rather than being spliced into the path, where it could address a
// different admin resource with the service account's credentials.
func (c *AdminClient) userURL(userID, suffix string) (string, error) {
	if userID == "" || strings.Contains(userID, "..") {
		return "", ErrUserNotFound
	}
	for _, r := range userID {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' || r == '@'
		if !ok {
			return "", ErrUserNotFound
		}
	}
	return c.adminURL("/users/" + userID + suffix), nil
}

func (c *AdminClient) do(ctx context.Context, method, urlStr string, body any) (*http.Response, error) {
	token, err := c.getToken(ctx)
	if err != nil {
		return nil, err
	}

	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("keycloak: marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, urlStr, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("keycloak: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.http.Do(req)
}

func readBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 512*1024))
}

// ─── ListUsers ────────────────────────────────────────────────────────────────

// ListUsers returns users in the realm whose org_id attribute matches orgID.
func (c *AdminClient) ListUsers(ctx context.Context, orgID, search string, limit, offset int) ([]User, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	users, err := c.listUsersPage(ctx, search, limit, offset, orgID)
	if err != nil {
		return nil, err
	}
	users, err = c.hydrateUsersMissingOrg(ctx, users, orgID)
	if err != nil {
		return nil, err
	}

	filtered := filterUsersByOrg(users, orgID)

	// Some Keycloak deployments do not reliably honor custom-attribute `q=`
	// filters for list-users, and some return brief representations that omit
	// custom attributes needed for local org filtering. Fall back to a broader
	// realm search and filter the org locally so the admin /users page still renders.
	if orgID == "" {
		return users, nil
	}
	if len(users) > 0 && len(filtered) == len(users) {
		return filtered, nil
	}

	// Page the realm until the requested org window is filled or the realm is
	// exhausted, filtering by org locally. The window is applied to the org's own
	// users, so orgs beyond the first realm page and deep offsets work.
	matched := make([]User, 0, offset+limit)
	for page := 0; page < maxFallbackPages && len(matched) < offset+limit; page++ {
		batch, err := c.listUsersPage(ctx, search, fallbackPageSize, page*fallbackPageSize, "")
		if err != nil {
			return nil, err
		}
		hydrated, err := c.hydrateUsersMissingOrg(ctx, batch, orgID)
		if err != nil {
			return nil, err
		}
		matched = append(matched, filterUsersByOrg(hydrated, orgID)...)
		if len(batch) < fallbackPageSize {
			break // realm exhausted
		}
	}
	if offset >= len(matched) {
		return []User{}, nil
	}
	matched = matched[offset:]
	if len(matched) > limit {
		matched = matched[:limit]
	}
	return matched, nil
}

const (
	// fallbackPageSize is the realm page size used by the local-org-filter
	// fallback; maxFallbackPages bounds the scan (100k realm users) so a
	// misbehaving server cannot loop forever.
	fallbackPageSize = 200
	maxFallbackPages = 500
)

func (c *AdminClient) hydrateUsersMissingOrg(ctx context.Context, users []User, orgID string) ([]User, error) {
	if orgID == "" || len(users) == 0 {
		return users, nil
	}
	out := make([]User, len(users))
	copy(out, users)
	for i := range out {
		if out[i].OrgID() != "" {
			continue
		}
		full, err := c.GetUser(ctx, out[i].ID)
		if err != nil {
			if errors.Is(err, ErrUserNotFound) {
				continue
			}
			return nil, err
		}
		if full != nil {
			out[i] = *full
		}
	}
	return out, nil
}

func filterUsersByOrg(users []User, orgID string) []User {
	if orgID == "" {
		return users
	}
	filtered := make([]User, 0, len(users))
	for _, user := range users {
		if user.OrgID() == orgID {
			filtered = append(filtered, user)
		}
	}
	return filtered
}

func (c *AdminClient) listUsersPage(ctx context.Context, search string, limit, offset int, orgID string) ([]User, error) {
	params := url.Values{
		"max":                 {fmt.Sprintf("%d", limit)},
		"first":               {fmt.Sprintf("%d", offset)},
		"briefRepresentation": {"false"},
	}
	if search != "" {
		params.Set("search", search)
	}
	// Filter by org_id attribute using Keycloak's q= attribute search syntax.
	if orgID != "" {
		params.Set("q", fmt.Sprintf("org_id:%s", orgID))
	}

	urlStr := c.adminURL("/users") + "?" + params.Encode()
	resp, err := c.do(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}
	body, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("keycloak: list users returned %d: %s", resp.StatusCode, body)
	}

	var users []User
	if err := json.Unmarshal(body, &users); err != nil {
		return nil, fmt.Errorf("keycloak: decode users: %w", err)
	}
	return users, nil
}

// ─── GetUser ──────────────────────────────────────────────────────────────────

func (c *AdminClient) GetUser(ctx context.Context, userID string) (*User, error) {
	urlStr, err := c.userURL(userID, "")
	if err != nil {
		return nil, err
	}
	resp, err := c.do(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}
	body, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrUserNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("keycloak: get user returned %d: %s", resp.StatusCode, body)
	}
	var u User
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("keycloak: decode user: %w", err)
	}
	return &u, nil
}

// ─── CreateUser ───────────────────────────────────────────────────────────────

func (c *AdminClient) CreateUser(ctx context.Context, email, firstName, lastName, orgID string) (string, error) {
	payload := map[string]any{
		"username":      strings.ToLower(strings.TrimSpace(email)),
		"email":         strings.TrimSpace(email),
		"firstName":     strings.TrimSpace(firstName),
		"lastName":      strings.TrimSpace(lastName),
		"enabled":       true,
		"emailVerified": false,
		"attributes": map[string][]string{
			"org_id": {orgID},
		},
	}

	resp, err := c.do(ctx, http.MethodPost, c.adminURL("/users"), payload)
	if err != nil {
		return "", err
	}
	body, err := readBody(resp)
	if err != nil {
		return "", err
	}

	switch resp.StatusCode {
	case http.StatusCreated:
		// Keycloak returns the new user URL in Location header.
		location := resp.Header.Get("Location")
		parts := strings.Split(strings.TrimRight(location, "/"), "/")
		if len(parts) == 0 {
			return "", fmt.Errorf("keycloak: create user: missing Location header")
		}
		return parts[len(parts)-1], nil
	case http.StatusConflict:
		return "", ErrConflict
	default:
		return "", fmt.Errorf("keycloak: create user returned %d: %s", resp.StatusCode, body)
	}
}

// ─── SendVerificationEmail ────────────────────────────────────────────────────

func (c *AdminClient) SendVerificationEmail(ctx context.Context, userID string) error {
	return c.sendActionsEmail(ctx, userID, []string{"VERIFY_EMAIL"})
}

// SendPasswordResetEmail triggers a Keycloak UPDATE_PASSWORD action email.
func (c *AdminClient) SendPasswordResetEmail(ctx context.Context, userID string) error {
	return c.sendActionsEmail(ctx, userID, []string{"UPDATE_PASSWORD"})
}

func (c *AdminClient) sendActionsEmail(ctx context.Context, userID string, actions []string) error {
	urlStr, err := c.userURL(userID, "/execute-actions-email")
	if err != nil {
		return err
	}
	resp, err := c.do(ctx, http.MethodPut, urlStr, actions)
	if err != nil {
		return err
	}
	body, err := readBody(resp)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		return ErrUserNotFound
	}
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("keycloak: send actions email returned %d: %s", resp.StatusCode, body)
	}
	return nil
}

// ─── SetUserPassword ──────────────────────────────────────────────────────────

// SetUserPassword sets a direct password credential on the given user.
// If temporary is true, Keycloak forces a password change on next login.
func (c *AdminClient) SetUserPassword(ctx context.Context, userID, password string, temporary bool) error {
	payload := map[string]any{
		"type":      "password",
		"value":     password,
		"temporary": temporary,
	}
	urlStr, err := c.userURL(userID, "/reset-password")
	if err != nil {
		return err
	}
	resp, err := c.do(ctx, http.MethodPut, urlStr, payload)
	if err != nil {
		return err
	}
	body, err := readBody(resp)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		return ErrUserNotFound
	}
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("keycloak: set password returned %d: %s", resp.StatusCode, body)
	}
	return nil
}

// ─── UpdateUser ───────────────────────────────────────────────────────────────

func (c *AdminClient) UpdateUser(ctx context.Context, userID string, fields UpdateUserFields) error {
	payload := make(map[string]any)
	if strings.TrimSpace(fields.FirstName) != "" {
		payload["firstName"] = strings.TrimSpace(fields.FirstName)
	}
	if strings.TrimSpace(fields.LastName) != "" {
		payload["lastName"] = strings.TrimSpace(fields.LastName)
	}
	if strings.TrimSpace(fields.Email) != "" {
		payload["email"] = strings.TrimSpace(fields.Email)
		payload["username"] = strings.ToLower(strings.TrimSpace(fields.Email))
	}
	if fields.Enabled != nil {
		payload["enabled"] = *fields.Enabled
	}
	if len(fields.Attributes) > 0 {
		payload["attributes"] = fields.Attributes
	}

	if len(payload) == 0 {
		return nil // nothing to update
	}

	urlStr, err := c.userURL(userID, "")
	if err != nil {
		return err
	}
	resp, err := c.do(ctx, http.MethodPut, urlStr, payload)
	if err != nil {
		return err
	}
	body, err := readBody(resp)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		return ErrUserNotFound
	}
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("keycloak: update user returned %d: %s", resp.StatusCode, body)
	}
	return nil
}

// ─── DeleteUser ───────────────────────────────────────────────────────────────

func (c *AdminClient) DeleteUser(ctx context.Context, userID string) error {
	urlStr, err := c.userURL(userID, "")
	if err != nil {
		return err
	}
	resp, err := c.do(ctx, http.MethodDelete, urlStr, nil)
	if err != nil {
		return err
	}
	body, err := readBody(resp)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		return ErrUserNotFound
	}
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("keycloak: delete user returned %d: %s", resp.StatusCode, body)
	}
	return nil
}

// ─── DisableUser / EnableUser ─────────────────────────────────────────────────

func (c *AdminClient) DisableUser(ctx context.Context, userID string) error {
	f := false
	return c.UpdateUser(ctx, userID, UpdateUserFields{Enabled: &f})
}

func (c *AdminClient) EnableUser(ctx context.Context, userID string) error {
	t := true
	return c.UpdateUser(ctx, userID, UpdateUserFields{Enabled: &t})
}

// ─── Realm roles ──────────────────────────────────────────────────────────────

// GetUserRealmRoles returns the realm roles assigned to the user.
func (c *AdminClient) GetUserRealmRoles(ctx context.Context, userID string) ([]Role, error) {
	urlStr, err := c.userURL(userID, "/role-mappings/realm")
	if err != nil {
		return nil, err
	}
	resp, err := c.do(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}
	body, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrUserNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("keycloak: get user roles returned %d: %s", resp.StatusCode, body)
	}
	var roles []Role
	if err := json.Unmarshal(body, &roles); err != nil {
		return nil, fmt.Errorf("keycloak: decode roles: %w", err)
	}
	return roles, nil
}

// ListUsersWithRealmRole pages GET /roles/{role}/users to exhaustion and keeps
// the users belonging to orgID (the endpoint is realm-wide).
func (c *AdminClient) ListUsersWithRealmRole(ctx context.Context, orgID, roleName string) ([]User, error) {
	var matched []User
	for page := 0; page < maxFallbackPages; page++ {
		params := url.Values{
			"first":               {fmt.Sprintf("%d", page*fallbackPageSize)},
			"max":                 {fmt.Sprintf("%d", fallbackPageSize)},
			"briefRepresentation": {"false"},
		}
		urlStr := c.adminURL("/roles/"+url.PathEscape(roleName)+"/users") + "?" + params.Encode()
		resp, err := c.do(ctx, http.MethodGet, urlStr, nil)
		if err != nil {
			return nil, err
		}
		body, err := readBody(resp)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusNotFound {
			return []User{}, nil
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("keycloak: list role members returned %d: %s", resp.StatusCode, body)
		}
		var batch []User
		if err := json.Unmarshal(body, &batch); err != nil {
			return nil, fmt.Errorf("keycloak: decode role members: %w", err)
		}
		hydrated, err := c.hydrateUsersMissingOrg(ctx, batch, orgID)
		if err != nil {
			return nil, err
		}
		matched = append(matched, filterUsersByOrg(hydrated, orgID)...)
		if len(batch) < fallbackPageSize {
			return matched, nil
		}
	}
	return nil, fmt.Errorf("keycloak: role %q membership exceeds the %d-page scan limit", roleName, maxFallbackPages)
}

// getRoleByName fetches a realm role by name and returns the full Role (with ID).
func (c *AdminClient) getRoleByName(ctx context.Context, roleName string) (*Role, error) {
	urlStr := c.adminURL("/roles/" + url.PathEscape(roleName))
	resp, err := c.do(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}
	body, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrRoleNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("keycloak: get role returned %d: %s", resp.StatusCode, body)
	}
	var role Role
	if err := json.Unmarshal(body, &role); err != nil {
		return nil, fmt.Errorf("keycloak: decode role: %w", err)
	}
	return &role, nil
}

// AssignRealmRole adds the named role to the user.
func (c *AdminClient) AssignRealmRole(ctx context.Context, userID, roleName string) error {
	role, err := c.getRoleByName(ctx, roleName)
	if err != nil {
		return err
	}
	urlStr, err := c.userURL(userID, "/role-mappings/realm")
	if err != nil {
		return err
	}
	resp, err := c.do(ctx, http.MethodPost, urlStr, []Role{*role})
	if err != nil {
		return err
	}
	body, err := readBody(resp)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		return ErrUserNotFound
	}
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("keycloak: assign role returned %d: %s", resp.StatusCode, body)
	}
	return nil
}

// RemoveRealmRole removes the named role from the user.
func (c *AdminClient) RemoveRealmRole(ctx context.Context, userID, roleName string) error {
	role, err := c.getRoleByName(ctx, roleName)
	if err != nil {
		return err
	}
	urlStr, err := c.userURL(userID, "/role-mappings/realm")
	if err != nil {
		return err
	}
	resp, err := c.do(ctx, http.MethodDelete, urlStr, []Role{*role})
	if err != nil {
		return err
	}
	body, err := readBody(resp)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		return ErrUserNotFound
	}
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("keycloak: remove role returned %d: %s", resp.StatusCode, body)
	}
	return nil
}

// EnsureRealmRolesExist idempotently creates each role if it does not exist.
func (c *AdminClient) EnsureRealmRolesExist(ctx context.Context, roleNames []string) error {
	for _, name := range roleNames {
		if err := c.ensureRole(ctx, name); err != nil {
			return fmt.Errorf("keycloak: ensure role %q: %w", name, err)
		}
	}
	return nil
}

func (c *AdminClient) ensureRole(ctx context.Context, name string) error {
	_, err := c.getRoleByName(ctx, name)
	if err == nil {
		return nil // already exists
	}
	if !errors.Is(err, ErrRoleNotFound) {
		return err
	}
	// Create the role.
	payload := map[string]string{"name": name}
	resp, err := c.do(ctx, http.MethodPost, c.adminURL("/roles"), payload)
	if err != nil {
		return err
	}
	body, err := readBody(resp)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusConflict {
		return nil // race: created between check and create
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("keycloak: create role returned %d: %s", resp.StatusCode, body)
	}
	return nil
}

// ─── Stub client ──────────────────────────────────────────────────────────────

// StubClient is an in-memory Keycloak client for testing and local development.
// All methods are safe for concurrent use.
type StubClient struct {
	mu    sync.Mutex
	users map[string]*User // keyed by ID
	roles map[string]*Role // keyed by name

	// userRoles maps userID → set of role names.
	userRoles map[string]map[string]struct{}

	// Error overrides for each method (nil = succeed).
	ListUsersErr             error
	GetUserErr               error
	CreateUserErr            error
	SendVerificationEmailErr error
	UpdateUserErr            error
	DeleteUserErr            error
	GetUserRealmRolesErr     error
	AssignRealmRoleErr       error
	RemoveRealmRoleErr       error
	EnsureRealmRolesExistErr error
	SendPasswordResetErr     error

	// Records for assertion.
	invitedEmails      []string
	verificationsSent  []string
	passwordResetsSent []string
	actionsLog         []StubAction
}

// StubAction records a mutating call on the StubClient for test assertions.
type StubAction struct {
	Action string
	UserID string
	Data   any
}

// NewStubClient returns a new StubClient with the default realm roles pre-populated.
func NewStubClient() *StubClient {
	roles := make(map[string]*Role)
	for i, name := range DefaultRealmRoles {
		roles[name] = &Role{
			ID:   fmt.Sprintf("role-id-%d", i+1),
			Name: name,
		}
	}
	return &StubClient{
		users:     make(map[string]*User),
		roles:     roles,
		userRoles: make(map[string]map[string]struct{}),
	}
}

// AddUser inserts a user directly into the stub for test setup.
func (s *StubClient) AddUser(u User) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := u
	s.users[u.ID] = &cp
	if s.userRoles[u.ID] == nil {
		s.userRoles[u.ID] = make(map[string]struct{})
	}
}

// Actions returns a snapshot of all recorded mutating calls.
func (s *StubClient) Actions() []StubAction {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]StubAction, len(s.actionsLog))
	copy(out, s.actionsLog)
	return out
}

func (s *StubClient) ListUsers(_ context.Context, orgID, search string, limit, offset int) ([]User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ListUsersErr != nil {
		return nil, s.ListUsersErr
	}
	if limit <= 0 {
		limit = 50
	}
	var out []User
	for _, u := range s.users {
		if orgID != "" && u.OrgID() != orgID {
			continue
		}
		if search != "" {
			q := strings.ToLower(search)
			if !strings.Contains(strings.ToLower(u.Email), q) &&
				!strings.Contains(strings.ToLower(u.FirstName), q) &&
				!strings.Contains(strings.ToLower(u.LastName), q) {
				continue
			}
		}
		out = append(out, *u)
	}
	// Apply offset and limit.
	if offset >= len(out) {
		return []User{}, nil
	}
	out = out[offset:]
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *StubClient) GetUser(_ context.Context, userID string) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.GetUserErr != nil {
		return nil, s.GetUserErr
	}
	u, ok := s.users[userID]
	if !ok {
		return nil, ErrUserNotFound
	}
	cp := *u
	return &cp, nil
}

func (s *StubClient) CreateUser(_ context.Context, email, firstName, lastName, orgID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.CreateUserErr != nil {
		return "", s.CreateUserErr
	}
	// Check for duplicate email.
	for _, u := range s.users {
		if strings.EqualFold(u.Email, email) {
			return "", ErrConflict
		}
	}
	// Match real Keycloak IDs so development users work with UUID-backed sessions.
	id := uuid.NewString()
	u := &User{
		ID:            id,
		Username:      strings.ToLower(email),
		Email:         email,
		FirstName:     firstName,
		LastName:      lastName,
		Enabled:       true,
		EmailVerified: false,
		Attributes:    map[string][]string{"org_id": {orgID}},
	}
	s.users[id] = u
	s.userRoles[id] = make(map[string]struct{})
	s.invitedEmails = append(s.invitedEmails, email)
	s.actionsLog = append(s.actionsLog, StubAction{Action: "create_user", UserID: id, Data: email})
	return id, nil
}

func (s *StubClient) SendVerificationEmail(_ context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.SendVerificationEmailErr != nil {
		return s.SendVerificationEmailErr
	}
	if _, ok := s.users[userID]; !ok {
		return ErrUserNotFound
	}
	s.verificationsSent = append(s.verificationsSent, userID)
	s.actionsLog = append(s.actionsLog, StubAction{Action: "send_verification_email", UserID: userID})
	return nil
}

func (s *StubClient) UpdateUser(_ context.Context, userID string, fields UpdateUserFields) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.UpdateUserErr != nil {
		return s.UpdateUserErr
	}
	u, ok := s.users[userID]
	if !ok {
		return ErrUserNotFound
	}
	if fields.FirstName != "" {
		u.FirstName = fields.FirstName
	}
	if fields.LastName != "" {
		u.LastName = fields.LastName
	}
	if fields.Email != "" {
		u.Email = fields.Email
		u.Username = strings.ToLower(fields.Email)
	}
	if fields.Enabled != nil {
		u.Enabled = *fields.Enabled
	}
	if len(fields.Attributes) > 0 {
		if u.Attributes == nil {
			u.Attributes = make(map[string][]string)
		}
		for k, v := range fields.Attributes {
			u.Attributes[k] = v
		}
	}
	s.actionsLog = append(s.actionsLog, StubAction{Action: "update_user", UserID: userID, Data: fields})
	return nil
}

func (s *StubClient) DeleteUser(_ context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.DeleteUserErr != nil {
		return s.DeleteUserErr
	}
	if _, ok := s.users[userID]; !ok {
		return ErrUserNotFound
	}
	delete(s.users, userID)
	delete(s.userRoles, userID)
	s.actionsLog = append(s.actionsLog, StubAction{Action: "delete_user", UserID: userID})
	return nil
}

func (s *StubClient) DisableUser(ctx context.Context, userID string) error {
	f := false
	return s.UpdateUser(ctx, userID, UpdateUserFields{Enabled: &f})
}

func (s *StubClient) EnableUser(ctx context.Context, userID string) error {
	t := true
	return s.UpdateUser(ctx, userID, UpdateUserFields{Enabled: &t})
}

func (s *StubClient) ListUsersWithRealmRole(_ context.Context, orgID, roleName string) ([]User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.GetUserRealmRolesErr != nil {
		return nil, s.GetUserRealmRolesErr
	}
	var out []User
	for id, u := range s.users {
		if orgID != "" && u.OrgID() != orgID {
			continue
		}
		if _, ok := s.userRoles[id][roleName]; ok {
			out = append(out, *u)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *StubClient) GetUserRealmRoles(_ context.Context, userID string) ([]Role, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.GetUserRealmRolesErr != nil {
		return nil, s.GetUserRealmRolesErr
	}
	if _, ok := s.users[userID]; !ok {
		return nil, ErrUserNotFound
	}
	var roles []Role
	for name := range s.userRoles[userID] {
		if r, ok := s.roles[name]; ok {
			roles = append(roles, *r)
		}
	}
	return roles, nil
}

func (s *StubClient) AssignRealmRole(_ context.Context, userID, roleName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.AssignRealmRoleErr != nil {
		return s.AssignRealmRoleErr
	}
	if _, ok := s.users[userID]; !ok {
		return ErrUserNotFound
	}
	if _, ok := s.roles[roleName]; !ok {
		return ErrRoleNotFound
	}
	s.userRoles[userID][roleName] = struct{}{}
	s.actionsLog = append(s.actionsLog, StubAction{Action: "assign_role", UserID: userID, Data: roleName})
	return nil
}

func (s *StubClient) RemoveRealmRole(_ context.Context, userID, roleName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.RemoveRealmRoleErr != nil {
		return s.RemoveRealmRoleErr
	}
	if _, ok := s.users[userID]; !ok {
		return ErrUserNotFound
	}
	if _, ok := s.roles[roleName]; !ok {
		return ErrRoleNotFound
	}
	delete(s.userRoles[userID], roleName)
	s.actionsLog = append(s.actionsLog, StubAction{Action: "remove_role", UserID: userID, Data: roleName})
	return nil
}

func (s *StubClient) EnsureRealmRolesExist(_ context.Context, roleNames []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.EnsureRealmRolesExistErr != nil {
		return s.EnsureRealmRolesExistErr
	}
	for i, name := range roleNames {
		if _, ok := s.roles[name]; !ok {
			s.roles[name] = &Role{
				ID:   fmt.Sprintf("stub-role-%d", i+len(s.roles)),
				Name: name,
			}
		}
	}
	return nil
}

func (s *StubClient) SendPasswordResetEmail(_ context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.SendPasswordResetErr != nil {
		return s.SendPasswordResetErr
	}
	if _, ok := s.users[userID]; !ok {
		return ErrUserNotFound
	}
	s.passwordResetsSent = append(s.passwordResetsSent, userID)
	s.actionsLog = append(s.actionsLog, StubAction{Action: "send_password_reset", UserID: userID})
	return nil
}

func (s *StubClient) SetUserPassword(_ context.Context, userID, _ string, _ bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[userID]; !ok {
		return ErrUserNotFound
	}
	s.actionsLog = append(s.actionsLog, StubAction{Action: "set_password", UserID: userID})
	return nil
}

// InvitedEmails returns a snapshot of all emails passed to CreateUser.
func (s *StubClient) InvitedEmails() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.invitedEmails))
	copy(out, s.invitedEmails)
	return out
}

// VerificationsSent returns a snapshot of userIDs that received verification emails.
func (s *StubClient) VerificationsSent() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.verificationsSent))
	copy(out, s.verificationsSent)
	return out
}

// PasswordResetsSent returns a snapshot of userIDs that received password reset emails.
func (s *StubClient) PasswordResetsSent() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.passwordResetsSent))
	copy(out, s.passwordResetsSent)
	return out
}

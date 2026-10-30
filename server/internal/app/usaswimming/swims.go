package usaswimming

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// This file implements USA Swimming's documented Third-Party Vendor API
// (https://www.usaswimming.org/about-usas/resources/stakeholder-engagement/usa-swimming-api).
//
// DOCUMENTED (public program page): tiered access, OAuth client-credentials
// bearer tokens, route paths under /swims/SwimsThirdParty/, camelCase JSON,
// registration-status values and the member-id format (14 alphanumerics).
//
// ASSUMED (only given to approved vendors): token request details (form-encoded
// client_credentials) and the exact response field names decoded below. They are
// decoded tolerantly; adjust once the Postman collection for the tier is in hand.
//
// NOT AVAILABLE: the program has no "list a swimmer's times" endpoint. Tier 3
// (paid) offers TimeVerification, which confirms a claimed time. OfficialTimes
// therefore reports ErrUnsupported on this client.

// ErrUnsupported is returned when the configured tier/API cannot do an operation.
var ErrUnsupported = errors.New("operation is not supported by the USA Swimming vendor API")

// Roster-facing types.
type Club struct {
	ClubID string `json:"club_id"`
	Name   string `json:"name"`
	Code   string `json:"code,omitempty"`
}

// RosterMember is one member on a club's SWIMS roster.
type RosterMember struct {
	MemberID            string `json:"member_id"`
	FirstName           string `json:"first_name"`
	LastName            string `json:"last_name"`
	DateOfBirth         string `json:"date_of_birth,omitempty"` // YYYY-MM-DD
	CompetitionCategory string `json:"competition_category,omitempty"`
	MembershipType      string `json:"membership_type,omitempty"`
	Expires             string `json:"expires,omitempty"`
}

// MemberCheck is the Tier 3 verification result for one member.
type MemberCheck struct {
	MemberID string `json:"member_id"`
	Valid    bool   `json:"valid"`
	Detail   string `json:"detail,omitempty"`
}

// VendorClient is the extra surface of the real vendor API, beyond Client.
type VendorClient interface {
	Client
	Clubs(ctx context.Context) ([]Club, error)
	Roster(ctx context.Context, clubID string, modifiedSince *time.Time) ([]RosterMember, error)
	RegistrationLink(ctx context.Context, clubID string) (string, error)
	// VerifyMembers is Tier 3 only; it returns ErrUnauthorized below that tier.
	VerifyMembers(ctx context.Context, clubID string, memberIDs []string) ([]MemberCheck, error)
	// VerifyTime is Tier 3 only. swimTime is formatted like "00:52.34".
	VerifyTime(ctx context.Context, memberID, swimEvent, swimTime string) (bool, error)
	// SubmitRegistration (Tier 2) submits one member registration/renewal.
	SubmitRegistration(ctx context.Context, clubID string, reg Registration) (RegistrationResult, error)
	// RegistrationStatus (Tier 2) returns Not Found | Payment Pending | Processed | Pending.
	RegistrationStatus(ctx context.Context, requestID string) (string, error)
}

// Registration is the member payload for Tier 2 submission. Field rules follow
// the program page; see ValidateRegistration.
type Registration struct {
	MemberID            string `json:"member_id,omitempty"` // 14 alphanumerics, empty for a new member
	FirstName           string `json:"first_name"`
	LastName            string `json:"last_name"`
	DateOfBirth         string `json:"date_of_birth"`
	CompetitionCategory string `json:"competition_category"` // FEMALE | MALE
	Phone               string `json:"phone,omitempty"`
	PhoneType           string `json:"phone_type,omitempty"` // Mobile|Work|Home|Other
	PostalCode          string `json:"postal_code,omitempty"`
	OfferingID          string `json:"offering_id,omitempty"`
}

// RegistrationResult mirrors the documented submit response.
type RegistrationResult struct {
	Status         string `json:"status"`
	RequestID      string `json:"request_id,omitempty"`
	RegistrationOK bool   `json:"ok"`
	Error          string `json:"error,omitempty"`
}

var (
	memberIDRE = regexp.MustCompile(`^[A-Za-z0-9]{14}$`)
	nameRE     = regexp.MustCompile(`^[\p{L}][\p{L} .'-]{0,99}$`)
	phoneRE    = regexp.MustCompile(`^(1|52)\d{10}$`)
	postalRE   = regexp.MustCompile(`^\d{5}(-?\d{4})?$`)
)

// ValidateRegistration applies the documented field rules so bad payloads fail
// locally instead of consuming vendor-API calls. Returns one message per problem.
func ValidateRegistration(r Registration) []string {
	var errs []string
	if r.MemberID != "" && !memberIDRE.MatchString(r.MemberID) {
		errs = append(errs, "member_id must be 14 letters/digits (or empty for a new member)")
	}
	if !nameRE.MatchString(r.FirstName) || !nameRE.MatchString(r.LastName) {
		errs = append(errs, "names must be 1-100 characters with no digits or special characters")
	}
	if _, err := time.Parse("2006-01-02", r.DateOfBirth); err != nil {
		errs = append(errs, "date_of_birth must be YYYY-MM-DD")
	}
	if c := strings.ToUpper(r.CompetitionCategory); c != "FEMALE" && c != "MALE" {
		errs = append(errs, "competition_category must be FEMALE or MALE")
	}
	if r.Phone != "" && !phoneRE.MatchString(r.Phone) {
		errs = append(errs, "phone must be 1 or 52 followed by 10 digits")
	}
	switch r.PhoneType {
	case "", "Mobile", "Work", "Home", "Other":
	default:
		errs = append(errs, "phone_type must be Mobile, Work, Home or Other")
	}
	if r.PostalCode != "" && !postalRE.MatchString(r.PostalCode) {
		errs = append(errs, "postal_code must be 5 digits with optional 4-digit extension")
	}
	return errs
}

// SwimsConfig configures SwimsClient.
type SwimsConfig struct {
	BaseURL      string
	TokenURL     string
	ClientID     string
	ClientSecret string
	Timeout      time.Duration
	HTTP         *http.Client
	// AllowLoopback permits http and loopback hosts; development/test only.
	AllowLoopback bool
}

// SwimsClient talks to the vendor API with a cached client-credentials token.
type SwimsClient struct {
	cfg SwimsConfig
	hc  *http.Client

	mu      sync.Mutex
	token   string
	expires time.Time
}

// NewSwimsClient validates both URLs and builds the client.
func NewSwimsClient(cfg SwimsConfig) (*SwimsClient, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.TokenURL) == "" || cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, ErrNotConfigured
	}
	for _, u := range []string{cfg.BaseURL, cfg.TokenURL} {
		if err := ValidateBaseURL(u, cfg.AllowLoopback); err != nil {
			return nil, err
		}
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	hc := cfg.HTTP
	if hc == nil {
		hc = newSafeHTTPClient(cfg.Timeout, cfg.AllowLoopback)
	}
	return &SwimsClient{cfg: cfg, hc: hc}, nil
}

func (c *SwimsClient) accessToken(ctx context.Context, force bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !force && c.token != "" && time.Now().Before(c.expires) {
		return c.token, nil
	}
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {c.cfg.ClientID}, "client_secret": {c.cfg.ClientSecret}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return "", errors.New("usa swimming token request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusBadRequest {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return "", ErrUnauthorized
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("usa swimming token endpoint returned HTTP %d", resp.StatusCode)
	}
	var body struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&body); err != nil || body.AccessToken == "" {
		return "", errors.New("usa swimming token endpoint returned an unreadable response")
	}
	ttl := time.Duration(body.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	c.token, c.expires = body.AccessToken, time.Now().Add(ttl-30*time.Second)
	return c.token, nil
}

// call performs an authenticated request, refreshing the token once on 401.
func (c *SwimsClient) call(ctx context.Context, method, path string, query url.Values, in, out any) error {
	for attempt := 0; attempt < 2; attempt++ {
		tok, err := c.accessToken(ctx, attempt > 0)
		if err != nil {
			return err
		}
		var rdr io.Reader
		if in != nil {
			b, _ := json.Marshal(in)
			rdr = bytes.NewReader(b)
		}
		u := strings.TrimRight(c.cfg.BaseURL, "/") + path
		if len(query) > 0 {
			u += "?" + query.Encode()
		}
		ctx2, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
		req, err := http.NewRequestWithContext(ctx2, method, u, rdr)
		if err != nil {
			cancel()
			return err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Accept", "application/json")
		if in != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.hc.Do(req)
		if err != nil {
			cancel()
			return errors.New("usa swimming request failed")
		}
		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
			resp.Body.Close()
			cancel()
			continue
		}
		err = func() error {
			defer cancel()
			defer resp.Body.Close()
			if serr := statusErr(resp); serr != nil {
				_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
				return serr
			}
			if out == nil {
				return nil
			}
			if derr := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(out); derr != nil {
				return errors.New("usa swimming returned an unreadable response")
			}
			return nil
		}()
		return err
	}
	return ErrUnauthorized
}

// TestConnection fetches a token and lists the vendor's clubs.
func (c *SwimsClient) TestConnection(ctx context.Context) error {
	_, err := c.Clubs(ctx)
	return err
}

// OfficialTimes is not offered by the vendor API.
func (c *SwimsClient) OfficialTimes(context.Context, string) ([]OfficialTime, error) {
	return nil, ErrUnsupported
}

func (c *SwimsClient) Clubs(ctx context.Context) ([]Club, error) {
	var raw []struct {
		ClubID   string `json:"clubId"`
		ClubName string `json:"clubName"`
		ClubCode string `json:"clubCode"`
	}
	if err := c.call(ctx, http.MethodGet, "/swims/SwimsThirdParty/VendorClubs/vendor", nil, nil, &raw); err != nil {
		return nil, err
	}
	out := make([]Club, 0, len(raw))
	for _, r := range raw {
		out = append(out, Club{ClubID: r.ClubID, Name: r.ClubName, Code: r.ClubCode})
	}
	return out, nil
}

func (c *SwimsClient) Roster(ctx context.Context, clubID string, since *time.Time) ([]RosterMember, error) {
	body := map[string]any{}
	if since != nil {
		body["lastModified"] = since.UTC().Format(time.RFC3339)
	}
	var raw []struct {
		MemberID    string `json:"memberId"`
		FirstName   string `json:"firstName"`
		LastName    string `json:"lastName"`
		BirthDate   string `json:"birthDate"`
		Category    string `json:"competitionCategory"`
		Membership  string `json:"membershipType"`
		ExpiresDate string `json:"expirationDate"`
	}
	if err := c.call(ctx, http.MethodPost, "/swims/SwimsThirdParty/MemberDetails/club/"+url.PathEscape(clubID), nil, body, &raw); err != nil {
		return nil, err
	}
	out := make([]RosterMember, 0, len(raw))
	for _, r := range raw {
		out = append(out, RosterMember{MemberID: r.MemberID, FirstName: r.FirstName, LastName: r.LastName,
			DateOfBirth: datePart(r.BirthDate), CompetitionCategory: strings.ToUpper(r.Category), MembershipType: r.Membership, Expires: datePart(r.ExpiresDate)})
	}
	return out, nil
}

func (c *SwimsClient) RegistrationLink(ctx context.Context, clubID string) (string, error) {
	var raw struct {
		Link string `json:"registrationLink"`
	}
	if err := c.call(ctx, http.MethodGet, "/swims/SwimsThirdParty/RegistrationLink/club/"+url.PathEscape(clubID), nil, nil, &raw); err != nil {
		return "", err
	}
	return raw.Link, nil
}

func (c *SwimsClient) VerifyMembers(ctx context.Context, clubID string, ids []string) ([]MemberCheck, error) {
	var raw []struct {
		MemberID string `json:"memberId"`
		Valid    bool   `json:"isValid"`
		Message  string `json:"message"`
	}
	in := map[string]any{"clubId": clubID, "memberId": ids}
	if err := c.call(ctx, http.MethodPost, "/swims/SwimsThirdParty/MemberVerification", nil, in, &raw); err != nil {
		return nil, err
	}
	out := make([]MemberCheck, 0, len(raw))
	for _, r := range raw {
		out = append(out, MemberCheck{MemberID: r.MemberID, Valid: r.Valid, Detail: r.Message})
	}
	return out, nil
}

func (c *SwimsClient) VerifyTime(ctx context.Context, memberID, swimEvent, swimTime string) (bool, error) {
	var raw struct {
		Valid bool `json:"isTimeValid"`
	}
	q := url.Values{"memberId": {memberID}, "swimEvent": {swimEvent}, "swimTime": {swimTime}}
	if err := c.call(ctx, http.MethodGet, "/swims/SwimsThirdParty/TimeVerification", q, nil, &raw); err != nil {
		return false, err
	}
	return raw.Valid, nil
}

// datePart trims an ISO timestamp to YYYY-MM-DD.
func datePart(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

func (c *SwimsClient) SubmitRegistration(ctx context.Context, clubID string, reg Registration) (RegistrationResult, error) {
	in := map[string]any{
		"memberId": reg.MemberID, "firstName": reg.FirstName, "lastName": reg.LastName, "birthDate": reg.DateOfBirth,
		"competitionCategory": strings.ToUpper(reg.CompetitionCategory), "phone": reg.Phone, "phoneType": reg.PhoneType,
		"postalCode": reg.PostalCode, "offeringId": reg.OfferingID,
	}
	var raw struct {
		Status    string `json:"status"`
		RequestID string `json:"clubRegistrationRequestId"`
		Error     string `json:"registrationError"`
	}
	if err := c.call(ctx, http.MethodPost, "/swims/SwimsThirdParty/memberRegistration/club/"+url.PathEscape(clubID), nil, in, &raw); err != nil {
		return RegistrationResult{}, err
	}
	return RegistrationResult{Status: raw.Status, RequestID: raw.RequestID, RegistrationOK: raw.Error == "", Error: raw.Error}, nil
}

func (c *SwimsClient) RegistrationStatus(ctx context.Context, requestID string) (string, error) {
	var raw struct {
		Status string `json:"status"`
	}
	if err := c.call(ctx, http.MethodGet, "/swims/SwimsThirdParty/memberRegistration/status/"+url.PathEscape(requestID), nil, nil, &raw); err != nil {
		return "", err
	}
	return raw.Status, nil
}

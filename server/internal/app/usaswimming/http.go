package usaswimming

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// ClientConfig configures HTTPClient. Zero values fall back to defaults.
type ClientConfig struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration // default 10s
	// PingPath and TimesPath are ASSUMED defaults (the real SWIMS contract is
	// unknown). TimesPath may contain "{id}".
	PingPath  string // default "/health"
	TimesPath string // default "/swimmers/{id}/times"
	HTTP      *http.Client
	// AllowLoopback permits http and loopback hosts; development/test only.
	AllowLoopback bool
}

// HTTPClient talks to a SWIMS-compatible JSON API with a bearer API key.
// ASSUMED response for the times endpoint:
//
//	{"times":[{"distance":100,"stroke":"free","course":"SCY","time_ms":58120,
//	           "swim_date":"2026-01-17","meet_name":"..."}]}
//
// 404 means unknown swimmer; 401/403 means bad credentials.
type HTTPClient struct {
	cfg ClientConfig
	hc  *http.Client
}

const maxResponseBytes = 4 << 20

// ValidateBaseURL requires an absolute https URL with no credentials, query or
// fragment. Loopback hosts (and plain http) are accepted only when allowLoopback
// is set, which is limited to development/test so local mock servers work.
// Private and link-local literals are always rejected; hostnames are checked
// again at connect time by safeDialControl.
func ValidateBaseURL(raw string, allowLoopback bool) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("base URL must be an absolute URL without credentials, query or fragment")
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	isLoopback := strings.EqualFold(host, "localhost") || (ip != nil && ip.IsLoopback())
	if ip != nil {
		if blockedIP(ip, allowLoopback) {
			return errors.New("base URL must not point at a private or loopback address")
		}
	} else if strings.EqualFold(host, "localhost") && !allowLoopback {
		return errors.New("base URL must not point at a private or loopback address")
	}
	if u.Scheme != "https" && !(allowLoopback && isLoopback && u.Scheme == "http") {
		return errors.New("base URL must use https")
	}
	return nil
}

// blockedIP reports addresses an outbound integration call must never reach.
func blockedIP(ip net.IP, allowLoopback bool) bool {
	if ip.IsLoopback() {
		return !allowLoopback
	}
	return ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast()
}

// safeDialControl rejects connections to blocked addresses after DNS resolution,
// so a hostname that resolves to an internal IP cannot be used for SSRF.
func safeDialControl(allowLoopback bool) func(network, address string, c syscall.RawConn) error {
	return func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		if ip := net.ParseIP(host); ip == nil || blockedIP(ip, allowLoopback) {
			return errors.New("connection to a private or loopback address is not allowed")
		}
		return nil
	}
}

// newSafeHTTPClient is the outbound client every integration call uses: no
// redirects, and connections to private/loopback addresses are refused.
func newSafeHTTPClient(timeout time.Duration, allowLoopback bool) *http.Client {
	return &http.Client{
		Timeout:       timeout,
		Transport:     &http.Transport{DialContext: (&net.Dialer{Timeout: 5 * time.Second, Control: safeDialControl(allowLoopback)}).DialContext},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// NewHTTPClient builds a client; it validates the base URL.
func NewHTTPClient(cfg ClientConfig) (*HTTPClient, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" || cfg.APIKey == "" {
		return nil, ErrNotConfigured
	}
	if err := ValidateBaseURL(cfg.BaseURL, cfg.AllowLoopback); err != nil {
		return nil, err
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.PingPath == "" {
		cfg.PingPath = "/health"
	}
	if cfg.TimesPath == "" {
		cfg.TimesPath = "/swimmers/{id}/times"
	}
	hc := cfg.HTTP
	if hc == nil {
		hc = newSafeHTTPClient(cfg.Timeout, cfg.AllowLoopback)
	}
	return &HTTPClient{cfg: cfg, hc: hc}, nil
}

func (c *HTTPClient) do(ctx context.Context, path string) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	// cancel is tied to the body via closeBody below.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.cfg.BaseURL, "/")+path, nil)
	if err != nil {
		cancel()
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("Accept", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		cancel()
		// url.Error embeds the URL, not the key; still, report only the cause class.
		var ue *url.Error
		if errors.As(err, &ue) {
			return nil, fmt.Errorf("usa swimming request failed: %w", ue.Err)
		}
		return nil, errors.New("usa swimming request failed")
	}
	resp.Body = &cancelBody{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

type cancelBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *cancelBody) Close() error { err := b.ReadCloser.Close(); b.cancel(); return err }

func statusErr(resp *http.Response) error {
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return ErrUnauthorized
	case resp.StatusCode == http.StatusNotFound:
		return ErrSwimmerNotFound
	case resp.StatusCode >= 300:
		return fmt.Errorf("usa swimming returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// TestConnection GETs PingPath.
func (c *HTTPClient) TestConnection(ctx context.Context) error {
	resp, err := c.do(ctx, c.cfg.PingPath)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode == http.StatusNotFound {
		return errors.New("usa swimming returned HTTP 404 for the health path; check the base URL")
	}
	return statusErr(resp)
}

// OfficialTimes GETs TimesPath for the swimmer.
func (c *HTTPClient) OfficialTimes(ctx context.Context, id string) ([]OfficialTime, error) {
	resp, err := c.do(ctx, strings.ReplaceAll(c.cfg.TimesPath, "{id}", url.PathEscape(id)))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := statusErr(resp); err != nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return nil, err
	}
	var body struct {
		Times []struct {
			Distance int    `json:"distance"`
			Stroke   string `json:"stroke"`
			Course   string `json:"course"`
			TimeMs   int    `json:"time_ms"`
			SwimDate string `json:"swim_date"`
			MeetName string `json:"meet_name"`
		} `json:"times"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&body); err != nil {
		return nil, errors.New("usa swimming returned an unreadable response")
	}
	out := make([]OfficialTime, 0, len(body.Times))
	for _, t := range body.Times {
		d, err := time.Parse("2006-01-02", t.SwimDate)
		if err != nil {
			// Leave SwimDate zero: the sync handler rejects (and counts) rows
			// without a valid swim date instead of recording them undated.
			d = time.Time{}
			slog.Warn("usa swimming: time with unparseable swim_date", "swim_date", t.SwimDate)
		}
		out = append(out, OfficialTime{Distance: t.Distance, Stroke: strings.ToLower(t.Stroke), Course: strings.ToUpper(t.Course), TimeMs: t.TimeMs, SwimDate: d, MeetName: t.MeetName})
	}
	return out, nil
}

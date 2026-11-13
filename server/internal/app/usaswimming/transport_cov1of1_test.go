package usaswimming

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type cov1xRT func(*http.Request) (*http.Response, error)

func (f cov1xRT) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func cov1xFailingHTTP() *http.Client {
	return &http.Client{Transport: cov1xRT(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("boom")
	})}
}

func TestCov1xSafeDialControl(t *testing.T) {
	cases := []struct {
		addr     string
		loopback bool
		wantErr  bool
	}{
		{"nohostport", false, true},
		{"example.com:80", false, true}, // hostname, not an IP literal after resolution
		{"127.0.0.1:80", false, true},
		{"127.0.0.1:80", true, false},
		{"10.0.0.1:80", true, true},
		{"169.254.169.254:80", false, true},
		{"8.8.8.8:443", false, false},
	}
	for _, c := range cases {
		err := safeDialControl(c.loopback)("tcp", c.addr, nil)
		if (err != nil) != c.wantErr {
			t.Errorf("safeDialControl(%q, %v) err=%v wantErr=%v", c.addr, c.loopback, err, c.wantErr)
		}
	}
}

func TestCov1xSafeHTTPClient(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Redirect(w, r, "/elsewhere", http.StatusFound)
	}))
	defer srv.Close()

	blocked := newSafeHTTPClient(time.Second, false)
	if _, err := blocked.Get(srv.URL); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("loopback dial should be refused, got %v", err)
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Fatal("server must not be reached when dial is refused")
	}

	allowed := newSafeHTTPClient(time.Second, true)
	resp, err := allowed.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound || atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("redirects must not be followed: status=%d hits=%d", resp.StatusCode, hits)
	}
}

func TestCov1xNewHTTPClientValidation(t *testing.T) {
	if _, err := NewHTTPClient(ClientConfig{BaseURL: "", APIKey: "k"}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("empty base: %v", err)
	}
	if _, err := NewHTTPClient(ClientConfig{BaseURL: "https://x.example.com"}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("empty key: %v", err)
	}
	if _, err := NewHTTPClient(ClientConfig{BaseURL: "http://x.example.com", APIKey: "k"}); err == nil {
		t.Fatal("http base should be rejected")
	}
	c, err := NewHTTPClient(ClientConfig{BaseURL: "https://x.example.com", APIKey: "k"})
	if err != nil {
		t.Fatal(err)
	}
	if c.cfg.Timeout != 10*time.Second || c.cfg.PingPath != "/health" || c.cfg.TimesPath != "/swimmers/{id}/times" || c.hc == nil {
		t.Fatalf("defaults not applied: %+v", c.cfg)
	}
	custom := &http.Client{}
	c2, err := NewHTTPClient(ClientConfig{BaseURL: "https://x.example.com", APIKey: "k", Timeout: time.Second, HTTP: custom})
	if err != nil || c2.hc != custom || c2.cfg.Timeout != time.Second {
		t.Fatalf("custom client not kept: %v", err)
	}
}

func TestCov1xHTTPClientErrors(t *testing.T) {
	ctx := context.Background()
	mode := "500"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch mode {
		case "500":
			w.WriteHeader(500)
		case "404":
			w.WriteHeader(404)
		case "garbage":
			_, _ = w.Write([]byte("not json"))
		}
	}))
	defer srv.Close()
	c, err := NewHTTPClient(ClientConfig{BaseURL: srv.URL, APIKey: "k", AllowLoopback: true,
		PingPath: "/ping", TimesPath: "/t/{id}"})
	if err != nil {
		t.Fatal(err)
	}

	if err := c.TestConnection(ctx); err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("ping 500: %v", err)
	}
	if _, err := c.OfficialTimes(ctx, "A1"); err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("times 500: %v", err)
	}
	mode = "404"
	if err := c.TestConnection(ctx); err == nil || !strings.Contains(err.Error(), "health path") {
		t.Fatalf("ping 404: %v", err)
	}
	mode = "garbage"
	if _, err := c.OfficialTimes(ctx, "A1"); err == nil || !strings.Contains(err.Error(), "unreadable") {
		t.Fatalf("garbage: %v", err)
	}

	// Transport failure: cause class reported, key never leaked.
	failing := &HTTPClient{cfg: ClientConfig{BaseURL: "https://x.example.com", APIKey: "topsecret", Timeout: time.Second, PingPath: "/p", TimesPath: "/t/{id}"}, hc: cov1xFailingHTTP()}
	err = failing.TestConnection(ctx)
	if err == nil || !strings.Contains(err.Error(), "usa swimming request failed") || !strings.Contains(err.Error(), "boom") || strings.Contains(err.Error(), "topsecret") {
		t.Fatalf("transport failure: %v", err)
	}
	if _, err := failing.OfficialTimes(ctx, "A1"); err == nil {
		t.Fatal("OfficialTimes should surface transport failure")
	}

	// Request construction failure (invalid URL).
	badURL := &HTTPClient{cfg: ClientConfig{BaseURL: "http://a\x7fb", APIKey: "k", Timeout: time.Second}, hc: http.DefaultClient}
	if _, err := badURL.do(ctx, "/x"); err == nil {
		t.Fatal("invalid URL should fail request construction")
	}
}

func TestCov1xStatusErr(t *testing.T) {
	cases := map[int]error{401: ErrUnauthorized, 403: ErrUnauthorized, 404: ErrSwimmerNotFound, 200: nil, 204: nil}
	for code, want := range cases {
		if got := statusErr(&http.Response{StatusCode: code}); got != want {
			t.Errorf("statusErr(%d) = %v want %v", code, got, want)
		}
	}
	if err := statusErr(&http.Response{StatusCode: 302}); err == nil || !strings.Contains(err.Error(), "302") {
		t.Fatalf("302: %v", err)
	}
}

func TestCov1xStubClient(t *testing.T) {
	ctx := context.Background()
	s := NewStubClient()
	if err := s.TestConnection(ctx); err != nil {
		t.Fatal(err)
	}
	s.PingErr = errors.New("down")
	if err := s.TestConnection(ctx); err == nil || err.Error() != "down" {
		t.Fatalf("ping err: %v", err)
	}
	ts, err := s.OfficialTimes(ctx, "STUB0001")
	if err != nil || len(ts) != 2 || ts[0].TimeMs != 58120 {
		t.Fatalf("%+v %v", ts, err)
	}
	ts[0].TimeMs = 1 // returned slice is a copy
	again, _ := s.OfficialTimes(ctx, "STUB0001")
	if again[0].TimeMs != 58120 {
		t.Fatal("stub data was mutated through the returned slice")
	}
	if _, err := s.OfficialTimes(ctx, "NOPE"); !errors.Is(err, ErrSwimmerNotFound) {
		t.Fatalf("unknown id: %v", err)
	}
}

// cov1xSwims is a configurable vendor-API fake.
type cov1xSwims struct {
	srv       *httptest.Server
	tokens    int32
	expiresIn int
	tokenCode int
	tokenBody string
	apiCode   func(call int32) int // status per API call (1-based)
	apiBody   string
	apiCalls  int32
	lastAuth  atomic.Value
	lastPath  atomic.Value
	lastBody  atomic.Value
}

func cov1xNewSwims(t *testing.T) *cov1xSwims {
	t.Helper()
	f := &cov1xSwims{expiresIn: 3600, tokenCode: 200, apiCode: func(int32) int { return 200 }, apiBody: `{}`}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			n := atomic.AddInt32(&f.tokens, 1)
			if f.tokenCode != 200 {
				w.WriteHeader(f.tokenCode)
				return
			}
			if f.tokenBody != "" {
				_, _ = w.Write([]byte(f.tokenBody))
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"tok` + string(rune('0'+n)) + `","expires_in":` + itoa(f.expiresIn) + `}`))
			return
		}
		n := atomic.AddInt32(&f.apiCalls, 1)
		f.lastAuth.Store(r.Header.Get("Authorization"))
		f.lastPath.Store(r.URL.RequestURI())
		b, _ := io.ReadAll(r.Body)
		f.lastBody.Store(string(b))
		if code := f.apiCode(n); code != 200 {
			w.WriteHeader(code)
			return
		}
		_, _ = w.Write([]byte(f.apiBody))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

func (f *cov1xSwims) client(t *testing.T) *SwimsClient {
	t.Helper()
	c, err := NewSwimsClient(SwimsConfig{BaseURL: f.srv.URL + "/", TokenURL: f.srv.URL + "/token", ClientID: "id", ClientSecret: "s", AllowLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCov1xNewSwimsClientValidation(t *testing.T) {
	base := SwimsConfig{BaseURL: "https://a.example.com", TokenURL: "https://t.example.com", ClientID: "i", ClientSecret: "s"}
	for name, mut := range map[string]func(*SwimsConfig){
		"base": func(c *SwimsConfig) { c.BaseURL = " " },
		"tok":  func(c *SwimsConfig) { c.TokenURL = "" },
		"id":   func(c *SwimsConfig) { c.ClientID = "" },
		"sec":  func(c *SwimsConfig) { c.ClientSecret = "" },
	} {
		cfg := base
		mut(&cfg)
		if _, err := NewSwimsClient(cfg); !errors.Is(err, ErrNotConfigured) {
			t.Errorf("%s: %v", name, err)
		}
	}
	cfg := base
	cfg.TokenURL = "http://t.example.com"
	if _, err := NewSwimsClient(cfg); err == nil {
		t.Fatal("insecure token URL should be rejected")
	}
	c, err := NewSwimsClient(base)
	if err != nil || c.cfg.Timeout != 15*time.Second || c.hc == nil {
		t.Fatalf("defaults: %v", err)
	}
	custom := &http.Client{}
	cfg = base
	cfg.HTTP, cfg.Timeout = custom, time.Second
	c, _ = NewSwimsClient(cfg)
	if c.hc != custom || c.cfg.Timeout != time.Second {
		t.Fatal("custom http client / timeout not kept")
	}
}

func TestCov1xAccessToken(t *testing.T) {
	ctx := context.Background()
	f := cov1xNewSwims(t)
	c := f.client(t)

	tok, err := c.accessToken(ctx, false)
	if err != nil || tok != "tok1" {
		t.Fatalf("%q %v", tok, err)
	}
	if tok, _ = c.accessToken(ctx, false); tok != "tok1" || f.tokens != 1 {
		t.Fatalf("cache miss: %q tokens=%d", tok, f.tokens)
	}
	if tok, _ = c.accessToken(ctx, true); tok != "tok2" {
		t.Fatalf("force refresh: %q", tok)
	}
	c.expires = time.Now().Add(-time.Second)
	if tok, _ = c.accessToken(ctx, false); tok != "tok3" {
		t.Fatalf("expired refresh: %q", tok)
	}

	// expires_in <= 0 falls back to a 5 minute ttl.
	f.expiresIn = 0
	if _, err := c.accessToken(ctx, true); err != nil {
		t.Fatal(err)
	}
	if d := time.Until(c.expires); d < 4*time.Minute || d > 5*time.Minute {
		t.Fatalf("default ttl wrong: %v", d)
	}

	for _, code := range []int{400, 401, 403} {
		f.tokenCode = code
		if _, err := c.accessToken(ctx, true); !errors.Is(err, ErrUnauthorized) {
			t.Errorf("token %d: %v", code, err)
		}
	}
	f.tokenCode = 502
	if _, err := c.accessToken(ctx, true); err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Fatalf("502: %v", err)
	}
	f.tokenCode = 200
	for _, body := range []string{"nope", `{"access_token":""}`} {
		f.tokenBody = body
		if _, err := c.accessToken(ctx, true); err == nil || !strings.Contains(err.Error(), "unreadable") {
			t.Errorf("body %q: %v", body, err)
		}
	}

	// Transport failure and request-construction failure.
	failing := &SwimsClient{cfg: SwimsConfig{TokenURL: "https://t.example.com/token", ClientID: "i", ClientSecret: "s"}, hc: cov1xFailingHTTP()}
	if _, err := failing.accessToken(ctx, true); err == nil || !strings.Contains(err.Error(), "token request failed") {
		t.Fatalf("transport: %v", err)
	}
	badURL := &SwimsClient{cfg: SwimsConfig{TokenURL: "http://a\x7fb"}, hc: http.DefaultClient}
	if _, err := badURL.accessToken(ctx, true); err == nil {
		t.Fatal("bad token URL should fail")
	}
}

func TestCov1xCallRetriesOnceOn401(t *testing.T) {
	ctx := context.Background()
	f := cov1xNewSwims(t)
	f.apiCode = func(n int32) int {
		if n == 1 {
			return 401
		}
		return 200
	}
	f.apiBody = `[{"clubId":"c1","clubName":"N","clubCode":"C"}]`
	c := f.client(t)
	clubs, err := c.Clubs(ctx)
	if err != nil || len(clubs) != 1 || clubs[0] != (Club{ClubID: "c1", Name: "N", Code: "C"}) {
		t.Fatalf("%+v %v", clubs, err)
	}
	if f.tokens != 2 || f.apiCalls != 2 {
		t.Fatalf("expected token refresh and one retry: tokens=%d calls=%d", f.tokens, f.apiCalls)
	}
	if f.lastAuth.Load() != "Bearer tok2" {
		t.Fatalf("retry must use refreshed token, got %v", f.lastAuth.Load())
	}

	// Persistent 401 surfaces ErrUnauthorized after exactly one retry.
	f.apiCode = func(int32) int { return 401 }
	atomic.StoreInt32(&f.apiCalls, 0)
	if _, err := c.Clubs(ctx); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("persistent 401: %v", err)
	}
	if f.apiCalls != 2 {
		t.Fatalf("calls=%d", f.apiCalls)
	}
	if err := c.TestConnection(ctx); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("TestConnection: %v", err)
	}
}

func TestCov1xCallFailures(t *testing.T) {
	ctx := context.Background()
	f := cov1xNewSwims(t)
	c := f.client(t)

	// Token failure propagates.
	f.tokenCode = 401
	if err := c.call(ctx, http.MethodGet, "/x", nil, nil, nil); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("token failure: %v", err)
	}
	f.tokenCode = 200

	// out == nil: success with no decode.
	if err := c.call(ctx, http.MethodGet, "/x", nil, nil, nil); err != nil {
		t.Fatalf("nil out: %v", err)
	}
	// Non-2xx mapped through statusErr.
	f.apiCode = func(int32) int { return 404 }
	if err := c.call(ctx, http.MethodGet, "/x", nil, nil, nil); !errors.Is(err, ErrSwimmerNotFound) {
		t.Fatalf("404: %v", err)
	}
	f.apiCode = func(int32) int { return 500 }
	if err := c.call(ctx, http.MethodGet, "/x", nil, nil, nil); err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("500: %v", err)
	}
	// Unreadable body.
	f.apiCode = func(int32) int { return 200 }
	f.apiBody = "not json"
	var out map[string]any
	if err := c.call(ctx, http.MethodGet, "/x", nil, nil, &out); err == nil || !strings.Contains(err.Error(), "unreadable") {
		t.Fatalf("bad json: %v", err)
	}

	// Request construction failure.
	badURL := &SwimsClient{cfg: SwimsConfig{BaseURL: "http://a\x7fb", Timeout: time.Second}, hc: http.DefaultClient, token: "t", expires: time.Now().Add(time.Hour)}
	if err := badURL.call(ctx, http.MethodGet, "/x", nil, nil, nil); err == nil {
		t.Fatal("invalid base URL should fail")
	}
	// Transport failure on the API call (token cached).
	failing := &SwimsClient{cfg: SwimsConfig{BaseURL: "https://x.example.com", Timeout: time.Second}, hc: cov1xFailingHTTP(), token: "t", expires: time.Now().Add(time.Hour)}
	if err := failing.call(ctx, http.MethodGet, "/x", nil, nil, nil); err == nil || !strings.Contains(err.Error(), "request failed") {
		t.Fatalf("transport: %v", err)
	}
}

func TestCov1xVendorEndpoints(t *testing.T) {
	ctx := context.Background()
	f := cov1xNewSwims(t)
	c := f.client(t)
	since := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

	f.apiBody = `[{"memberId":"M1","firstName":"A","lastName":"B","birthDate":"2012-04-05T00:00:00","competitionCategory":"male","membershipType":"Full","expirationDate":"2027-08-31T00:00:00"},{"memberId":"M2","birthDate":"x"}]`
	roster, err := c.Roster(ctx, "club/1", &since)
	if err != nil || len(roster) != 2 {
		t.Fatalf("%+v %v", roster, err)
	}
	if roster[0] != (RosterMember{MemberID: "M1", FirstName: "A", LastName: "B", DateOfBirth: "2012-04-05", CompetitionCategory: "MALE", MembershipType: "Full", Expires: "2027-08-31"}) {
		t.Fatalf("roster[0]=%+v", roster[0])
	}
	if roster[1].DateOfBirth != "x" {
		t.Fatalf("short dates are passed through: %+v", roster[1])
	}
	if f.lastPath.Load() != "/swims/SwimsThirdParty/MemberDetails/club/club%2F1" || !strings.Contains(f.lastBody.Load().(string), `"lastModified":"2026-03-04T05:06:07Z"`) {
		t.Fatalf("roster request: %v %v", f.lastPath.Load(), f.lastBody.Load())
	}
	f.apiCode = func(int32) int { return 500 }
	if _, err := c.Roster(ctx, "c1", nil); err == nil {
		t.Fatal("roster 500 should fail")
	}
	f.apiCode = func(int32) int { return 200 }

	f.apiBody = `{"registrationLink":"https://join.example.com/x"}`
	link, err := c.RegistrationLink(ctx, "c1")
	if err != nil || link != "https://join.example.com/x" {
		t.Fatalf("%q %v", link, err)
	}

	f.apiBody = `[{"memberId":"M1","isValid":true,"message":"ok"},{"memberId":"M2","isValid":false,"message":"expired"}]`
	checks, err := c.VerifyMembers(ctx, "c1", []string{"M1", "M2"})
	if err != nil || len(checks) != 2 || !checks[0].Valid || checks[1].Valid || checks[1].Detail != "expired" {
		t.Fatalf("%+v %v", checks, err)
	}
	if !strings.Contains(f.lastBody.Load().(string), `"memberId":["M1","M2"]`) {
		t.Fatalf("body %v", f.lastBody.Load())
	}

	f.apiBody = `{"isTimeValid":true}`
	ok, err := c.VerifyTime(ctx, "M1", "100 Free", "00:52.34")
	if err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	if p := f.lastPath.Load().(string); !strings.Contains(p, "memberId=M1") || !strings.Contains(p, "swimTime=00%3A52.34") {
		t.Fatalf("query %v", p)
	}

	f.apiBody = `{"status":"Processed","clubRegistrationRequestId":"R1"}`
	res, err := c.SubmitRegistration(ctx, "c1", Registration{FirstName: "A", LastName: "B", DateOfBirth: "2012-04-05", CompetitionCategory: "male"})
	if err != nil || res != (RegistrationResult{Status: "Processed", RequestID: "R1", RegistrationOK: true}) {
		t.Fatalf("%+v %v", res, err)
	}
	if !strings.Contains(f.lastBody.Load().(string), `"competitionCategory":"MALE"`) {
		t.Fatalf("category should be upper-cased: %v", f.lastBody.Load())
	}
	f.apiBody = `{"status":"Pending","registrationError":"dup"}`
	if res, _ = c.SubmitRegistration(ctx, "c1", Registration{}); res.RegistrationOK || res.Error != "dup" {
		t.Fatalf("%+v", res)
	}

	f.apiBody = `{"status":"Payment Pending"}`
	st, err := c.RegistrationStatus(ctx, "R 1")
	if err != nil || st != "Payment Pending" || f.lastPath.Load() != "/swims/SwimsThirdParty/memberRegistration/status/R%201" {
		t.Fatalf("%q %v %v", st, err, f.lastPath.Load())
	}

	// Every endpoint propagates upstream failure.
	f.apiCode = func(int32) int { return 500 }
	if _, err := c.RegistrationLink(ctx, "c1"); err == nil {
		t.Error("RegistrationLink should fail")
	}
	if _, err := c.VerifyMembers(ctx, "c1", nil); err == nil {
		t.Error("VerifyMembers should fail")
	}
	if ok, err := c.VerifyTime(ctx, "M", "e", "t"); err == nil || ok {
		t.Error("VerifyTime should fail")
	}
	if _, err := c.SubmitRegistration(ctx, "c1", Registration{}); err == nil {
		t.Error("SubmitRegistration should fail")
	}
	if _, err := c.RegistrationStatus(ctx, "R"); err == nil {
		t.Error("RegistrationStatus should fail")
	}
	f.apiCode = func(int32) int { return 403 }
	if _, err := c.VerifyMembers(ctx, "c1", nil); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("tier gate: %v", err)
	}
}

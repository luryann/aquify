package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/app/billing"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/payments"
	stripelib "github.com/luryann/aquify/server/internal/stripe"
	"github.com/luryann/aquify/server/internal/telemetry"
	"github.com/luryann/aquify/server/internal/testutil"
)

const (
	cvg4Org      = "7a1c0000-0000-4000-8000-0000000000c4"
	cvg4ReturnOK = "https://app.example.test"
)

// cvg4Client wraps the in-memory Stripe stub so tests can record the exact
// parameters the handlers send and inject signature/balance failures.
type cvg4Client struct {
	*stripelib.StubClient
	mu       sync.Mutex
	intents  []stripelib.PaymentIntentParams
	sessions []stripelib.CheckoutSessionParams
	parse    func(payload []byte, sig, secret string) (*stripelib.WebhookEventData, error)
	balErr   error
	acctErr  error
	onAcct   func()
}

func (c *cvg4Client) CreatePaymentIntent(ctx context.Context, p stripelib.PaymentIntentParams) (*stripelib.PaymentIntentResult, error) {
	c.mu.Lock()
	c.intents = append(c.intents, p)
	c.mu.Unlock()
	return c.StubClient.CreatePaymentIntent(ctx, p)
}

func (c *cvg4Client) CreateCheckoutSession(ctx context.Context, p stripelib.CheckoutSessionParams) (*stripelib.CheckoutSessionResult, error) {
	c.mu.Lock()
	c.sessions = append(c.sessions, p)
	c.mu.Unlock()
	return c.StubClient.CreateCheckoutSession(ctx, p)
}

func (c *cvg4Client) ParseWebhookEvent(payload []byte, sig, secret string) (*stripelib.WebhookEventData, error) {
	if c.parse != nil {
		return c.parse(payload, sig, secret)
	}
	return c.StubClient.ParseWebhookEvent(payload, sig, secret)
}

func (c *cvg4Client) FetchBalanceCents(ctx context.Context, account string) (int, error) {
	if c.balErr != nil {
		return 0, c.balErr
	}
	return c.StubClient.FetchBalanceCents(ctx, account)
}

func (c *cvg4Client) GetConnectedAccount(ctx context.Context, id string) (*stripelib.ConnectedAccount, error) {
	if c.onAcct != nil {
		c.onAcct()
	}
	if c.acctErr != nil {
		return nil, c.acctErr
	}
	return c.StubClient.GetConnectedAccount(ctx, id)
}

func (c *cvg4Client) lastIntent(t *testing.T) stripelib.PaymentIntentParams {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.intents) == 0 {
		t.Fatal("no payment intent was created")
	}
	return c.intents[len(c.intents)-1]
}

func (c *cvg4Client) lastSession(t *testing.T) stripelib.CheckoutSessionParams {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.sessions) == 0 {
		t.Fatal("no checkout session was created")
	}
	return c.sessions[len(c.sessions)-1]
}

func (c *cvg4Client) created() (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.intents), len(c.sessions)
}

func cvg4Tokens() tokenResolver {
	mk := func(id, hh string, roles ...string) *appmiddleware.Principal {
		return &appmiddleware.Principal{UserID: id, OrgID: cvg4Org, HouseholdID: hh, Roles: roles}
	}
	return tokenResolver{
		"admin":   mk("u-admin", "", "admin"),
		"coach":   mk("u-coach", "", "coach"),
		"parent":  mk("u-parent", "hh-1", "parent"),
		"parent2": mk("u-parent2", "hh-2", "parent"),
		"swimmer": mk("u-swim", "hh-1", "swimmer"),
	}
}

type cvg4Env struct {
	t      *testing.T
	svc    *billing.Service
	client *cvg4Client
	accts  *payments.Service
	h      *StripeHandlers
	router http.Handler
	bare   http.Handler
}

func cvg4NewEnv(t *testing.T, svc *billing.Service, opts ...func(*cvg4Env)) *cvg4Env {
	t.Helper()
	e := &cvg4Env{t: t, svc: svc, client: &cvg4Client{StubClient: stripelib.NewStubClient()}}
	e.h = NewStripeHandlers(e.client, svc, "whsec_primary", "", 100, slog.New(slog.NewTextHandler(io.Discard, nil)))
	e.h.SetAllowedReturnOrigins([]string{cvg4ReturnOK})
	for _, o := range opts {
		o(e)
	}
	e.build()
	return e
}

func (e *cvg4Env) build() {
	r := chi.NewRouter()
	e.h.RegisterPublicRoutes(r)
	r.Group(func(p chi.Router) {
		p.Use(appmiddleware.Auth(cvg4Tokens()))
		p.Use(appmiddleware.RequireOrgScope)
		e.h.RegisterProtectedRoutes(p)
	})
	e.router = r
	bare := chi.NewRouter()
	e.h.RegisterProtectedRoutes(bare)
	e.bare = bare
}

func (e *cvg4Env) do(method, path, token, body string, hdr map[string]string) *httptest.ResponseRecorder {
	e.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Org-Id", cvg4Org)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	e.router.ServeHTTP(rr, req)
	return rr
}

func (e *cvg4Env) idem(key string) map[string]string {
	return map[string]string{"Idempotency-Key": key}
}

func cvg4ErrCode(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode %q: %v", rr.Body.String(), err)
	}
	return env.Error.Code
}

func cvg4WantErr(t *testing.T, rr *httptest.ResponseRecorder, status int, code, label string) {
	t.Helper()
	if rr.Code != status || cvg4ErrCode(t, rr) != code {
		t.Fatalf("%s: want %d %s, got %d %s", label, status, code, rr.Code, rr.Body.String())
	}
}

// cvg4Invoice creates an invoice for hh with one line item; issued invoices are payable.
func cvg4Invoice(t *testing.T, svc *billing.Service, hh, number, notes string, cents int, issue bool) *billing.Invoice {
	t.Helper()
	admin := &appmiddleware.Principal{UserID: "u-admin", OrgID: cvg4Org, Roles: []string{"admin"}}
	inv, err := svc.CreateInvoice(cvg4Org, "u-admin", admin, billing.CreateInvoiceInput{HouseholdID: hh, InvoiceNumber: number, Notes: notes})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddInvoiceItem(cvg4Org, "u-admin", admin, inv.ID, billing.AddInvoiceItemInput{Description: "Dues", Quantity: 1, UnitAmountCents: cents, Category: "program_fee"}); err != nil {
		t.Fatal(err)
	}
	if issue {
		if _, _, err := svc.IssueInvoice(cvg4Org, "u-admin", admin, inv.ID); err != nil {
			t.Fatal(err)
		}
	}
	got, err := svc.GetInvoice(cvg4Org, admin, inv.ID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func cvg4Event(id, typ, account string, created int64, obj map[string]any) []byte {
	payload := map[string]any{"id": id, "type": typ, "created": created, "data": map[string]any{"object": obj}}
	if account != "" {
		payload["account"] = account
	}
	b, _ := json.Marshal(payload)
	return b
}

func (e *cvg4Env) webhook(payload []byte) *httptest.ResponseRecorder {
	e.t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(payload))
	req.Header.Set("Stripe-Signature", "t=1,v1=sig")
	rr := httptest.NewRecorder()
	e.router.ServeHTTP(rr, req)
	return rr
}

func cvg4Status(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	var out struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %q: %v", rr.Body.String(), err)
	}
	return out.Status
}

// storedEvent returns the persisted webhook record. Events that could not be
// attributed to an org are stored without one and are invisible to a club's
// admin listing, so the service is queried across orgs.
func (e *cvg4Env) storedEvent(eventID string) billing.WebhookEventRecord {
	e.t.Helper()
	admin := &appmiddleware.Principal{UserID: "u-admin", OrgID: cvg4Org, Roles: []string{"admin"}}
	events, err := e.svc.ListWebhookEvents("", admin)
	if err != nil {
		e.t.Fatal(err)
	}
	for _, ev := range events {
		if ev.EventID == eventID {
			return *ev
		}
	}
	e.t.Fatalf("event %s was not persisted", eventID)
	return billing.WebhookEventRecord{}
}

// ─── Pure helpers ─────────────────────────────────────────────────────────────

func TestCvg4StripeConstructorDefaults(t *testing.T) {
	t.Parallel()
	h := NewStripeHandlers(stripelib.NewStubClient(), billing.NewService(), "whsec", "", 5, nil)
	if h.logger == nil || h.currency != "usd" || h.reconcileAlertThresholdCents != 5 || h.webhookSecret != "whsec" || h.validate == nil {
		t.Fatalf("unexpected defaults: %+v", h)
	}
	if h.currencyFor(cvg4Org) != "usd" {
		t.Fatalf("deployment default currency expected, got %q", h.currencyFor(cvg4Org))
	}
	h.SetOrgCurrency(func(org string) string {
		if org == "org-cad" {
			return "  cad "
		}
		return "   "
	})
	if got := h.currencyFor("org-cad"); got != "cad" {
		t.Fatalf("club currency must be trimmed and win, got %q", got)
	}
	if got := h.currencyFor("org-other"); got != "usd" {
		t.Fatalf("blank club currency falls back to the default, got %q", got)
	}
	h2 := NewStripeHandlers(stripelib.NewStubClient(), billing.NewService(), "whsec", "eur", 5, slog.Default())
	if h2.currency != "eur" {
		t.Fatalf("explicit currency must be kept, got %q", h2.currency)
	}
	m := &telemetry.Metrics{}
	h.SetMetrics(m)
	if h.metrics != m {
		t.Fatal("SetMetrics must store the metrics sink")
	}
}

func TestCvg4IsAllowedReturnURL(t *testing.T) {
	t.Parallel()
	allowed := []string{" https://app.example.test ", "https://Other.example.test"}
	for _, tc := range []struct {
		name    string
		raw     string
		allowed []string
		want    bool
	}{
		{"no allowlist fails closed", "https://app.example.test/ok", nil, false},
		{"match with path", "https://app.example.test/billing?x=1", allowed, true},
		{"case-insensitive origin", "https://other.example.test/p", allowed, true},
		{"surrounding whitespace", "  https://app.example.test/ok ", allowed, true},
		{"unparseable", "http://[::1", allowed, false},
		{"relative url", "/billing", allowed, false},
		{"no host", "https:///x", allowed, false},
		{"different scheme", "http://app.example.test/ok", allowed, false},
		{"different host", "https://evil.example.test/ok", allowed, false},
		{"userinfo trick", "https://app.example.test@evil.example.test/", allowed, false},
	} {
		if got := isAllowedReturnURL(tc.raw, tc.allowed); got != tc.want {
			t.Errorf("%s: isAllowedReturnURL(%q) = %v, want %v", tc.name, tc.raw, got, tc.want)
		}
	}
}

func TestCvg4StripeMetadataRoutingKeysWin(t *testing.T) {
	t.Parallel()
	in := map[string]string{"invoice_id": "evil", "org_id": "evil", "note": "keep"}
	got := stripeMetadata(in, "inv-1", "org-1")
	if got["invoice_id"] != "inv-1" || got["org_id"] != "org-1" || got["note"] != "keep" || len(got) != 3 {
		t.Fatalf("unexpected metadata %v", got)
	}
	if in["invoice_id"] != "evil" {
		t.Fatal("caller map must not be mutated")
	}
	if got := stripeMetadata(nil, "i", "o"); len(got) != 2 {
		t.Fatalf("nil extra metadata: %v", got)
	}
}

func TestCvg4ObjectInt64(t *testing.T) {
	t.Parallel()
	obj := map[string]any{"f": float64(1250), "i64": int64(7), "i": 9, "s": "12", "nil": nil}
	for key, want := range map[string]int64{"f": 1250, "i64": 7, "i": 9, "s": 0, "nil": 0, "absent": 0} {
		if got := objectInt64(obj, key); got != want {
			t.Errorf("objectInt64(%q) = %d, want %d", key, got, want)
		}
	}
}

func TestCvg4ExtractLatestRefundID(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		obj  map[string]any
		want string
	}{
		{"no refunds key", map[string]any{}, ""},
		{"refunds not an object", map[string]any{"refunds": "x"}, ""},
		{"data missing", map[string]any{"refunds": map[string]any{}}, ""},
		{"data empty", map[string]any{"refunds": map[string]any{"data": []any{}}}, ""},
		{"first not an object", map[string]any{"refunds": map[string]any{"data": []any{"re_1"}}}, ""},
		{"id not a string", map[string]any{"refunds": map[string]any{"data": []any{map[string]any{"id": 5}}}}, ""},
		{"first refund id", map[string]any{"refunds": map[string]any{"data": []any{map[string]any{"id": "re_9"}, map[string]any{"id": "re_8"}}}}, "re_9"},
	} {
		if got := extractLatestRefundID(tc.obj); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestCvg4IsPermanentWebhookError(t *testing.T) {
	t.Parallel()
	old := time.Now().Add(-2 * webhookLagGrace).Unix()
	recent := time.Now().Add(-time.Minute).Unix()
	for _, tc := range []struct {
		name    string
		err     error
		created int64
		want    bool
	}{
		{"invoice missing, unknown age", billing.ErrInvoiceNotFound, 0, false},
		{"invoice missing, recent", fmt.Errorf("x: %w", billing.ErrInvoiceNotFound), recent, false},
		{"invoice missing, old", billing.ErrInvoiceNotFound, old, true},
		{"payment missing, old", fmt.Errorf("x: %w", billing.ErrPaymentNotFound), old, true},
		{"payment missing, recent", billing.ErrPaymentNotFound, recent, false},
		{"org mismatch", errWebhookOrgMismatch, 0, true},
		{"unattributable", fmt.Errorf("%w: detail", errWebhookUnattributable), recent, true},
		{"invalid transition", billing.ErrInvalidTransition, 0, true},
		{"finance validation", &billing.FinanceValidationError{Field: "amount", Message: "bad"}, 0, true},
		{"wrapped finance validation", fmt.Errorf("w: %w", &billing.FinanceValidationError{Field: "a", Message: "b"}), 0, true},
		{"transient database error", errors.New("connection reset"), old, false},
	} {
		if got := isPermanentWebhookError(tc.err, tc.created); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestCvg4WriteStripeError(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodPost, "/x", nil)

	rr := httptest.NewRecorder()
	writeStripeError(rr, req, fmt.Errorf("dial: %w", context.DeadlineExceeded), "ignored")
	cvg4WantErr(t, rr, http.StatusServiceUnavailable, "stripe_unavailable", "timeout")
	if !strings.Contains(rr.Body.String(), "temporarily unavailable") {
		t.Fatalf("unexpected body %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	writeStripeError(rr, req, errors.New("card_declined: secret detail"), "Failed to create payment intent.")
	cvg4WantErr(t, rr, http.StatusBadGateway, "stripe_error", "api error")
	if !strings.Contains(rr.Body.String(), "Failed to create payment intent.") || strings.Contains(rr.Body.String(), "secret detail") {
		t.Fatalf("the cause must not reach the client: %s", rr.Body.String())
	}
}

// ─── Route wiring ─────────────────────────────────────────────────────────────

func TestCvg4StripeRoutesRequireAuthContext(t *testing.T) {
	t.Parallel()
	e := cvg4NewEnv(t, billing.NewService())
	for _, rt := range []struct{ method, path string }{
		{http.MethodGet, "/stripe/webhook-events"},
		{http.MethodGet, "/stripe/reconciliation"},
		{http.MethodPost, "/stripe/payment-intents"},
		{http.MethodPost, "/stripe/checkout-sessions"},
	} {
		req := httptest.NewRequest(rt.method, rt.path, strings.NewReader(`{}`))
		rr := httptest.NewRecorder()
		e.bare.ServeHTTP(rr, req)
		cvg4WantErr(t, rr, http.StatusUnauthorized, "unauthorized", rt.method+" "+rt.path)
		// And through the real router without a token.
		rr = e.do(rt.method, rt.path, "", `{}`, nil)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without credentials: want 401, got %d", rt.method, rt.path, rr.Code)
		}
	}
}

func TestCvg4StripeWriteLimiterOnlyGuardsPaymentWrites(t *testing.T) {
	t.Parallel()
	var hits int
	e := cvg4NewEnv(t, billing.NewService(), func(e *cvg4Env) {
		e.h.SetWriteLimiter(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits++
				http.Error(w, "slow down", http.StatusTooManyRequests)
			})
		})
	})
	for _, p := range []string{"/stripe/payment-intents", "/stripe/checkout-sessions"} {
		rr := e.do(http.MethodPost, p, "admin", `{}`, nil)
		if rr.Code != http.StatusTooManyRequests {
			t.Fatalf("%s: limiter must run first, got %d", p, rr.Code)
		}
	}
	if hits != 2 {
		t.Fatalf("limiter hits = %d, want 2", hits)
	}
	rr := e.do(http.MethodGet, "/stripe/webhook-events", "admin", "", nil)
	expectStatus(t, rr, http.StatusOK, "read routes are not rate limited by the write limiter")
	if hits != 2 {
		t.Fatalf("limiter must not guard reads, hits=%d", hits)
	}
}

// ─── Payment intents ──────────────────────────────────────────────────────────

func TestCvg4CreatePaymentIntent(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	e := cvg4NewEnv(t, svc)
	open := cvg4Invoice(t, svc, "hh-1", "INV-PI-1", "", 5000, true)
	draft := cvg4Invoice(t, svc, "hh-1", "INV-PI-2", "", 5000, false)
	body := func(inv string, cents int) string {
		return fmt.Sprintf(`{"invoice_id":%q,"amount_cents":%d,"description":"d","metadata":{"invoice_id":"evil","org_id":"evil","k":"v"}}`, inv, cents)
	}

	cvg4WantErr(t, e.do("POST", "/stripe/payment-intents", "parent", body(open.ID, 5000), e.idem("k1")), 403, "forbidden", "parent cannot create intents")
	cvg4WantErr(t, e.do("POST", "/stripe/payment-intents", "coach", body(open.ID, 5000), e.idem("k1")), 403, "forbidden", "coach cannot create intents")
	cvg4WantErr(t, e.do("POST", "/stripe/payment-intents", "admin", `{"invoice_id":`, e.idem("k1")), 400, "invalid_request_body", "malformed JSON")
	cvg4WantErr(t, e.do("POST", "/stripe/payment-intents", "admin", `{"invoice_id":"x","amount_cents":0}`, e.idem("k1")), 422, "validation_error", "zero amount")
	cvg4WantErr(t, e.do("POST", "/stripe/payment-intents", "admin", body(open.ID, 5000), nil), 400, "missing_idempotency_key", "no key")
	cvg4WantErr(t, e.do("POST", "/stripe/payment-intents", "admin", body("missing", 100), e.idem("k1")), 404, "invoice_not_found", "unknown invoice")
	cvg4WantErr(t, e.do("POST", "/stripe/payment-intents", "admin", body(draft.ID, 100), e.idem("k1")), 409, "invoice_not_payable", "draft invoice")
	cvg4WantErr(t, e.do("POST", "/stripe/payment-intents", "admin", body(open.ID, 5001), e.idem("k1")), 400, "invalid_amount", "over balance")
	if i, s := e.client.created(); i != 0 || s != 0 {
		t.Fatalf("rejected requests must not reach Stripe (intents=%d sessions=%d)", i, s)
	}

	rr := e.do("POST", "/stripe/payment-intents", "admin", body(open.ID, 2500), e.idem("pay-1"))
	expectStatus(t, rr, http.StatusCreated, "partial intent")
	var out struct {
		PI struct {
			ID          string `json:"id"`
			AmountCents int    `json:"amount_cents"`
		} `json:"payment_intent"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil || !strings.HasPrefix(out.PI.ID, "pi_stub_") || out.PI.AmountCents != 2500 {
		t.Fatalf("unexpected intent response %s (%v)", rr.Body.String(), err)
	}
	p := e.client.lastIntent(t)
	if p.IdempotencyKey != cvg4Org+":pay-1" || p.Currency != "usd" || p.AmountCents != 2500 || p.Description != "d" || p.StripeAccount != "" {
		t.Fatalf("unexpected intent params %+v", p)
	}
	if p.Metadata["invoice_id"] != open.ID || p.Metadata["org_id"] != cvg4Org || p.Metadata["k"] != "v" {
		t.Fatalf("routing metadata must be server-controlled: %v", p.Metadata)
	}
}

func TestCvg4CreatePaymentIntentStripeFailures(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	e := cvg4NewEnv(t, svc)
	inv := cvg4Invoice(t, svc, "hh-1", "INV-PI-F", "", 1000, true)
	body := fmt.Sprintf(`{"invoice_id":%q,"amount_cents":1000}`, inv.ID)

	e.client.CreatePaymentIntentErr = errors.New("stripe 500")
	cvg4WantErr(t, e.do("POST", "/stripe/payment-intents", "admin", body, e.idem("a")), 502, "stripe_error", "api failure")
	e.client.CreatePaymentIntentErr = fmt.Errorf("dial: %w", context.DeadlineExceeded)
	cvg4WantErr(t, e.do("POST", "/stripe/payment-intents", "admin", body, e.idem("b")), 503, "stripe_unavailable", "timeout")
}

func TestCvg4CreatePaymentIntentConnectAccounts(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	accts := payments.NewService()
	e := cvg4NewEnv(t, svc, func(e *cvg4Env) { e.accts = accts; e.h.SetConnect(accts, true, "") })
	inv := cvg4Invoice(t, svc, "hh-1", "INV-PI-C", "", 1000, true)
	body := fmt.Sprintf(`{"invoice_id":%q,"amount_cents":1000}`, inv.ID)

	// Connect required and the club has not onboarded.
	cvg4WantErr(t, e.do("POST", "/stripe/payment-intents", "admin", body, e.idem("a")), 409, "payments_not_enabled", "no account")
	// Account exists but charges are not enabled yet.
	if _, err := accts.Create(context.Background(), cvg4Org, "acct_cvg4"); err != nil {
		t.Fatal(err)
	}
	cvg4WantErr(t, e.do("POST", "/stripe/payment-intents", "admin", body, e.idem("b")), 409, "payments_not_enabled", "charges disabled")
	if _, err := accts.UpdateStatus(context.Background(), "acct_cvg4", true, true, true); err != nil {
		t.Fatal(err)
	}
	expectStatus(t, e.do("POST", "/stripe/payment-intents", "admin", body, e.idem("c")), http.StatusCreated, "onboarded club")
	if got := e.client.lastIntent(t).StripeAccount; got != "acct_cvg4" {
		t.Fatalf("payment must be created on the club's connected account, got %q", got)
	}

	// Not required: a club without an account uses the platform account.
	svc2 := billing.NewService()
	e2 := cvg4NewEnv(t, svc2, func(e *cvg4Env) { e.h.SetConnect(payments.NewService(), false, "") })
	inv2 := cvg4Invoice(t, svc2, "hh-1", "INV-PI-C2", "", 1000, true)
	expectStatus(t, e2.do("POST", "/stripe/payment-intents", "admin", fmt.Sprintf(`{"invoice_id":%q,"amount_cents":1000}`, inv2.ID), e2.idem("d")), http.StatusCreated, "platform fallback")
	if got := e2.client.lastIntent(t).StripeAccount; got != "" {
		t.Fatalf("platform account expected, got %q", got)
	}

	// A lookup failure is an internal error and never falls back to the platform account.
	svc3 := billing.NewService()
	e3 := cvg4NewEnv(t, svc3, func(e *cvg4Env) { e.h.SetConnect(payments.NewServiceWithPool(testutil.ClosedPool(t)), false, "") })
	inv3 := cvg4Invoice(t, svc3, "hh-1", "INV-PI-C3", "", 1000, true)
	cvg4WantErr(t, e3.do("POST", "/stripe/payment-intents", "admin", fmt.Sprintf(`{"invoice_id":%q,"amount_cents":1000}`, inv3.ID), e3.idem("e")), 500, "internal_error", "account lookup failure")
	if i, _ := e3.client.created(); i != 0 {
		t.Fatalf("no intent may be created when the account is unknown, got %d", i)
	}
}

func TestCvg4StripeOrgCurrencyIsUsed(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	e := cvg4NewEnv(t, svc, func(e *cvg4Env) { e.h.SetOrgCurrency(func(string) string { return "cad" }) })
	inv := cvg4Invoice(t, svc, "hh-1", "INV-CUR", "", 1000, true)
	expectStatus(t, e.do("POST", "/stripe/payment-intents", "admin", fmt.Sprintf(`{"invoice_id":%q,"amount_cents":1000}`, inv.ID), e.idem("a")), http.StatusCreated, "intent")
	if got := e.client.lastIntent(t).Currency; got != "cad" {
		t.Fatalf("intent currency = %q, want cad", got)
	}
}

func TestCvg4InvoiceLookupFailureReturns500(t *testing.T) {
	t.Parallel()
	svc := billing.NewServiceWithPool(testutil.ClosedPool(t))
	e := cvg4NewEnv(t, svc)
	pi := `{"invoice_id":"inv-1","amount_cents":100}`
	cs := fmt.Sprintf(`{"invoice_id":"inv-1","amount_cents":100,"success_url":"%s/ok","cancel_url":"%s/no"}`, cvg4ReturnOK, cvg4ReturnOK)
	rr := e.do("POST", "/stripe/payment-intents", "admin", pi, e.idem("a"))
	cvg4WantErr(t, rr, 500, "invoice_lookup_failed", "payment intent")
	if strings.Contains(rr.Body.String(), "127.0.0.1") {
		t.Fatalf("infrastructure details leaked: %s", rr.Body.String())
	}
	cvg4WantErr(t, e.do("POST", "/stripe/checkout-sessions", "admin", cs, e.idem("b")), 500, "invoice_lookup_failed", "staff checkout")
	// Non-staff callers must not learn that the failure was infrastructural.
	cvg4WantErr(t, e.do("POST", "/stripe/checkout-sessions", "parent", cs, e.idem("c")), 403, "forbidden", "parent checkout")
}

// ─── Checkout sessions ────────────────────────────────────────────────────────

func cvg4CheckoutBody(inv string, cents int, success, cancel string) string {
	return fmt.Sprintf(`{"invoice_id":%q,"amount_cents":%d,"description":"ignored","success_url":%q,"cancel_url":%q,"metadata":{"org_id":"evil","k":"v"}}`, inv, cents, success, cancel)
}

func TestCvg4CreateCheckoutSessionParentFlow(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	e := cvg4NewEnv(t, svc)
	noted := cvg4Invoice(t, svc, "hh-1", "INV-CS-1", "  Fall dues  ", 4200, true)
	plain := cvg4Invoice(t, svc, "hh-1", "INV-CS-2", "", 1800, true)
	draft := cvg4Invoice(t, svc, "hh-1", "INV-CS-3", "", 900, false)
	ok, no := cvg4ReturnOK+"/paid", cvg4ReturnOK+"/cancel"

	rr := e.do("POST", "/stripe/checkout-sessions", "parent", cvg4CheckoutBody(noted.ID, 4200, ok, no), e.idem("s1"))
	expectStatus(t, rr, http.StatusCreated, "parent pays own invoice")
	var out struct {
		Session struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"checkout_session"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil || !strings.HasPrefix(out.Session.URL, "https://checkout.stripe.com/") {
		t.Fatalf("unexpected session %s (%v)", rr.Body.String(), err)
	}
	p := e.client.lastSession(t)
	if p.Description != "Fall dues" || p.AmountCents != 4200 || p.SuccessURL != ok || p.CancelURL != no || p.IdempotencyKey != cvg4Org+":s1" {
		t.Fatalf("unexpected session params %+v", p)
	}
	if p.Metadata["org_id"] != cvg4Org || p.Metadata["invoice_id"] != noted.ID || p.Metadata["k"] != "v" {
		t.Fatalf("routing metadata must be server-controlled: %v", p.Metadata)
	}

	// Without notes the description is the invoice number.
	expectStatus(t, e.do("POST", "/stripe/checkout-sessions", "parent", cvg4CheckoutBody(plain.ID, 1800, ok, no), e.idem("s2")), http.StatusCreated, "plain invoice")
	if got := e.client.lastSession(t).Description; got != "INV-CS-2" {
		t.Fatalf("description = %q, want the invoice number", got)
	}

	_, sessionsBefore := e.client.created()
	for _, tc := range []struct{ name, token, body string }{
		{"partial amount by a parent", "parent", cvg4CheckoutBody(plain.ID, 1000, ok, no)},
		{"overpayment by a parent", "parent", cvg4CheckoutBody(plain.ID, 1801, ok, no)},
		{"other household", "parent2", cvg4CheckoutBody(plain.ID, 1800, ok, no)},
		{"draft invoice", "parent", cvg4CheckoutBody(draft.ID, 900, ok, no)},
		{"unknown invoice hides existence", "parent", cvg4CheckoutBody("nope", 100, ok, no)},
		{"swimmer", "swimmer", cvg4CheckoutBody(plain.ID, 1800, ok, no)},
	} {
		cvg4WantErr(t, e.do("POST", "/stripe/checkout-sessions", tc.token, tc.body, e.idem("x")), 403, "forbidden", tc.name)
	}
	if _, after := e.client.created(); after != sessionsBefore {
		t.Fatalf("denied requests must not create sessions (%d -> %d)", sessionsBefore, after)
	}
}

func TestCvg4CreateCheckoutSessionStaffFlow(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	e := cvg4NewEnv(t, svc)
	inv := cvg4Invoice(t, svc, "hh-1", "INV-CS-S", "", 3000, true)
	ok, no := cvg4ReturnOK+"/paid", cvg4ReturnOK+"/cancel"

	expectStatus(t, e.do("POST", "/stripe/checkout-sessions", "admin", cvg4CheckoutBody(inv.ID, 1200, ok, no), e.idem("a")), http.StatusCreated, "staff partial payment")
	cvg4WantErr(t, e.do("POST", "/stripe/checkout-sessions", "admin", cvg4CheckoutBody(inv.ID, 3001, ok, no), e.idem("b")), 403, "forbidden", "staff overpayment")
	cvg4WantErr(t, e.do("POST", "/stripe/checkout-sessions", "admin", cvg4CheckoutBody("missing", 100, ok, no), e.idem("c")), 404, "invoice_not_found", "staff sees not found")
	draft := cvg4Invoice(t, svc, "hh-1", "INV-CS-D", "", 500, false)
	cvg4WantErr(t, e.do("POST", "/stripe/checkout-sessions", "admin", cvg4CheckoutBody(draft.ID, 500, ok, no), e.idem("c")), 403, "forbidden", "staff cannot pay a draft invoice")
	cvg4WantErr(t, e.do("POST", "/stripe/checkout-sessions", "admin", `{"invoice_id":`, e.idem("c")), 400, "invalid_request_body", "malformed JSON")
	cvg4WantErr(t, e.do("POST", "/stripe/checkout-sessions", "admin", `{"invoice_id":"x","amount_cents":5,"success_url":"nope","cancel_url":"nope"}`, e.idem("c")), 422, "validation_error", "non-URL return URLs")
	cvg4WantErr(t, e.do("POST", "/stripe/checkout-sessions", "admin", cvg4CheckoutBody(inv.ID, 100, ok, no), nil), 400, "missing_idempotency_key", "missing key")
	cvg4WantErr(t, e.do("POST", "/stripe/checkout-sessions", "admin", cvg4CheckoutBody(inv.ID, 100, ok, no), e.idem(strings.Repeat("k", 129))), 400, "invalid_idempotency_key", "oversized key")

	// Return URLs must stay on a trusted origin; checked after authorization.
	cvg4WantErr(t, e.do("POST", "/stripe/checkout-sessions", "admin", cvg4CheckoutBody(inv.ID, 100, "https://evil.example.test/paid", no), e.idem("d")), 400, "invalid_return_url", "evil success url")
	cvg4WantErr(t, e.do("POST", "/stripe/checkout-sessions", "admin", cvg4CheckoutBody(inv.ID, 100, ok, "https://evil.example.test/cancel"), e.idem("e")), 400, "invalid_return_url", "evil cancel url")

	e.client.CreateCheckoutSessionErr = errors.New("stripe 500")
	cvg4WantErr(t, e.do("POST", "/stripe/checkout-sessions", "admin", cvg4CheckoutBody(inv.ID, 100, ok, no), e.idem("f")), 502, "stripe_error", "api failure")
	e.client.CreateCheckoutSessionErr = fmt.Errorf("dial: %w", context.DeadlineExceeded)
	cvg4WantErr(t, e.do("POST", "/stripe/checkout-sessions", "admin", cvg4CheckoutBody(inv.ID, 100, ok, no), e.idem("g")), 503, "stripe_unavailable", "timeout")
}

func TestCvg4CreateCheckoutSessionWithoutAllowedOriginsFailsClosed(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	e := cvg4NewEnv(t, svc, func(e *cvg4Env) { e.h.SetAllowedReturnOrigins(nil) })
	inv := cvg4Invoice(t, svc, "hh-1", "INV-CS-N", "", 1000, true)
	cvg4WantErr(t, e.do("POST", "/stripe/checkout-sessions", "admin", cvg4CheckoutBody(inv.ID, 1000, cvg4ReturnOK+"/a", cvg4ReturnOK+"/b"), e.idem("a")), 400, "invalid_return_url", "no allowlist")
}

func TestCvg4CreateCheckoutSessionConnectAccounts(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	accts := payments.NewService()
	e := cvg4NewEnv(t, svc, func(e *cvg4Env) { e.h.SetConnect(accts, true, "") })
	inv := cvg4Invoice(t, svc, "hh-1", "INV-CS-C", "", 1000, true)
	body := cvg4CheckoutBody(inv.ID, 1000, cvg4ReturnOK+"/a", cvg4ReturnOK+"/b")
	cvg4WantErr(t, e.do("POST", "/stripe/checkout-sessions", "admin", body, e.idem("a")), 409, "payments_not_enabled", "not onboarded")
	if _, err := accts.Create(context.Background(), cvg4Org, "acct_cs"); err != nil {
		t.Fatal(err)
	}
	if _, err := accts.UpdateStatus(context.Background(), "acct_cs", true, true, true); err != nil {
		t.Fatal(err)
	}
	expectStatus(t, e.do("POST", "/stripe/checkout-sessions", "admin", body, e.idem("b")), http.StatusCreated, "onboarded")
	if got := e.client.lastSession(t).StripeAccount; got != "acct_cs" {
		t.Fatalf("session account = %q", got)
	}
}

// ─── Webhook events & reconciliation ──────────────────────────────────────────

func TestCvg4ListWebhookEvents(t *testing.T) {
	t.Parallel()
	e := cvg4NewEnv(t, billing.NewService())
	cvg4WantErr(t, e.do("GET", "/stripe/webhook-events", "parent", "", nil), 403, "forbidden", "parent")
	cvg4WantErr(t, e.do("GET", "/stripe/webhook-events", "coach", "", nil), 403, "forbidden", "coach")
	rr := e.do("GET", "/stripe/webhook-events", "admin", "", nil)
	expectStatus(t, rr, http.StatusOK, "admin")
	if !strings.Contains(rr.Body.String(), `"count":0`) {
		t.Fatalf("expected an empty list, got %s", rr.Body.String())
	}
	if rr := e.webhook(cvg4Event("evt_list_1", "customer.created", "", time.Now().Unix(), map[string]any{"id": "cus_1", "metadata": map[string]any{"org_id": cvg4Org}})); rr.Code != 200 {
		t.Fatalf("webhook: %d %s", rr.Code, rr.Body.String())
	}
	rr = e.do("GET", "/stripe/webhook-events", "admin", "", nil)
	if !strings.Contains(rr.Body.String(), `"count":1`) || !strings.Contains(rr.Body.String(), "evt_list_1") {
		t.Fatalf("expected the stored event, got %s", rr.Body.String())
	}

	// A database failure is a 500 without the cause.
	broken := cvg4NewEnv(t, billing.NewServiceWithPool(testutil.ClosedPool(t)))
	rr = broken.do("GET", "/stripe/webhook-events", "admin", "", nil)
	cvg4WantErr(t, rr, 500, "internal_error", "closed pool")
	if strings.Contains(rr.Body.String(), "127.0.0.1") {
		t.Fatalf("infrastructure details leaked: %s", rr.Body.String())
	}
}

func TestCvg4Reconcile(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	e := cvg4NewEnv(t, svc)
	cvg4WantErr(t, e.do("GET", "/stripe/reconciliation", "parent", "", nil), 403, "forbidden", "parent")
	cvg4WantErr(t, e.do("GET", "/stripe/reconciliation", "coach", "", nil), 403, "forbidden", "coach")

	rr := e.do("GET", "/stripe/reconciliation", "admin", "", nil)
	expectStatus(t, rr, http.StatusOK, "no mismatch")
	var out struct {
		R billing.ReconciliationReport `json:"reconciliation"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil || out.R.AlertTriggered || out.R.StripeReportedCents != 0 {
		t.Fatalf("unexpected report %s (%v)", rr.Body.String(), err)
	}

	// A balance far from the ledger triggers the alert flag.
	e.client.BalanceCents = 10_000
	rr = e.do("GET", "/stripe/reconciliation", "admin", "", nil)
	expectStatus(t, rr, http.StatusOK, "mismatch")
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil || !out.R.AlertTriggered || out.R.StripeReportedCents != 10_000 || out.R.MismatchCents != 10_000 {
		t.Fatalf("expected an alerting report, got %s (%v)", rr.Body.String(), err)
	}

	e.client.balErr = errors.New("stripe balance 500")
	cvg4WantErr(t, e.do("GET", "/stripe/reconciliation", "admin", "", nil), 502, "stripe_error", "balance failure")
	e.client.balErr = fmt.Errorf("dial: %w", context.DeadlineExceeded)
	cvg4WantErr(t, e.do("GET", "/stripe/reconciliation", "admin", "", nil), 503, "stripe_unavailable", "balance timeout")
}

func TestCvg4ReconcileAccountResolution(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	e := cvg4NewEnv(t, svc)

	svc.SetStripeAccountResolver(func(string) (string, error) { return "", billing.ErrPaymentAccountNotConnected })
	cvg4WantErr(t, e.do("GET", "/stripe/reconciliation", "admin", "", nil), 409, "payments_not_enabled", "not connected")

	svc.SetStripeAccountResolver(func(string) (string, error) { return "", errors.New("registry down") })
	cvg4WantErr(t, e.do("GET", "/stripe/reconciliation", "admin", "", nil), 500, "internal_error", "resolver failure")

	svc.SetStripeAccountResolver(func(string) (string, error) { return "acct_rec", nil })
	e.client.SetAccountBalance("acct_rec", 777)
	rr := e.do("GET", "/stripe/reconciliation", "admin", "", nil)
	expectStatus(t, rr, http.StatusOK, "connected club")
	if !strings.Contains(rr.Body.String(), `"stripe_reported_cents":777`) {
		t.Fatalf("balance of the connected account must be used: %s", rr.Body.String())
	}
}

func TestCvg4ReconcileReportFailure(t *testing.T) {
	t.Parallel()
	e := cvg4NewEnv(t, billing.NewServiceWithPool(testutil.ClosedPool(t)))
	rr := e.do("GET", "/stripe/reconciliation", "admin", "", nil)
	cvg4WantErr(t, rr, 500, "internal_error", "report failure")
}

// ─── Webhook intake ───────────────────────────────────────────────────────────

func TestCvg4WebhookBodyReadFailure(t *testing.T) {
	t.Parallel()
	e := cvg4NewEnv(t, billing.NewService())
	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", nil)
	req.Body = io.NopCloser(iotest.ErrReader(errors.New("connection reset")))
	rr := httptest.NewRecorder()
	e.router.ServeHTTP(rr, req)
	cvg4WantErr(t, rr, 400, "read_error", "unreadable body")
}

func TestCvg4WebhookSignatureFailures(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	e := cvg4NewEnv(t, svc)
	var calls []string
	failWith := func(err error) {
		e.client.parse = func(_ []byte, _, secret string) (*stripelib.WebhookEventData, error) {
			calls = append(calls, secret)
			return nil, err
		}
	}
	payload := cvg4Event("evt_sig", "customer.created", "", time.Now().Unix(), map[string]any{"id": "cus_1"})

	for name, err := range map[string]error{
		"invalid signature":  stripelib.ErrInvalidSignature,
		"expired timestamp":  fmt.Errorf("wrapped: %w", stripelib.ErrExpiredTimestamp),
		"malformed header":   stripelib.ErrMalformedHeader,
		"also wrapped dated": fmt.Errorf("a: %w", fmt.Errorf("b: %w", stripelib.ErrInvalidSignature)),
	} {
		failWith(err)
		cvg4WantErr(t, e.webhook(payload), 401, "invalid_signature", name)
	}
	failWith(errors.New("json: cannot unmarshal"))
	rr := e.webhook(payload)
	cvg4WantErr(t, rr, 400, "webhook_parse_error", "unparseable payload")
	if strings.Contains(rr.Body.String(), "unmarshal") {
		t.Fatalf("parser detail leaked: %s", rr.Body.String())
	}
	for _, s := range calls {
		if s != "whsec_primary" {
			t.Fatalf("only the primary secret may be tried without a Connect secret, saw %q", s)
		}
	}
	if got := e.do("GET", "/stripe/webhook-events", "admin", "", nil); !strings.Contains(got.Body.String(), `"count":0`) {
		t.Fatalf("rejected deliveries must not be stored: %s", got.Body.String())
	}
}

func TestCvg4WebhookConnectSecretFallback(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	e := cvg4NewEnv(t, svc, func(e *cvg4Env) { e.h.SetConnect(payments.NewService(), false, "whsec_connect") })
	var secrets []string
	e.client.parse = func(p []byte, sig, secret string) (*stripelib.WebhookEventData, error) {
		secrets = append(secrets, secret)
		if secret != "whsec_connect" {
			return nil, stripelib.ErrInvalidSignature
		}
		return e.client.StubClient.ParseWebhookEvent(p, sig, secret)
	}
	rr := e.webhook(cvg4Event("evt_conn_ok", "customer.created", "", time.Now().Unix(), map[string]any{"id": "cus"}))
	expectStatus(t, rr, http.StatusOK, "connect secret accepted")
	if cvg4Status(t, rr) != "processed" || len(secrets) != 2 || secrets[0] != "whsec_primary" || secrets[1] != "whsec_connect" {
		t.Fatalf("expected primary then connect secret, status=%s secrets=%v", cvg4Status(t, rr), secrets)
	}

	// Both secrets failing is an invalid signature; only signature errors fall back.
	secrets = nil
	e.client.parse = func(_ []byte, _, secret string) (*stripelib.WebhookEventData, error) {
		secrets = append(secrets, secret)
		return nil, stripelib.ErrInvalidSignature
	}
	cvg4WantErr(t, e.webhook([]byte(`{}`)), 401, "invalid_signature", "neither secret matches")
	if len(secrets) != 2 {
		t.Fatalf("both secrets must be tried, got %v", secrets)
	}
	secrets = nil
	e.client.parse = func(_ []byte, _, secret string) (*stripelib.WebhookEventData, error) {
		secrets = append(secrets, secret)
		return nil, errors.New("bad json")
	}
	cvg4WantErr(t, e.webhook([]byte(`{}`)), 400, "webhook_parse_error", "parse error is not retried with another secret")
	if len(secrets) != 1 {
		t.Fatalf("non-signature errors must not trigger the fallback, got %v", secrets)
	}
}

func TestCvg4WebhookRecordFailureReturns500(t *testing.T) {
	t.Parallel()
	e := cvg4NewEnv(t, billing.NewServiceWithPool(testutil.ClosedPool(t)))
	rr := e.webhook(cvg4Event("evt_rec_fail", "customer.created", "", time.Now().Unix(), map[string]any{"id": "c"}))
	cvg4WantErr(t, rr, 500, "internal_error", "persist failure")
	if strings.Contains(rr.Body.String(), "127.0.0.1") {
		t.Fatalf("infrastructure details leaked: %s", rr.Body.String())
	}
}

func TestCvg4WebhookInFlightRequestsRetry(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	e := cvg4NewEnv(t, svc)
	// Claim the event as another delivery would, leaving its lease active.
	if isNew, err := svc.RecordWebhookEvent("stripe", "", "evt_inflight", "customer.created", json.RawMessage(`{}`)); err != nil || !isNew {
		t.Fatalf("claim: %v %v", isNew, err)
	}
	rr := e.webhook(cvg4Event("evt_inflight", "customer.created", "", time.Now().Unix(), map[string]any{"id": "c"}))
	cvg4WantErr(t, rr, 503, "webhook_in_flight", "active lease")
}

func TestCvg4WebhookUnknownEventTypeAcknowledged(t *testing.T) {
	t.Parallel()
	e := cvg4NewEnv(t, billing.NewService())
	rr := e.webhook(cvg4Event("evt_unknown", "customer.subscription.updated", "", time.Now().Unix(), map[string]any{"id": "sub"}))
	expectStatus(t, rr, http.StatusOK, "unknown type")
	if cvg4Status(t, rr) != "processed" {
		t.Fatalf("unknown event types are acknowledged: %s", rr.Body.String())
	}
	if ev := e.storedEvent("evt_unknown"); !ev.Processed || ev.ErrorMessage != "" {
		t.Fatalf("unexpected record %+v", ev)
	}
}

func TestCvg4WebhookRecordsProcessingLagMetric(t *testing.T) {
	t.Parallel()
	e := cvg4NewEnv(t, billing.NewService())
	m := telemetry.NewMetrics()
	e.h.SetMetrics(m)
	rr := e.webhook(cvg4Event("evt_lag", "customer.created", "", time.Now().Unix(), map[string]any{"id": "c"}))
	expectStatus(t, rr, http.StatusOK, "webhook with metrics")
	if cvg4Status(t, rr) != "processed" {
		t.Fatalf("unexpected response %s", rr.Body.String())
	}
}

// ─── payment_intent.succeeded / failed / charge.refunded ──────────────────────

func TestCvg4WebhookPaymentOutcomes(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	e := cvg4NewEnv(t, svc)
	inv := cvg4Invoice(t, svc, "hh-1", "INV-WH-1", "", 6000, true)
	admin := &appmiddleware.Principal{UserID: "u-admin", OrgID: cvg4Org, Roles: []string{"admin"}}
	now := time.Now().Unix()
	meta := map[string]any{"org_id": cvg4Org, "invoice_id": inv.ID}

	// A malformed succeeded event (no intent id) is transient: Stripe must redeliver.
	rr := e.webhook(cvg4Event("evt_ok_noid", "payment_intent.succeeded", "", now, map[string]any{"amount": 100, "metadata": meta}))
	cvg4WantErr(t, rr, 500, "webhook_processing_failed", "missing intent id")
	if ev := e.storedEvent("evt_ok_noid"); ev.Processed || !ev.Retryable || !strings.Contains(ev.ErrorMessage, "missing payment intent ID") {
		t.Fatalf("unexpected record %+v", ev)
	}

	// Events without routing metadata can never be attributed: stored and acknowledged.
	rr = e.webhook(cvg4Event("evt_ok_nometa", "payment_intent.succeeded", "", now, map[string]any{"id": "pi_x", "amount": 100}))
	expectStatus(t, rr, http.StatusOK, "unattributable")
	if cvg4Status(t, rr) != "persisted_with_error" {
		t.Fatalf("status = %s", cvg4Status(t, rr))
	}
	if ev := e.storedEvent("evt_ok_nometa"); ev.Processed || ev.Retryable || !strings.Contains(ev.ErrorMessage, "manual attribution required") {
		t.Fatalf("unexpected record %+v", ev)
	}

	// An unknown invoice is retried while the event is fresh (another instance may not
	// have seen it yet) and acknowledged once it is older than the lag grace.
	ghost := map[string]any{"org_id": cvg4Org, "invoice_id": "ghost"}
	rr = e.webhook(cvg4Event("evt_ok_ghost_new", "payment_intent.succeeded", "", now, map[string]any{"id": "pi_g1", "amount": 100, "metadata": ghost}))
	cvg4WantErr(t, rr, 500, "webhook_processing_failed", "fresh ghost invoice")
	rr = e.webhook(cvg4Event("evt_ok_ghost_old", "payment_intent.succeeded", "", now-int64((2*webhookLagGrace).Seconds()), map[string]any{"id": "pi_g2", "amount": 100, "metadata": ghost}))
	expectStatus(t, rr, http.StatusOK, "old ghost invoice")
	if cvg4Status(t, rr) != "persisted_with_error" {
		t.Fatalf("status = %s", cvg4Status(t, rr))
	}

	// payment_failed needs the intent id; an unknown intent is a no-op.
	cvg4WantErr(t, e.webhook(cvg4Event("evt_fail_noid", "payment_intent.payment_failed", "", now, map[string]any{"metadata": meta})), 500, "webhook_processing_failed", "failed without id")
	rr = e.webhook(cvg4Event("evt_fail_unknown", "payment_intent.payment_failed", "", now, map[string]any{"id": "pi_none", "metadata": meta}))
	expectStatus(t, rr, http.StatusOK, "failed for unknown intent")
	if cvg4Status(t, rr) != "processed" {
		t.Fatalf("status = %s", cvg4Status(t, rr))
	}

	// charge.refunded without a payment intent is skipped; with an intent it needs routing.
	rr = e.webhook(cvg4Event("evt_ref_noint", "charge.refunded", "", now, map[string]any{"amount_refunded": 100}))
	expectStatus(t, rr, http.StatusOK, "refund without intent")
	if cvg4Status(t, rr) != "processed" {
		t.Fatalf("status = %s", cvg4Status(t, rr))
	}
	rr = e.webhook(cvg4Event("evt_ref_nometa", "charge.refunded", "", now, map[string]any{"payment_intent": "pi_x", "amount_refunded": 100}))
	expectStatus(t, rr, http.StatusOK, "refund without metadata")
	if cvg4Status(t, rr) != "persisted_with_error" {
		t.Fatalf("status = %s", cvg4Status(t, rr))
	}

	// Happy path end to end: pay in full, then refund part of it.
	rr = e.webhook(cvg4Event("evt_pay", "payment_intent.succeeded", "", now, map[string]any{"id": "pi_ok", "amount": float64(6000), "metadata": meta}))
	if rr.Code != 200 || cvg4Status(t, rr) != "processed" {
		t.Fatalf("payment: %d %s", rr.Code, rr.Body.String())
	}
	paid, err := svc.GetInvoice(cvg4Org, admin, inv.ID)
	if err != nil || paid.Status != "paid" || paid.AmountDueCents != 0 {
		t.Fatalf("invoice must be paid: %+v %v", paid, err)
	}
	rr = e.webhook(cvg4Event("evt_refund_part", "charge.refunded", "", now, map[string]any{
		"payment_intent": "pi_ok", "amount_refunded": float64(1500), "metadata": meta,
		"refunds": map[string]any{"data": []any{map[string]any{"id": "re_part"}}},
	}))
	if rr.Code != 200 || cvg4Status(t, rr) != "processed" {
		t.Fatalf("refund: %d %s", rr.Code, rr.Body.String())
	}
	refunded, err := svc.GetInvoice(cvg4Org, admin, inv.ID)
	if err != nil || refunded.Status != "refunded_partial" {
		t.Fatalf("invoice must be partially refunded: %+v %v", refunded, err)
	}
}

// ─── Connect events & account.updated ─────────────────────────────────────────

func TestCvg4WebhookConnectAttribution(t *testing.T) {
	t.Parallel()
	now := time.Now().Unix()

	t.Run("connected accounts not configured", func(t *testing.T) {
		e := cvg4NewEnv(t, billing.NewService())
		rr := e.webhook(cvg4Event("evt_c0", "customer.created", "acct_x", now, map[string]any{"id": "c"}))
		expectStatus(t, rr, http.StatusOK, "unattributable")
		if cvg4Status(t, rr) != "persisted_with_error" {
			t.Fatalf("status = %s", cvg4Status(t, rr))
		}
		if ev := e.storedEvent("evt_c0"); !strings.Contains(ev.ErrorMessage, "connected accounts are not configured") || ev.OrgID != "" {
			t.Fatalf("unexpected record %+v", ev)
		}
	})

	t.Run("unknown connected account", func(t *testing.T) {
		e := cvg4NewEnv(t, billing.NewService(), func(e *cvg4Env) { e.h.SetConnect(payments.NewService(), true, "") })
		rr := e.webhook(cvg4Event("evt_c1", "customer.created", "acct_nobody", now, map[string]any{"id": "c"}))
		expectStatus(t, rr, http.StatusOK, "unknown account")
		if ev := e.storedEvent("evt_c1"); !strings.Contains(ev.ErrorMessage, "belongs to no organization") {
			t.Fatalf("unexpected record %+v", ev)
		}
	})

	t.Run("account lookup failure is retried", func(t *testing.T) {
		e := cvg4NewEnv(t, billing.NewService(), func(e *cvg4Env) {
			e.h.SetConnect(payments.NewServiceWithPool(testutil.ClosedPool(t)), true, "")
		})
		rr := e.webhook(cvg4Event("evt_c2", "customer.created", "acct_x", now, map[string]any{"id": "c"}))
		cvg4WantErr(t, rr, 500, "webhook_processing_failed", "lookup failure")
		if ev := e.storedEvent("evt_c2"); ev.Processed || !ev.Retryable {
			t.Fatalf("unexpected record %+v", ev)
		}
	})

	t.Run("metadata naming another org is rejected", func(t *testing.T) {
		accts := payments.NewService()
		if _, err := accts.Create(context.Background(), cvg4Org, "acct_mine"); err != nil {
			t.Fatal(err)
		}
		e := cvg4NewEnv(t, billing.NewService(), func(e *cvg4Env) { e.h.SetConnect(accts, true, "") })
		rr := e.webhook(cvg4Event("evt_c3", "customer.created", "acct_mine", now, map[string]any{"id": "c", "metadata": map[string]any{"org_id": "someone-else"}}))
		expectStatus(t, rr, http.StatusOK, "forged org")
		ev := e.storedEvent("evt_c3")
		if ev.OrgID != "" || !strings.Contains(ev.ErrorMessage, "does not match the connected account's org") {
			t.Fatalf("a rejected event must not be stored under the forged org: %+v", ev)
		}
	})

	t.Run("account is authoritative for the org", func(t *testing.T) {
		accts := payments.NewService()
		if _, err := accts.Create(context.Background(), cvg4Org, "acct_mine"); err != nil {
			t.Fatal(err)
		}
		e := cvg4NewEnv(t, billing.NewService(), func(e *cvg4Env) { e.h.SetConnect(accts, true, "") })
		// Metadata that matches, and metadata that is absent, are both stamped with the account's org.
		e.client.parse = func(p []byte, sig, secret string) (*stripelib.WebhookEventData, error) {
			ev, err := e.client.StubClient.ParseWebhookEvent(p, sig, secret)
			if err == nil && strings.Contains(string(p), "evt_c4b") {
				ev.Metadata = nil
			}
			return ev, err
		}
		for _, id := range []string{"evt_c4a", "evt_c4b"} {
			obj := map[string]any{"id": "c"}
			if id == "evt_c4a" {
				obj["metadata"] = map[string]any{"org_id": cvg4Org}
			}
			rr := e.webhook(cvg4Event(id, "customer.created", "acct_mine", now, obj))
			expectStatus(t, rr, http.StatusOK, id)
			if cvg4Status(t, rr) != "processed" {
				t.Fatalf("%s status = %s", id, cvg4Status(t, rr))
			}
			if ev := e.storedEvent(id); ev.OrgID != cvg4Org {
				t.Fatalf("%s must be stored under the account's org, got %q", id, ev.OrgID)
			}
		}
	})
}

func TestCvg4WebhookAccountUpdated(t *testing.T) {
	t.Parallel()
	now := time.Now().Unix()
	ctx := context.Background()

	t.Run("syncs live capability flags", func(t *testing.T) {
		accts := payments.NewService()
		e := cvg4NewEnv(t, billing.NewService(), func(e *cvg4Env) { e.h.SetConnect(accts, true, "") })
		acct, err := e.client.CreateConnectedAccount(ctx, stripelib.ConnectedAccountParams{OrgID: cvg4Org})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := accts.Create(ctx, cvg4Org, acct.ID); err != nil {
			t.Fatal(err)
		}
		// The payload claims nothing is enabled; the live account is authoritative.
		rr := e.webhook(cvg4Event("evt_au1", "account.updated", acct.ID, now, map[string]any{"id": acct.ID, "charges_enabled": false}))
		expectStatus(t, rr, http.StatusOK, "account.updated")
		if cvg4Status(t, rr) != "processed" {
			t.Fatalf("status = %s %s", cvg4Status(t, rr), rr.Body.String())
		}
		got, err := accts.Get(ctx, cvg4Org)
		if err != nil || !got.ChargesEnabled || !got.PayoutsEnabled || !got.DetailsSubmitted {
			t.Fatalf("flags must follow the live account: %+v %v", got, err)
		}
	})

	t.Run("missing id and unconfigured accounts are ignored", func(t *testing.T) {
		e := cvg4NewEnv(t, billing.NewService(), func(e *cvg4Env) { e.h.SetConnect(payments.NewService(), true, "") })
		rr := e.webhook(cvg4Event("evt_au2", "account.updated", "", now, map[string]any{}))
		if rr.Code != 200 || cvg4Status(t, rr) != "processed" {
			t.Fatalf("no id: %d %s", rr.Code, rr.Body.String())
		}
		e2 := cvg4NewEnv(t, billing.NewService())
		rr = e2.webhook(cvg4Event("evt_au3", "account.updated", "", now, map[string]any{"id": "acct_any"}))
		if rr.Code != 200 || cvg4Status(t, rr) != "processed" {
			t.Fatalf("no accounts service: %d %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("live fetch failure is retried", func(t *testing.T) {
		e := cvg4NewEnv(t, billing.NewService(), func(e *cvg4Env) { e.h.SetConnect(payments.NewService(), true, "") })
		rr := e.webhook(cvg4Event("evt_au4", "account.updated", "", now, map[string]any{"id": "acct_unknown_to_stripe"}))
		cvg4WantErr(t, rr, 500, "webhook_processing_failed", "fetch failure")
		if ev := e.storedEvent("evt_au4"); !ev.Retryable || !strings.Contains(ev.ErrorMessage, "fetch live account acct_unknown_to_stripe") {
			t.Fatalf("unexpected record %+v", ev)
		}
	})

	t.Run("account unknown to the club registry is permanent", func(t *testing.T) {
		e := cvg4NewEnv(t, billing.NewService(), func(e *cvg4Env) { e.h.SetConnect(payments.NewService(), true, "") })
		acct, err := e.client.CreateConnectedAccount(ctx, stripelib.ConnectedAccountParams{OrgID: "x"})
		if err != nil {
			t.Fatal(err)
		}
		rr := e.webhook(cvg4Event("evt_au5", "account.updated", "", now, map[string]any{"id": acct.ID}))
		expectStatus(t, rr, http.StatusOK, "unknown to registry")
		if cvg4Status(t, rr) != "persisted_with_error" {
			t.Fatalf("status = %s", cvg4Status(t, rr))
		}
		if ev := e.storedEvent("evt_au5"); ev.Retryable || !strings.Contains(ev.ErrorMessage, "account.updated for unknown account") {
			t.Fatalf("unexpected record %+v", ev)
		}
	})

	t.Run("registry failure is retried", func(t *testing.T) {
		e := cvg4NewEnv(t, billing.NewService(), func(e *cvg4Env) {
			e.h.SetConnect(payments.NewServiceWithPool(testutil.ClosedPool(t)), true, "")
		})
		acct, err := e.client.CreateConnectedAccount(ctx, stripelib.ConnectedAccountParams{OrgID: "x"})
		if err != nil {
			t.Fatal(err)
		}
		rr := e.webhook(cvg4Event("evt_au6", "account.updated", "", now, map[string]any{"id": acct.ID}))
		cvg4WantErr(t, rr, 500, "webhook_processing_failed", "registry failure")
		if ev := e.storedEvent("evt_au6"); !ev.Retryable {
			t.Fatalf("unexpected record %+v", ev)
		}
	})
}

// A failure to persist the outcome after processing must ask Stripe to retry
// instead of acknowledging an event whose result was lost. The Stripe call made
// during processing closes the database pool, so recording the event succeeded
// but marking it processed cannot.
func TestCvg4WebhookOutcomePersistenceFailure(t *testing.T) {
	base := testutil.IsolatedSchemaPool(t)
	pool, err := pgxpool.NewWithConfig(context.Background(), base.Config().Copy())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	svc := billing.NewServiceWithPool(pool)
	e := cvg4NewEnv(t, svc, func(e *cvg4Env) { e.h.SetConnect(payments.NewService(), true, "") })
	e.client.onAcct = pool.Close

	org := "11111111-2222-4333-8444-555555555555"
	rr := e.webhook(cvg4Event("evt_outcome", "account.updated", "", time.Now().Unix(), map[string]any{
		"id": "acct_outcome", "metadata": map[string]any{"org_id": org},
	}))
	cvg4WantErr(t, rr, 500, "webhook_outcome_failed", "outcome persistence failure")
}

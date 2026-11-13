package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/app/billing"
	"github.com/luryann/aquify/server/internal/app/comms"
	"github.com/luryann/aquify/server/internal/app/payments"
	stripelib "github.com/luryann/aquify/server/internal/stripe"
	"github.com/luryann/aquify/server/internal/testutil"
)

func TestCov1yRouterDefaultsANilLogger(t *testing.T) {
	router := NewRouterWithServices(Config{AppEnv: "test"}, nil, RouterDeps{})
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("live probe: %d %s", rr.Code, rr.Body.String())
	}
}

func TestCov1yLocalDevHouseholdSeedFailureIsLoggedNotFatal(t *testing.T) {
	// The database refuses every new household, so the dev seed insert fails.
	pool := testutil.IsolatedSchemaPool(t)
	for _, q := range []string{
		`CREATE FUNCTION cov1y_block_household_insert() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'households are frozen'; END $$`,
		`CREATE TRIGGER cov1y_freeze BEFORE INSERT ON households FOR EACH ROW EXECUTE FUNCTION cov1y_block_household_insert()`,
	} {
		if _, err := pool.Exec(context.Background(), q); err != nil {
			t.Fatal(err)
		}
	}
	cov1xUseDBPool(t, pool)
	logger, logs := cov1xLogger()
	router := NewRouterWithServices(Config{AppEnv: "development", DatabaseURL: "postgres://fixture.invalid/aquify", Auth: AuthConfig{EnableDevStubTokens: true}}, logger, RouterDeps{})
	if !strings.Contains(logs.String(), "local dev household seed failed") || !strings.Contains(logs.String(), "households are frozen") {
		t.Fatalf("expected the seed failure to be logged, got %q", logs.String())
	}
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("the API must still start: %d", rr.Code)
	}
}

func TestCov1yRouterConnectsTheRealtimeBroadcaster(t *testing.T) {
	redisServer := miniredis.RunT(t)
	commsSvc := comms.NewService()
	NewRouterWithServices(Config{AppEnv: "test", Worker: WorkerConfig{RedisURL: "redis://" + redisServer.Addr()}},
		slog.New(slog.NewTextHandler(io.Discard, nil)), RouterDeps{Comms: commsSvc})
	b := commsSvc.Broadcaster()
	if b == nil {
		t.Fatal("a reachable REDIS_URL must attach the realtime broadcaster to comms")
	}

	const org = "org-cov1y-realtime"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := b.Subscribe(ctx, org)
	defer sub.Close()
	if _, err := sub.Receive(ctx); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if _, err := commsSvc.SendMessage(comms.CreateMessageInput{OrgID: org, HouseholdID: "hh-1", SwimmerID: "sw-1", Body: "pool is open", SenderUserID: "coach-1", SenderRole: "coach"}); err != nil {
		t.Fatal(err)
	}
	msg, err := sub.ReceiveMessage(ctx)
	if err != nil || !strings.Contains(msg.Payload, "message.created") {
		t.Fatalf("realtime event: %+v %v", msg, err)
	}
}

func TestCov1yStripeAccountResolverEnforcesConnectPolicy(t *testing.T) {
	const org = "org-cov1y-connect"
	yes, no := true, false
	build := func(required *bool, pay *payments.Service) *billing.Service {
		svc := billing.NewService()
		NewRouterWithServices(Config{AppEnv: "test", Stripe: StripeConfig{ConnectRequired: required}},
			slog.New(slog.NewTextHandler(io.Discard, nil)), RouterDeps{Billing: svc, Payments: pay})
		return svc
	}

	// No connected account: refused while Connect is required, platform account otherwise.
	if _, err := build(&yes, payments.NewService()).StripeAccountFor(org); err != billing.ErrPaymentAccountNotConnected {
		t.Fatalf("required + unconnected = %v, want ErrPaymentAccountNotConnected", err)
	}
	if acct, err := build(&no, payments.NewService()).StripeAccountFor(org); err != nil || acct != "" {
		t.Fatalf("optional + unconnected = %q %v, want the platform account", acct, err)
	}
	// Connected: the club's own account.
	pay := payments.NewService()
	if _, err := pay.Create(context.Background(), org, "acct_cov1y"); err != nil {
		t.Fatal(err)
	}
	if acct, err := build(&yes, pay).StripeAccountFor(org); err != nil || acct != "acct_cov1y" {
		t.Fatalf("connected = %q %v", acct, err)
	}

	// A storage failure is an error, never a silent fallback to the platform account.
	pool := testutil.IsolatedSchemaPool(t)
	if acct, err := build(&no, payments.NewServiceWithPool(cov1xClosedCopy(t, pool))).StripeAccountFor(uuid.NewString()); err == nil || acct != "" {
		t.Fatalf("lookup failure = %q %v, want an error", acct, err)
	}

	// A stored account row with a blank Stripe id is as good as none while Connect is required.
	blankOrg := uuid.NewString()
	if _, err := pool.Exec(context.Background(), `INSERT INTO org_payment_accounts (org_id, stripe_account_id) VALUES ($1, '')`, blankOrg); err != nil {
		t.Skipf("schema refuses a blank account id, so that state cannot exist: %v", err)
	}
	dbPay := payments.NewServiceWithPool(pool)
	if _, err := build(&yes, dbPay).StripeAccountFor(blankOrg); err != billing.ErrPaymentAccountNotConnected {
		t.Fatalf("blank account id with Connect required = %v, want ErrPaymentAccountNotConnected", err)
	}
	if acct, err := build(&no, dbPay).StripeAccountFor(blankOrg); err != nil || acct != "" {
		t.Fatalf("blank account id with Connect optional = %q %v", acct, err)
	}
}

// cov1yStripe records the intents the API asks Stripe for; Stripe is the real
// external boundary here.
type cov1yStripe struct {
	stripelib.Client
	mu      sync.Mutex
	intents []stripelib.PaymentIntentParams
}

func (s *cov1yStripe) CreatePaymentIntent(ctx context.Context, p stripelib.PaymentIntentParams) (*stripelib.PaymentIntentResult, error) {
	s.mu.Lock()
	s.intents = append(s.intents, p)
	s.mu.Unlock()
	return s.Client.CreatePaymentIntent(ctx, p)
}

func (s *cov1yStripe) currencies() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.intents))
	for _, p := range s.intents {
		out = append(out, p.Currency)
	}
	return out
}

type cov1yDBClub struct {
	t       *testing.T
	pool    *pgxpool.Pool
	router  http.Handler
	org, hh string
	admin   string
	parent  string
	swimmer string
	billing *billing.Service
	stripe  *cov1yStripe
}

func (c *cov1yDBClub) do(token, method, path, body string, header ...string) *httptest.ResponseRecorder {
	c.t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-Id", c.org)
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	rr := httptest.NewRecorder()
	c.router.ServeHTTP(rr, req)
	return rr
}

func cov1yNewDBClub(t *testing.T) *cov1yDBClub {
	t.Helper()
	pool := testutil.IsolatedSchemaPool(t)
	cov1xUseDBPool(t, pool)
	// The test database role is a superuser, which the production role check refuses.
	origRole, origExit := validateDBRole, exitOnStartupFailure
	t.Cleanup(func() { validateDBRole, exitOnStartupFailure = origRole, origExit })
	validateDBRole = func(context.Context, *pgxpool.Pool) error { return nil }
	exitOnStartupFailure = func(_ *slog.Logger, msg string, err error) { t.Errorf("unexpected startup failure: %s: %v", msg, err) }
	c := &cov1yDBClub{t: t, pool: pool, org: uuid.NewString(), hh: uuid.NewString()}
	ctx := context.Background()
	adminID, parentID := uuid.NewString(), uuid.NewString()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO users (id, org_id, email, oidc_subject) VALUES ($1, $2, $3, $4)`, []any{adminID, c.org, "admin-" + adminID[:8] + "@example.test", "sub-" + adminID}},
		{`INSERT INTO users (id, org_id, email, oidc_subject) VALUES ($1, $2, $3, $4)`, []any{parentID, c.org, "parent-" + parentID[:8] + "@example.test", "sub-" + parentID}},
		{`INSERT INTO households (id, org_id, household_name) VALUES ($1, $2, 'Closure Family')`, []any{c.hh, c.org}},
	} {
		if _, err := pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	c.billing = billing.NewServiceWithPool(pool)
	c.stripe = &cov1yStripe{Client: stripelib.NewStubClient()}
	no := false
	c.router = NewRouterWithServices(Config{
		// Staging is the shape where providers have no dev stubs and the sender policy is enforced.
		AppEnv: "staging", DatabaseURL: "postgres://fixture.invalid/aquify", Auth: AuthConfig{EnableDevStubTokens: true, AllowDevSessionIssue: true},
		Stripe: StripeConfig{Currency: "cad", ConnectRequired: &no},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), RouterDeps{Billing: c.billing, Stripe: c.stripe})
	c.admin = issueSession(t, c.router, sessionSpec{UserID: adminID, OrgID: c.org, Roles: []string{"admin"}, MFA: true})
	c.parent = issueSession(t, c.router, sessionSpec{UserID: parentID, OrgID: c.org, HouseholdID: c.hh, Roles: []string{"parent"}})
	c.swimmer = issueSession(t, c.router, sessionSpec{UserID: uuid.NewString(), OrgID: c.org, Roles: []string{"swimmer"}})
	return c
}

// payInvoiceIntent issues a fresh invoice and asks for a payment intent on it.
func (c *cov1yDBClub) payInvoiceIntent(number string) *httptest.ResponseRecorder {
	c.t.Helper()
	sys := engagementSystemPrincipal(c.org)
	inv, err := c.billing.CreateInvoice(c.org, "", sys, billing.CreateInvoiceInput{HouseholdID: c.hh, InvoiceNumber: number})
	if err != nil {
		c.t.Fatal(err)
	}
	if _, err := c.billing.AddInvoiceItem(c.org, "", sys, inv.ID, billing.AddInvoiceItemInput{Description: "Dues", Quantity: 1, UnitAmountCents: 4000, Category: "manual"}); err != nil {
		c.t.Fatal(err)
	}
	if _, _, err := c.billing.IssueInvoice(c.org, "", sys, inv.ID); err != nil {
		c.t.Fatal(err)
	}
	return c.do(c.admin, http.MethodPost, "/api/v1/stripe/payment-intents",
		`{"invoice_id":"`+inv.ID+`","amount_cents":4000}`, "Idempotency-Key", "cov1y-"+number)
}

func (c *cov1yDBClub) clubEventInvoiceDue(start time.Time, title string) string {
	c.t.Helper()
	body, _ := json.Marshal(map[string]any{
		"title": title, "category": "banquet", "starts_at": start.Format(time.RFC3339),
		"rsvp_enabled": true, "rsvp_mode": "household", "fee_cents": 1500, "audience": "all", "status": "scheduled",
	})
	rr := c.do(c.admin, http.MethodPost, "/api/v1/club-events", string(body))
	if rr.Code != http.StatusCreated {
		c.t.Fatalf("create event: %d %s", rr.Code, rr.Body.String())
	}
	var created struct {
		Event struct{ ID string } `json:"event"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil || created.Event.ID == "" {
		c.t.Fatalf("event id: %v %s", err, rr.Body.String())
	}
	rr = c.do(c.parent, http.MethodPut, "/api/v1/club-events/"+created.Event.ID+"/rsvps", `{"household_id":"`+c.hh+`","response":"yes"}`)
	if rr.Code != http.StatusOK && rr.Code != http.StatusCreated {
		c.t.Fatalf("rsvp: %d %s", rr.Code, rr.Body.String())
	}
	var due string
	if err := c.pool.QueryRow(context.Background(), `SELECT due_date::text FROM invoices WHERE household_id = $1 AND external_ref LIKE $2`,
		c.hh, "club_event:"+created.Event.ID+":%").Scan(&due); err != nil {
		c.t.Fatalf("rsvp invoice: %v", err)
	}
	return due
}

// The club settings drive currency, calendar names, fee due-date timezones, the
// campaign sender and swimmer sign-in. When they cannot be read, each consumer
// must degrade to a safe default (or refuse) rather than act on a guess.
func TestCov1yRouterDegradesSafelyWhenClubSettingsAreUnreadable(t *testing.T) {
	c := cov1yNewDBClub(t)
	start := time.Now().UTC().AddDate(0, 0, 14).Truncate(24 * time.Hour).Add(3 * time.Hour) // previous evening in Los Angeles
	utcDay, laDay := start.Format("2006-01-02"), start.AddDate(0, 0, -1).Format("2006-01-02")

	if rr := c.do(c.admin, http.MethodPut, "/api/v1/org-settings",
		`{"team_name":"Cov Aquatics","timezone":"America/Los_Angeles","currency":"gbp","email_sender_name":"Cov Club","email_sender_address":"Club@Example.test"}`); rr.Code != http.StatusOK {
		t.Fatalf("org settings: %d %s", rr.Code, rr.Body.String())
	}
	tokenResp := c.do(c.parent, http.MethodPost, "/api/v1/calendar/feed-token", "")
	var tok struct {
		FeedPath string `json:"feed_path"`
	}
	if tokenResp.Code != http.StatusCreated || json.Unmarshal(tokenResp.Body.Bytes(), &tok) != nil || tok.FeedPath == "" {
		t.Fatalf("feed token: %d %s", tokenResp.Code, tokenResp.Body.String())
	}
	campaign := `{"name":"News","subject":"Hello","body_html":"<p>Hi</p>","from_name":"Custom Sender","from_email":"someone-else@example.test","audience":"all"}`
	feedBody := func() string {
		rr := doRequest(c.router, http.MethodGet, tok.FeedPath, "", "", "")
		if rr.Code != http.StatusOK {
			t.Fatalf("feed: %d %s", rr.Code, rr.Body.String())
		}
		return rr.Body.String()
	}

	// Readable settings: the club's own values win.
	if rr := c.payInvoiceIntent("COV1Y-1"); rr.Code != http.StatusOK && rr.Code != http.StatusCreated {
		t.Fatalf("payment intent: %d %s", rr.Code, rr.Body.String())
	}
	if got := c.stripe.currencies(); len(got) != 1 || got[0] != "gbp" {
		t.Fatalf("currency with settings = %v", got)
	}
	if feed := feedBody(); !strings.Contains(feed, "X-WR-CALNAME:Cov Aquatics calendar") || !strings.Contains(feed, "X-WR-TIMEZONE:America/Los_Angeles") {
		t.Fatalf("feed should carry the club profile:\n%s", feed)
	}
	if due := c.clubEventInvoiceDue(start, "Banquet A"); due != laDay {
		t.Fatalf("fee due date with a club timezone = %s, want %s", due, laDay)
	}
	rr := c.do(c.admin, http.MethodPost, "/api/v1/email-campaigns", campaign)
	if rr.Code != http.StatusCreated {
		t.Fatalf("campaign: %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"club@example.test"`) || strings.Contains(rr.Body.String(), "someone-else@example.test") {
		t.Fatalf("the sender must be pinned to the club's configured address: %s", rr.Body.String())
	}

	// Unreadable settings.
	cov1yDropTable(t, c.pool, "org_settings")

	if rr := c.payInvoiceIntent("COV1Y-2"); rr.Code != http.StatusOK && rr.Code != http.StatusCreated {
		t.Fatalf("payment intent without settings: %d %s", rr.Code, rr.Body.String())
	}
	if got := c.stripe.currencies(); len(got) != 2 || got[1] != "cad" {
		t.Fatalf("currency without settings = %v, want the deployment default", got)
	}
	feed := feedBody()
	if !strings.Contains(feed, "X-WR-CALNAME:Swim club calendar") || strings.Contains(feed, "X-WR-TIMEZONE") {
		t.Fatalf("feed should fall back to the generic profile:\n%s", feed)
	}
	if due := c.clubEventInvoiceDue(start.Add(time.Hour), "Banquet B"); due != utcDay {
		t.Fatalf("fee due date without a timezone = %s, want the UTC date %s", due, utcDay)
	}
	if rr := c.do(c.admin, http.MethodPost, "/api/v1/email-campaigns", campaign); rr.Code != http.StatusUnprocessableEntity || !strings.Contains(rr.Body.String(), "sender_not_configured") {
		t.Fatalf("campaigns must be refused without a verified sender: %d %s", rr.Code, rr.Body.String())
	}
	if rr := c.do(c.swimmer, http.MethodGet, "/api/v1/members/households", ""); rr.Code != http.StatusInternalServerError || !strings.Contains(rr.Body.String(), "Could not verify swimmer access") {
		t.Fatalf("a swimmer whose access cannot be verified must be refused: %d %s", rr.Code, rr.Body.String())
	}
	if rr := c.do(c.parent, http.MethodGet, "/api/v1/members/households", ""); rr.Code != http.StatusOK {
		t.Fatalf("parents are unaffected by swimmer sign-in checks: %d %s", rr.Code, rr.Body.String())
	}
}

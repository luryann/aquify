package stripe

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	stripeapi "github.com/stripe/stripe-go/v84"
)

type cov1xReq struct {
	Method, Path, Account, IdemKey string
	Form                           url.Values
}

// cov1xSDK builds an SDKClient whose HTTP transport answers via handler and
// records every request it sees.
func cov1xSDK(t *testing.T, handler func(r *http.Request) (int, string)) (*SDKClient, *[]cov1xReq) {
	t.Helper()
	var reqs []cov1xReq
	rt := sdkTransport(func(r *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(b))
		if r.Method == http.MethodGet {
			form = r.URL.Query()
		}
		reqs = append(reqs, cov1xReq{r.Method, r.URL.Path, r.Header.Get("Stripe-Account"), r.Header.Get("Idempotency-Key"), form})
		code, body := handler(r)
		return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	be := stripeapi.GetBackendWithConfig(stripeapi.APIBackend, &stripeapi.BackendConfig{
		MaxNetworkRetries: stripeapi.Int64(0),
		HTTPClient:        &http.Client{Transport: rt},
	})
	return newSDKClientWithBackend("sk_test_cov1x", be), &reqs
}

const cov1xErrBody = `{"error":{"type":"invalid_request_error","message":"cov1x boom"}}`

func cov1xFail(*http.Request) (int, string) { return 400, cov1xErrBody }

func cov1xWantStripeErr(t *testing.T, err error) {
	t.Helper()
	var se *stripeapi.Error
	if !errors.As(err, &se) || !strings.Contains(se.Msg, "cov1x boom") {
		t.Fatalf("want stripe API error, got %v", err)
	}
}

func TestCov1xVerifySignatureBranches(t *testing.T) {
	payload := []byte(`{"id":"evt_1"}`)
	const secret = "whsec_cov1x"
	now := time.Now()
	good := signedHeader(payload, secret, now)
	sig := good[strings.Index(good, "v1="):]

	cases := []struct {
		name, header, secret string
		want                 error
	}{
		{"empty secret", good, "  ", ErrInvalidSignature},
		{"bad timestamp", "t=abc," + sig, secret, ErrMalformedHeader},
		{"missing v1", "t=123", secret, ErrMalformedHeader},
		{"missing t", sig, secret, ErrMalformedHeader},
		{"stale", signedHeader(payload, secret, now.Add(-time.Hour)), secret, ErrExpiredTimestamp},
		{"wrong secret", good, "whsec_other", ErrInvalidSignature},
		{"non-hex v1 skipped then mismatch", "t=" + good[2:strings.Index(good, ",")] + ",v1=zz", secret, ErrInvalidSignature},
	}
	for _, tc := range cases {
		if err := VerifyStripeSignature(payload, tc.header, tc.secret, time.Minute); !errors.Is(err, tc.want) {
			t.Errorf("%s: want %v, got %v", tc.name, tc.want, err)
		}
	}
	// A bad v1 followed by a good one still verifies.
	if err := VerifyStripeSignature(payload, good+",v1=zz", secret, 0); err != nil {
		t.Errorf("valid signature among several rejected: %v", err)
	}
}

func TestCov1xParseWebhookPayload(t *testing.T) {
	if _, err := parseWebhookPayload([]byte("{nope")); err == nil || !strings.Contains(err.Error(), "failed to parse webhook payload") {
		t.Fatalf("want parse error, got %v", err)
	}
	p := []byte(`{"id":"evt_9","type":"x.y","created":42,"account":" acct_1 ","data":{"object":{"metadata":{"a":"b","n":5}}}}`)
	ev, err := parseWebhookPayload(p)
	if err != nil {
		t.Fatal(err)
	}
	if ev.ID != "evt_9" || ev.Type != "x.y" || ev.Created != 42 || ev.Account != "acct_1" {
		t.Errorf("bad event: %+v", ev)
	}
	if len(ev.Metadata) != 1 || ev.Metadata["a"] != "b" {
		t.Errorf("non-string metadata must be dropped: %v", ev.Metadata)
	}
	p[0] = 'X'
	if string(ev.RawPayload)[0] != '{' {
		t.Error("RawPayload must be a copy")
	}
}

func TestCov1xStubClient(t *testing.T) {
	ctx := context.Background()
	c := NewStubClient()
	if err := c.Ready(ctx); err != nil {
		t.Fatal(err)
	}

	pi, err := c.CreatePaymentIntent(ctx, PaymentIntentParams{AmountCents: 500, IdempotencyKey: " k1 ", StripeAccount: "acct_a"})
	if err != nil || pi.Currency != "usd" || pi.AmountCents != 500 {
		t.Fatalf("intent: %+v %v", pi, err)
	}
	again, _ := c.CreatePaymentIntent(ctx, PaymentIntentParams{AmountCents: 500, IdempotencyKey: "k1"})
	if again != pi || len(c.CreatedIntents()) != 1 {
		t.Error("idempotent retry must return the same intent")
	}
	if got := c.IntentAccounts(); len(got) != 1 || got[0] != "acct_a" {
		t.Errorf("intent accounts: %v", got)
	}

	cs, err := c.CreateCheckoutSession(ctx, CheckoutSessionParams{IdempotencyKey: "s1", StripeAccount: "acct_b"})
	if err != nil || !strings.HasPrefix(cs.URL, "https://checkout.stripe.com/pay/cs_stub_") {
		t.Fatalf("session: %+v %v", cs, err)
	}
	cs2, _ := c.CreateCheckoutSession(ctx, CheckoutSessionParams{IdempotencyKey: "s1"})
	if cs2 != cs || len(c.CreatedSessions()) != 1 {
		t.Error("idempotent retry must return the same session")
	}
	if got := c.SessionAccounts(); len(got) != 1 || got[0] != "acct_b" {
		t.Errorf("session accounts: %v", got)
	}

	ev, err := c.ParseWebhookEvent([]byte(`{"id":"e","type":"t"}`), "", "")
	if err != nil || ev.ID != "e" {
		t.Fatalf("stub webhook: %+v %v", ev, err)
	}

	c.BalanceCents = 77
	c.SetAccountBalance("acct_a", 900)
	if v, _ := c.FetchBalanceCents(ctx, ""); v != 77 {
		t.Errorf("platform balance %d", v)
	}
	if v, _ := c.FetchBalanceCents(ctx, "acct_a"); v != 900 {
		t.Errorf("account balance %d", v)
	}

	r, err := c.CreateRefund(ctx, RefundParams{PaymentIntentID: "pi_1", AmountCents: 25})
	if err != nil || r.Status != "succeeded" || r.AmountCents != 25 || len(c.CreatedRefunds()) != 1 {
		t.Fatalf("refund: %+v %v", r, err)
	}

	boom := errors.New("cov1x stub err")
	c.CreatePaymentIntentErr, c.CreateCheckoutSessionErr, c.CreateRefundErr = boom, boom, boom
	if _, err := c.CreatePaymentIntent(ctx, PaymentIntentParams{}); err != boom {
		t.Error("intent err not returned")
	}
	if _, err := c.CreateCheckoutSession(ctx, CheckoutSessionParams{}); err != boom {
		t.Error("session err not returned")
	}
	if _, err := c.CreateRefund(ctx, RefundParams{}); err != boom {
		t.Error("refund err not returned")
	}
}

func TestCov1xStubConnectedAccounts(t *testing.T) {
	ctx := context.Background()
	c := NewStubClient()
	if _, err := c.GetConnectedAccount(ctx, "acct_missing"); err == nil {
		t.Fatal("unknown account must error")
	}
	c.MarkAccountReady("acct_missing") // no-op

	c.AccountsNotReady = true
	a, err := c.CreateConnectedAccount(ctx, ConnectedAccountParams{OrgID: "o"})
	if err != nil || a.ChargesEnabled || a.PayoutsEnabled || a.DetailsSubmitted {
		t.Fatalf("not-ready account: %+v %v", a, err)
	}
	link, _ := c.CreateAccountLink(ctx, a.ID, "r", "b")
	if link != "https://connect.stripe.com/setup/"+a.ID {
		t.Errorf("link %s", link)
	}
	c.MarkAccountReady(a.ID)
	got, err := c.GetConnectedAccount(ctx, a.ID)
	if err != nil || !got.ChargesEnabled || !got.PayoutsEnabled || !got.DetailsSubmitted {
		t.Fatalf("ready account: %+v %v", got, err)
	}
}

type cov1xTimeoutErr struct{}

func (cov1xTimeoutErr) Error() string   { return "timed out" }
func (cov1xTimeoutErr) Timeout() bool   { return true }
func (cov1xTimeoutErr) Temporary() bool { return false }

type cov1xNetErr struct{}

func (cov1xNetErr) Error() string   { return "refused" }
func (cov1xNetErr) Timeout() bool   { return false }
func (cov1xNetErr) Temporary() bool { return false }

func TestCov1xIsTimeout(t *testing.T) {
	if IsTimeout(nil) {
		t.Error("nil is not a timeout")
	}
	if !IsTimeout(context.DeadlineExceeded) {
		t.Error("deadline exceeded is a timeout")
	}
	if !IsTimeout(cov1xTimeoutErr{}) {
		t.Error("net timeout is a timeout")
	}
	if IsTimeout(cov1xNetErr{}) || IsTimeout(errors.New("plain")) {
		t.Error("non-timeout errors misclassified")
	}
}

func TestCov1xNewSDKClient(t *testing.T) {
	if _, err := NewSDKClient("  "); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("want ErrInvalidConfig, got %v", err)
	}
	c, err := NewSDKClient(" sk_test_x ")
	if err != nil || c.secretKey != "sk_test_x" {
		t.Fatalf("client: %+v %v", c, err)
	}
}

func TestCov1xSDKSuccessPaths(t *testing.T) {
	c, reqs := cov1xSDK(t, func(r *http.Request) (int, string) {
		switch {
		case r.URL.Path == "/v1/account":
			return 200, `{"id":"acct_self"}`
		case r.URL.Path == "/v1/payment_intents":
			return 200, `{"id":"pi_1","client_secret":"pi_1_sec","status":"requires_payment_method","currency":"usd","amount":1234}`
		case r.URL.Path == "/v1/checkout/sessions":
			return 200, `{"id":"cs_1","url":"https://checkout.example/cs_1"}`
		case r.URL.Path == "/v1/balance_transactions":
			return 200, `{"object":"list","data":[{"id":"txn_1","amount":500},{"id":"txn_2","amount":250}],"has_more":false}`
		case r.URL.Path == "/v1/refunds":
			return 200, `{"id":"re_1","status":"succeeded","amount":300}`
		case r.URL.Path == "/v1/accounts" && r.Method == http.MethodPost:
			return 200, `{"id":"acct_new","charges_enabled":false,"payouts_enabled":true,"details_submitted":true}`
		case r.URL.Path == "/v1/account_links":
			return 200, `{"url":"https://connect.example/onboard"}`
		case r.URL.Path == "/v1/accounts/acct_get":
			return 200, `{"id":"acct_get","charges_enabled":true,"payouts_enabled":true,"details_submitted":true}`
		}
		return 404, cov1xErrBody
	})
	ctx := context.Background()

	if err := c.Ready(ctx); err != nil {
		t.Fatal(err)
	}

	pi, err := c.CreatePaymentIntent(ctx, PaymentIntentParams{AmountCents: 1234, Currency: " USD ", Description: "dues", Metadata: map[string]string{"invoice": "i1"}, IdempotencyKey: " idem-pi ", StripeAccount: " acct_x "})
	if err != nil || *pi != (PaymentIntentResult{ID: "pi_1", ClientSecret: "pi_1_sec", Status: "requires_payment_method", Currency: "usd", AmountCents: 1234}) {
		t.Fatalf("intent: %+v %v", pi, err)
	}
	r := (*reqs)[len(*reqs)-1]
	if r.IdemKey != "idem-pi" || r.Account != "acct_x" || r.Form.Get("currency") != "usd" || r.Form.Get("amount") != "1234" || r.Form.Get("metadata[invoice]") != "i1" {
		t.Errorf("intent request: %+v", r)
	}

	cs, err := c.CreateCheckoutSession(ctx, CheckoutSessionParams{AmountCents: 900, Currency: "USD", Description: "fees", SuccessURL: "https://ok", CancelURL: "https://no", Metadata: map[string]string{"org": "o1"}, IdempotencyKey: "idem-cs", StripeAccount: "acct_y"})
	if err != nil || cs.ID != "cs_1" || cs.URL != "https://checkout.example/cs_1" {
		t.Fatalf("session: %+v %v", cs, err)
	}
	r = (*reqs)[len(*reqs)-1]
	if r.IdemKey != "idem-cs" || r.Account != "acct_y" || r.Form.Get("mode") != "payment" ||
		r.Form.Get("payment_intent_data[metadata][org]") != "o1" || r.Form.Get("line_items[0][price_data][unit_amount]") != "900" {
		t.Errorf("session request: %+v", r)
	}

	bal, err := c.FetchBalanceCents(ctx, " acct_z ")
	if err != nil || bal != 750 {
		t.Fatalf("balance %d %v", bal, err)
	}
	r = (*reqs)[len(*reqs)-1]
	if r.Account != "acct_z" || r.Form.Get("type") != "charge" {
		t.Errorf("balance request: %+v", r)
	}
	if _, err := c.FetchBalanceCents(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if (*reqs)[len(*reqs)-1].Account != "" {
		t.Error("platform balance must not set Stripe-Account")
	}

	for reason, want := range map[string]string{" Fraudulent ": "fraudulent", "DUPLICATE": "duplicate", "": "requested_by_customer", "other": "requested_by_customer"} {
		ref, err := c.CreateRefund(ctx, RefundParams{PaymentIntentID: "pi_1", AmountCents: 300, Reason: reason, IdempotencyKey: "idem-re", StripeAccount: "acct_r"})
		if err != nil || ref.ID != "re_1" || ref.Status != "succeeded" || ref.AmountCents != 300 {
			t.Fatalf("refund: %+v %v", ref, err)
		}
		r = (*reqs)[len(*reqs)-1]
		if r.Form.Get("reason") != want || r.Form.Get("payment_intent") != "pi_1" || r.IdemKey != "idem-re" || r.Account != "acct_r" {
			t.Errorf("refund reason %q: %+v", reason, r)
		}
	}

	acct, err := c.CreateConnectedAccount(ctx, ConnectedAccountParams{OrgID: "org1", Email: " a@b.test ", IdempotencyKey: " idem-acct "})
	if err != nil || *acct != (ConnectedAccount{ID: "acct_new", ChargesEnabled: false, PayoutsEnabled: true, DetailsSubmitted: true}) {
		t.Fatalf("account: %+v %v", acct, err)
	}
	r = (*reqs)[len(*reqs)-1]
	if r.Form.Get("type") != "standard" || r.Form.Get("email") != "a@b.test" || r.Form.Get("metadata[org_id]") != "org1" || r.IdemKey != "idem-acct" {
		t.Errorf("account request: %+v", r)
	}
	if _, err := c.CreateConnectedAccount(ctx, ConnectedAccountParams{OrgID: "org1"}); err != nil {
		t.Fatal(err)
	}
	r = (*reqs)[len(*reqs)-1]
	if _, ok := r.Form["email"]; ok {
		t.Error("empty email must be omitted")
	}

	link, err := c.CreateAccountLink(ctx, "acct_new", "https://refresh", "https://return")
	if err != nil || link != "https://connect.example/onboard" {
		t.Fatalf("link %q %v", link, err)
	}
	r = (*reqs)[len(*reqs)-1]
	if r.Form.Get("account") != "acct_new" || r.Form.Get("type") != "account_onboarding" || r.Form.Get("return_url") != "https://return" {
		t.Errorf("link request: %+v", r)
	}

	got, err := c.GetConnectedAccount(ctx, "acct_get")
	if err != nil || *got != (ConnectedAccount{ID: "acct_get", ChargesEnabled: true, PayoutsEnabled: true, DetailsSubmitted: true}) {
		t.Fatalf("get account: %+v %v", got, err)
	}
}

func TestCov1xSDKErrorPaths(t *testing.T) {
	c, _ := cov1xSDK(t, cov1xFail)
	ctx := context.Background()

	cov1xWantStripeErr(t, c.Ready(ctx))
	if _, err := c.CreatePaymentIntent(ctx, PaymentIntentParams{AmountCents: 1, Currency: "usd"}); true {
		cov1xWantStripeErr(t, err)
	}
	if _, err := c.CreateCheckoutSession(ctx, CheckoutSessionParams{AmountCents: 1, Currency: "usd"}); true {
		cov1xWantStripeErr(t, err)
	}
	if _, err := c.FetchBalanceCents(ctx, ""); true {
		cov1xWantStripeErr(t, err)
	}
	if _, err := c.CreateRefund(ctx, RefundParams{PaymentIntentID: "pi", AmountCents: 1}); true {
		cov1xWantStripeErr(t, err)
	}
	if _, err := c.CreateConnectedAccount(ctx, ConnectedAccountParams{OrgID: "o"}); true {
		cov1xWantStripeErr(t, err)
	}
	if link, err := c.CreateAccountLink(ctx, "a", "r", "b"); link != "" {
		t.Errorf("link on error: %q", link)
	} else {
		cov1xWantStripeErr(t, err)
	}
	if a, err := c.GetConnectedAccount(ctx, "acct_1"); a != nil {
		t.Errorf("account on error: %+v", a)
	} else {
		cov1xWantStripeErr(t, err)
	}
}

func TestCov1xSDKParseWebhookEvent(t *testing.T) {
	c, _ := cov1xSDK(t, cov1xFail)
	payload := []byte(`{"id":"evt_s","type":"payment_intent.succeeded","data":{"object":{"metadata":{"k":"v"}}}}`)
	const secret = "whsec_cov1x"

	ev, err := c.ParseWebhookEvent(payload, signedHeader(payload, secret, time.Now()), secret)
	if err != nil || ev.ID != "evt_s" || ev.Metadata["k"] != "v" {
		t.Fatalf("verified event: %+v %v", ev, err)
	}
	if _, err := c.ParseWebhookEvent(payload, signedHeader(payload, "whsec_wrong", time.Now()), secret); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("want ErrInvalidSignature, got %v", err)
	}
}

package jobhandlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/luryann/aquify/server/internal/testutil"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/luryann/aquify/server/internal/app/billing"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	stripelib "github.com/luryann/aquify/server/internal/stripe"
	"github.com/luryann/aquify/server/internal/workers"
)

const cov1xBillOrg = "org-cov1x-billing"

func cov1xDiscardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func cov1xBufLogger() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return slog.New(slog.NewTextHandler(buf, nil)), buf
}

func cov1xRawTask(typ, payload string) *workers.Task {
	return &workers.Task{Type: typ, Payload: json.RawMessage(payload)}
}

func cov1xTask(t *testing.T, typ string, payload any) *workers.Task {
	t.Helper()
	task, err := workers.NewTask(typ, payload)
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}
	return task
}

// cov1xBrokenBilling returns a DB-backed billing service whose pool was closed
// before first use, so every read that must reach the database fails.
func cov1xBrokenBilling(t *testing.T) *billing.Service {
	t.Helper()
	return billing.NewServiceWithPool(testutil.ClosedPool(t))
}

type cov1xFailingBalanceStripe struct {
	stripelib.Client
	err error
}

func (c cov1xFailingBalanceStripe) FetchBalanceCents(context.Context, string) (int, error) {
	return 0, c.err
}

func TestCov1xBillingReconcileHandlerRejectsBadInput(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	stub := &stripelib.StubClient{}
	cases := []struct {
		name    string
		handler workers.Handler
		payload string
		want    string
	}{
		{"bad json", BillingReconcileHandler(cov1xDiscardLogger(), svc, stub, 100), `{`, "billing reconcile: decode payload"},
		{"missing org", BillingReconcileHandler(cov1xDiscardLogger(), svc, stub, 100), `{}`, "billing reconcile: missing org_id"},
		{"nil billing", BillingReconcileHandler(cov1xDiscardLogger(), nil, stub, 100), `{"org_id":"o"}`, "billing service is not configured"},
		{"nil stripe", BillingReconcileHandler(cov1xDiscardLogger(), svc, nil, 100), `{"org_id":"o"}`, "stripe client is not configured"},
	}
	for _, tc := range cases {
		err := tc.handler(context.Background(), cov1xRawTask(workers.TypeBillingReconcile, tc.payload))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want containing %q", tc.name, err, tc.want)
		}
	}
}

func TestCov1xBillingReconcileSkipsClubWithoutConnectedAccount(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	svc.SetStripeAccountResolver(func(string) (string, error) { return "", billing.ErrPaymentAccountNotConnected })
	var fetched atomic.Int32
	stripeClient := &cov1xCountingStripe{Client: &stripelib.StubClient{}, fetched: &fetched}
	logger, buf := cov1xBufLogger()
	err := BillingReconcileHandler(logger, svc, stripeClient, 100)(context.Background(),
		cov1xTask(t, workers.TypeBillingReconcile, BillingReconcilePayload{OrgID: cov1xBillOrg}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if fetched.Load() != 0 {
		t.Fatal("must not query the platform Stripe balance for a club without a connected account")
	}
	if !strings.Contains(buf.String(), "no connected payment account") {
		t.Fatalf("expected skip log, got %q", buf.String())
	}
}

type cov1xCountingStripe struct {
	stripelib.Client
	fetched *atomic.Int32
}

func (c *cov1xCountingStripe) FetchBalanceCents(ctx context.Context, acct string) (int, error) {
	c.fetched.Add(1)
	return c.Client.FetchBalanceCents(ctx, acct)
}

func TestCov1xBillingReconcileResolverFailureIsWrapped(t *testing.T) {
	t.Parallel()
	boom := errors.New("resolver down")
	svc := billing.NewService()
	svc.SetStripeAccountResolver(func(string) (string, error) { return "", boom })
	err := BillingReconcileHandler(cov1xDiscardLogger(), svc, &stripelib.StubClient{}, 100)(context.Background(),
		cov1xTask(t, workers.TypeBillingReconcile, BillingReconcilePayload{OrgID: cov1xBillOrg}))
	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "resolve stripe account") {
		t.Fatalf("err = %v", err)
	}
}

func TestCov1xBillingReconcileStripeBalanceFailure(t *testing.T) {
	t.Parallel()
	boom := errors.New("stripe 500")
	stripeClient := cov1xFailingBalanceStripe{Client: &stripelib.StubClient{}, err: boom}
	err := BillingReconcileHandler(cov1xDiscardLogger(), billing.NewService(), stripeClient, 100)(context.Background(),
		cov1xTask(t, workers.TypeBillingReconcile, BillingReconcilePayload{OrgID: cov1xBillOrg}))
	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "fetch stripe balance") {
		t.Fatalf("err = %v", err)
	}
}

func TestCov1xBillingReconcileReportFailure(t *testing.T) {
	t.Parallel()
	boom := errors.New("second lookup failed")
	var calls atomic.Int32
	svc := billing.NewService()
	// The handler resolves the account once itself, then the report resolves it again.
	svc.SetStripeAccountResolver(func(string) (string, error) {
		if calls.Add(1) > 1 {
			return "", boom
		}
		return "", nil
	})
	err := BillingReconcileHandler(cov1xDiscardLogger(), svc, &stripelib.StubClient{}, 100)(context.Background(),
		cov1xTask(t, workers.TypeBillingReconcile, BillingReconcilePayload{OrgID: cov1xBillOrg}))
	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "compute report") {
		t.Fatalf("err = %v", err)
	}
}

func TestCov1xBillingReconcileLogsAlertWhenMismatchExceedsThreshold(t *testing.T) {
	t.Parallel()
	logger, buf := cov1xBufLogger()
	// Ledger has no Stripe payments; Stripe reports 5000 cents, so mismatch is 5000 > 100.
	err := BillingReconcileHandler(logger, billing.NewService(), &stripelib.StubClient{BalanceCents: 5000}, 100)(context.Background(),
		cov1xTask(t, workers.TypeBillingReconcile, BillingReconcilePayload{OrgID: cov1xBillOrg}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "mismatch exceeds threshold") || !strings.Contains(out, "mismatch_cents=5000") {
		t.Fatalf("expected alert log with mismatch, got %q", out)
	}
}

func TestCov1xNightlyIntegrityRejectsBadInput(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	cases := []struct {
		name    string
		svc     *billing.Service
		payload string
		want    string
	}{
		{"bad json", svc, `{`, "nightly integrity: decode payload"},
		{"missing org", svc, `{"date":"2026-07-01"}`, "missing org_id"},
		{"nil billing", nil, `{"org_id":"o"}`, "billing service is not configured"},
		{"invalid date", svc, `{"org_id":"o","date":"07/01/2026"}`, `invalid date "07/01/2026"`},
	}
	for _, tc := range cases {
		err := NightlyIntegrityHandler(cov1xDiscardLogger(), tc.svc)(context.Background(), cov1xRawTask(workers.TypeNightlyIntegrity, tc.payload))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want containing %q", tc.name, err, tc.want)
		}
	}
}

func TestCov1xNightlyIntegrityCancelledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := NightlyIntegrityHandler(cov1xDiscardLogger(), billing.NewService())(ctx,
		cov1xTask(t, workers.TypeNightlyIntegrity, NightlyIntegrityPayload{OrgID: cov1xBillOrg}))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestCov1xNightlyIntegritySnapshotLoadFailure(t *testing.T) {
	t.Parallel()
	err := NightlyIntegrityHandler(cov1xDiscardLogger(), cov1xBrokenBilling(t))(context.Background(),
		cov1xTask(t, workers.TypeNightlyIntegrity, NightlyIntegrityPayload{OrgID: cov1xBillOrg}))
	if err == nil || !strings.Contains(err.Error(), "nightly integrity: load billing state") {
		t.Fatalf("err = %v", err)
	}
}

type cov1xLedgerState struct {
	Invoices map[string]billing.Invoice       `json:"invoices"`
	Items    map[string][]billing.InvoiceItem `json:"items"`
	Payments map[string][]billing.Payment     `json:"payments"`
	Refunds  map[string][]billing.Refund      `json:"refunds"`
	Ledger   []billing.LedgerEvent            `json:"ledger"`
}

func cov1xGoodInvoice() billing.Invoice {
	return billing.Invoice{ID: "inv-1", OrgID: cov1xBillOrg, InvoiceNumber: "INV-1", Status: "issued",
		SubtotalCents: 1000, TotalCents: 1000, AmountDueCents: 1000}
}

func cov1xGoodState() cov1xLedgerState {
	return cov1xLedgerState{
		Invoices: map[string]billing.Invoice{"inv-1": cov1xGoodInvoice()},
		Items:    map[string][]billing.InvoiceItem{},
		Payments: map[string][]billing.Payment{},
		Refunds:  map[string][]billing.Refund{},
	}
}

func cov1xRunIntegrity(t *testing.T, st cov1xLedgerState) error {
	t.Helper()
	raw, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	svc := billing.NewService()
	if err := svc.LoadRuntimeSnapshot(raw); err != nil {
		t.Fatalf("LoadRuntimeSnapshot: %v", err)
	}
	return NightlyIntegrityHandler(cov1xDiscardLogger(), svc)(context.Background(),
		cov1xTask(t, workers.TypeNightlyIntegrity, NightlyIntegrityPayload{OrgID: cov1xBillOrg, Date: "2026-07-01"}))
}

func TestCov1xNightlyIntegrityAcceptsConsistentState(t *testing.T) {
	t.Parallel()
	st := cov1xGoodState()
	now := time.Now().UTC()
	st.Items["inv-1"] = []billing.InvoiceItem{{ID: "it-1", OrgID: cov1xBillOrg, InvoiceID: "inv-1", Quantity: 2, UnitAmountCents: 500, TotalCents: 1000, CreatedAt: now}}
	st.Payments["inv-1"] = []billing.Payment{{ID: "pay-1", OrgID: cov1xBillOrg, InvoiceID: "inv-1", AmountCents: 400, Status: "succeeded"}}
	st.Refunds["inv-1"] = []billing.Refund{{ID: "ref-1", OrgID: cov1xBillOrg, InvoiceID: "inv-1", PaymentID: "pay-1", AmountCents: 100, Status: "processed"}}
	st.Ledger = []billing.LedgerEvent{
		{ID: "l1", OrgID: cov1xBillOrg, InvoiceID: "inv-1", PaymentID: "pay-1", RefundID: "ref-1", EventType: "x", AmountCents: -100, CreatedAt: now},
		{ID: "l2", OrgID: cov1xBillOrg, EventType: "y", AmountCents: 5, CreatedAt: now},
	}
	if err := cov1xRunIntegrity(t, st); err != nil {
		t.Fatalf("consistent state rejected: %v", err)
	}
}

func TestCov1xNightlyIntegrityDetectsCorruption(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		mutate func(st *cov1xLedgerState)
		want   string
	}{
		{"invoice total mismatch", func(st *cov1xLedgerState) {
			inv := st.Invoices["inv-1"]
			inv.TaxCents = 50
			st.Invoices["inv-1"] = inv
		}, "invoice inv-1 total mismatch"},
		{"overpaid invoice balance mismatch", func(st *cov1xLedgerState) {
			st.Payments["inv-1"] = []billing.Payment{{ID: "pay-1", OrgID: cov1xBillOrg, InvoiceID: "inv-1", AmountCents: 1500, Status: "succeeded"}}
		}, "invoice inv-1 balance mismatch"},
		{"item missing invoice", func(st *cov1xLedgerState) {
			st.Items["ghost"] = []billing.InvoiceItem{{ID: "it-1", OrgID: cov1xBillOrg, InvoiceID: "ghost", Quantity: 1, UnitAmountCents: 5, TotalCents: 5}}
		}, "invoice item it-1 references missing invoice ghost"},
		{"item total mismatch", func(st *cov1xLedgerState) {
			st.Items["inv-1"] = []billing.InvoiceItem{{ID: "it-1", OrgID: cov1xBillOrg, InvoiceID: "inv-1", Quantity: 2, UnitAmountCents: 5, TotalCents: 99}}
		}, "invoice item it-1 total mismatch"},
		{"payment missing invoice", func(st *cov1xLedgerState) {
			st.Payments["ghost"] = []billing.Payment{{ID: "pay-1", OrgID: cov1xBillOrg, InvoiceID: "ghost", AmountCents: 5, Status: "succeeded"}}
		}, "payment pay-1 references missing invoice ghost"},
		{"refund missing invoice", func(st *cov1xLedgerState) {
			st.Refunds["ghost"] = []billing.Refund{{ID: "ref-1", OrgID: cov1xBillOrg, InvoiceID: "ghost", PaymentID: "pay-1", Status: "processed"}}
		}, "refund ref-1 references missing invoice ghost"},
		{"refund missing payment", func(st *cov1xLedgerState) {
			st.Refunds["inv-1"] = []billing.Refund{{ID: "ref-1", OrgID: cov1xBillOrg, InvoiceID: "inv-1", PaymentID: "nopay", Status: "pending"}}
		}, "refund ref-1 references missing payment nopay"},
		{"ledger missing invoice", func(st *cov1xLedgerState) {
			st.Ledger = []billing.LedgerEvent{{ID: "l1", OrgID: cov1xBillOrg, InvoiceID: "ghost", AmountCents: 1}}
		}, "ledger event l1 references missing invoice ghost"},
		{"ledger missing payment", func(st *cov1xLedgerState) {
			st.Ledger = []billing.LedgerEvent{{ID: "l1", OrgID: cov1xBillOrg, PaymentID: "ghost", AmountCents: 1}}
		}, "ledger event l1 references missing payment ghost"},
		{"ledger missing refund", func(st *cov1xLedgerState) {
			st.Ledger = []billing.LedgerEvent{{ID: "l1", OrgID: cov1xBillOrg, RefundID: "ghost", AmountCents: 1}}
		}, "ledger event l1 references missing refund ghost"},
		{"ledger zero amount", func(st *cov1xLedgerState) {
			st.Ledger = []billing.LedgerEvent{{ID: "l1", OrgID: cov1xBillOrg, AmountCents: 0}}
		}, "ledger event l1 has zero amount"},
	}
	for _, tc := range cases {
		st := cov1xGoodState()
		tc.mutate(&st)
		err := cov1xRunIntegrity(t, st)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want containing %q", tc.name, err, tc.want)
		}
	}
}

func TestCov1xBillingAutomationRejectsBadPayloads(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	for name, h := range map[string]workers.Handler{
		"recurring": BillingRecurringRunHandler(cov1xDiscardLogger(), svc),
		"latefees":  BillingLateFeesHandler(cov1xDiscardLogger(), svc),
	} {
		for payload, want := range map[string]string{
			`{`:                                  "decode payload",
			`{"date":"2026-01-01"}`:              "missing org_id",
			`{"org_id":"o","date":"01/02/2026"}`: `invalid date "01/02/2026"`,
		} {
			err := h(context.Background(), cov1xRawTask(workers.TypeBillingRecurringRun, payload))
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("%s %s: err = %v, want containing %q", name, payload, err, want)
			}
		}
	}
	for name, h := range map[string]workers.Handler{
		"recurring": BillingRecurringRunHandler(cov1xDiscardLogger(), nil),
		"latefees":  BillingLateFeesHandler(cov1xDiscardLogger(), nil),
	} {
		err := h(context.Background(), cov1xRawTask(workers.TypeBillingRecurringRun, `{"org_id":"o"}`))
		if err == nil || !strings.Contains(err.Error(), "billing service is not configured") {
			t.Errorf("%s nil service: err = %v", name, err)
		}
	}
}

func TestCov1xDecodeBillingAutomationDefaultsToToday(t *testing.T) {
	t.Parallel()
	p, day, err := decodeBillingAutomation(cov1xRawTask("t", `{"org_id":"o","triggered_by":"scheduler"}`), "job")
	if err != nil {
		t.Fatal(err)
	}
	if p.OrgID != "o" || p.TriggeredBy != "scheduler" {
		t.Fatalf("payload = %+v", p)
	}
	if d := time.Since(day); d < 0 || d > time.Minute {
		t.Fatalf("default day = %v, want ~now", day)
	}
	_, day, err = decodeBillingAutomation(cov1xRawTask("t", `{"org_id":"o","date":"2026-03-04"}`), "job")
	if err != nil || day.Format("2006-01-02") != "2026-03-04" {
		t.Fatalf("explicit day = %v err=%v", day, err)
	}
}

func cov1xFinRoster() billing.RosterSnapshot {
	oct := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	return billing.RosterSnapshot{
		Households:    []billing.RosterHousehold{{ID: "hh-a", Name: "Alpha", Status: "active"}},
		Swimmers:      []billing.RosterSwimmer{{ID: "sw-1", HouseholdID: "hh-a", FirstName: "Ann", LastName: "Alpha", DateOfBirth: "2012-01-01", Status: "active"}},
		GroupLinks:    []billing.RosterGroupLink{{SwimmerID: "sw-1", GroupCode: "SILVER", IsPrimary: true, AssignedAt: oct}},
		Registrations: []billing.RosterRegistration{{ID: "r1", HouseholdID: "hh-a", SwimmerID: "sw-1", GroupCode: "SILVER", Season: "2026-27", Status: "active"}},
		Groups:        []billing.RosterGroup{{Code: "SILVER", Name: "Silver"}},
	}
}

func cov1xAdmin() *appmiddleware.Principal {
	return &appmiddleware.Principal{UserID: "admin-cov1x", OrgID: cov1xBillOrg, Roles: []string{"admin"}}
}

func TestCov1xBillingRecurringRunNothingToBillWhenAutomationDisabled(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	logger, buf := cov1xBufLogger()
	err := BillingRecurringRunHandler(logger, svc)(context.Background(),
		cov1xTask(t, workers.TypeBillingRecurringRun, BillingAutomationPayload{OrgID: cov1xBillOrg, Date: "2026-11-20"}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !strings.Contains(buf.String(), "nothing to bill") {
		t.Fatalf("expected nothing-to-bill log, got %q", buf.String())
	}
	if n := len(svc.SnapshotInvoices(cov1xBillOrg)); n != 0 {
		t.Fatalf("invoices created = %d, want 0", n)
	}
}

func TestCov1xBillingRecurringRunCommitsRunAndIsIdempotent(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	svc.SetRosterSource(func(string) (billing.RosterSnapshot, error) { return cov1xFinRoster(), nil })
	p := cov1xAdmin()
	if _, err := svc.CreateFeeSchedule(cov1xBillOrg, p.UserID, p, billing.FeeScheduleInput{
		Name: "Silver dues", Kind: "dues", GroupCode: "SILVER", Frequency: "monthly", AmountCents: 15000,
		StartDate: "2026-10-01", Proration: "none", AutoBill: true,
	}); err != nil {
		t.Fatalf("CreateFeeSchedule: %v", err)
	}
	if _, _, err := svc.UpdateBillingSettings(cov1xBillOrg, p.UserID, p, billing.BillingSettingsInput{
		LateFeeType: billing.LateFeeTypeFlat, AutoRunEnabled: true, AutoRunDay: 1, AutoRunInvoiceStatus: "draft", AutoRunDueDays: 10,
	}); err != nil {
		t.Fatalf("UpdateBillingSettings: %v", err)
	}
	logger, buf := cov1xBufLogger()
	h := BillingRecurringRunHandler(logger, svc)
	task := cov1xTask(t, workers.TypeBillingRecurringRun, BillingAutomationPayload{OrgID: cov1xBillOrg, Date: "2026-11-20"})
	if err := h(context.Background(), task); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if !strings.Contains(buf.String(), "billing recurring run: completed") {
		t.Fatalf("expected completed log, got %q", buf.String())
	}
	invoices := svc.SnapshotInvoices(cov1xBillOrg)
	if len(invoices) == 0 {
		t.Fatal("expected the run to create invoices")
	}
	before := len(invoices)
	if err := h(context.Background(), task); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if after := len(svc.SnapshotInvoices(cov1xBillOrg)); after != before {
		t.Fatalf("re-run double-billed: %d -> %d invoices", before, after)
	}
}

func TestCov1xBillingRecurringRunSurfacesBillingFailure(t *testing.T) {
	t.Parallel()
	err := BillingRecurringRunHandler(cov1xDiscardLogger(), cov1xBrokenBilling(t))(context.Background(),
		cov1xTask(t, workers.TypeBillingRecurringRun, BillingAutomationPayload{OrgID: cov1xBillOrg, Date: "2026-11-20"}))
	if err == nil || !strings.Contains(err.Error(), "billing recurring run:") {
		t.Fatalf("err = %v", err)
	}
}

func TestCov1xBillingLateFeesAppliesOneFeeToOverdueInvoice(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	p := cov1xAdmin()
	if _, _, err := svc.UpdateBillingSettings(cov1xBillOrg, p.UserID, p, billing.BillingSettingsInput{
		LateFeeEnabled: true, LateFeeType: billing.LateFeeTypeFlat, LateFeeFlatCents: 500, LateFeeGraceDays: 0,
		AutoRunInvoiceStatus: "draft", AutoRunDay: 1, AutoRunDueDays: 10,
	}); err != nil {
		t.Fatalf("UpdateBillingSettings: %v", err)
	}
	inv, err := svc.CreateInvoice(cov1xBillOrg, p.UserID, p, billing.CreateInvoiceInput{InvoiceNumber: "LF-1", DueDate: "2026-10-01"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddInvoiceItem(cov1xBillOrg, p.UserID, p, inv.ID, billing.AddInvoiceItemInput{Description: "dues", Quantity: 1, UnitAmountCents: 10000, Category: "manual"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.IssueInvoice(cov1xBillOrg, p.UserID, p, inv.ID); err != nil {
		t.Fatal(err)
	}
	logger, buf := cov1xBufLogger()
	h := BillingLateFeesHandler(logger, svc)
	task := cov1xTask(t, workers.TypeBillingLateFees, BillingAutomationPayload{OrgID: cov1xBillOrg, Date: "2026-11-20"})
	for i := 0; i < 2; i++ {
		if err := h(context.Background(), task); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	logs := buf.String()
	if !strings.Contains(logs, "applied=1") || !strings.Contains(logs, "total_cents=500") {
		t.Fatalf("first sweep should apply one 500c fee: %q", logs)
	}
	if !strings.Contains(logs, "applied=0") {
		t.Fatalf("second sweep must not charge a second fee: %q", logs)
	}
	for _, got := range svc.SnapshotInvoices(cov1xBillOrg) {
		if got.ID == inv.ID && got.TotalCents != 10500 {
			t.Fatalf("invoice total = %d, want 10500 (one late fee)", got.TotalCents)
		}
	}
}

func TestCov1xBillingLateFeesSurfacesSweepFailure(t *testing.T) {
	t.Parallel()
	err := BillingLateFeesHandler(cov1xDiscardLogger(), cov1xBrokenBilling(t))(context.Background(),
		cov1xTask(t, workers.TypeBillingLateFees, BillingAutomationPayload{OrgID: cov1xBillOrg, Date: "2026-11-20"}))
	if err == nil || !strings.Contains(err.Error(), "billing late fees:") {
		t.Fatalf("err = %v", err)
	}
}

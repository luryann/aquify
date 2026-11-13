package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/billing"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	stripelib "github.com/luryann/aquify/server/internal/stripe"
)

func TestCov6xMapBillingErrorStatusAndCode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		err    error
		status int
		code   string
	}{
		{billing.ErrForbidden, http.StatusForbidden, "forbidden"},
		{billing.ErrInvoiceNotFound, http.StatusNotFound, "invoice_not_found"},
		{billing.ErrHouseholdNotFound, http.StatusNotFound, "household_not_found"},
		{billing.ErrInvoiceItemNotFound, http.StatusNotFound, "item_not_found"},
		{billing.ErrPaymentNotFound, http.StatusNotFound, "payment_not_found"},
		{billing.ErrRefundNotFound, http.StatusNotFound, "refund_not_found"},
		{billing.ErrInvoiceNotDraft, http.StatusConflict, "invoice_not_draft"},
		{billing.ErrInvoiceNotIssuable, http.StatusConflict, "invoice_not_issuable"},
		{billing.ErrInvoiceEmpty, http.StatusUnprocessableEntity, "invoice_empty"},
		{billing.ErrInvoiceNotVoidable, http.StatusConflict, "invoice_not_voidable"},
		{billing.ErrInvoiceNotWriteOffable, http.StatusConflict, "invoice_not_write_offable"},
		{billing.ErrInvalidTransition, http.StatusConflict, "invalid_transition"},
		{billing.ErrPaymentExceedsBalance, http.StatusUnprocessableEntity, "payment_exceeds_balance"},
		{billing.ErrRefundExceedsPayment, http.StatusUnprocessableEntity, "refund_exceeds_payment"},
		{billing.ErrRefundRequiresReason, http.StatusUnprocessableEntity, "refund_requires_reason"},
		{billing.ErrDuplicateInvoiceNumber, http.StatusConflict, "duplicate_invoice_number"},
		{billing.ErrAutopayHouseholdNotFound, http.StatusNotFound, "household_not_found"},
		{fmt.Errorf("wrapped: %w", billing.ErrInvalidFinanceInput), http.StatusUnprocessableEntity, "validation_error"},
		{billing.ErrCreditPaymentRefundToCredit, http.StatusUnprocessableEntity, "credit_payment_refund_to_credit"},
		{billing.ErrHouseholdRequired, http.StatusUnprocessableEntity, "household_required"},
		{billing.ErrAutopayPaymentMethodRequired, http.StatusUnprocessableEntity, "autopay_payment_method_required"},
		{errors.New("database exploded"), http.StatusInternalServerError, "internal_error"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.code, func(t *testing.T) {
			t.Parallel()
			rr := httptest.NewRecorder()
			mapBillingError(rr, httptest.NewRequest(http.MethodGet, "/x", nil), tc.err)
			cov6xWant(t, rr, tc.status, `"`+tc.code+`"`)
			if tc.status == http.StatusInternalServerError && strings.Contains(rr.Body.String(), "database exploded") {
				t.Fatalf("internal error detail leaked to client: %s", rr.Body.String())
			}
		})
	}
}

// cov6xBillingFixture is a real in-memory billing service behind the real
// handlers.
type cov6xBillingFixture struct {
	svc    *billing.Service
	audit  *audit.Service
	router http.Handler
	admin  *appmiddleware.Principal
}

func cov6xNewBilling(t *testing.T, withAudit bool, stripe stripelib.Client) cov6xBillingFixture {
	t.Helper()
	svc := billing.NewService()
	f := cov6xBillingFixture{
		svc:   svc,
		admin: &appmiddleware.Principal{UserID: "u-admin", OrgID: cov6xOrg, Roles: []string{"admin"}, MFAAuthenticated: true},
	}
	var h *BillingHandlers
	switch {
	case stripe != nil:
		f.audit = audit.NewService()
		h = NewBillingHandlersWithAuditAndStripe(svc, f.audit, stripe)
	case withAudit:
		f.audit = audit.NewService()
		h = NewBillingHandlersWithAudit(svc, f.audit)
	default:
		h = NewBillingHandlers(svc)
	}
	f.router = cov6xRouter(h.RegisterRoutes)
	return f
}

// invoice creates an invoice with one line item and issues it.
func (f cov6xBillingFixture) issuedInvoice(t *testing.T, number string, cents int) *billing.Invoice {
	t.Helper()
	inv, err := f.svc.CreateInvoice(cov6xOrg, f.admin.UserID, f.admin, billing.CreateInvoiceInput{InvoiceNumber: number, DueDate: "2030-01-31"})
	if err != nil {
		t.Fatalf("create invoice: %v", err)
	}
	if _, err := f.svc.AddInvoiceItem(cov6xOrg, f.admin.UserID, f.admin, inv.ID, billing.AddInvoiceItemInput{
		Description: "Dues", Quantity: 1, UnitAmountCents: cents, Category: "program_fee",
	}); err != nil {
		t.Fatalf("add item: %v", err)
	}
	issued, _, err := f.svc.IssueInvoice(cov6xOrg, f.admin.UserID, f.admin, inv.ID)
	if err != nil {
		t.Fatalf("issue invoice: %v", err)
	}
	return issued
}

func cov6xDecode(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not JSON: %v: %s", err, rr.Body.String())
	}
	return out
}

func cov6xAuditActions(t *testing.T, a *audit.Service) map[string][]audit.AuditEvent {
	t.Helper()
	events, err := a.Query(audit.Filter{OrgID: cov6xOrg})
	if err != nil {
		t.Fatalf("query audit: %v", err)
	}
	out := map[string][]audit.AuditEvent{}
	for _, e := range events {
		out[e.Action] = append(out[e.Action], e)
	}
	return out
}

func TestCov6xBillingInvoiceLifecycleWithoutAudit(t *testing.T) {
	t.Parallel()
	f := cov6xNewBilling(t, false, nil)

	// Validation failure on create (missing invoice_number) is a 422.
	rr := cov6xDo(f.router, http.MethodPost, "/invoices", "admin", `{"household_id":"hh-1"}`)
	cov6xWant(t, rr, http.StatusUnprocessableEntity, "")

	rr = cov6xDo(f.router, http.MethodPost, "/invoices", "admin", `{"invoice_number":"INV-1","due_date":"2030-01-31"}`)
	cov6xWant(t, rr, http.StatusCreated, "INV-1")
	invID := cov6xDecode(t, rr)["invoice"].(map[string]any)["id"].(string)

	// Duplicate numbers map to 409.
	rr = cov6xDo(f.router, http.MethodPost, "/invoices", "admin", `{"invoice_number":"INV-1"}`)
	cov6xWant(t, rr, http.StatusConflict, "duplicate_invoice_number")

	rr = cov6xDo(f.router, http.MethodGet, "/invoices/"+invID, "admin", "")
	cov6xWant(t, rr, http.StatusOK, invID)
	rr = cov6xDo(f.router, http.MethodGet, "/invoices/missing", "admin", "")
	cov6xWant(t, rr, http.StatusNotFound, "invoice_not_found")

	rr = cov6xDo(f.router, http.MethodGet, "/invoices?status=draft", "admin", "")
	cov6xWant(t, rr, http.StatusOK, `"count":1`)
	rr = cov6xDo(f.router, http.MethodGet, "/invoices?status=paid", "admin", "")
	cov6xWant(t, rr, http.StatusOK, `"count":0`)
	// Parents only ever see their own household's invoices; none exist here.
	rr = cov6xDo(f.router, http.MethodGet, "/invoices", "parent", "")
	cov6xWant(t, rr, http.StatusOK, `"count":0`)

	// Issuing an empty invoice is refused with 422.
	rr = cov6xDo(f.router, http.MethodPost, "/invoices/"+invID+"/issue", "admin", "")
	cov6xWant(t, rr, http.StatusUnprocessableEntity, "invoice_empty")

	// Items: invalid body, create, list, delete, and delete of unknown item.
	rr = cov6xDo(f.router, http.MethodPost, "/invoices/"+invID+"/items", "admin", `{"description":"x"}`)
	cov6xWant(t, rr, http.StatusUnprocessableEntity, "")
	rr = cov6xDo(f.router, http.MethodPost, "/invoices/"+invID+"/items", "admin",
		`{"description":"Dues","quantity":2,"unit_amount_cents":1500,"category":"program_fee"}`)
	cov6xWant(t, rr, http.StatusCreated, "Dues")
	itemID := cov6xDecode(t, rr)["item"].(map[string]any)["id"].(string)
	rr = cov6xDo(f.router, http.MethodPost, "/invoices/missing/items", "admin",
		`{"description":"Dues","quantity":1,"unit_amount_cents":100,"category":"manual"}`)
	cov6xWant(t, rr, http.StatusNotFound, "invoice_not_found")

	rr = cov6xDo(f.router, http.MethodGet, "/invoices/"+invID+"/items", "admin", "")
	cov6xWant(t, rr, http.StatusOK, `"count":1`)
	rr = cov6xDo(f.router, http.MethodGet, "/invoices/missing/items", "admin", "")
	cov6xWant(t, rr, http.StatusNotFound, "invoice_not_found")

	rr = cov6xDo(f.router, http.MethodDelete, "/invoices/"+invID+"/items/nope", "admin", "")
	cov6xWant(t, rr, http.StatusNotFound, "item_not_found")
	rr = cov6xDo(f.router, http.MethodDelete, "/invoices/"+invID+"/items/"+itemID, "admin", "")
	if rr.Code != http.StatusNoContent {
		t.Fatalf("delete item: got %d %s", rr.Code, rr.Body.String())
	}
	rr = cov6xDo(f.router, http.MethodGet, "/invoices/"+invID+"/items", "admin", "")
	cov6xWant(t, rr, http.StatusOK, `"count":0`)

	// Re-add and issue; issue then reports the transition and ledger event.
	rr = cov6xDo(f.router, http.MethodPost, "/invoices/"+invID+"/items", "admin",
		`{"description":"Dues","quantity":1,"unit_amount_cents":5000,"category":"program_fee"}`)
	cov6xWant(t, rr, http.StatusCreated, "")
	rr = cov6xDo(f.router, http.MethodPost, "/invoices/"+invID+"/issue", "admin", "")
	cov6xWant(t, rr, http.StatusOK, `"status":"issued"`)
	rr = cov6xDo(f.router, http.MethodPost, "/invoices/"+invID+"/issue", "admin", "")
	cov6xWant(t, rr, http.StatusConflict, "")

	// Payments need an MFA-authenticated admin.
	pay := `{"amount_cents":2000,"method":"check"}`
	rr = cov6xDo(f.router, http.MethodPost, "/invoices/"+invID+"/payments", "adminnomfa", pay)
	cov6xWant(t, rr, http.StatusForbidden, "mfa_required")
	rr = cov6xDo(f.router, http.MethodPost, "/invoices/"+invID+"/payments", "coach", pay)
	cov6xWant(t, rr, http.StatusForbidden, `"forbidden"`)
	rr = cov6xDo(f.router, http.MethodPost, "/invoices/"+invID+"/payments", "admin", `{"amount_cents":0}`)
	cov6xWant(t, rr, http.StatusUnprocessableEntity, "")
	rr = cov6xDo(f.router, http.MethodPost, "/invoices/"+invID+"/payments", "admin", `{"amount_cents":999999,"method":"check"}`)
	cov6xWant(t, rr, http.StatusUnprocessableEntity, "payment_exceeds_balance")
	rr = cov6xDo(f.router, http.MethodPost, "/invoices/"+invID+"/payments", "admin", pay)
	cov6xWant(t, rr, http.StatusCreated, `"amount_cents":2000`)

	rr = cov6xDo(f.router, http.MethodGet, "/invoices/"+invID+"/payments", "admin", "")
	cov6xWant(t, rr, http.StatusOK, `"count":1`)
	rr = cov6xDo(f.router, http.MethodGet, "/invoices/missing/payments", "admin", "")
	cov6xWant(t, rr, http.StatusNotFound, "invoice_not_found")
	rr = cov6xDo(f.router, http.MethodGet, "/payments?invoice_id="+invID+"&status=succeeded", "admin", "")
	cov6xWant(t, rr, http.StatusOK, `"count":1`)
	rr = cov6xDo(f.router, http.MethodGet, "/payments", "parent", "")
	cov6xWant(t, rr, http.StatusOK, `"count":0`)

	rr = cov6xDo(f.router, http.MethodGet, "/ledger-events?invoice_id="+invID, "admin", "")
	cov6xWant(t, rr, http.StatusOK, `"events"`)
	rr = cov6xDo(f.router, http.MethodGet, "/ledger-events", "parent", "")
	cov6xWant(t, rr, http.StatusForbidden, "forbidden")

	// Voiding and writing off are privileged and state-checked.
	rr = cov6xDo(f.router, http.MethodPost, "/invoices/"+invID+"/void", "adminnomfa", "")
	cov6xWant(t, rr, http.StatusForbidden, "mfa_required")
	rr = cov6xDo(f.router, http.MethodPost, "/invoices/"+invID+"/write-off", "coach", "")
	cov6xWant(t, rr, http.StatusForbidden, `"forbidden"`)
	rr = cov6xDo(f.router, http.MethodPost, "/invoices/missing/void", "admin", "")
	cov6xWant(t, rr, http.StatusNotFound, "invoice_not_found")
	rr = cov6xDo(f.router, http.MethodPost, "/invoices/"+invID+"/write-off", "admin", "")
	cov6xWant(t, rr, http.StatusOK, "written_off")

	// A voided draft cannot be voided again.
	draft, err := f.svc.CreateInvoice(cov6xOrg, f.admin.UserID, f.admin, billing.CreateInvoiceInput{InvoiceNumber: "INV-D"})
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	rr = cov6xDo(f.router, http.MethodPost, "/invoices/"+draft.ID+"/write-off", "admin", "")
	cov6xWant(t, rr, http.StatusConflict, "invoice_not_write_offable")
	rr = cov6xDo(f.router, http.MethodPost, "/invoices/"+draft.ID+"/void", "admin", "")
	cov6xWant(t, rr, http.StatusOK, `"status":"voided"`)
	rr = cov6xDo(f.router, http.MethodPost, "/invoices/"+draft.ID+"/void", "admin", "")
	cov6xWant(t, rr, http.StatusConflict, "invoice_not_voidable")

	// Refund error mapping and the (empty) refund listing.
	rr = cov6xDo(f.router, http.MethodGet, "/invoices/"+invID+"/refunds", "admin", "")
	cov6xWant(t, rr, http.StatusOK, `"count":0`)
	rr = cov6xDo(f.router, http.MethodGet, "/invoices/missing/refunds", "admin", "")
	cov6xWant(t, rr, http.StatusNotFound, "invoice_not_found")
	rr = cov6xDo(f.router, http.MethodPost, "/invoices/"+invID+"/refunds", "adminnomfa", `{"payment_id":"p","amount_cents":1,"reason":"r"}`)
	cov6xWant(t, rr, http.StatusForbidden, "mfa_required")
	rr = cov6xDo(f.router, http.MethodPost, "/invoices/"+invID+"/refunds", "admin", `{"payment_id":"p"}`)
	cov6xWant(t, rr, http.StatusUnprocessableEntity, "")
	rr = cov6xDo(f.router, http.MethodPost, "/invoices/"+invID+"/refunds", "admin", `{"payment_id":"nope","amount_cents":100,"reason":"dup"}`)
	cov6xWant(t, rr, http.StatusConflict, "invalid_transition")
}

func TestCov6xBillingHouseholdAutopayWithAndWithoutAudit(t *testing.T) {
	t.Parallel()
	body := `{"payment_method_type":"card","payment_method_brand":"visa","payment_method_last4":"4242","payment_method_label":"Visa 4242","trigger_days_before_due":3}`

	for _, withAudit := range []bool{false, true} {
		withAudit := withAudit
		t.Run(fmt.Sprintf("audit=%v", withAudit), func(t *testing.T) {
			t.Parallel()
			f := cov6xNewBilling(t, withAudit, nil)

			rr := cov6xDo(f.router, http.MethodGet, "/households/hh-1/autopay", "admin", "")
			cov6xWant(t, rr, http.StatusOK, `"autopay"`)
			rr = cov6xDo(f.router, http.MethodGet, "/households/hh-1/autopay", "stranger", "")
			cov6xWant(t, rr, http.StatusForbidden, "forbidden")

			rr = cov6xDo(f.router, http.MethodPut, "/households/hh-1/autopay", "admin", `{"payment_method_type":"bitcoin"}`)
			cov6xWant(t, rr, http.StatusUnprocessableEntity, "")
			rr = cov6xDo(f.router, http.MethodPut, "/households/hh-1/autopay", "coach", body)
			cov6xWant(t, rr, http.StatusForbidden, "forbidden")
			rr = cov6xDo(f.router, http.MethodPut, "/households/hh-1/autopay", "parent", body)
			cov6xWant(t, rr, http.StatusOK, `"enabled":true`)
			// A parent cannot enroll another household.
			rr = cov6xDo(f.router, http.MethodPut, "/households/hh-2/autopay", "parent", body)
			cov6xWant(t, rr, http.StatusForbidden, "forbidden")

			rr = cov6xDo(f.router, http.MethodGet, "/autopay/households?household_id=hh-1", "admin", "")
			cov6xWant(t, rr, http.StatusOK, `"count":1`)
			// A parent's listing is scoped to their own household.
			rr = cov6xDo(f.router, http.MethodGet, "/autopay/households", "parent", "")
			cov6xWant(t, rr, http.StatusOK, `"household_id":"hh-1"`)

			rr = cov6xDo(f.router, http.MethodPost, "/households/hh-1/autopay/opt-out", "stranger", "")
			cov6xWant(t, rr, http.StatusForbidden, "forbidden")
			rr = cov6xDo(f.router, http.MethodPost, "/households/hh-1/autopay/opt-out", "parent", "")
			cov6xWant(t, rr, http.StatusOK, `"enabled":false`)

			rr = cov6xDo(f.router, http.MethodGet, "/households/hh-1/invoices?status=draft", "admin", "")
			cov6xWant(t, rr, http.StatusOK, `"household_id":"hh-1"`)
			rr = cov6xDo(f.router, http.MethodGet, "/households/hh-2/invoices", "parent", "")
			cov6xWant(t, rr, http.StatusForbidden, "forbidden")

			if !withAudit {
				return
			}
			actions := cov6xAuditActions(t, f.audit)
			enroll := actions[audit.ActionBillingAutopayEnroll]
			if len(enroll) != 1 || enroll[0].ResourceID != "hh-1" || enroll[0].ActorUserID != "u-parent" {
				t.Fatalf("expected one enroll audit event for hh-1 by the parent, got %+v", enroll)
			}
			optOut := actions[audit.ActionBillingAutopayOptOut]
			if len(optOut) != 1 || len(optOut[0].BeforeState) == 0 {
				t.Fatalf("expected one opt-out audit event with before-state, got %+v", optOut)
			}
		})
	}
}

func TestCov6xBillingAuditedTransitionsCaptureBeforeState(t *testing.T) {
	t.Parallel()
	f := cov6xNewBilling(t, true, nil)
	inv, err := f.svc.CreateInvoice(cov6xOrg, f.admin.UserID, f.admin, billing.CreateInvoiceInput{InvoiceNumber: "INV-A"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.AddInvoiceItem(cov6xOrg, f.admin.UserID, f.admin, inv.ID, billing.AddInvoiceItemInput{
		Description: "Dues", Quantity: 1, UnitAmountCents: 4000, Category: "program_fee",
	}); err != nil {
		t.Fatal(err)
	}

	rr := cov6xDo(f.router, http.MethodPost, "/invoices/"+inv.ID+"/issue", "admin", "")
	cov6xWant(t, rr, http.StatusOK, `"status":"issued"`)
	rr = cov6xDo(f.router, http.MethodPost, "/invoices/"+inv.ID+"/payments", "admin", `{"amount_cents":1000,"method":"cash"}`)
	cov6xWant(t, rr, http.StatusCreated, "")

	actions := cov6xAuditActions(t, f.audit)
	issue := actions[audit.ActionBillingInvoiceIssue]
	if len(issue) != 1 || string(issue[0].BeforeState) == "" || issue[0].ResourceID != inv.ID {
		t.Fatalf("issue audit missing before-state: %+v", issue)
	}
	var before map[string]any
	if err := json.Unmarshal(issue[0].BeforeState, &before); err != nil || before["status"] != "draft" {
		t.Fatalf("issue audit before-state should show draft status, got %s (%v)", issue[0].BeforeState, err)
	}
	if pay := actions[audit.ActionBillingPaymentRecord]; len(pay) != 1 || pay[0].Metadata["method"] != "cash" || pay[0].Metadata["invoice_id"] != inv.ID {
		t.Fatalf("payment audit metadata wrong: %+v", pay)
	}

	// A failed transition is never audited.
	before2 := len(cov6xAuditActions(t, f.audit)[audit.ActionBillingInvoiceVoid])
	rr = cov6xDo(f.router, http.MethodPost, "/invoices/"+inv.ID+"/void", "admin", "")
	cov6xWant(t, rr, http.StatusConflict, "invoice_not_voidable")
	if got := len(cov6xAuditActions(t, f.audit)[audit.ActionBillingInvoiceVoid]); got != before2 {
		t.Fatalf("failed void must not be audited")
	}
}

// cov6xHookStripe is a Stripe client whose refund call can run a side effect
// (modelling a concurrent webhook) or fail.
type cov6xHookStripe struct {
	*stripelib.StubClient
	onRefund func(params stripelib.RefundParams) error
}

func (c *cov6xHookStripe) CreateRefund(ctx context.Context, params stripelib.RefundParams) (*stripelib.RefundResult, error) {
	if c.onRefund != nil {
		if err := c.onRefund(params); err != nil {
			return nil, err
		}
	}
	return c.StubClient.CreateRefund(ctx, params)
}

func TestCov6xBillingCreateRefundStripePaths(t *testing.T) {
	t.Parallel()

	// setup issues a $50 invoice, takes a Stripe payment and returns the ids.
	setup := func(t *testing.T, hook func(f cov6xBillingFixture, invoiceID, paymentID string) func(stripelib.RefundParams) error) (cov6xBillingFixture, *cov6xHookStripe, string, string) {
		t.Helper()
		client := &cov6xHookStripe{StubClient: stripelib.NewStubClient()}
		f := cov6xNewBilling(t, false, client)
		inv := f.issuedInvoice(t, "INV-R", 5000)
		pay, _, err := f.svc.RecordPayment(cov6xOrg, f.admin.UserID, f.admin, inv.ID, billing.RecordPaymentInput{
			AmountCents: 5000, Method: "stripe", StripePaymentIntentID: "pi_cov6x",
		})
		if err != nil {
			t.Fatalf("record payment: %v", err)
		}
		if hook != nil {
			client.onRefund = hook(f, inv.ID, pay.ID)
		}
		return f, client, inv.ID, pay.ID
	}
	refundBody := func(paymentID string, cents int) string {
		return fmt.Sprintf(`{"payment_id":%q,"amount_cents":%d,"reason":"requested by family"}`, paymentID, cents)
	}

	t.Run("success calls stripe with idempotency key and audits reason", func(t *testing.T) {
		t.Parallel()
		f, client, invID, payID := setup(t, nil)
		rr := cov6xDo(f.router, http.MethodPost, "/invoices/"+invID+"/refunds", "admin", refundBody(payID, 1000))
		cov6xWant(t, rr, http.StatusCreated, `"amount_cents":1000`)
		refunds := client.CreatedRefunds()
		if len(refunds) != 1 || refunds[0].PaymentIntentID != "pi_cov6x" || refunds[0].AmountCents != 1000 {
			t.Fatalf("unexpected stripe refund calls: %+v", refunds)
		}
		wantKey := fmt.Sprintf("refund:%s:%s:%s:1000:0", cov6xOrg, invID, payID)
		if refunds[0].IdempotencyKey != wantKey {
			t.Fatalf("idempotency key = %q, want %q", refunds[0].IdempotencyKey, wantKey)
		}

		// A second same-amount refund must use a distinct key (prior count = 1).
		rr = cov6xDo(f.router, http.MethodPost, "/invoices/"+invID+"/refunds", "admin", refundBody(payID, 1000))
		cov6xWant(t, rr, http.StatusCreated, "")
		refunds = client.CreatedRefunds()
		if len(refunds) != 2 || refunds[1].IdempotencyKey == refunds[0].IdempotencyKey {
			t.Fatalf("second refund reused the idempotency key: %+v", refunds)
		}

		rr = cov6xDo(f.router, http.MethodGet, "/invoices/"+invID+"/refunds", "admin", "")
		cov6xWant(t, rr, http.StatusOK, `"count":2`)

		events := cov6xAuditActions(t, f.audit)[audit.ActionBillingRefundCreate]
		if len(events) != 2 || events[0].Metadata["reason"] != "requested by family" || events[0].Metadata["invoice_id"] != invID {
			t.Fatalf("refund audit metadata wrong: %+v", events)
		}
	})

	t.Run("stripe failure is a 502 and nothing is recorded locally", func(t *testing.T) {
		t.Parallel()
		f, client, invID, payID := setup(t, nil)
		client.StubClient.CreateRefundErr = errors.New("stripe is down")
		rr := cov6xDo(f.router, http.MethodPost, "/invoices/"+invID+"/refunds", "admin", refundBody(payID, 1000))
		cov6xWant(t, rr, http.StatusBadGateway, "stripe_error")
		refunds, err := f.svc.ListRefunds(cov6xOrg, f.admin, invID)
		if err != nil || len(refunds) != 0 {
			t.Fatalf("no local refund expected after stripe failure, got %v err=%v", refunds, err)
		}
	})

	t.Run("refund already recorded by webhook is acknowledged and audited", func(t *testing.T) {
		t.Parallel()
		f, _, invID, payID := setup(t, func(f cov6xBillingFixture, invoiceID, paymentID string) func(stripelib.RefundParams) error {
			return func(stripelib.RefundParams) error {
				// The webhook lands first and records a partial refund that
				// leaves too little for the admin's request.
				_, _, err := f.svc.CreateRefund(cov6xOrg, f.admin.UserID, f.admin, invoiceID, billing.CreateRefundInput{
					PaymentID: paymentID, AmountCents: 4000, Reason: "webhook", StripeRefundID: "re_webhook",
				})
				return err
			}
		})
		rr := cov6xDo(f.router, http.MethodPost, "/invoices/"+invID+"/refunds", "admin", refundBody(payID, 3000))
		cov6xWant(t, rr, http.StatusOK, "refund_recorded_by_webhook")
		events := cov6xAuditActions(t, f.audit)[audit.ActionBillingRefundCreate]
		if len(events) != 1 || events[0].Metadata["recorded_by"] != "webhook" {
			t.Fatalf("expected a webhook-attributed audit event, got %+v", events)
		}
		refunds, _ := f.svc.ListRefunds(cov6xOrg, f.admin, invID)
		if len(refunds) != 1 {
			t.Fatalf("only the webhook refund should exist, got %d", len(refunds))
		}
	})

	t.Run("local persistence failure after stripe success needs operator reconcile", func(t *testing.T) {
		t.Parallel()
		f, _, invID, payID := setup(t, func(f cov6xBillingFixture, invoiceID, paymentID string) func(stripelib.RefundParams) error {
			return func(stripelib.RefundParams) error {
				// A concurrent full refund moves the invoice out of a
				// refundable state, so the local write is rejected.
				_, _, err := f.svc.CreateRefund(cov6xOrg, f.admin.UserID, f.admin, invoiceID, billing.CreateRefundInput{
					PaymentID: paymentID, AmountCents: 5000, Reason: "full", StripeRefundID: "re_full",
				})
				return err
			}
		})
		rr := cov6xDo(f.router, http.MethodPost, "/invoices/"+invID+"/refunds", "admin", refundBody(payID, 1000))
		cov6xWant(t, rr, http.StatusBadGateway, "refund_persistence_failed")
		if events := cov6xAuditActions(t, f.audit)[audit.ActionBillingRefundCreate]; len(events) != 0 {
			t.Fatalf("a failed local write must not be audited as created: %+v", events)
		}
	})

	t.Run("exceeding the payment is refused before stripe is called", func(t *testing.T) {
		t.Parallel()
		f, client, invID, payID := setup(t, nil)
		rr := cov6xDo(f.router, http.MethodPost, "/invoices/"+invID+"/refunds", "admin", refundBody(payID, 5001))
		cov6xWant(t, rr, http.StatusUnprocessableEntity, "refund_exceeds_payment")
		if n := len(client.CreatedRefunds()); n != 0 {
			t.Fatalf("stripe must not be called, got %d calls", n)
		}
	})
}

func TestCov6xBillingRouterRoutesAreAllMounted(t *testing.T) {
	t.Parallel()
	routes := cov6xWalkRoutes(t, func(r chi.Router) { NewBillingHandlers(billing.NewService()).RegisterRoutes(r) })
	seen := map[string]bool{}
	for _, rt := range routes {
		seen[rt.method+" "+rt.path] = true
	}
	for _, want := range []string{
		"POST /invoices", "GET /invoices/x", "POST /invoices/x/refunds", "GET /ledger-events",
		"PUT /households/x/autopay", "POST /households/x/autopay/opt-out", "GET /autopay/households",
	} {
		if !seen[want] {
			t.Errorf("route %q not mounted", want)
		}
	}
}

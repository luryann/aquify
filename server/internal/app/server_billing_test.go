package app

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/billing"
	stripelib "github.com/dareaquatics/aquify/server/internal/stripe"
)

// ─── Envelope helpers ─────────────────────────────────────────────────────────

type invoiceEnvelope struct {
	Invoice struct {
		ID             string `json:"id"`
		OrgID          string `json:"org_id"`
		HouseholdID    string `json:"household_id"`
		InvoiceNumber  string `json:"invoice_number"`
		Status         string `json:"status"`
		SubtotalCents  int    `json:"subtotal_cents"`
		TotalCents     int    `json:"total_cents"`
		AmountDueCents int    `json:"amount_due_cents"`
	} `json:"invoice"`
}

type invoiceWithLedgerEnvelope struct {
	Invoice     invoiceEnvelope  `json:"invoice"`
	LedgerEvent ledgerEvEnvelope `json:"ledger_event"`
}

type invoiceItemEnvelope struct {
	Item struct {
		ID              string `json:"id"`
		InvoiceID       string `json:"invoice_id"`
		Description     string `json:"description"`
		Quantity        int    `json:"quantity"`
		UnitAmountCents int    `json:"unit_amount_cents"`
		TotalCents      int    `json:"total_cents"`
		Category        string `json:"category"`
	} `json:"item"`
}

type invoiceItemsListEnvelope struct {
	InvoiceID string `json:"invoice_id"`
	Count     int    `json:"count"`
	Items     []struct {
		ID string `json:"id"`
	} `json:"items"`
}

type issueInvoiceEnvelope struct {
	Invoice struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"invoice"`
	LedgerEvent ledgerEvEnvelope `json:"ledger_event"`
}

type paymentEnvelope struct {
	Payment struct {
		ID          string `json:"id"`
		InvoiceID   string `json:"invoice_id"`
		AmountCents int    `json:"amount_cents"`
		Status      string `json:"status"`
		Method      string `json:"method"`
	} `json:"payment"`
	LedgerEvent ledgerEvEnvelope `json:"ledger_event"`
}

type paymentsListEnvelope struct {
	InvoiceID string `json:"invoice_id"`
	Count     int    `json:"count"`
	Payments  []struct {
		ID string `json:"id"`
	} `json:"payments"`
}

type refundEnvelope struct {
	Refund struct {
		ID             string `json:"id"`
		InvoiceID      string `json:"invoice_id"`
		PaymentID      string `json:"payment_id"`
		AmountCents    int    `json:"amount_cents"`
		Status         string `json:"status"`
		StripeRefundID string `json:"stripe_refund_id"`
	} `json:"refund"`
	LedgerEvent ledgerEvEnvelope `json:"ledger_event"`
}

type ledgerEvEnvelope struct {
	ID          string `json:"id"`
	EventType   string `json:"event_type"`
	AmountCents int    `json:"amount_cents"`
}

type ledgerListEnvelope struct {
	Count  int `json:"count"`
	Events []struct {
		ID        string `json:"id"`
		EventType string `json:"event_type"`
	} `json:"events"`
}

type householdInvoicesEnvelope struct {
	HouseholdID string `json:"household_id"`
	Count       int    `json:"count"`
	Invoices    []struct {
		ID string `json:"id"`
	} `json:"invoices"`
}

type orgPaymentsEnvelope struct {
	Count    int `json:"count"`
	Payments []struct {
		ID        string `json:"id"`
		InvoiceID string `json:"invoice_id"`
	} `json:"payments"`
}

// ─── Router helper ────────────────────────────────────────────────────────────

func newBillingRouter(t *testing.T, svc *billing.Service) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouterWithServices(Config{Auth: AuthConfig{EnableDevStubTokens: true}}, logger, nil, nil, nil, nil, nil, nil, nil, svc, nil, nil, nil, nil, nil)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func billingBody(fields map[string]any) string {
	b, _ := json.Marshal(fields)
	return string(b)
}

func createInvoiceForHousehold(t *testing.T, router http.Handler, token, orgID, householdID, number string) invoiceEnvelope {
	t.Helper()
	body := billingBody(map[string]any{
		"household_id":   householdID,
		"invoice_number": number,
		"notes":          "test invoice",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/invoices", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", orgID)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("createInvoiceForHousehold: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var env invoiceEnvelope
	if err := json.NewDecoder(rr.Body).Decode(&env); err != nil {
		t.Fatalf("createInvoiceForHousehold decode: %v", err)
	}
	return env
}

func createInvoice(t *testing.T, router http.Handler, token, orgID, number string) invoiceEnvelope {
	t.Helper()
	body := billingBody(map[string]any{
		"household_id":   "hh-001",
		"invoice_number": number,
		"notes":          "test invoice",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/invoices", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", orgID)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("createInvoice: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var env invoiceEnvelope
	if err := json.NewDecoder(rr.Body).Decode(&env); err != nil {
		t.Fatalf("createInvoice decode: %v", err)
	}
	return env
}

func addItem(t *testing.T, router http.Handler, token, orgID, invoiceID string, unitCents int) invoiceItemEnvelope {
	t.Helper()
	body := billingBody(map[string]any{
		"description":       "Program fee",
		"quantity":          1,
		"unit_amount_cents": unitCents,
		"category":          "program_fee",
	})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/invoices/%s/items", invoiceID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", orgID)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("addItem: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var env invoiceItemEnvelope
	if err := json.NewDecoder(rr.Body).Decode(&env); err != nil {
		t.Fatalf("addItem decode: %v", err)
	}
	return env
}

func issueInvoiceReq(t *testing.T, router http.Handler, token, orgID, invoiceID string) issueInvoiceEnvelope {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/invoices/%s/issue", invoiceID), http.NoBody)
	req.Header.Set("X-Org-Id", orgID)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("issueInvoice: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var env issueInvoiceEnvelope
	if err := json.NewDecoder(rr.Body).Decode(&env); err != nil {
		t.Fatalf("issueInvoice decode: %v", err)
	}
	return env
}

func recordPaymentReq(t *testing.T, router http.Handler, token, orgID, invoiceID string, amountCents int) paymentEnvelope {
	t.Helper()
	body := billingBody(map[string]any{
		"amount_cents": amountCents,
		"method":       "check",
	})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/invoices/%s/payments", invoiceID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", orgID)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("recordPayment: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var env paymentEnvelope
	if err := json.NewDecoder(rr.Body).Decode(&env); err != nil {
		t.Fatalf("recordPayment decode: %v", err)
	}
	return env
}

func TestOrgWideBillingListEndpointsReturnInvoicesAndPayments(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	router := newBillingRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"billing"}, false)

	first := createInvoiceForHousehold(t, router, token, "org-demo", "hh-001", "INV-ORG-1")
	addItem(t, router, token, "org-demo", first.Invoice.ID, 5000)
	issueInvoiceReq(t, router, token, "org-demo", first.Invoice.ID)
	recordPaymentReq(t, router, token, "org-demo", first.Invoice.ID, 5000)

	second := createInvoiceForHousehold(t, router, token, "org-demo", "hh-002", "INV-ORG-2")
	addItem(t, router, token, "org-demo", second.Invoice.ID, 7000)
	issueInvoiceReq(t, router, token, "org-demo", second.Invoice.ID)
	recordPaymentReq(t, router, token, "org-demo", second.Invoice.ID, 7000)

	invoicesReq := httptest.NewRequest(http.MethodGet, "/api/v1/invoices", nil)
	invoicesReq.Header.Set("X-Org-Id", "org-demo")
	invoicesReq.Header.Set("Authorization", "Bearer "+token)
	invoicesRes := httptest.NewRecorder()
	router.ServeHTTP(invoicesRes, invoicesReq)
	if invoicesRes.Code != http.StatusOK {
		t.Fatalf("expected 200 for invoice list, got %d: %s", invoicesRes.Code, invoicesRes.Body.String())
	}

	var invoicesEnv struct {
		Count    int `json:"count"`
		Invoices []struct {
			ID string `json:"id"`
		} `json:"invoices"`
	}
	if err := json.NewDecoder(invoicesRes.Body).Decode(&invoicesEnv); err != nil {
		t.Fatalf("decode invoices list: %v", err)
	}
	if invoicesEnv.Count != 2 {
		t.Fatalf("expected 2 invoices, got %d", invoicesEnv.Count)
	}

	paymentsReq := httptest.NewRequest(http.MethodGet, "/api/v1/payments", nil)
	paymentsReq.Header.Set("X-Org-Id", "org-demo")
	paymentsReq.Header.Set("Authorization", "Bearer "+token)
	paymentsRes := httptest.NewRecorder()
	router.ServeHTTP(paymentsRes, paymentsReq)
	if paymentsRes.Code != http.StatusOK {
		t.Fatalf("expected 200 for payments list, got %d: %s", paymentsRes.Code, paymentsRes.Body.String())
	}

	var paymentsEnv orgPaymentsEnvelope
	if err := json.NewDecoder(paymentsRes.Body).Decode(&paymentsEnv); err != nil {
		t.Fatalf("decode payments list: %v", err)
	}
	if paymentsEnv.Count != 2 {
		t.Fatalf("expected 2 payments, got %d", paymentsEnv.Count)
	}
}

// ─── Tests: Invoice creation ──────────────────────────────────────────────────

func TestCreateInvoiceRequiresAuth(t *testing.T) {
	t.Parallel()
	router := newBillingRouter(t, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/invoices",
		strings.NewReader(`{"invoice_number":"INV-001"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestCreateInvoiceAdminSuccess(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	router := newBillingRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	env := createInvoice(t, router, token, "org-demo", "INV-001")
	if env.Invoice.ID == "" {
		t.Fatal("expected invoice.id to be populated")
	}
	if env.Invoice.Status != "draft" {
		t.Fatalf("expected status=draft, got %q", env.Invoice.Status)
	}
	if env.Invoice.InvoiceNumber != "INV-001" {
		t.Fatalf("expected invoice_number=INV-001, got %q", env.Invoice.InvoiceNumber)
	}
}

func TestCreateInvoiceBillingRoleSuccess(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	router := newBillingRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"billing"}, false)

	env := createInvoice(t, router, token, "org-demo", "INV-B001")
	if env.Invoice.ID == "" {
		t.Fatal("expected invoice.id")
	}
}

func TestCreateInvoiceParentForbidden(t *testing.T) {
	t.Parallel()
	router := newBillingRouter(t, nil)
	token := issueSessionToken(t, router, "org-demo", []string{"parent"}, false)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/invoices",
		strings.NewReader(`{"invoice_number":"INV-X"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestCreateInvoiceDuplicateNumberConflict(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	router := newBillingRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	createInvoice(t, router, token, "org-demo", "INV-DUP")

	// Second creation with same number should fail.
	body := billingBody(map[string]any{"invoice_number": "INV-DUP"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/invoices", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409 on duplicate invoice number, got %d", rr.Code)
	}
}

// ─── Tests: Invoice items ─────────────────────────────────────────────────────

func TestAddInvoiceItemDraftOnly(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	router := newBillingRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	env := createInvoice(t, router, token, "org-demo", "INV-ITEM")
	invoiceID := env.Invoice.ID

	itemEnv := addItem(t, router, token, "org-demo", invoiceID, 15000)
	if itemEnv.Item.ID == "" {
		t.Fatal("expected item.id")
	}
	if itemEnv.Item.TotalCents != 15000 {
		t.Fatalf("expected total_cents=15000, got %d", itemEnv.Item.TotalCents)
	}
}

func TestAddInvoiceItemUpdatesInvoiceTotals(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	router := newBillingRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, true)

	env := createInvoice(t, router, token, "org-demo", "INV-TOTALS")
	invoiceID := env.Invoice.ID

	addItem(t, router, token, "org-demo", invoiceID, 10000)
	addItem(t, router, token, "org-demo", invoiceID, 5000)

	// GET invoice and verify totals.
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/invoices/%s", invoiceID), nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var invEnv invoiceEnvelope
	json.NewDecoder(rr.Body).Decode(&invEnv)
	if invEnv.Invoice.SubtotalCents != 15000 {
		t.Fatalf("expected subtotal=15000, got %d", invEnv.Invoice.SubtotalCents)
	}
}

func TestAddItemToIssuedInvoiceConflict(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	router := newBillingRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, true)

	env := createInvoice(t, router, token, "org-demo", "INV-NOMOD")
	invoiceID := env.Invoice.ID
	addItem(t, router, token, "org-demo", invoiceID, 5000)
	issueInvoiceReq(t, router, token, "org-demo", invoiceID)

	// Try to add item after issuance.
	body := billingBody(map[string]any{
		"description":       "Late fee",
		"quantity":          1,
		"unit_amount_cents": 1000,
		"category":          "late_fee",
	})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/invoices/%s/items", invoiceID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409 adding item to issued invoice, got %d", rr.Code)
	}
}

func TestRemoveInvoiceItemDraft(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	router := newBillingRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, true)

	inv := createInvoice(t, router, token, "org-demo", "INV-DEL")
	item := addItem(t, router, token, "org-demo", inv.Invoice.ID, 8000)

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/invoices/%s/items/%s", inv.Invoice.ID, item.Item.ID), nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rr.Code)
	}

	// List items — should be empty.
	req2 := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/invoices/%s/items", inv.Invoice.ID), nil)
	req2.Header.Set("X-Org-Id", "org-demo")
	req2.Header.Set("Authorization", "Bearer "+token)
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)
	var listEnv invoiceItemsListEnvelope
	json.NewDecoder(rr2.Body).Decode(&listEnv)
	if listEnv.Count != 0 {
		t.Fatalf("expected 0 items after removal, got %d", listEnv.Count)
	}
}

// ─── Tests: Invoice lifecycle ─────────────────────────────────────────────────

func TestIssueInvoiceDraftToIssued(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	router := newBillingRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, true)

	inv := createInvoice(t, router, token, "org-demo", "INV-ISSUE")
	addItem(t, router, token, "org-demo", inv.Invoice.ID, 20000)

	issEnv := issueInvoiceReq(t, router, token, "org-demo", inv.Invoice.ID)
	if issEnv.Invoice.Status != "issued" {
		t.Fatalf("expected status=issued, got %q", issEnv.Invoice.Status)
	}
	if issEnv.LedgerEvent.ID == "" {
		t.Fatal("expected ledger_event.id to be set")
	}
	if issEnv.LedgerEvent.EventType != "invoice_issued" {
		t.Fatalf("expected event_type=invoice_issued, got %q", issEnv.LedgerEvent.EventType)
	}
	if issEnv.LedgerEvent.AmountCents != 20000 {
		t.Fatalf("expected ledger amount=20000, got %d", issEnv.LedgerEvent.AmountCents)
	}
}

func TestIssueAlreadyIssuedConflict(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	router := newBillingRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, true)

	inv := createInvoice(t, router, token, "org-demo", "INV-REISSUE")
	addItem(t, router, token, "org-demo", inv.Invoice.ID, 5000)
	issueInvoiceReq(t, router, token, "org-demo", inv.Invoice.ID)

	// Second issue attempt.
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/invoices/%s/issue", inv.Invoice.ID), nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409 re-issuing, got %d", rr.Code)
	}
}

func TestVoidInvoiceIssuedToVoided(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	router := newBillingRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, true)

	inv := createInvoice(t, router, token, "org-demo", "INV-VOID")
	addItem(t, router, token, "org-demo", inv.Invoice.ID, 5000)
	issueInvoiceReq(t, router, token, "org-demo", inv.Invoice.ID)

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/invoices/%s/void", inv.Invoice.ID), nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var env struct {
		Invoice struct {
			Status string `json:"status"`
		} `json:"invoice"`
		LedgerEvent ledgerEvEnvelope `json:"ledger_event"`
	}
	json.NewDecoder(rr.Body).Decode(&env)
	if env.Invoice.Status != "voided" {
		t.Fatalf("expected status=voided, got %q", env.Invoice.Status)
	}
	if env.LedgerEvent.EventType != "invoice_voided" {
		t.Fatalf("expected event_type=invoice_voided, got %q", env.LedgerEvent.EventType)
	}
}

func TestWriteOffInvoiceAdminOnly(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	router := newBillingRouter(t, svc)
	billingToken := issueSessionToken(t, router, "org-demo", []string{"billing"}, false)
	adminToken := issueSessionToken(t, router, "org-demo", []string{"admin"}, true)

	inv := createInvoice(t, router, adminToken, "org-demo", "INV-WO")
	addItem(t, router, adminToken, "org-demo", inv.Invoice.ID, 5000)
	issueInvoiceReq(t, router, adminToken, "org-demo", inv.Invoice.ID)

	// billing role should be rejected.
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/invoices/%s/write-off", inv.Invoice.ID), nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+billingToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for billing role on write-off, got %d", rr.Code)
	}

	// admin role should succeed.
	req2 := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/invoices/%s/write-off", inv.Invoice.ID), nil)
	req2.Header.Set("X-Org-Id", "org-demo")
	req2.Header.Set("Authorization", "Bearer "+adminToken)
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin write-off, got %d: %s", rr2.Code, rr2.Body.String())
	}
	var env struct {
		Invoice struct {
			Status string `json:"status"`
		} `json:"invoice"`
		LedgerEvent ledgerEvEnvelope `json:"ledger_event"`
	}
	json.NewDecoder(rr2.Body).Decode(&env)
	if env.Invoice.Status != "written_off" {
		t.Fatalf("expected status=written_off, got %q", env.Invoice.Status)
	}
	if env.LedgerEvent.EventType != "invoice_written_off" {
		t.Fatalf("expected event_type=invoice_written_off, got %q", env.LedgerEvent.EventType)
	}
}

// ─── Tests: Payments ──────────────────────────────────────────────────────────

func TestRecordPaymentSucceeded(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	router := newBillingRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, true)

	inv := createInvoice(t, router, token, "org-demo", "INV-PAY")
	addItem(t, router, token, "org-demo", inv.Invoice.ID, 30000)
	issueInvoiceReq(t, router, token, "org-demo", inv.Invoice.ID)

	payEnv := recordPaymentReq(t, router, token, "org-demo", inv.Invoice.ID, 30000)
	if payEnv.Payment.ID == "" {
		t.Fatal("expected payment.id")
	}
	if payEnv.Payment.Status != "succeeded" {
		t.Fatalf("expected status=succeeded, got %q", payEnv.Payment.Status)
	}
	if payEnv.LedgerEvent.EventType != "payment_succeeded" {
		t.Fatalf("expected event_type=payment_succeeded, got %q", payEnv.LedgerEvent.EventType)
	}
	if payEnv.LedgerEvent.AmountCents != -30000 {
		t.Fatalf("expected ledger amount=-30000, got %d", payEnv.LedgerEvent.AmountCents)
	}
}

func TestRecordPaymentUpdatesInvoiceStatusPaid(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	router := newBillingRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, true)

	inv := createInvoice(t, router, token, "org-demo", "INV-PAID")
	addItem(t, router, token, "org-demo", inv.Invoice.ID, 10000)
	issueInvoiceReq(t, router, token, "org-demo", inv.Invoice.ID)
	recordPaymentReq(t, router, token, "org-demo", inv.Invoice.ID, 10000)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/invoices/%s", inv.Invoice.ID), nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	var invEnv invoiceEnvelope
	json.NewDecoder(rr.Body).Decode(&invEnv)
	if invEnv.Invoice.Status != "paid" {
		t.Fatalf("expected status=paid after full payment, got %q", invEnv.Invoice.Status)
	}
	if invEnv.Invoice.AmountDueCents != 0 {
		t.Fatalf("expected amount_due_cents=0 after full payment, got %d", invEnv.Invoice.AmountDueCents)
	}
}

func TestRecordPartialPaymentStatus(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	router := newBillingRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, true)

	inv := createInvoice(t, router, token, "org-demo", "INV-PARTIAL")
	addItem(t, router, token, "org-demo", inv.Invoice.ID, 20000)
	issueInvoiceReq(t, router, token, "org-demo", inv.Invoice.ID)
	recordPaymentReq(t, router, token, "org-demo", inv.Invoice.ID, 10000)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/invoices/%s", inv.Invoice.ID), nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	var invEnv invoiceEnvelope
	json.NewDecoder(rr.Body).Decode(&invEnv)
	if invEnv.Invoice.Status != "partially_paid" {
		t.Fatalf("expected status=partially_paid, got %q", invEnv.Invoice.Status)
	}
	if invEnv.Invoice.AmountDueCents != 10000 {
		t.Fatalf("expected amount_due_cents=10000, got %d", invEnv.Invoice.AmountDueCents)
	}
}

func TestRecordPaymentExceedsBalance(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	router := newBillingRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, true)

	inv := createInvoice(t, router, token, "org-demo", "INV-OVER")
	addItem(t, router, token, "org-demo", inv.Invoice.ID, 5000)
	issueInvoiceReq(t, router, token, "org-demo", inv.Invoice.ID)

	body := billingBody(map[string]any{"amount_cents": 9999, "method": "check"})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/invoices/%s/payments", inv.Invoice.ID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for over-payment, got %d", rr.Code)
	}
}

func TestListPaymentsForInvoice(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	router := newBillingRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, true)

	inv := createInvoice(t, router, token, "org-demo", "INV-LPAY")
	addItem(t, router, token, "org-demo", inv.Invoice.ID, 20000)
	issueInvoiceReq(t, router, token, "org-demo", inv.Invoice.ID)
	recordPaymentReq(t, router, token, "org-demo", inv.Invoice.ID, 10000)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/invoices/%s/payments", inv.Invoice.ID), nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var env paymentsListEnvelope
	json.NewDecoder(rr.Body).Decode(&env)
	if env.Count != 1 {
		t.Fatalf("expected 1 payment, got %d", env.Count)
	}
}

// ─── Tests: Refunds ───────────────────────────────────────────────────────────

func TestCreateRefundSuccess(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	router := newBillingRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, true)

	inv := createInvoice(t, router, token, "org-demo", "INV-REFUND")
	addItem(t, router, token, "org-demo", inv.Invoice.ID, 20000)
	issueInvoiceReq(t, router, token, "org-demo", inv.Invoice.ID)
	payEnv := recordPaymentReq(t, router, token, "org-demo", inv.Invoice.ID, 20000)

	body := billingBody(map[string]any{
		"payment_id":   payEnv.Payment.ID,
		"amount_cents": 20000,
		"reason":       "Swimmer withdrew from program",
	})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/invoices/%s/refunds", inv.Invoice.ID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var env refundEnvelope
	json.NewDecoder(rr.Body).Decode(&env)
	if env.Refund.ID == "" {
		t.Fatal("expected refund.id")
	}
	if env.Refund.Status != "processed" {
		t.Fatalf("expected status=processed, got %q", env.Refund.Status)
	}
	if env.LedgerEvent.EventType != "refund_processed" {
		t.Fatalf("expected event_type=refund_processed, got %q", env.LedgerEvent.EventType)
	}
}

func TestCreateRefundRequiresMFA(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	router := newBillingRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	inv := createInvoice(t, router, token, "org-demo", "INV-REFUND-MFA")
	addItem(t, router, token, "org-demo", inv.Invoice.ID, 20000)
	issueInvoiceReq(t, router, token, "org-demo", inv.Invoice.ID)
	payEnv := recordPaymentReq(t, router, token, "org-demo", inv.Invoice.ID, 20000)

	body := billingBody(map[string]any{
		"payment_id":   payEnv.Payment.ID,
		"amount_cents": 20000,
		"reason":       "Needs MFA",
	})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/invoices/%s/refunds", inv.Invoice.ID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestCreateRefundUpdatesInvoiceStatusFull(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	router := newBillingRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, true)

	inv := createInvoice(t, router, token, "org-demo", "INV-RFULL")
	addItem(t, router, token, "org-demo", inv.Invoice.ID, 10000)
	issueInvoiceReq(t, router, token, "org-demo", inv.Invoice.ID)
	payEnv := recordPaymentReq(t, router, token, "org-demo", inv.Invoice.ID, 10000)

	body := billingBody(map[string]any{
		"payment_id":   payEnv.Payment.ID,
		"amount_cents": 10000,
		"reason":       "Full refund",
	})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/invoices/%s/refunds", inv.Invoice.ID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rr.Code)
	}

	// Check invoice status.
	req2 := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/invoices/%s", inv.Invoice.ID), nil)
	req2.Header.Set("X-Org-Id", "org-demo")
	req2.Header.Set("Authorization", "Bearer "+token)
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)
	var invEnv invoiceEnvelope
	json.NewDecoder(rr2.Body).Decode(&invEnv)
	if invEnv.Invoice.Status != "refunded_full" {
		t.Fatalf("expected status=refunded_full, got %q", invEnv.Invoice.Status)
	}
}

func TestCreateRefundForStripePaymentStoresStripeRefundID(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	router := newBillingRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, true)

	inv := createInvoice(t, router, token, "org-demo", "INV-RSTRIPE")
	addItem(t, router, token, "org-demo", inv.Invoice.ID, 10000)
	issueInvoiceReq(t, router, token, "org-demo", inv.Invoice.ID)

	paymentBody := billingBody(map[string]any{
		"amount_cents":             10000,
		"method":                   "stripe",
		"stripe_payment_intent_id": "pi_test_refundable",
	})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/invoices/%s/payments", inv.Invoice.ID), strings.NewReader(paymentBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("record stripe payment: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var payEnv paymentEnvelope
	if err := json.NewDecoder(rr.Body).Decode(&payEnv); err != nil {
		t.Fatalf("decode stripe payment: %v", err)
	}

	refundBody := billingBody(map[string]any{
		"payment_id":   payEnv.Payment.ID,
		"amount_cents": 10000,
		"reason":       "Requested by customer",
	})
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/invoices/%s/refunds", inv.Invoice.ID), strings.NewReader(refundBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("refund stripe payment: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var env refundEnvelope
	if err := json.NewDecoder(rr.Body).Decode(&env); err != nil {
		t.Fatalf("decode refund: %v", err)
	}
	if env.Refund.StripeRefundID == "" {
		t.Fatal("expected stripe_refund_id to be recorded for stripe-backed refund")
	}
}

func TestCreateRefundValidatesLocallyBeforeCallingStripe(t *testing.T) {
	t.Parallel()

	svc := billing.NewService()
	stripeStub := stripelib.NewStubClient()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouterWithServices(
		Config{AppEnv: "test", Auth: AuthConfig{EnableDevStubTokens: true}},
		logger,
		nil, nil, nil, nil, nil, nil, nil, svc, stripeStub, nil, nil, nil, nil,
	)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, true)

	inv := createInvoice(t, router, token, "org-demo", "INV-RPRECHECK")
	addItem(t, router, token, "org-demo", inv.Invoice.ID, 5000)
	issueInvoiceReq(t, router, token, "org-demo", inv.Invoice.ID)

	paymentBody := billingBody(map[string]any{
		"amount_cents":             5000,
		"method":                   "stripe",
		"stripe_payment_intent_id": "pi_test_preflight",
	})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/invoices/%s/payments", inv.Invoice.ID), strings.NewReader(paymentBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("record stripe payment: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var payEnv paymentEnvelope
	if err := json.NewDecoder(rr.Body).Decode(&payEnv); err != nil {
		t.Fatalf("decode stripe payment: %v", err)
	}

	refundBody := billingBody(map[string]any{
		"payment_id":   payEnv.Payment.ID,
		"amount_cents": 6000,
		"reason":       "Too much",
	})
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/invoices/%s/refunds", inv.Invoice.ID), strings.NewReader(refundBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("refund should fail local validation first: expected 422, got %d: %s", rr.Code, rr.Body.String())
	}
	if got := len(stripeStub.CreatedRefunds()); got != 0 {
		t.Fatalf("expected Stripe not to be called when local validation fails, got %d refund calls", got)
	}
}

func TestRefundExceedsPaymentAmount(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	router := newBillingRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, true)

	inv := createInvoice(t, router, token, "org-demo", "INV-REXCEED")
	addItem(t, router, token, "org-demo", inv.Invoice.ID, 5000)
	issueInvoiceReq(t, router, token, "org-demo", inv.Invoice.ID)
	payEnv := recordPaymentReq(t, router, token, "org-demo", inv.Invoice.ID, 5000)

	body := billingBody(map[string]any{
		"payment_id":   payEnv.Payment.ID,
		"amount_cents": 9999,
		"reason":       "Too much",
	})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/invoices/%s/refunds", inv.Invoice.ID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", rr.Code)
	}
}

func TestRefundRequiresReason(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	router := newBillingRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, true)

	inv := createInvoice(t, router, token, "org-demo", "INV-RNOREASON")
	addItem(t, router, token, "org-demo", inv.Invoice.ID, 5000)
	issueInvoiceReq(t, router, token, "org-demo", inv.Invoice.ID)
	payEnv := recordPaymentReq(t, router, token, "org-demo", inv.Invoice.ID, 5000)

	body := billingBody(map[string]any{
		"payment_id":   payEnv.Payment.ID,
		"amount_cents": 5000,
		"reason":       "",
	})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/invoices/%s/refunds", inv.Invoice.ID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	// reason is empty → 422 from service layer
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for missing reason, got %d", rr.Code)
	}
}

func TestRefundOnWrittenOffInvoiceRejected(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	router := newBillingRouter(t, svc)
	adminToken := issueSessionToken(t, router, "org-demo", []string{"admin"}, true)

	// Create, add item, issue, record partial payment, then write off.
	inv := createInvoice(t, router, adminToken, "org-demo", "INV-WO-REFUND")
	addItem(t, router, adminToken, "org-demo", inv.Invoice.ID, 10000)
	issueInvoiceReq(t, router, adminToken, "org-demo", inv.Invoice.ID)
	payEnv := recordPaymentReq(t, router, adminToken, "org-demo", inv.Invoice.ID, 4000)

	// Write off remaining balance.
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/invoices/%s/write-off", inv.Invoice.ID), nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("write-off: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Attempt to refund the succeeded payment after write-off — must be rejected.
	body := billingBody(map[string]any{
		"payment_id":   payEnv.Payment.ID,
		"amount_cents": 2000,
		"reason":       "Should not be allowed",
	})
	req2 := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/invoices/%s/refunds", inv.Invoice.ID), strings.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Org-Id", "org-demo")
	req2.Header.Set("Authorization", "Bearer "+adminToken)
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusConflict {
		t.Fatalf("expected 409 for refund on written_off invoice, got %d: %s", rr2.Code, rr2.Body.String())
	}
}

// ─── Tests: Household invoices ────────────────────────────────────────────────

func TestListHouseholdInvoices(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	router := newBillingRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	// Create 2 invoices for hh-001 and 1 for hh-002. The hh-002 invoice must
	// not appear in hh-001's list — this verifies household-scoped data isolation.
	createInvoiceForHousehold(t, router, token, "org-demo", "hh-001", "INV-HH1A")
	createInvoiceForHousehold(t, router, token, "org-demo", "hh-001", "INV-HH1B")
	createInvoiceForHousehold(t, router, token, "org-demo", "hh-002", "INV-HH2A")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/households/hh-001/invoices", nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var env householdInvoicesEnvelope
	json.NewDecoder(rr.Body).Decode(&env)
	if env.HouseholdID != "hh-001" {
		t.Fatalf("expected household_id=hh-001, got %q", env.HouseholdID)
	}
	// Must be exactly 2 — the hh-002 invoice must be excluded.
	if env.Count != 2 {
		t.Fatalf("expected 2 invoices for hh-001 (isolation check: hh-002 invoice must be excluded), got %d", env.Count)
	}
}

func TestListHouseholdInvoicesParentAllowed(t *testing.T) {
	t.Parallel()
	router := newBillingRouter(t, nil)
	token := issueSessionToken(t, router, "org-demo", []string{"parent"}, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/households/hh-parent/invoices", nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	// Parent can read (returns empty list, not forbidden).
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for parent listing own invoices, got %d", rr.Code)
	}
}

// ─── Tests: Ledger events ─────────────────────────────────────────────────────

func TestListLedgerEventsAdminCanRead(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	router := newBillingRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	inv := createInvoice(t, router, token, "org-demo", "INV-LEDGER")
	addItem(t, router, token, "org-demo", inv.Invoice.ID, 15000)
	issueInvoiceReq(t, router, token, "org-demo", inv.Invoice.ID)
	recordPaymentReq(t, router, token, "org-demo", inv.Invoice.ID, 15000)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ledger-events", nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var env ledgerListEnvelope
	json.NewDecoder(rr.Body).Decode(&env)
	if env.Count < 2 {
		t.Fatalf("expected at least 2 ledger events (issued + payment), got %d", env.Count)
	}
}

func TestListLedgerEventsParentForbidden(t *testing.T) {
	t.Parallel()
	router := newBillingRouter(t, nil)
	token := issueSessionToken(t, router, "org-demo", []string{"parent"}, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ledger-events", nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for parent on ledger, got %d", rr.Code)
	}
}

func TestListLedgerEventsFilterByInvoice(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	router := newBillingRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	invA := createInvoice(t, router, token, "org-demo", "INV-LA")
	addItem(t, router, token, "org-demo", invA.Invoice.ID, 5000)
	issueInvoiceReq(t, router, token, "org-demo", invA.Invoice.ID)

	invB := createInvoice(t, router, token, "org-demo", "INV-LB")
	addItem(t, router, token, "org-demo", invB.Invoice.ID, 8000)
	issueInvoiceReq(t, router, token, "org-demo", invB.Invoice.ID)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/ledger-events?invoice_id=%s", invA.Invoice.ID), nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	var env ledgerListEnvelope
	json.NewDecoder(rr.Body).Decode(&env)
	if env.Count != 1 {
		t.Fatalf("expected 1 ledger event for invoice A, got %d", env.Count)
	}
}

// ─── Tests: Get invoice ───────────────────────────────────────────────────────

func TestGetInvoiceNotFound(t *testing.T) {
	t.Parallel()
	router := newBillingRouter(t, nil)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/invoices/non-existent-id", nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestGetInvoiceCoachForbidden(t *testing.T) {
	t.Parallel()
	router := newBillingRouter(t, nil)
	token := issueSessionToken(t, router, "org-demo", []string{"coach"}, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/invoices/some-id", nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for coach, got %d", rr.Code)
	}
}

package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/httpx"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/payments"
	stripelib "github.com/luryann/aquify/server/internal/stripe"
)

// PaymentAccountHandlers lets a club's admins connect their own Stripe account
// (hosted onboarding) and see whether they can take payments.
type PaymentAccountHandlers struct {
	accounts       *payments.Service
	client         stripelib.Client
	audit          *audit.Service
	allowedOrigins []string
	required       bool
}

func NewPaymentAccountHandlers(accounts *payments.Service, client stripelib.Client, a *audit.Service, allowedOrigins []string, required bool) *PaymentAccountHandlers {
	return &PaymentAccountHandlers{accounts: accounts, client: client, audit: a, allowedOrigins: allowedOrigins, required: required}
}

func (h *PaymentAccountHandlers) RegisterRoutes(r chi.Router) {
	r.Route("/billing/payment-account", func(p chi.Router) {
		admin := p.With(appmiddleware.RBACStub("admin"))
		admin.Get("/", h.get)
		admin.Post("/onboarding", h.onboard)
	})
}

type paymentAccountView struct {
	Connected        bool   `json:"connected"`
	Required         bool   `json:"required"`
	CanTakePayments  bool   `json:"can_take_payments"`
	ChargesEnabled   bool   `json:"charges_enabled"`
	PayoutsEnabled   bool   `json:"payouts_enabled"`
	DetailsSubmitted bool   `json:"details_submitted"`
	StripeAccountID  string `json:"stripe_account_id,omitempty"`
}

func (h *PaymentAccountHandlers) view(a *payments.Account) paymentAccountView {
	if a == nil {
		return paymentAccountView{Required: h.required, CanTakePayments: !h.required}
	}
	return paymentAccountView{Connected: true, Required: h.required, CanTakePayments: a.ChargesEnabled,
		ChargesEnabled: a.ChargesEnabled, PayoutsEnabled: a.PayoutsEnabled, DetailsSubmitted: a.DetailsSubmitted, StripeAccountID: a.StripeAccountID}
}

// refresh syncs flags from Stripe so the page does not depend on webhook timing.
func (h *PaymentAccountHandlers) refresh(r *http.Request, a *payments.Account) *payments.Account {
	live, err := h.client.GetConnectedAccount(r.Context(), a.StripeAccountID)
	if err != nil || live == nil {
		return a
	}
	if live.ChargesEnabled == a.ChargesEnabled && live.PayoutsEnabled == a.PayoutsEnabled && live.DetailsSubmitted == a.DetailsSubmitted {
		return a
	}
	if upd, err := h.accounts.UpdateStatus(r.Context(), a.StripeAccountID, live.ChargesEnabled, live.PayoutsEnabled, live.DetailsSubmitted); err == nil {
		return upd
	}
	return a
}

func (h *PaymentAccountHandlers) get(w http.ResponseWriter, r *http.Request) {
	_, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	a, err := h.accounts.Get(r.Context(), orgID)
	if errors.Is(err, payments.ErrNotFound) {
		_ = httpx.WriteJSON(w, http.StatusOK, h.view(nil))
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Could not load the payment account.", err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, h.view(h.refresh(r, a)))
}

type onboardingRequest struct {
	ReturnURL  string `json:"return_url"`
	RefreshURL string `json:"refresh_url"`
}

func (h *PaymentAccountHandlers) onboard(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	var req onboardingRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeJSONDecodeError(w, r, err, "invalid_request", "Invalid request body.")
		return
	}
	req.ReturnURL, req.RefreshURL = strings.TrimSpace(req.ReturnURL), strings.TrimSpace(req.RefreshURL)
	if req.RefreshURL == "" {
		req.RefreshURL = req.ReturnURL
	}
	if !isAllowedReturnURL(req.ReturnURL, h.allowedOrigins) || !isAllowedReturnURL(req.RefreshURL, h.allowedOrigins) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_return_url", "return_url and refresh_url must belong to a trusted application origin.", nil)
		return
	}

	a, err := h.accounts.Get(r.Context(), orgID)
	if errors.Is(err, payments.ErrNotFound) {
		created, cerr := h.client.CreateConnectedAccount(r.Context(), stripelib.ConnectedAccountParams{
			OrgID: orgID, Email: principal.Email, IdempotencyKey: "connect-account:" + orgID,
		})
		if cerr != nil {
			httpx.WriteError(w, r, http.StatusBadGateway, "stripe_error", "Could not create the Stripe account.", cerr)
			return
		}
		a, err = h.accounts.Create(r.Context(), orgID, created.ID)
		if errors.Is(err, payments.ErrAlreadyConnected) { // lost a race with another admin request
			a, err = h.accounts.Get(r.Context(), orgID)
		}
		if err == nil {
			_ = recordAudit(h.audit, principal, orgID, audit.ActionBillingPaymentAccountCreate, "payment_account", a.StripeAccountID, nil, nil, nil)
		}
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Could not set up the payment account.", err)
		return
	}
	link, err := h.client.CreateAccountLink(r.Context(), a.StripeAccountID, req.RefreshURL, req.ReturnURL)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "stripe_error", "Could not create the onboarding link.", err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{"url": link, "account": h.view(a)})
}

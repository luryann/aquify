package middleware

import (
	"errors"
	"fmt"
	"slices"
)

type Action string

const (
	ActionAssignRole           Action = "roles.assign"
	ActionBillingRefund        Action = "billing.refund.create"
	ActionBillingVoid          Action = "billing.invoice.void"
	ActionBillingWriteOff      Action = "billing.invoice.write_off"
	ActionBillingPaymentRecord Action = "billing.payment.record"
)

var ErrPermissionDenied = errors.New("permission denied")
var ErrMFARequired = errors.New("mfa required")

// adminActionsRequireMFA is the set of privileged actions gated to the admin
// role; every action in this framework is admin-only, so the map's value is
// just whether MFA step-up is additionally required for that action.
var adminActionsRequireMFA = map[Action]bool{
	ActionAssignRole:           true,
	ActionBillingRefund:        true,
	ActionBillingVoid:          true,
	ActionBillingWriteOff:      true,
	ActionBillingPaymentRecord: true,
}

func CheckPermission(principal *Principal, action Action) error {
	if principal == nil {
		return ErrPermissionDenied
	}

	requireMFA, ok := adminActionsRequireMFA[action]
	if !ok {
		return fmt.Errorf("%w: unknown action %q", ErrPermissionDenied, action)
	}

	if !slices.Contains(principal.Roles, "admin") {
		return ErrPermissionDenied
	}
	if requireMFA && !principal.MFAAuthenticated {
		return ErrMFARequired
	}
	return nil
}

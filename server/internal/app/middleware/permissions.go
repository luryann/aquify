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

type actionRule struct {
	AllowedRoles []string
	RequireMFA   bool
}

var actionRules = map[Action]actionRule{
	ActionAssignRole: {
		AllowedRoles: []string{"owner", "admin"},
		RequireMFA:   true,
	},
	ActionBillingRefund: {
		AllowedRoles: []string{"owner", "admin", "billing"},
		RequireMFA:   true,
	},
	ActionBillingVoid: {
		AllowedRoles: []string{"owner", "admin"},
		RequireMFA:   true,
	},
	ActionBillingWriteOff: {
		AllowedRoles: []string{"owner", "admin"},
		RequireMFA:   true,
	},
	ActionBillingPaymentRecord: {
		AllowedRoles: []string{"owner", "admin", "billing"},
		RequireMFA:   true,
	},
}

func CheckPermission(principal *Principal, action Action) error {
	if principal == nil {
		return ErrPermissionDenied
	}

	rule, ok := actionRules[action]
	if !ok {
		return fmt.Errorf("%w: unknown action %q", ErrPermissionDenied, action)
	}

	for _, role := range principal.Roles {
		if slices.Contains(rule.AllowedRoles, role) {
			if rule.RequireMFA && !principal.MFAAuthenticated {
				return ErrMFARequired
			}
			return nil
		}
	}

	return ErrPermissionDenied
}

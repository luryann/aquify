package authn

import (
	"strings"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

const (
	LocalDevOrgID          = "00000000-0000-0000-0000-00000000a001"
	legacyLocalDevOrgID    = "org-demo"
	LocalDevOwnerUserID    = "00000000-0000-0000-0000-00000000a101"
	LocalDevAdminUserID    = "00000000-0000-0000-0000-00000000a102"
	LocalDevCoachUserID    = "00000000-0000-0000-0000-00000000a103"
	LocalDevParentUserID   = "00000000-0000-0000-0000-00000000a104"
	LocalDevBillingUserID  = "00000000-0000-0000-0000-00000000a105"
	LocalDevReadOnlyUserID = "00000000-0000-0000-0000-00000000a106"
	LocalDevSwimmerUserID  = "00000000-0000-0000-0000-00000000a107"
)

type LocalDevUser struct {
	Token  string
	UserID string
	Email  string
	Name   string
	Roles  []string
}

var localDevUsers = []LocalDevUser{
	{Token: "stub-owner", UserID: LocalDevOwnerUserID, Email: "owner@dare.local", Name: "Dev Owner", Roles: []string{"owner"}},
	{Token: "stub-admin", UserID: LocalDevAdminUserID, Email: "admin@dare.local", Name: "Dev Admin", Roles: []string{"admin"}},
	{Token: "stub-coach", UserID: LocalDevCoachUserID, Email: "coach@dare.local", Name: "Dev Coach", Roles: []string{"coach"}},
	{Token: "stub-parent", UserID: LocalDevParentUserID, Email: "parent@dare.local", Name: "Dev Parent", Roles: []string{"parent"}},
	{Token: "stub-billing", UserID: LocalDevBillingUserID, Email: "billing@dare.local", Name: "Dev Billing", Roles: []string{"billing"}},
	{Token: "stub-readonly", UserID: LocalDevReadOnlyUserID, Email: "readonly@dare.local", Name: "Dev Read-Only", Roles: []string{"read-only"}},
	{Token: "stub-swimmer", UserID: LocalDevSwimmerUserID, Email: "swimmer@dare.local", Name: "Emma Chen", Roles: []string{"swimmer"}},
}

var localDevUserIDs = map[string]struct{}{
	LocalDevOwnerUserID:    {},
	LocalDevAdminUserID:    {},
	LocalDevCoachUserID:    {},
	LocalDevParentUserID:   {},
	LocalDevBillingUserID:  {},
	LocalDevReadOnlyUserID: {},
	LocalDevSwimmerUserID:  {},
}

func LocalDevUsers() []LocalDevUser {
	users := make([]LocalDevUser, 0, len(localDevUsers))
	for _, user := range localDevUsers {
		users = append(users, LocalDevUser{
			Token:  user.Token,
			UserID: user.UserID,
			Email:  user.Email,
			Name:   user.Name,
			Roles:  append([]string{}, user.Roles...),
		})
	}
	return users
}

func localDevStaticPrincipals() map[string]*appmiddleware.Principal {
	principals := make(map[string]*appmiddleware.Principal, len(localDevUsers)+1)
	for _, user := range localDevUsers {
		principals[user.Token] = &appmiddleware.Principal{
			UserID: user.UserID,
			Email:  user.Email,
			Name:   user.Name,
			OrgID:  LocalDevOrgID,
			Roles:  append([]string{}, user.Roles...),
		}
	}
	principals["stub-owner"].MFAAuthenticated = true
	principals["stub-admin-mfa"] = &appmiddleware.Principal{
		UserID:           LocalDevAdminUserID,
		Email:            "admin@dare.local",
		Name:             "Dev Admin",
		OrgID:            LocalDevOrgID,
		Roles:            []string{"admin"},
		MFAAuthenticated: true,
	}
	return principals
}

func normalizeLocalDevOrgID(userID, email, orgID string) string {
	orgID = strings.TrimSpace(orgID)
	if !strings.EqualFold(orgID, legacyLocalDevOrgID) {
		return orgID
	}

	if _, ok := localDevUserIDs[strings.TrimSpace(userID)]; ok {
		return LocalDevOrgID
	}
	if strings.HasSuffix(strings.ToLower(strings.TrimSpace(email)), "@dare.local") {
		return LocalDevOrgID
	}

	return orgID
}

func normalizeLocalDevPrincipal(principal appmiddleware.Principal) appmiddleware.Principal {
	principal.OrgID = normalizeLocalDevOrgID(principal.UserID, principal.Email, principal.OrgID)
	return principal
}

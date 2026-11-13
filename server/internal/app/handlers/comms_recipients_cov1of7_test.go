package handlers

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/luryann/aquify/server/internal/app/comms"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

// A group campaign must only reach households whose registration is pending or
// active; once the registration is inactive or withdrawn the household drops out.
func TestCov1x7GroupRecipientsExcludeEndedRegistrations(t *testing.T) {
	c := cov1o7NewComms(t, "org-cov1x-ended")
	ctx := context.Background()
	admin := &appmiddleware.Principal{UserID: cov1o7UAdmin, OrgID: c.org, Roles: []string{"admin"}}
	campaign := &comms.EmailCampaign{OrgID: c.org, Audience: "group", GroupID: c.groupID}

	before, err := c.h.resolveCampaignRecipients(ctx, c.org, campaign)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) == 0 {
		t.Fatal("household with a pending registration must receive group campaigns")
	}
	for _, r := range before {
		if strings.EqualFold(r.Email, "b@example.com") {
			t.Fatalf("household B has no registration in the group but is a recipient: %v", before)
		}
	}
	if !slices.ContainsFunc(before, func(r comms.Recipient) bool { return strings.EqualFold(r.Email, "a@example.com") }) {
		t.Fatalf("household A primary email missing from recipients: %v", before)
	}

	for _, to := range []string{"active", "inactive", "withdrawn"} {
		if _, _, err := c.grp.TransitionRegistrationStatus(admin, c.org, c.reg, to); err != nil {
			t.Fatalf("transition to %s: %v", to, err)
		}
		got, err := c.h.resolveCampaignRecipients(ctx, c.org, campaign)
		if err != nil {
			t.Fatal(err)
		}
		wantRecipients := to == "active"
		if (len(got) > 0) != wantRecipients {
			t.Fatalf("registration %s: recipients = %v, want recipients=%v", to, got, wantRecipients)
		}
	}
}

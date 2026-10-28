package roster

import (
	"strings"

	"github.com/dareaquatics/aquify/server/internal/app/groups"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

// MembersDirectory is the production Directory backed by the members and
// groups services. Both services re-sync from Postgres on these read paths,
// so multiple API instances observe the same roster.
type MembersDirectory struct {
	members *members.Service
	groups  *groups.Service
}

// NewMembersDirectory builds a Directory over the members and groups services.
// groupsService may be nil (group IDs then resolve as codes).
func NewMembersDirectory(membersService *members.Service, groupsService *groups.Service) *MembersDirectory {
	return &MembersDirectory{members: membersService, groups: groupsService}
}

// systemPrincipal is used for trusted server-side roster reads; RBAC is
// enforced by the calling handler before any data is returned.
func systemPrincipal(orgID string) *appmiddleware.Principal {
	return &appmiddleware.Principal{UserID: "system:roster", OrgID: orgID, Roles: []string{"admin"}}
}

// Swimmers implements Directory.
func (d *MembersDirectory) Swimmers(orgID string) ([]Swimmer, error) {
	if d == nil || d.members == nil {
		return []Swimmer{}, nil
	}
	actor := systemPrincipal(orgID)
	views, err := d.members.ListSwimmers(actor, orgID, members.ListSwimmersInput{})
	if err != nil {
		return nil, err
	}
	links, err := d.members.ListSwimmerGroupLinks(actor, orgID)
	if err != nil {
		return nil, err
	}
	codes := map[string][]string{}
	primary := map[string]string{}
	for _, link := range links {
		code := CanonicalGroupCode(link.GroupCode)
		codes[link.SwimmerID] = append(codes[link.SwimmerID], code)
		if link.IsPrimary {
			primary[link.SwimmerID] = code
		}
	}
	out := make([]Swimmer, 0, len(views))
	for _, v := range views {
		out = append(out, Swimmer{
			ID:           v.ID,
			OrgID:        v.OrgID,
			HouseholdID:  v.HouseholdID,
			FirstName:    v.FirstName,
			LastName:     v.LastName,
			DisplayName:  DisplayName(v.PreferredName, v.FirstName, v.LastName),
			GroupCodes:   append([]string{}, codes[v.ID]...),
			PrimaryGroup: primary[v.ID],
		})
	}
	SortByName(out)
	return out, nil
}

// HouseholdIDsForPrincipal implements Directory using the members service's
// own household scoping (parents may manage several associated households).
func (d *MembersDirectory) HouseholdIDsForPrincipal(principal *appmiddleware.Principal, orgID string) ([]string, error) {
	if principal == nil || !HasAnyRole(principal, "parent", "swimmer") {
		return []string{}, nil
	}
	ids := []string{}
	seen := map[string]bool{}
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if d == nil || d.members == nil {
		add(principal.HouseholdID)
		return ids, nil
	}
	// Ask with a parent/swimmer-only view of the principal so staff roles on
	// a mixed-role account do not widen the household set.
	scoped := *principal
	scoped.Roles = []string{}
	for _, r := range principal.Roles {
		if strings.EqualFold(r, "parent") || strings.EqualFold(r, "swimmer") {
			scoped.Roles = append(scoped.Roles, strings.ToLower(r))
		}
	}
	households, err := d.members.ListHouseholds(&scoped, orgID)
	if err != nil {
		return nil, err
	}
	for _, h := range households {
		add(h.ID)
	}
	return ids, nil
}

// ResolveGroupCode implements Directory: practices may store a group's ID or
// its code; both resolve to the canonical code.
func (d *MembersDirectory) ResolveGroupCode(orgID, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if d != nil && d.groups != nil {
		if grp, err := d.groups.GetGroup(systemPrincipal(orgID), orgID, ref); err == nil {
			return CanonicalGroupCode(grp.Code)
		}
	}
	return CanonicalGroupCode(ref)
}

// HouseholdNames implements Directory.
func (d *MembersDirectory) HouseholdNames(orgID string) (map[string]string, error) {
	out := map[string]string{}
	if d == nil || d.members == nil {
		return out, nil
	}
	households, err := d.members.ListHouseholds(systemPrincipal(orgID), orgID)
	if err != nil {
		return nil, err
	}
	for _, h := range households {
		out[h.ID] = h.HouseholdName
	}
	return out, nil
}

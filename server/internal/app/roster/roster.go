// Package roster gives the engagement domains (club events, wellness,
// attendance summaries) a narrow, read-only view of swimmers, households and
// group membership without importing the members/groups services directly.
package roster

import (
	"sort"
	"strings"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

// Swimmer is the minimal swimmer projection the engagement domains need.
type Swimmer struct {
	ID           string   `json:"id"`
	OrgID        string   `json:"org_id"`
	HouseholdID  string   `json:"household_id"`
	FirstName    string   `json:"first_name"`
	LastName     string   `json:"last_name"`
	DisplayName  string   `json:"display_name"`
	GroupCodes   []string `json:"group_codes"`
	PrimaryGroup string   `json:"primary_group,omitempty"`
}

// InGroup reports whether the swimmer belongs to the group code (exact,
// canonical-uppercase comparison).
func (s Swimmer) InGroup(code string) bool {
	code = CanonicalGroupCode(code)
	for _, c := range s.GroupCodes {
		if c == code {
			return true
		}
	}
	return false
}

// InAnyGroup reports whether the swimmer belongs to at least one of codes.
func (s Swimmer) InAnyGroup(codes []string) bool {
	for _, code := range codes {
		if s.InGroup(code) {
			return true
		}
	}
	return false
}

// Directory resolves roster facts. Implementations are trusted server-side
// lookups: they are NOT role-scoped, so callers must enforce RBAC themselves.
type Directory interface {
	// Swimmers returns every swimmer in the org.
	Swimmers(orgID string) ([]Swimmer, error)
	// HouseholdIDsForPrincipal returns the households a parent/swimmer
	// principal may act for (empty for staff-only principals).
	HouseholdIDsForPrincipal(principal *appmiddleware.Principal, orgID string) ([]string, error)
	// ResolveGroupCode maps a group ID or code to its canonical code.
	ResolveGroupCode(orgID, ref string) string
	// HouseholdNames returns household ID -> display name for the org.
	HouseholdNames(orgID string) (map[string]string, error)
}

// CanonicalGroupCode normalises a group code to the canonical uppercase form.
func CanonicalGroupCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// IsStaff reports whether the principal holds admin or coach.
func IsStaff(principal *appmiddleware.Principal) bool {
	return HasAnyRole(principal, "admin", "coach")
}

// HasAnyRole reports whether the principal holds any of roles.
func HasAnyRole(principal *appmiddleware.Principal, roles ...string) bool {
	if principal == nil {
		return false
	}
	for _, have := range principal.Roles {
		for _, want := range roles {
			if strings.EqualFold(strings.TrimSpace(have), want) {
				return true
			}
		}
	}
	return false
}

// SwimmerByID returns the swimmer with id from list.
func SwimmerByID(list []Swimmer, id string) (Swimmer, bool) {
	id = strings.TrimSpace(id)
	for _, sw := range list {
		if sw.ID == id {
			return sw, true
		}
	}
	return Swimmer{}, false
}

// SwimmersInHousehold filters list to one household, sorted by display name.
func SwimmersInHousehold(list []Swimmer, householdID string) []Swimmer {
	out := []Swimmer{}
	for _, sw := range list {
		if sw.HouseholdID == householdID {
			out = append(out, sw)
		}
	}
	SortByName(out)
	return out
}

// SortByName orders swimmers by last name, then first name, then ID.
func SortByName(list []Swimmer) {
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if !strings.EqualFold(a.LastName, b.LastName) {
			return strings.ToLower(a.LastName) < strings.ToLower(b.LastName)
		}
		if !strings.EqualFold(a.FirstName, b.FirstName) {
			return strings.ToLower(a.FirstName) < strings.ToLower(b.FirstName)
		}
		return a.ID < b.ID
	})
}

// StaticDirectory is an in-memory Directory used by tests and fixtures.
type StaticDirectory struct {
	SwimmerList []Swimmer
	// ParentHouseholds maps a parent user ID to the households they manage.
	ParentHouseholds map[string][]string
	// GroupIDs maps a group ID to its canonical code.
	GroupIDs map[string]string
	// Households maps household ID to name.
	Households map[string]string
}

// HouseholdNames implements Directory.
func (d *StaticDirectory) HouseholdNames(_ string) (map[string]string, error) {
	out := map[string]string{}
	for k, v := range d.Households {
		out[k] = v
	}
	return out, nil
}

// Swimmers implements Directory.
func (d *StaticDirectory) Swimmers(orgID string) ([]Swimmer, error) {
	out := []Swimmer{}
	for _, sw := range d.SwimmerList {
		if sw.OrgID == orgID {
			out = append(out, sw)
		}
	}
	return out, nil
}

// HouseholdIDsForPrincipal implements Directory.
func (d *StaticDirectory) HouseholdIDsForPrincipal(principal *appmiddleware.Principal, _ string) ([]string, error) {
	if principal == nil {
		return nil, nil
	}
	seen := map[string]bool{}
	out := []string{}
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	if HasAnyRole(principal, "parent", "swimmer") {
		add(principal.HouseholdID)
	}
	if HasAnyRole(principal, "parent") {
		for _, id := range d.ParentHouseholds[principal.UserID] {
			add(id)
		}
	}
	return out, nil
}

// ResolveGroupCode implements Directory.
func (d *StaticDirectory) ResolveGroupCode(_ string, ref string) string {
	if code, ok := d.GroupIDs[strings.TrimSpace(ref)]; ok {
		return CanonicalGroupCode(code)
	}
	return CanonicalGroupCode(ref)
}

// DisplayName builds a swimmer's display name from preferred/first/last.
func DisplayName(preferred, first, last string) string {
	first = strings.TrimSpace(first)
	if p := strings.TrimSpace(preferred); p != "" {
		first = p
	}
	return strings.TrimSpace(first + " " + strings.TrimSpace(last))
}

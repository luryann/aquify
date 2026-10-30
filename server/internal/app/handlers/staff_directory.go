package handlers

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/dareaquatics/aquify/server/internal/keycloak"
)

// staffDirectoryLimit bounds the Keycloak page scanned for staff; clubs have
// far fewer accounts than this, and the admin /users list uses the same cap.
const staffDirectoryLimit = 500

// StaffMember is an enabled admin/coach account in an org.
type StaffMember struct {
	ID        string
	FirstName string
	LastName  string
	Email     string
	// Roles are the normalized realm roles (sorted, deduplicated).
	Roles []string
}

// PersonalName is first + last name only; empty when the account has none.
func (m StaffMember) PersonalName() string {
	return strings.TrimSpace(m.FirstName + " " + m.LastName)
}

// DisplayName falls back to the email so authenticated staff screens never
// render a blank. Public surfaces must use PersonalName instead.
func (m StaffMember) DisplayName() string {
	if name := m.PersonalName(); name != "" {
		return name
	}
	return m.Email
}

// PublicRole is the label shown on the public team site.
func (m StaffMember) PublicRole() string {
	switch {
	case slices.Contains(m.Roles, "coach"):
		return "Coach"
	case slices.Contains(m.Roles, "admin"):
		return "Administrator"
	}
	return ""
}

// StaffLister lists an org's staff. Both /users/staff and the public team site
// read it, so they cannot disagree about who is staff.
type StaffLister interface {
	ListStaff(ctx context.Context, orgID string) ([]StaffMember, error)
}

// StaffDirectory is the single place that decides who is staff: enabled users
// holding the admin or coach realm role, sorted case-insensitively by name.
type StaffDirectory struct {
	kc keycloak.Client
}

func NewStaffDirectory(kc keycloak.Client) *StaffDirectory { return &StaffDirectory{kc: kc} }

func (d *StaffDirectory) ListStaff(ctx context.Context, orgID string) ([]StaffMember, error) {
	users, err := d.kc.ListUsers(ctx, orgID, "", staffDirectoryLimit, 0)
	if err != nil {
		return nil, err
	}
	staff := make([]StaffMember, 0)
	for _, u := range users {
		if !u.Enabled {
			continue
		}
		realmRoles, err := d.kc.GetUserRealmRoles(ctx, u.ID)
		if err != nil {
			return nil, err
		}
		roles := normalizeRealmRoles(realmRoles)
		if !slices.Contains(roles, "admin") && !slices.Contains(roles, "coach") {
			continue
		}
		staff = append(staff, StaffMember{ID: u.ID, FirstName: u.FirstName, LastName: u.LastName, Email: u.Email, Roles: roles})
	}
	slices.SortFunc(staff, func(a, b StaffMember) int {
		return strings.Compare(strings.ToLower(a.DisplayName()), strings.ToLower(b.DisplayName()))
	})
	return staff, nil
}

// CachedStaffLister caches successful per-org results for a short TTL so
// unauthenticated traffic cannot fan out into one Keycloak call per staff
// account per request. Errors are never cached.
type CachedStaffLister struct {
	inner StaffLister
	ttl   time.Duration
	now   func() time.Time

	mu       sync.Mutex
	entries  map[string]cachedStaff
	inflight map[string]*staffCall
}

// staffCall is one in-flight inner lookup shared by every concurrent miss for
// the same org (a minimal singleflight).
type staffCall struct {
	done  chan struct{}
	staff []StaffMember
	err   error
}

type cachedStaff struct {
	staff   []StaffMember
	expires time.Time
}

// NewCachedStaffLister wraps inner; now defaults to time.Now when nil.
func NewCachedStaffLister(inner StaffLister, ttl time.Duration, now func() time.Time) *CachedStaffLister {
	if now == nil {
		now = time.Now
	}
	return &CachedStaffLister{inner: inner, ttl: ttl, now: now, entries: map[string]cachedStaff{}, inflight: map[string]*staffCall{}}
}

func (c *CachedStaffLister) ListStaff(ctx context.Context, orgID string) ([]StaffMember, error) {
	c.mu.Lock()
	if e, ok := c.entries[orgID]; ok && c.now().Before(e.expires) {
		out := slices.Clone(e.staff)
		c.mu.Unlock()
		return out, nil
	}
	// Concurrent misses for the same org share one inner call.
	if call, ok := c.inflight[orgID]; ok {
		c.mu.Unlock()
		select {
		case <-call.done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if call.err != nil {
			return nil, call.err
		}
		return slices.Clone(call.staff), nil
	}
	call := &staffCall{done: make(chan struct{})}
	c.inflight[orgID] = call
	c.mu.Unlock()

	// The lookup is shared, so it must not die with the first caller's request.
	call.staff, call.err = c.inner.ListStaff(context.WithoutCancel(ctx), orgID)

	c.mu.Lock()
	delete(c.inflight, orgID)
	if call.err == nil {
		c.entries[orgID] = cachedStaff{staff: slices.Clone(call.staff), expires: c.now().Add(c.ttl)}
	}
	c.mu.Unlock()
	close(call.done)

	if call.err != nil {
		return nil, call.err
	}
	return slices.Clone(call.staff), nil
}

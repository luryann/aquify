// Package orgs is the tenant registry: org identity, lifecycle status, slugs and
// custom domains. Every other domain keys its rows by org_id; this package decides
// whether that org exists and may operate.
package orgs

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/dbutil"
	"github.com/luryann/aquify/server/internal/tenantdb"
)

const (
	StatusProvisioning = "provisioning"
	StatusActive       = "active"
	StatusSuspended    = "suspended"
	StatusOffboarding  = "offboarding"
	StatusDeleted      = "deleted"
	// StatusPurging is the claimed state of an in-progress purge: cancel-offboarding
	// is refused, access stays off, and the purge may be retried until it finishes.
	StatusPurging = "purging"
)

var (
	ErrNotFound      = errors.New("org not found")
	ErrSlugTaken     = errors.New("org slug is already taken")
	ErrInvalidSlug   = errors.New("slug must be 3-40 chars of a-z, 0-9 and hyphen")
	ErrInvalidStatus = errors.New("invalid org status transition")
	ErrInvalidDomain = errors.New("invalid domain")
	ErrDomainTaken   = errors.New("domain is already registered")
)

var (
	slugRe   = regexp.MustCompile(`^[a-z0-9-]{3,40}$`)
	domainRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)
)

// Org is one tenant.
type Org struct {
	ID          string     `json:"id"`
	Slug        string     `json:"slug,omitempty"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Plan        string     `json:"plan,omitempty"`
	Region      string     `json:"region,omitempty"`
	SuspendedAt *time.Time `json:"suspended_at,omitempty"`
	// OffboardingAt is when the org entered offboarding; deletion waits out a cooling-off period from here.
	OffboardingAt *time.Time `json:"offboarding_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// CreateInput describes a new org. ID is optional (generated when empty).
type CreateInput struct {
	ID     string
	Slug   string
	Name   string
	Plan   string
	Region string
}

// allowed lifecycle transitions.
var transitions = map[string][]string{
	StatusProvisioning: {StatusActive, StatusDeleted},
	StatusActive:       {StatusSuspended, StatusOffboarding},
	StatusSuspended:    {StatusActive, StatusOffboarding},
	StatusOffboarding:  {StatusDeleted, StatusSuspended, StatusPurging},
	StatusPurging:      {StatusDeleted},
	StatusDeleted:      {},
}

// ValidSlug reports whether s is an acceptable public org slug.
func ValidSlug(s string) bool { return slugRe.MatchString(s) }

// Service reads and writes the registry. With no pool it is in-memory: orgs that
// were never registered are treated as active (dev/test), registered ones follow
// their status.
type Service struct {
	pool *pgxpool.Pool
	ttl  time.Duration
	now  func() time.Time

	mu      sync.RWMutex
	mem     map[string]*Org
	domains map[string]memDomain
	cache   map[string]cacheEntry
	gen     uint64 // bumped by invalidate; a Status read that started before a bump must not cache
}

type memDomain struct {
	orgID    string
	verified bool
}

type cacheEntry struct {
	status  string
	found   bool
	expires time.Time
}

// NewService returns an in-memory registry.
func NewService() *Service {
	return &Service{ttl: 15 * time.Second, now: time.Now, mem: map[string]*Org{}, domains: map[string]memDomain{}, cache: map[string]cacheEntry{}}
}

// NewServiceWithPool returns a Postgres-backed registry.
// at binds orgID to statements on the tenant-owned org_domains table (row-level security).
func (s *Service) at(orgID string) tenantdb.Scoped { return tenantdb.Pool{P: s.pool}.For(orgID) }

// forDomain binds a request host for the one lookup made before the club is known
// (policy domain_lookup exposes only that host's verified row).
func (s *Service) forDomain(host string) tenantdb.Scoped {
	return tenantdb.Pool{P: s.pool}.ForLookup(tenantdb.LookupDomain, host)
}

func NewServiceWithPool(pool *pgxpool.Pool) *Service {
	s := NewService()
	s.pool = pool
	return s
}

// SetStatusCacheTTL tunes how long IsActive results are cached (tests).
func (s *Service) SetStatusCacheTTL(d time.Duration) { s.ttl = d }

func (s *Service) invalidate(orgID string) {
	s.mu.Lock()
	delete(s.cache, orgID)
	s.gen++
	s.mu.Unlock()
}

// Create registers a new org in the provisioning state.
func (s *Service) Create(ctx context.Context, in CreateInput) (*Org, error) {
	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	in.Name = strings.TrimSpace(in.Name)
	if in.Slug != "" && !slugRe.MatchString(in.Slug) {
		return nil, ErrInvalidSlug
	}
	if in.Name == "" {
		return nil, errors.New("name is required")
	}
	if strings.TrimSpace(in.ID) == "" {
		in.ID = uuid.NewString()
	} else if _, err := uuid.Parse(in.ID); err != nil {
		return nil, fmt.Errorf("id must be a UUID: %w", err)
	}
	now := s.now().UTC()
	o := &Org{ID: in.ID, Slug: in.Slug, Name: in.Name, Status: StatusProvisioning, Plan: in.Plan, Region: in.Region, CreatedAt: now, UpdatedAt: now}
	if s.pool == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		if _, ok := s.mem[o.ID]; ok {
			return nil, fmt.Errorf("org %s already exists", o.ID)
		}
		if o.Slug != "" {
			for _, x := range s.mem {
				if x.Slug == o.Slug {
					return nil, ErrSlugTaken
				}
			}
		}
		cp := *o
		s.mem[o.ID] = &cp
		return o, nil
	}
	var slug any
	if o.Slug != "" {
		slug = o.Slug
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO orgs (id, slug, name, status, plan, region) VALUES ($1::uuid, $2, $3, 'provisioning', $4, $5)`,
		o.ID, slug, o.Name, o.Plan, o.Region)
	if err != nil {
		if dbutil.IsUniqueViolation(err, "orgs_slug_uidx") {
			return nil, ErrSlugTaken
		}
		return nil, fmt.Errorf("create org: %w", err)
	}
	return s.Get(ctx, o.ID)
}

const orgCols = `id::text, COALESCE(slug, ''), name, status, plan, region, suspended_at, created_at, updated_at, offboarding_at`

func scanOrg(row pgx.Row) (*Org, error) {
	o := &Org{}
	if err := row.Scan(&o.ID, &o.Slug, &o.Name, &o.Status, &o.Plan, &o.Region, &o.SuspendedAt, &o.CreatedAt, &o.UpdatedAt, &o.OffboardingAt); err != nil {
		return nil, err
	}
	return o, nil
}

// Get returns one org or ErrNotFound.
func (s *Service) Get(ctx context.Context, id string) (*Org, error) {
	id = strings.TrimSpace(id)
	if s.pool == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		if o, ok := s.mem[id]; ok {
			cp := *o
			return &cp, nil
		}
		return nil, ErrNotFound
	}
	if _, err := uuid.Parse(id); err != nil {
		return nil, ErrNotFound
	}
	o, err := scanOrg(s.pool.QueryRow(ctx, `SELECT `+orgCols+` FROM orgs WHERE id = $1::uuid`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return o, err
}

// BySlug returns the org that owns slug, or ErrNotFound.
func (s *Service) BySlug(ctx context.Context, slug string) (*Org, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if slug == "" {
		return nil, ErrNotFound
	}
	if s.pool == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, o := range s.mem {
			if o.Slug == slug {
				cp := *o
				return &cp, nil
			}
		}
		return nil, ErrNotFound
	}
	o, err := scanOrg(s.pool.QueryRow(ctx, `SELECT `+orgCols+` FROM orgs WHERE slug = $1`, slug))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return o, err
}

// BySlugOrLegacy resolves a public slug: the registry first, then the slug clubs
// set in their settings before the registry existed (legacyOwner returns the org id
// that holds slug there, "" if none). The returned org carries that slug as its handle.
func (s *Service) BySlugOrLegacy(ctx context.Context, slug string, legacyOwner func(slug string) (orgID string, err error)) (*Org, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	o, err := s.BySlug(ctx, slug)
	if err == nil || !errors.Is(err, ErrNotFound) {
		return o, err
	}
	if legacyOwner == nil {
		return nil, ErrNotFound
	}
	id, err := legacyOwner(slug)
	if err != nil || id == "" {
		return nil, ErrNotFound
	}
	byID, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	cp := *byID
	cp.Slug = slug
	return &cp, nil
}

// List returns orgs ordered by creation time; status "" returns all.
func (s *Service) List(ctx context.Context, status string) ([]*Org, error) {
	if s.pool == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		out := []*Org{}
		for _, o := range s.mem {
			if status == "" || o.Status == status {
				cp := *o
				out = append(out, &cp)
			}
		}
		sort.Slice(out, func(i, j int) bool {
			return out[i].CreatedAt.Before(out[j].CreatedAt) || (out[i].CreatedAt.Equal(out[j].CreatedAt) && out[i].ID < out[j].ID)
		})
		return out, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT `+orgCols+` FROM orgs WHERE ($1 = '' OR status = $1) ORDER BY created_at, id`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Org{}
	for rows.Next() {
		o, err := scanOrg(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// ActiveIDs returns the ids of every active org (scheduler discovery).
func (s *Service) ActiveIDs(ctx context.Context) ([]string, error) {
	list, err := s.List(ctx, StatusActive)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(list))
	for _, o := range list {
		ids = append(ids, o.ID)
	}
	return ids, nil
}

// SetStatus moves an org along the lifecycle.
func (s *Service) SetStatus(ctx context.Context, id, to string) (*Org, error) {
	cur, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	ok := false
	for _, n := range transitions[cur.Status] {
		if n == to {
			ok = true
		}
	}
	if !ok {
		return nil, fmt.Errorf("%w: %s -> %s", ErrInvalidStatus, cur.Status, to)
	}
	now := s.now().UTC()
	if s.pool == nil {
		s.mu.Lock()
		o := s.mem[cur.ID]
		o.Status, o.UpdatedAt = to, now
		if to == StatusSuspended {
			o.SuspendedAt = &now
		} else if to == StatusActive {
			o.SuspendedAt = nil
		}
		if to == StatusOffboarding && cur.Status != StatusOffboarding {
			o.OffboardingAt = &now
		} else if to != StatusOffboarding && to != StatusDeleted && to != StatusPurging {
			o.OffboardingAt = nil // offboarding was cancelled
		}
		s.mu.Unlock()
		s.invalidate(cur.ID)
		return s.Get(ctx, cur.ID)
	}
	// Guard on the status we read so concurrent transitions cannot interleave.
	tag, err := s.pool.Exec(ctx, `UPDATE orgs SET status = $2,
		suspended_at = CASE WHEN $2 = 'suspended' THEN now() WHEN $2 = 'active' THEN NULL ELSE suspended_at END,
		offboarding_at = CASE WHEN $2 = 'offboarding' THEN now() WHEN $2 IN ('deleted', 'purging') THEN offboarding_at ELSE NULL END
		WHERE id = $1::uuid AND status = $3`, cur.ID, to, cur.Status)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, fmt.Errorf("%w: org changed concurrently", ErrInvalidStatus)
	}
	s.invalidate(cur.ID)
	return s.Get(ctx, cur.ID)
}

// Tombstone atomically releases the slug and marks a purging org deleted, so a
// failure can never leave an org with its slug cleared but its status unchanged.
func (s *Service) Tombstone(ctx context.Context, id string) error {
	if s.pool == nil {
		_, err := s.SetStatus(ctx, id, StatusDeleted)
		if err == nil {
			s.mu.Lock()
			if o := s.mem[strings.TrimSpace(id)]; o != nil {
				o.Slug = ""
			}
			s.mu.Unlock()
		}
		return err
	}
	tag, err := s.pool.Exec(ctx, `UPDATE orgs SET slug = NULL, status = 'deleted' WHERE id = $1::uuid AND status = 'purging'`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: org is not being purged", ErrInvalidStatus)
	}
	s.invalidate(id)
	return nil
}

// Status returns the org's status. found is false when the org is unknown.
func (s *Service) Status(ctx context.Context, id string) (status string, found bool, err error) {
	id = strings.TrimSpace(id)
	// Hit path: a shared read lock, so concurrent requests of all orgs do not serialise.
	s.mu.RLock()
	e, hit := s.cache[id]
	gen := s.gen
	s.mu.RUnlock()
	if hit && s.now().Before(e.expires) {
		return e.status, e.found, nil
	}
	o, err := s.Get(ctx, id)
	switch {
	case errors.Is(err, ErrNotFound):
		status, found = "", false
	case err != nil:
		return "", false, err
	default:
		status, found = o.Status, true
	}
	s.mu.Lock()
	// Skip the store if an invalidation ran while we were reading: our row may be stale.
	if s.gen == gen {
		s.cache[id] = cacheEntry{status: status, found: found, expires: s.now().Add(s.ttl)}
	}
	s.mu.Unlock()
	return status, found, nil
}

// IsActive reports whether the org may serve requests. In-memory mode treats an
// unregistered org as active so dev/test fixtures need no registry rows; with a
// database an unknown org is never active.
func (s *Service) IsActive(ctx context.Context, id string) (bool, error) {
	status, found, err := s.Status(ctx, id)
	if err != nil {
		return false, err
	}
	if !found {
		return s.pool == nil, nil
	}
	return status == StatusActive, nil
}

// AddDomain registers a custom domain for an org (unverified until VerifyDomain).
func (s *Service) AddDomain(ctx context.Context, orgID, domain string) error {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if !domainRe.MatchString(domain) {
		return ErrInvalidDomain
	}
	if _, err := s.Get(ctx, orgID); err != nil {
		return err
	}
	if s.pool == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		if _, ok := s.domains[domain]; ok {
			return ErrDomainTaken
		}
		s.domains[domain] = memDomain{orgID: orgID}
		return nil
	}
	if _, err := s.at(orgID).Exec(ctx, `INSERT INTO org_domains (domain, org_id) VALUES ($1, $2::uuid)`, domain, orgID); err != nil {
		if dbutil.IsUniqueViolation(err, "") {
			return ErrDomainTaken
		}
		return err
	}
	return nil
}

// VerifyDomain marks a domain verified (after the DNS challenge succeeded).
func (s *Service) VerifyDomain(ctx context.Context, orgID, domain string) error {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if s.pool == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		d, ok := s.domains[domain]
		if !ok || d.orgID != orgID {
			return ErrNotFound
		}
		d.verified = true
		s.domains[domain] = d
		return nil
	}
	tag, err := s.at(orgID).Exec(ctx, `UPDATE org_domains SET verified_at = COALESCE(verified_at, now()) WHERE domain = $1 AND org_id = $2::uuid`, domain, orgID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ResolveDomain maps a request host to its org. Only verified domains resolve.
func (s *Service) ResolveDomain(ctx context.Context, host string) (*Org, error) {
	host = strings.ToLower(strings.TrimSpace(host))
	if i := strings.LastIndexByte(host, ':'); i > 0 && !strings.Contains(host[i:], "]") {
		host = host[:i]
	}
	if host == "" {
		return nil, ErrNotFound
	}
	if s.pool == nil {
		s.mu.Lock()
		d, ok := s.domains[host]
		s.mu.Unlock()
		if !ok || !d.verified {
			return nil, ErrNotFound
		}
		return s.Get(ctx, d.orgID)
	}
	o, err := scanOrg(s.forDomain(host).QueryRow(ctx, `SELECT o.id::text, COALESCE(o.slug, ''), o.name, o.status, o.plan, o.region, o.suspended_at, o.created_at, o.updated_at, o.offboarding_at
		FROM org_domains d JOIN orgs o ON o.id = d.org_id WHERE d.domain = $1 AND d.verified_at IS NOT NULL`, host))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return o, err
}

// CanonicalID returns the canonical lowercase hyphenated form of an org UUID in
// any spelling uuid.Parse accepts (uppercase, undashed, urn:uuid:, braces), so
// ids can be compared safely. Non-UUID input is trimmed and lowercased.
func CanonicalID(s string) string {
	s = strings.TrimSpace(s)
	if u, err := uuid.Parse(s); err == nil {
		return u.String()
	}
	return strings.ToLower(s)
}

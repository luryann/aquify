package volunteer

import (
	"context"
	"sort"
	"strings"
	"sync"
)

// store is the persistence contract shared by the in-memory store and the
// Postgres repository. insertSignup must enforce capacity and the
// one-live-spot-per-person rule atomically.
type store interface {
	getSettings(ctx context.Context, orgID string) (*Settings, bool, error)
	upsertSettings(ctx context.Context, st *Settings) error

	listJobs(ctx context.Context, orgID, meetID string) ([]*Job, error)
	getJob(ctx context.Context, orgID, id string) (*Job, error)
	insertJobs(ctx context.Context, jobs []*Job) error
	updateJob(ctx context.Context, job *Job) error
	deleteJob(ctx context.Context, orgID, id string) error

	listSignups(ctx context.Context, orgID string, f SignupFilter) ([]*Signup, error)
	getSignup(ctx context.Context, orgID, id string) (*Signup, error)
	insertSignup(ctx context.Context, su *Signup) error
	updateSignup(ctx context.Context, su *Signup) error

	listRequirements(ctx context.Context, orgID, seasonID string) ([]*Requirement, error)
	upsertRequirement(ctx context.Context, req *Requirement) (*Requirement, error)
	deleteRequirement(ctx context.Context, orgID, id string) error

	listCredits(ctx context.Context, orgID, seasonID, householdID string) ([]*Credit, error)
	insertCredit(ctx context.Context, c *Credit) error

	listBuyouts(ctx context.Context, orgID, seasonID string) ([]*Buyout, error)
	reserveBuyout(ctx context.Context, b *Buyout) error
	updateBuyout(ctx context.Context, b *Buyout) error
	deleteBuyout(ctx context.Context, orgID, id string) error
}

// ─── In-memory store ──────────────────────────────────────────────────────────

type memoryStore struct {
	mu       sync.Mutex
	settings map[string]*Settings
	jobs     map[string]*Job
	signups  map[string]*Signup
	reqs     map[string]*Requirement
	credits  []*Credit
	buyouts  map[string]*Buyout
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		settings: map[string]*Settings{},
		jobs:     map[string]*Job{},
		signups:  map[string]*Signup{},
		reqs:     map[string]*Requirement{},
		buyouts:  map[string]*Buyout{},
	}
}

func (m *memoryStore) getSettings(_ context.Context, orgID string) (*Settings, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.settings[orgID]
	if !ok {
		return nil, false, nil
	}
	cp := *st
	return &cp, true, nil
}

func (m *memoryStore) upsertSettings(_ context.Context, st *Settings) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *st
	m.settings[st.OrgID] = &cp
	return nil
}

func sortJobs(jobs []*Job) {
	sort.SliceStable(jobs, func(i, j int) bool {
		if !jobs[i].StartsAt.Equal(jobs[j].StartsAt) {
			return jobs[i].StartsAt.Before(jobs[j].StartsAt)
		}
		if jobs[i].Title != jobs[j].Title {
			return jobs[i].Title < jobs[j].Title
		}
		return jobs[i].ID < jobs[j].ID
	})
}

func (m *memoryStore) listJobs(_ context.Context, orgID, meetID string) ([]*Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []*Job{}
	for _, j := range m.jobs {
		if j.OrgID != orgID || (meetID != "" && j.MeetID != meetID) {
			continue
		}
		cp := *j
		out = append(out, &cp)
	}
	sortJobs(out)
	return out, nil
}

func (m *memoryStore) getJob(_ context.Context, orgID, id string) (*Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	if !ok || j.OrgID != orgID {
		return nil, ErrJobNotFound
	}
	cp := *j
	return &cp, nil
}

func (m *memoryStore) insertJobs(_ context.Context, jobs []*Job) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range jobs {
		cp := *j
		m.jobs[j.ID] = &cp
	}
	return nil
}

func (m *memoryStore) updateJob(_ context.Context, job *Job) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cur, ok := m.jobs[job.ID]; !ok || cur.OrgID != job.OrgID {
		return ErrJobNotFound
	}
	cp := *job
	m.jobs[job.ID] = &cp
	return nil
}

func (m *memoryStore) deleteJob(_ context.Context, orgID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	if !ok || j.OrgID != orgID {
		return ErrJobNotFound
	}
	delete(m.jobs, id)
	for sid, su := range m.signups {
		if su.JobID == id {
			delete(m.signups, sid)
		}
	}
	return nil
}

func matchSignup(su *Signup, orgID string, f SignupFilter) bool {
	if su.OrgID != orgID {
		return false
	}
	if f.JobID != "" && su.JobID != f.JobID {
		return false
	}
	if f.HouseholdID != "" && su.HouseholdID != f.HouseholdID {
		return false
	}
	if f.Status != "" && su.Status != f.Status {
		return false
	}
	return true
}

func (m *memoryStore) listSignups(_ context.Context, orgID string, f SignupFilter) ([]*Signup, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []*Signup{}
	for _, su := range m.signups {
		if !matchSignup(su, orgID, f) {
			continue
		}
		cp := *su
		out = append(out, &cp)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (m *memoryStore) getSignup(_ context.Context, orgID, id string) (*Signup, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	su, ok := m.signups[id]
	if !ok || su.OrgID != orgID {
		return nil, ErrSignupNotFound
	}
	cp := *su
	return &cp, nil
}

func (m *memoryStore) insertSignup(_ context.Context, su *Signup) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[su.JobID]
	if !ok || job.OrgID != su.OrgID {
		return ErrJobNotFound
	}
	filled := 0
	for _, other := range m.signups {
		if other.JobID != su.JobID || !occupiesSpot(other.Status) {
			continue
		}
		filled++
		if other.HouseholdID == su.HouseholdID && strings.EqualFold(other.VolunteerName, su.VolunteerName) {
			return ErrDuplicateSignup
		}
	}
	if filled >= job.Capacity {
		return ErrJobFull
	}
	cp := *su
	m.signups[su.ID] = &cp
	return nil
}

func (m *memoryStore) updateSignup(_ context.Context, su *Signup) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cur, ok := m.signups[su.ID]; !ok || cur.OrgID != su.OrgID {
		return ErrSignupNotFound
	}
	cp := *su
	m.signups[su.ID] = &cp
	return nil
}

func (m *memoryStore) listRequirements(_ context.Context, orgID, seasonID string) ([]*Requirement, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []*Requirement{}
	for _, r := range m.reqs {
		if r.OrgID != orgID || (seasonID != "" && r.SeasonID != seasonID) {
			continue
		}
		cp := *r
		out = append(out, &cp)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SeasonID != out[j].SeasonID {
			return out[i].SeasonID < out[j].SeasonID
		}
		return out[i].HouseholdID < out[j].HouseholdID
	})
	return out, nil
}

func (m *memoryStore) upsertRequirement(_ context.Context, req *Requirement) (*Requirement, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.reqs {
		if r.OrgID == req.OrgID && r.SeasonID == req.SeasonID && r.HouseholdID == req.HouseholdID {
			r.RequiredHours, r.BuyoutCentsPerHour, r.Notes = req.RequiredHours, req.BuyoutCentsPerHour, req.Notes
			r.UpdatedAt, r.UpdatedBy = req.UpdatedAt, req.UpdatedBy
			cp := *r
			return &cp, nil
		}
	}
	cp := *req
	m.reqs[req.ID] = &cp
	out := cp
	return &out, nil
}

func (m *memoryStore) deleteRequirement(_ context.Context, orgID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.reqs[id]
	if !ok || r.OrgID != orgID {
		return ErrRequirementNotFound
	}
	delete(m.reqs, id)
	return nil
}

func (m *memoryStore) listCredits(_ context.Context, orgID, seasonID, householdID string) ([]*Credit, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []*Credit{}
	for _, c := range m.credits {
		if c.OrgID != orgID || (seasonID != "" && c.SeasonID != seasonID) || (householdID != "" && c.HouseholdID != householdID) {
			continue
		}
		cp := *c
		out = append(out, &cp)
	}
	return out, nil
}

func (m *memoryStore) insertCredit(_ context.Context, c *Credit) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *c
	m.credits = append(m.credits, &cp)
	return nil
}

func (m *memoryStore) listBuyouts(_ context.Context, orgID, seasonID string) ([]*Buyout, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []*Buyout{}
	for _, b := range m.buyouts {
		if b.OrgID != orgID || (seasonID != "" && b.SeasonID != seasonID) {
			continue
		}
		cp := *b
		out = append(out, &cp)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (m *memoryStore) reserveBuyout(_ context.Context, b *Buyout) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, other := range m.buyouts {
		if other.OrgID == b.OrgID && other.SeasonID == b.SeasonID && other.HouseholdID == b.HouseholdID {
			return ErrBuyoutExists
		}
	}
	cp := *b
	m.buyouts[b.ID] = &cp
	return nil
}

func (m *memoryStore) updateBuyout(_ context.Context, b *Buyout) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.buyouts[b.ID]; !ok {
		return ErrBuyoutExists
	}
	cp := *b
	m.buyouts[b.ID] = &cp
	return nil
}

func (m *memoryStore) deleteBuyout(_ context.Context, orgID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if b, ok := m.buyouts[id]; ok && b.OrgID == orgID {
		delete(m.buyouts, id)
	}
	return nil
}

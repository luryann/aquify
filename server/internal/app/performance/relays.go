package performance

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

var (
	ErrRelayInvalid        = errors.New("invalid relay request")
	ErrRelayLineupNotFound = errors.New("relay lineup not found")
)

const maxRelayTeams = 6

// RelayCandidate is one swimmer available to the relay optimizer with their
// flat-start best time (ms) per stroke at the relay leg distance.
type RelayCandidate struct {
	SwimmerID string
	Name      string
	Gender    string // M | F
	Times     map[string]int
}

// RelayLegLock pins a swimmer to a team (0 = A) and leg (1–4).
type RelayLegLock struct {
	Team      int    `json:"team"`
	Leg       int    `json:"leg"`
	SwimmerID string `json:"swimmer_id"`
}

// RelaySpec describes the relay being built.
type RelaySpec struct {
	RelayType string // free | medley
	Distance  int    // 200 | 400 | 800 (medley: 200 | 400)
	Gender    string // M | F | X (mixed: exactly two men and two women)
	Teams     int    // number of relays to propose (A, B, C, …)
}

// RelayLeg is one leg of a proposed relay.
type RelayLeg struct {
	Leg         int    `json:"leg"`
	Stroke      string `json:"stroke"`
	SwimmerID   string `json:"swimmer_id"`
	SwimmerName string `json:"swimmer_name"`
	Gender      string `json:"gender,omitempty"`
	TimeMs      int    `json:"time_ms"`
	Locked      bool   `json:"locked"`
}

// RelayTeam is one proposed relay.
type RelayTeam struct {
	Label   string     `json:"label"`
	Legs    []RelayLeg `json:"legs"`
	TotalMs int        `json:"total_ms"`
}

// RelayProposal is the optimizer output.
type RelayProposal struct {
	Teams    []RelayTeam `json:"teams"`
	Warnings []string    `json:"warnings"`
}

// LegStrokes returns the stroke for each leg in swim order.
func LegStrokes(relayType string) []string {
	if relayType == "medley" {
		return []string{"back", "breast", "fly", "free"}
	}
	return []string{"free", "free", "free", "free"}
}

func (sp RelaySpec) validate() error {
	switch {
	case sp.RelayType != "free" && sp.RelayType != "medley":
		return fmt.Errorf("%w: relay_type must be free or medley", ErrRelayInvalid)
	case sp.RelayType == "medley" && sp.Distance != 200 && sp.Distance != 400:
		return fmt.Errorf("%w: medley relays are 200 or 400", ErrRelayInvalid)
	case sp.Distance != 200 && sp.Distance != 400 && sp.Distance != 800:
		return fmt.Errorf("%w: distance must be 200, 400 or 800", ErrRelayInvalid)
	case sp.Gender != "M" && sp.Gender != "F" && sp.Gender != "X":
		return fmt.Errorf("%w: gender must be M, F or X (mixed)", ErrRelayInvalid)
	case sp.Teams < 1 || sp.Teams > maxRelayTeams:
		return fmt.Errorf("%w: teams must be between 1 and %d", ErrRelayInvalid, maxRelayTeams)
	}
	return nil
}

func teamLabel(i int) string { return string(rune('A' + i)) }

// OptimizeRelays proposes relays A, B, C… in priority order: relay A is the
// fastest possible relay from the pool, B the fastest from what remains, and
// so on. Each relay is solved exactly (an assignment problem over the four
// legs with distinct swimmers, honoring locked legs and the mixed-gender 2+2
// rule). Only the four fastest swimmers per stroke and gender can appear in
// an optimal relay (the other three legs use at most three of them), so the
// search runs over at most 8 candidates per leg with branch-and-bound pruning.
func OptimizeRelays(spec RelaySpec, pool []RelayCandidate, locks []RelayLegLock, excluded map[string]bool) (RelayProposal, error) {
	out := RelayProposal{Teams: []RelayTeam{}, Warnings: []string{}}
	if err := spec.validate(); err != nil {
		return out, err
	}
	strokes := LegStrokes(spec.RelayType)
	byID := map[string]*RelayCandidate{}
	for i := range pool {
		c := &pool[i]
		if spec.Gender != "X" && c.Gender != spec.Gender {
			continue
		}
		if spec.Gender == "X" && c.Gender != "M" && c.Gender != "F" {
			continue
		}
		byID[c.SwimmerID] = c
	}

	// Validate locks and reserve locked swimmers for their team.
	lockAt := map[[2]int]string{}
	lockedSlot := map[string][2]int{}
	reservedBy := map[string]int{}
	for _, l := range locks {
		if l.Team < 0 || l.Team >= spec.Teams || l.Leg < 1 || l.Leg > 4 {
			return out, fmt.Errorf("%w: lock team/leg out of range", ErrRelayInvalid)
		}
		c, ok := byID[l.SwimmerID]
		if !ok {
			return out, fmt.Errorf("%w: locked swimmer %s is not eligible for this relay", ErrRelayInvalid, l.SwimmerID)
		}
		if excluded[l.SwimmerID] {
			return out, fmt.Errorf("%w: locked swimmer %s is also excluded", ErrRelayInvalid, c.Name)
		}
		if _, ok := c.Times[strokes[l.Leg-1]]; !ok {
			return out, fmt.Errorf("%w: %s has no %s time for this leg", ErrRelayInvalid, c.Name, strokeLabel(strokes[l.Leg-1]))
		}
		slot := [2]int{l.Team, l.Leg}
		if prev, dup := lockAt[slot]; dup && prev != l.SwimmerID {
			return out, fmt.Errorf("%w: two swimmers locked to relay %s leg %d", ErrRelayInvalid, teamLabel(l.Team), l.Leg)
		}
		if prevSlot, dup := lockedSlot[l.SwimmerID]; dup && prevSlot != slot {
			return out, fmt.Errorf("%w: %s is locked to more than one leg", ErrRelayInvalid, c.Name)
		}
		lockAt[slot] = l.SwimmerID
		lockedSlot[l.SwimmerID] = slot
		reservedBy[l.SwimmerID] = l.Team
	}

	used := map[string]bool{}
	for t := 0; t < spec.Teams; t++ {
		fixed := [4]string{}
		men, women := 0, 0
		for leg := 1; leg <= 4; leg++ {
			if id := lockAt[[2]int{t, leg}]; id != "" {
				fixed[leg-1] = id
				if byID[id].Gender == "M" {
					men++
				} else {
					women++
				}
			}
		}
		if spec.Gender == "X" && (men > 2 || women > 2) {
			return out, fmt.Errorf("%w: mixed relay %s has more than two locked swimmers of one gender", ErrRelayInvalid, teamLabel(t))
		}
		available := func(c *RelayCandidate) bool {
			if used[c.SwimmerID] || excluded[c.SwimmerID] {
				return false
			}
			_, reserved := reservedBy[c.SwimmerID]
			return !reserved
		}
		// Candidate lists per open leg: top 4 per gender by that stroke.
		cands := [4][]*RelayCandidate{}
		for leg := 0; leg < 4; leg++ {
			if fixed[leg] != "" {
				continue
			}
			stroke := strokes[leg]
			var list []*RelayCandidate
			for _, c := range byID {
				if _, ok := c.Times[stroke]; ok && available(c) {
					list = append(list, c)
				}
			}
			sort.Slice(list, func(i, j int) bool {
				a, b := list[i].Times[stroke], list[j].Times[stroke]
				if a != b {
					return a < b
				}
				return list[i].SwimmerID < list[j].SwimmerID
			})
			perGender := map[string]int{}
			for _, c := range list {
				if perGender[c.Gender] < 4 {
					cands[leg] = append(cands[leg], c)
					perGender[c.Gender]++
				}
			}
		}
		// Lower bound per open leg for pruning.
		minLeg := [4]int{}
		for leg := 0; leg < 4; leg++ {
			if fixed[leg] == "" && len(cands[leg]) > 0 {
				minLeg[leg] = cands[leg][0].Times[strokes[leg]]
			}
		}
		bestSum := -1
		var bestPick [4]*RelayCandidate
		var pick [4]*RelayCandidate
		inUse := map[string]bool{}
		for leg := 0; leg < 4; leg++ {
			if fixed[leg] != "" {
				pick[leg] = byID[fixed[leg]]
				inUse[fixed[leg]] = true
			}
		}
		var dfs func(leg, sum, m, w int)
		dfs = func(leg, sum, m, w int) {
			if leg == 4 {
				if spec.Gender == "X" && (m != 2 || w != 2) {
					return
				}
				if bestSum < 0 || sum < bestSum {
					bestSum = sum
					bestPick = pick
				}
				return
			}
			if fixed[leg] != "" {
				if pick[leg].Gender == "M" {
					dfs(leg+1, sum+pick[leg].Times[strokes[leg]], m+1, w)
				} else {
					dfs(leg+1, sum+pick[leg].Times[strokes[leg]], m, w+1)
				}
				return
			}
			if bestSum >= 0 {
				bound := sum
				for l := leg; l < 4; l++ {
					if fixed[l] != "" {
						bound += pick[l].Times[strokes[l]]
					} else {
						bound += minLeg[l]
					}
				}
				if bound >= bestSum {
					return
				}
			}
			for _, c := range cands[leg] {
				if inUse[c.SwimmerID] {
					continue
				}
				nm, nw := m, w
				if c.Gender == "M" {
					nm++
				} else {
					nw++
				}
				if spec.Gender == "X" && (nm > 2 || nw > 2) {
					continue
				}
				inUse[c.SwimmerID] = true
				pick[leg] = c
				dfs(leg+1, sum+c.Times[strokes[leg]], nm, nw)
				inUse[c.SwimmerID] = false
				pick[leg] = nil
			}
		}
		dfs(0, 0, 0, 0)
		if bestSum < 0 {
			out.Warnings = append(out.Warnings, fmt.Sprintf("Not enough eligible swimmers with times to fill relay %s.", teamLabel(t)))
			break
		}
		if spec.RelayType == "free" {
			bestPick = orderFreeLegs(bestPick, fixed, strokes)
		}
		team := RelayTeam{Label: teamLabel(t), Legs: make([]RelayLeg, 0, 4)}
		for leg := 0; leg < 4; leg++ {
			c := bestPick[leg]
			tm := c.Times[strokes[leg]]
			team.Legs = append(team.Legs, RelayLeg{
				Leg: leg + 1, Stroke: strokes[leg], SwimmerID: c.SwimmerID, SwimmerName: c.Name, Gender: c.Gender,
				TimeMs: tm, Locked: fixed[leg] != "",
			})
			team.TotalMs += tm
			used[c.SwimmerID] = true
		}
		out.Teams = append(out.Teams, team)
	}
	return out, nil
}

// orderFreeLegs arranges the unlocked swimmers of a free relay in the
// conventional order: fastest anchors, second fastest leads off, then the
// remaining two. Locked legs stay where the coach put them.
func orderFreeLegs(pick [4]*RelayCandidate, fixed [4]string, strokes []string) [4]*RelayCandidate {
	var open []int
	var swimmers []*RelayCandidate
	for leg := 0; leg < 4; leg++ {
		if fixed[leg] == "" {
			open = append(open, leg)
			swimmers = append(swimmers, pick[leg])
		}
	}
	sort.Slice(swimmers, func(i, j int) bool {
		a, b := swimmers[i].Times[strokes[0]], swimmers[j].Times[strokes[0]]
		if a != b {
			return a < b
		}
		return swimmers[i].SwimmerID < swimmers[j].SwimmerID
	})
	preference := []int{3, 0, 1, 2}
	var slots []int
	for _, p := range preference {
		for _, o := range open {
			if o == p {
				slots = append(slots, p)
			}
		}
	}
	for i, slot := range slots {
		pick[slot] = swimmers[i]
	}
	return pick
}

// ─── Service: optimize from club best times ──────────────────────────────────

// RelayOptimizeInput is the relay-builder request.
type RelayOptimizeInput struct {
	MeetID             string         `json:"meet_id"`
	Course             string         `json:"course"`
	Gender             string         `json:"gender"`
	AgeGroup           string         `json:"age_group"`
	RelayType          string         `json:"relay_type"`
	Distance           int            `json:"distance"`
	Teams              int            `json:"teams"`
	AsOf               string         `json:"as_of"`
	GroupCode          string         `json:"group_code"`
	Locks              []RelayLegLock `json:"locks"`
	ExcludedSwimmerIDs []string       `json:"excluded_swimmer_ids"`
	IncludeSwimmerIDs  []string       `json:"include_swimmer_ids"`
}

// RelayPoolSwimmer describes an eligible swimmer for the builder UI.
type RelayPoolSwimmer struct {
	SwimmerID string         `json:"swimmer_id"`
	Name      string         `json:"name"`
	Gender    string         `json:"gender"`
	Age       *int           `json:"age,omitempty"`
	Times     map[string]int `json:"times"`
}

// RelayOptimizeResult is the relay-builder response.
type RelayOptimizeResult struct {
	Course    string             `json:"course"`
	Gender    string             `json:"gender"`
	AgeGroup  string             `json:"age_group"`
	RelayType string             `json:"relay_type"`
	Distance  int                `json:"distance"`
	LegLength int                `json:"leg_distance"`
	AsOf      string             `json:"as_of"`
	MeetID    string             `json:"meet_id,omitempty"`
	Teams     []RelayTeam        `json:"teams"`
	Warnings  []string           `json:"warnings"`
	Pool      []RelayPoolSwimmer `json:"pool"`
}

// OptimizeRelay builds the eligible pool from best times and runs the
// optimizer. Staff only.
func (s *Service) OptimizeRelay(orgID string, principal *appmiddleware.Principal, in RelayOptimizeInput) (*RelayOptimizeResult, error) {
	if err := s.requireAdminOrCoach(principal); err != nil {
		return nil, err
	}
	in.Course = strings.ToUpper(strings.TrimSpace(in.Course))
	in.Gender = strings.ToUpper(strings.TrimSpace(in.Gender))
	in.AgeGroup = strings.ToUpper(strings.TrimSpace(in.AgeGroup))
	in.RelayType = strings.ToLower(strings.TrimSpace(in.RelayType))
	if in.Teams == 0 {
		in.Teams = 3
	}
	if in.AgeGroup == "" {
		in.AgeGroup = AgeGroupOpen
	}
	if !ValidAgeGroup(in.AgeGroup) {
		return nil, fmt.Errorf("%w: unknown age group", ErrRelayInvalid)
	}
	dir := s.snapshotDirectory(orgID)
	asOf := strings.TrimSpace(in.AsOf)
	if in.MeetID != "" {
		m, ok := dir.meets[in.MeetID]
		if !ok {
			return nil, fmt.Errorf("%w: meet not found", ErrRelayInvalid)
		}
		if asOf == "" {
			asOf = m.StartDate
		}
		if in.Course == "" {
			in.Course = strings.ToUpper(m.Course)
		}
	}
	if asOf == "" {
		asOf = time.Now().UTC().Format("2006-01-02")
	}
	if t, err := parseDateOnly(asOf); err != nil {
		return nil, fmt.Errorf("%w: as_of must be YYYY-MM-DD", ErrRelayInvalid)
	} else {
		asOf = t.Format("2006-01-02")
	}
	if in.Course != "SCY" && in.Course != "SCM" && in.Course != "LCM" {
		return nil, fmt.Errorf("%w: course must be SCY, SCM or LCM", ErrRelayInvalid)
	}
	spec := RelaySpec{RelayType: in.RelayType, Distance: in.Distance, Gender: in.Gender, Teams: in.Teams}
	if err := spec.validate(); err != nil {
		return nil, err
	}
	legDistance := in.Distance / 4
	strokes := LegStrokes(in.RelayType)
	include := map[string]bool{}
	for _, id := range in.IncludeSwimmerIDs {
		include[strings.TrimSpace(id)] = true
	}
	excluded := map[string]bool{}
	for _, id := range in.ExcludedSwimmerIDs {
		excluded[strings.TrimSpace(id)] = true
	}
	groupCode := strings.ToUpper(strings.TrimSpace(in.GroupCode))

	s.mu.RLock()
	timesBySwimmer := map[string]map[string]int{}
	for _, bt := range s.bestTimes {
		if bt.OrgID != orgID || bt.Course != in.Course || bt.EventDistance != legDistance || bt.BestTimeMs == nil {
			continue
		}
		needed := false
		for _, st := range strokes {
			if st == bt.Stroke {
				needed = true
			}
		}
		if !needed {
			continue
		}
		m := timesBySwimmer[bt.SwimmerID]
		if m == nil {
			m = map[string]int{}
			timesBySwimmer[bt.SwimmerID] = m
		}
		m[bt.Stroke] = *bt.BestTimeMs
	}
	s.mu.RUnlock()

	var pool []RelayCandidate
	result := &RelayOptimizeResult{
		Course: in.Course, Gender: in.Gender, AgeGroup: in.AgeGroup, RelayType: in.RelayType, Distance: in.Distance,
		LegLength: legDistance, AsOf: asOf, MeetID: in.MeetID, Pool: []RelayPoolSwimmer{},
	}
	for swimmerID, times := range timesBySwimmer {
		p, ok := dir.swimmers[swimmerID]
		if !ok {
			continue
		}
		if len(include) > 0 && !include[swimmerID] {
			continue
		}
		if groupCode != "" && !containsFold(p.GroupCodes, groupCode) {
			continue
		}
		gender := normalizeGender("", p.Gender)
		if gender == "" || (in.Gender != "X" && gender != in.Gender) {
			continue
		}
		age := AgeOn(p.DateOfBirth, asOf)
		if !ageGroupContains(in.AgeGroup, age) {
			continue
		}
		pool = append(pool, RelayCandidate{SwimmerID: swimmerID, Name: p.DisplayName(), Gender: gender, Times: times})
		result.Pool = append(result.Pool, RelayPoolSwimmer{SwimmerID: swimmerID, Name: p.DisplayName(), Gender: gender, Age: age, Times: times})
	}
	sort.Slice(pool, func(i, j int) bool { return pool[i].SwimmerID < pool[j].SwimmerID })
	sort.Slice(result.Pool, func(i, j int) bool { return result.Pool[i].Name < result.Pool[j].Name })
	proposal, err := OptimizeRelays(spec, pool, in.Locks, excluded)
	if err != nil {
		return nil, err
	}
	result.Teams = proposal.Teams
	result.Warnings = proposal.Warnings
	return result, nil
}

func containsFold(list []string, v string) bool {
	for _, x := range list {
		if strings.EqualFold(strings.TrimSpace(x), v) {
			return true
		}
	}
	return false
}

// ─── Saved lineups ───────────────────────────────────────────────────────────

// RelayLineup is a saved relay plan for a meet.
type RelayLineup struct {
	ID        string      `json:"id"`
	OrgID     string      `json:"org_id"`
	MeetID    string      `json:"meet_id"`
	Name      string      `json:"name"`
	Course    string      `json:"course"`
	Gender    string      `json:"gender"`
	AgeGroup  string      `json:"age_group"`
	RelayType string      `json:"relay_type"`
	Distance  int         `json:"distance"`
	Teams     []RelayTeam `json:"teams"`
	Notes     string      `json:"notes,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
	CreatedBy string      `json:"created_by"`
	UpdatedBy string      `json:"updated_by"`
}

// RelayLineupInput creates or replaces a lineup.
type RelayLineupInput struct {
	MeetID    string      `json:"meet_id"`
	Name      string      `json:"name"`
	Course    string      `json:"course"`
	Gender    string      `json:"gender"`
	AgeGroup  string      `json:"age_group"`
	RelayType string      `json:"relay_type"`
	Distance  int         `json:"distance"`
	Teams     []RelayTeam `json:"teams"`
	Notes     string      `json:"notes"`
}

func (s *Service) normalizeLineup(orgID string, in RelayLineupInput, dir directorySnapshot, hasDir bool) (RelayLineupInput, error) {
	in.MeetID = strings.TrimSpace(in.MeetID)
	in.Name = strings.TrimSpace(in.Name)
	in.Course = strings.ToUpper(strings.TrimSpace(in.Course))
	in.Gender = strings.ToUpper(strings.TrimSpace(in.Gender))
	in.AgeGroup = strings.ToUpper(strings.TrimSpace(in.AgeGroup))
	in.RelayType = strings.ToLower(strings.TrimSpace(in.RelayType))
	in.Notes = strings.TrimSpace(in.Notes)
	if in.MeetID == "" {
		return in, fmt.Errorf("%w: meet_id is required", ErrRelayInvalid)
	}
	if hasDir {
		if _, ok := dir.meets[in.MeetID]; !ok {
			return in, fmt.Errorf("%w: meet not found", ErrRelayInvalid)
		}
	}
	if in.Course != "SCY" && in.Course != "SCM" && in.Course != "LCM" {
		return in, fmt.Errorf("%w: course must be SCY, SCM or LCM", ErrRelayInvalid)
	}
	if !ValidAgeGroup(in.AgeGroup) {
		return in, fmt.Errorf("%w: unknown age group", ErrRelayInvalid)
	}
	if err := (RelaySpec{RelayType: in.RelayType, Distance: in.Distance, Gender: in.Gender, Teams: 1}).validate(); err != nil {
		return in, err
	}
	if len(in.Name) > 120 || len(in.Notes) > 1000 {
		return in, fmt.Errorf("%w: name/notes too long", ErrRelayInvalid)
	}
	if len(in.Teams) == 0 || len(in.Teams) > maxRelayTeams {
		return in, fmt.Errorf("%w: a lineup needs 1–%d relays", ErrRelayInvalid, maxRelayTeams)
	}
	strokes := LegStrokes(in.RelayType)
	seen := map[string]bool{}
	for ti := range in.Teams {
		t := &in.Teams[ti]
		t.Label = teamLabel(ti)
		if len(t.Legs) != 4 {
			return in, fmt.Errorf("%w: relay %s needs exactly 4 legs", ErrRelayInvalid, t.Label)
		}
		sort.Slice(t.Legs, func(i, j int) bool { return t.Legs[i].Leg < t.Legs[j].Leg })
		t.TotalMs = 0
		for li := range t.Legs {
			l := &t.Legs[li]
			if l.Leg != li+1 {
				return in, fmt.Errorf("%w: relay %s legs must be numbered 1–4", ErrRelayInvalid, t.Label)
			}
			l.Stroke = strokes[li]
			l.SwimmerID = strings.TrimSpace(l.SwimmerID)
			if l.SwimmerID == "" {
				return in, fmt.Errorf("%w: relay %s leg %d has no swimmer", ErrRelayInvalid, t.Label, l.Leg)
			}
			if seen[l.SwimmerID] {
				return in, fmt.Errorf("%w: a swimmer appears on more than one leg", ErrRelayInvalid)
			}
			seen[l.SwimmerID] = true
			if hasDir {
				p, ok := dir.swimmers[l.SwimmerID]
				if !ok {
					return in, fmt.Errorf("%w: unknown swimmer on relay %s leg %d", ErrRelayInvalid, t.Label, l.Leg)
				}
				l.SwimmerName = p.DisplayName()
			}
			if l.TimeMs < 0 {
				l.TimeMs = 0
			}
			t.TotalMs += l.TimeMs
		}
	}
	return in, nil
}

// ListRelayLineups returns saved lineups, optionally for one meet. Staff only.
func (s *Service) ListRelayLineups(orgID string, principal *appmiddleware.Principal, meetID string) ([]*RelayLineup, error) {
	if err := s.requireAdminOrCoach(principal); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []*RelayLineup{}
	for _, l := range s.lineups {
		if l.OrgID == orgID && (meetID == "" || l.MeetID == meetID) {
			cp := *l
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// SaveRelayLineup creates (lineupID == "") or replaces a lineup. Staff only.
func (s *Service) SaveRelayLineup(orgID, actorID string, principal *appmiddleware.Principal, lineupID string, in RelayLineupInput) (*RelayLineup, error) {
	if err := s.requireAdminOrCoach(principal); err != nil {
		return nil, err
	}
	dir := s.snapshotDirectory(orgID)
	s.mu.RLock()
	hasDir := s.directory != nil
	s.mu.RUnlock()
	norm, err := s.normalizeLineup(orgID, in, dir, hasDir)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	l := &RelayLineup{ID: s.newID(), OrgID: orgID, CreatedAt: now, CreatedBy: actorID}
	if lineupID != "" {
		existing, ok := s.lineups[lineupID]
		if !ok || existing.OrgID != orgID {
			return nil, ErrRelayLineupNotFound
		}
		l.ID, l.CreatedAt, l.CreatedBy = existing.ID, existing.CreatedAt, existing.CreatedBy
	}
	l.MeetID, l.Name, l.Course, l.Gender, l.AgeGroup = norm.MeetID, norm.Name, norm.Course, norm.Gender, norm.AgeGroup
	l.RelayType, l.Distance, l.Teams, l.Notes = norm.RelayType, norm.Distance, norm.Teams, norm.Notes
	l.UpdatedAt, l.UpdatedBy = now, actorID
	if l.Name == "" {
		gender := map[string]string{"M": "Men", "F": "Women", "X": "Mixed"}[l.Gender]
		l.Name = fmt.Sprintf("%s %s %d %s", gender, l.AgeGroup, l.Distance, map[string]string{"free": "Free Relay", "medley": "Medley Relay"}[l.RelayType])
	}
	if s.repo != nil {
		if err := s.repo.UpsertRelayLineup(context.Background(), l); err != nil {
			return nil, fmt.Errorf("persist relay lineup: %w", err)
		}
	}
	s.lineups[l.ID] = l
	s.appendAuditLocked(orgID, actorID, "relay_lineup.saved", "relay_lineup", l.ID, map[string]any{"meet_id": l.MeetID})
	cp := *l
	return &cp, nil
}

// DeleteRelayLineup removes a saved lineup. Staff only.
func (s *Service) DeleteRelayLineup(orgID, actorID string, principal *appmiddleware.Principal, lineupID string) error {
	if err := s.requireAdminOrCoach(principal); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.lineups[lineupID]
	if !ok || l.OrgID != orgID {
		return ErrRelayLineupNotFound
	}
	if s.repo != nil {
		if err := s.repo.DeleteRelayLineup(context.Background(), lineupID); err != nil {
			return fmt.Errorf("delete relay lineup: %w", err)
		}
	}
	delete(s.lineups, lineupID)
	s.appendAuditLocked(orgID, actorID, "relay_lineup.deleted", "relay_lineup", lineupID, map[string]any{"meet_id": l.MeetID})
	return nil
}

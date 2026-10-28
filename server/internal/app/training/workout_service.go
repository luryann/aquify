package training

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
)

var (
	ErrWorkoutNotFound   = errors.New("workout not found")
	ErrPracticeNoWorkout = errors.New("practice has no workout")
	ErrInvalidAssignment = errors.New("assignment needs practice_id or a date")
)

// WorkoutFilter narrows library listings.
type WorkoutFilter struct {
	TemplatesOnly bool
	Query         string
	Tag           string
}

// DuplicateInput controls a duplicate / save-as-template copy.
type DuplicateInput struct {
	Title      *string
	IsTemplate *bool
}

// AssignInput assigns a workout to an existing practice or a new one.
type AssignInput struct {
	PracticeID string
	Date       string
	GroupID    string
	Type       string
}

func cloneWorkout(w *Workout) *Workout {
	if w == nil {
		return nil
	}
	cp := *w
	cp.Tags = append([]string{}, w.Tags...)
	cp.Sections = cloneSections(w.Sections)
	return &cp
}

func cloneSections(in []WorkoutSection) []WorkoutSection {
	out := make([]WorkoutSection, len(in))
	for i, sec := range in {
		out[i] = sec
		out[i].Sets = cloneSets(sec.Sets)
	}
	return out
}

func cloneSets(in []WorkoutSet) []WorkoutSet {
	if in == nil {
		return nil
	}
	out := make([]WorkoutSet, len(in))
	for i, set := range in {
		out[i] = set
		out[i].Equipment = append([]string(nil), set.Equipment...)
		out[i].Sets = cloneSets(set.Sets)
	}
	return out
}

// derivePractice overwrites a practice's yardage/stroke fields from a workout.
func derivePractice(p *Practice, w *Workout) {
	if p == nil || w == nil {
		return
	}
	p.WorkoutID = w.ID
	p.YardageTarget = w.Totals.Distance
	p.Strokes = PracticeStrokes{
		Free:   w.Totals.ByStroke["free"],
		Back:   w.Totals.ByStroke["back"],
		Breast: w.Totals.ByStroke["breast"],
		Fly:    w.Totals.ByStroke["fly"],
		IM:     w.Totals.ByStroke["im"],
	}
}

func buildWorkout(orgID, id string, in WorkoutInput, actor string, now time.Time) *Workout {
	return &Workout{
		ID:          id,
		OrgID:       orgID,
		Title:       in.Title,
		Description: in.Description,
		Course:      in.Course,
		IsTemplate:  in.IsTemplate,
		Tags:        in.Tags,
		Sections:    in.Sections,
		Totals:      ComputeTotals(in.Sections, in.Course),
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   actor,
		UpdatedBy:   actor,
	}
}

// CreateWorkout validates and stores a new workout (template or instance).
func (s *Service) CreateWorkout(orgID string, in WorkoutInput, actor string) (*Workout, error) {
	norm, err := NormalizeWorkoutInput(in)
	if err != nil {
		return nil, err
	}
	w := buildWorkout(orgID, newID(), norm, actor, time.Now().UTC())
	if s.repo != nil {
		if err := upsertWorkout(context.Background(), s.repo.pool, w); err != nil {
			return nil, err
		}
		return w, nil
	}
	s.mu.Lock()
	s.workouts[w.ID] = cloneWorkout(w)
	s.mu.Unlock()
	return w, nil
}

// GetWorkout returns one workout (org-scoped).
func (s *Service) GetWorkout(orgID, id string) (*Workout, error) {
	if s.repo != nil {
		return getWorkout(context.Background(), s.repo.pool, orgID, id)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	w, ok := s.workouts[id]
	if !ok || w.OrgID != orgID {
		return nil, ErrWorkoutNotFound
	}
	return cloneWorkout(w), nil
}

// ListWorkouts returns library workouts matching the filter, newest first.
func (s *Service) ListWorkouts(orgID string, f WorkoutFilter) ([]*Workout, error) {
	if s.repo != nil {
		return s.repo.ListWorkouts(context.Background(), orgID, f)
	}
	query := strings.ToLower(strings.TrimSpace(f.Query))
	tag := strings.ToLower(strings.TrimSpace(f.Tag))
	s.mu.RLock()
	out := []*Workout{}
	for _, w := range s.workouts {
		if w.OrgID != orgID || (f.TemplatesOnly && !w.IsTemplate) {
			continue
		}
		if tag != "" && !containsString(w.Tags, tag) {
			continue
		}
		if query != "" && !workoutMatches(w, query) {
			continue
		}
		out = append(out, cloneWorkout(w))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if !out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func containsString(items []string, v string) bool {
	for _, item := range items {
		if item == v {
			return true
		}
	}
	return false
}

func workoutMatches(w *Workout, query string) bool {
	if strings.Contains(strings.ToLower(w.Title), query) || strings.Contains(strings.ToLower(w.Description), query) {
		return true
	}
	for _, t := range w.Tags {
		if strings.Contains(t, query) {
			return true
		}
	}
	return false
}

// UpdateWorkout replaces a workout's content. is_template is fixed at
// creation (use DuplicateWorkout to save an instance as a template). When the
// workout is attached to a practice, the practice's derived yardage follows.
func (s *Service) UpdateWorkout(orgID, id string, in WorkoutInput, actor string) (*Workout, error) {
	ctx := context.Background()
	existing, err := s.GetWorkout(orgID, id)
	if err != nil {
		return nil, err
	}
	in.IsTemplate = existing.IsTemplate
	norm, err := NormalizeWorkoutInput(in)
	if err != nil {
		return nil, err
	}
	w := buildWorkout(orgID, id, norm, existing.CreatedBy, existing.CreatedAt)
	w.SourceWorkoutID = existing.SourceWorkoutID
	w.UpdatedAt = time.Now().UTC()
	w.UpdatedBy = actor

	if s.repo != nil {
		err := s.repo.withTx(ctx, func(q querier) error {
			if err := upsertWorkout(ctx, q, w); err != nil {
				return err
			}
			_, err := q.Exec(ctx, `
				UPDATE practices SET yardage_target = $3, strokes_free = $4, strokes_back = $5,
					strokes_breast = $6, strokes_fly = $7, strokes_im = $8, updated_at = $9, updated_by = $10
				WHERE org_id = $1 AND workout_id = $2::uuid`,
				orgID, id, w.Totals.Distance, w.Totals.ByStroke["free"], w.Totals.ByStroke["back"],
				w.Totals.ByStroke["breast"], w.Totals.ByStroke["fly"], w.Totals.ByStroke["im"], w.UpdatedAt, actor)
			return err
		})
		if err != nil {
			return nil, err
		}
		return w, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.workouts[id] = cloneWorkout(w)
	for pid, p := range s.practices {
		if p.OrgID == orgID && p.WorkoutID == id {
			cp := *p
			derivePractice(&cp, w)
			cp.UpdatedAt, cp.UpdatedBy = w.UpdatedAt, actor
			s.practices[pid] = &cp
		}
	}
	return w, nil
}

// DeleteWorkout removes a workout. A practice that carried it keeps its last
// derived yardage but loses the link (FK ON DELETE SET NULL).
func (s *Service) DeleteWorkout(orgID, id string) error {
	if s.repo != nil {
		return deleteWorkout(context.Background(), s.repo.pool, orgID, id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.workouts[id]
	if !ok || w.OrgID != orgID {
		return ErrWorkoutNotFound
	}
	delete(s.workouts, id)
	for pid, p := range s.practices {
		if p.WorkoutID == id {
			cp := *p
			cp.WorkoutID = ""
			s.practices[pid] = &cp
		}
	}
	return nil
}

// DuplicateWorkout copies a workout. Defaults: title "<title> (copy)", same
// template flag as the source. Passing IsTemplate=true saves a practice's
// workout into the library.
func (s *Service) DuplicateWorkout(orgID, id string, in DuplicateInput, actor string) (*Workout, error) {
	src, err := s.GetWorkout(orgID, id)
	if err != nil {
		return nil, err
	}
	title := src.Title + " (copy)"
	if in.Title != nil && strings.TrimSpace(*in.Title) != "" {
		title = *in.Title
	}
	if len(title) > MaxTitleLength {
		title = title[:MaxTitleLength]
	}
	isTemplate := src.IsTemplate
	if in.IsTemplate != nil {
		isTemplate = *in.IsTemplate
	}
	norm, err := NormalizeWorkoutInput(WorkoutInput{
		Title: title, Description: src.Description, Course: src.Course,
		IsTemplate: isTemplate, Tags: src.Tags, Sections: cloneSections(src.Sections),
	})
	if err != nil {
		return nil, err
	}
	w := buildWorkout(orgID, newID(), norm, actor, time.Now().UTC())
	w.SourceWorkoutID = src.ID
	if s.repo != nil {
		if err := upsertWorkout(context.Background(), s.repo.pool, w); err != nil {
			return nil, err
		}
		return w, nil
	}
	s.mu.Lock()
	s.workouts[w.ID] = cloneWorkout(w)
	s.mu.Unlock()
	return w, nil
}

// AssignWorkout copies a workout into a practice-owned instance and links it,
// either to an existing practice (replacing any previous instance) or to a
// new water practice on Date for GroupID.
func (s *Service) AssignWorkout(orgID, workoutID string, in AssignInput, actor string) (*Practice, *Workout, error) {
	src, err := s.GetWorkout(orgID, workoutID)
	if err != nil {
		return nil, nil, err
	}
	in.PracticeID = strings.TrimSpace(in.PracticeID)
	in.Date = strings.TrimSpace(in.Date)
	if in.PracticeID == "" && in.Date == "" {
		return nil, nil, ErrInvalidAssignment
	}
	if in.PracticeID == "" {
		if _, err := time.Parse("2006-01-02", in.Date); err != nil {
			return nil, nil, &WorkoutValidationError{Field: "date", Message: "must be YYYY-MM-DD"}
		}
	}
	practiceType := strings.TrimSpace(in.Type)
	if practiceType == "" {
		practiceType = "water"
	}
	if !validTypes[practiceType] {
		return nil, nil, ErrInvalidType
	}
	now := time.Now().UTC()
	instance := cloneWorkout(src)
	instance.ID = newID()
	instance.IsTemplate = false
	instance.SourceWorkoutID = src.ID
	instance.CreatedAt, instance.UpdatedAt = now, now
	instance.CreatedBy, instance.UpdatedBy = actor, actor

	newPractice := func() *Practice {
		return &Practice{
			ID: newID(), OrgID: orgID, Date: in.Date, GroupID: strings.TrimSpace(in.GroupID),
			Type: practiceType, Description: src.Title,
			CreatedAt: now, UpdatedAt: now, CreatedBy: actor, UpdatedBy: actor,
		}
	}
	return s.linkInstance(orgID, in.PracticeID, newPractice, instance, actor, now)
}

// linkInstance stores instance and links it to practiceID (or a new practice
// from mk), deleting the practice's previous non-template instance.
func (s *Service) linkInstance(orgID, practiceID string, mk func() *Practice, instance *Workout, actor string, now time.Time) (*Practice, *Workout, error) {
	ctx := context.Background()
	if s.repo != nil {
		var out *Practice
		err := s.repo.withTx(ctx, func(q querier) error {
			var p *Practice
			if practiceID != "" {
				existing, err := getPractice(ctx, q, orgID, practiceID)
				if err != nil {
					return err
				}
				p = existing
			} else {
				p = mk()
			}
			previous := p.WorkoutID
			if err := upsertWorkout(ctx, q, instance); err != nil {
				return err
			}
			derivePractice(p, instance)
			p.UpdatedAt, p.UpdatedBy = now, actor
			if err := upsertPractice(ctx, q, p); err != nil {
				return err
			}
			if previous != "" && previous != instance.ID {
				if _, err := q.Exec(ctx, `DELETE FROM training_workouts WHERE id = $1::uuid AND org_id = $2 AND NOT is_template`, previous, orgID); err != nil {
					return err
				}
			}
			out = p
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
		return out, instance, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	var p *Practice
	if practiceID != "" {
		existing, ok := s.practices[practiceID]
		if !ok || existing.OrgID != orgID {
			return nil, nil, ErrNotFound
		}
		cp := *existing
		p = &cp
	} else {
		p = mk()
	}
	previous := p.WorkoutID
	s.workouts[instance.ID] = cloneWorkout(instance)
	derivePractice(p, instance)
	p.UpdatedAt, p.UpdatedBy = now, actor
	s.practices[p.ID] = p
	if previous != "" && previous != instance.ID {
		if old, ok := s.workouts[previous]; ok && !old.IsTemplate {
			delete(s.workouts, previous)
		}
	}
	cp := *p
	return &cp, instance, nil
}

// SetPracticeWorkout creates or replaces the structured workout carried by a
// practice. The instance is never a template.
func (s *Service) SetPracticeWorkout(orgID, practiceID string, in WorkoutInput, actor string) (*Practice, *Workout, error) {
	p, err := s.Get(orgID, practiceID)
	if err != nil {
		return nil, nil, err
	}
	in.IsTemplate = false
	if strings.TrimSpace(in.Title) == "" {
		in.Title = p.Description
		if strings.TrimSpace(in.Title) == "" {
			in.Title = "Practice " + p.Date
		}
	}
	if p.WorkoutID != "" {
		if existing, err := s.GetWorkout(orgID, p.WorkoutID); err == nil && !existing.IsTemplate {
			w, err := s.UpdateWorkout(orgID, existing.ID, in, actor)
			if err != nil {
				return nil, nil, err
			}
			updated, err := s.Get(orgID, practiceID)
			if err != nil {
				return nil, nil, err
			}
			return updated, w, nil
		}
	}
	norm, err := NormalizeWorkoutInput(in)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now().UTC()
	instance := buildWorkout(orgID, newID(), norm, actor, now)
	return s.linkInstance(orgID, practiceID, nil, instance, actor, now)
}

// ClearPracticeWorkout unlinks (and deletes) a practice's workout instance.
// The practice keeps its last derived yardage so history is preserved.
func (s *Service) ClearPracticeWorkout(orgID, practiceID, actor string) (*Practice, error) {
	ctx := context.Background()
	p, err := s.Get(orgID, practiceID)
	if err != nil {
		return nil, err
	}
	if p.WorkoutID == "" {
		return nil, ErrPracticeNoWorkout
	}
	previous := p.WorkoutID
	p.WorkoutID = ""
	p.UpdatedAt, p.UpdatedBy = time.Now().UTC(), actor
	if s.repo != nil {
		err := s.repo.withTx(ctx, func(q querier) error {
			if err := upsertPractice(ctx, q, p); err != nil {
				return err
			}
			_, err := q.Exec(ctx, `DELETE FROM training_workouts WHERE id = $1::uuid AND org_id = $2 AND NOT is_template`, previous, orgID)
			return err
		})
		if err != nil {
			return nil, err
		}
		return p, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.practices[p.ID] = p
	if old, ok := s.workouts[previous]; ok && !old.IsTemplate {
		delete(s.workouts, previous)
	}
	cp := *p
	return &cp, nil
}

// GetPracticeWorkout returns the workout linked to a practice.
func (s *Service) GetPracticeWorkout(orgID, practiceID string) (*Practice, *Workout, error) {
	p, err := s.Get(orgID, practiceID)
	if err != nil {
		return nil, nil, err
	}
	if p.WorkoutID == "" {
		return p, nil, ErrPracticeNoWorkout
	}
	w, err := s.GetWorkout(orgID, p.WorkoutID)
	if err != nil {
		if errors.Is(err, ErrWorkoutNotFound) {
			return p, nil, ErrPracticeNoWorkout
		}
		return nil, nil, err
	}
	return p, w, nil
}

// WorkoutsForPractices loads the workouts linked to the given practices.
func (s *Service) WorkoutsForPractices(orgID string, practices []*Practice) (map[string]*Workout, error) {
	ids := []string{}
	for _, p := range practices {
		if p.WorkoutID != "" {
			ids = append(ids, p.WorkoutID)
		}
	}
	if s.repo != nil {
		return s.repo.WorkoutsByIDs(context.Background(), orgID, ids)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]*Workout, len(ids))
	for _, id := range ids {
		if w, ok := s.workouts[id]; ok && w.OrgID == orgID {
			out[id] = cloneWorkout(w)
		}
	}
	return out, nil
}

// attendanceForPractices returns attendance rows for the given practices.
func (s *Service) attendanceForPractices(orgID string, practiceIDs []string) ([]*PracticeAttendance, error) {
	if s.repo != nil {
		return s.repo.AttendanceForPractices(context.Background(), orgID, practiceIDs)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []*PracticeAttendance{}
	for _, id := range practiceIDs {
		for _, rec := range s.attendance[id] {
			if rec.OrgID == orgID {
				cp := *rec
				out = append(out, &cp)
			}
		}
	}
	return out, nil
}

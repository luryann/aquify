package meets

import "encoding/json"

type runtimeSnapshot struct {
	Meets           map[string]Meet                `json:"meets"`
	Sessions        map[string]MeetSession         `json:"sessions"`
	Events          map[string]MeetEvent           `json:"events"`
	Commitments     map[string]Commitment          `json:"commitments"`
	Selections      map[string]EventSelection      `json:"selections"`
	Entries         map[string]Entry               `json:"entries"`
	EntryLegs       map[string]EntryLeg            `json:"entry_legs"`
	Swimmers        map[string]swimmerRef         `json:"swimmers"`
	Households      map[string]householdRef       `json:"households"`
	ParentHousehold map[string]map[string]struct{} `json:"parent_household"`
	AuditEvents     []MeetAuditEvent               `json:"audit_events"`
}

func (s *Service) ExportRuntimeSnapshot() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := runtimeSnapshot{
		Meets:           cloneMeets(s.meets),
		Sessions:        cloneMeetSessions(s.sessions),
		Events:          cloneMeetEvents(s.events),
		Commitments:     cloneCommitments(s.commitments),
		Selections:      cloneSelections(s.selections),
		Entries:         cloneEntries(s.entries),
		EntryLegs:       cloneEntryLegs(s.entryLegs),
		Swimmers:        cloneSwimmerRefs(s.swimmers),
		Households:      cloneHouseholdRefs(s.households),
		ParentHousehold: cloneStringSetMap(s.parentHousehold),
		AuditEvents:     append([]MeetAuditEvent{}, s.auditEvents...),
	}
	return json.Marshal(snapshot)
}

func (s *Service) LoadRuntimeSnapshot(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	var snapshot runtimeSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.meets = cloneMeets(snapshot.Meets)
	s.sessions = cloneMeetSessions(snapshot.Sessions)
	s.events = cloneMeetEvents(snapshot.Events)
	s.commitments = cloneCommitments(snapshot.Commitments)
	s.selections = cloneSelections(snapshot.Selections)
	s.entries = cloneEntries(snapshot.Entries)
	s.entryLegs = cloneEntryLegs(snapshot.EntryLegs)
	s.swimmers = cloneSwimmerRefs(snapshot.Swimmers)
	s.households = cloneHouseholdRefs(snapshot.Households)
	s.parentHousehold = cloneStringSetMap(snapshot.ParentHousehold)
	s.auditEvents = append([]MeetAuditEvent{}, snapshot.AuditEvents...)
	return nil
}

func cloneMeets(in map[string]Meet) map[string]Meet {
	out := make(map[string]Meet, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneMeetSessions(in map[string]MeetSession) map[string]MeetSession {
	out := make(map[string]MeetSession, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneMeetEvents(in map[string]MeetEvent) map[string]MeetEvent {
	out := make(map[string]MeetEvent, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneCommitments(in map[string]Commitment) map[string]Commitment {
	out := make(map[string]Commitment, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneSelections(in map[string]EventSelection) map[string]EventSelection {
	out := make(map[string]EventSelection, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneEntries(in map[string]Entry) map[string]Entry {
	out := make(map[string]Entry, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneEntryLegs(in map[string]EntryLeg) map[string]EntryLeg {
	out := make(map[string]EntryLeg, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneSwimmerRefs(in map[string]swimmerRef) map[string]swimmerRef {
	out := make(map[string]swimmerRef, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneHouseholdRefs(in map[string]householdRef) map[string]householdRef {
	out := make(map[string]householdRef, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneStringSetMap(in map[string]map[string]struct{}) map[string]map[string]struct{} {
	out := make(map[string]map[string]struct{}, len(in))
	for key, values := range in {
		cloned := make(map[string]struct{}, len(values))
		for value := range values {
			cloned[value] = struct{}{}
		}
		out[key] = cloned
	}
	return out
}

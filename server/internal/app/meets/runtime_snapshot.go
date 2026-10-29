package meets

import (
	"encoding/json"

	"github.com/dareaquatics/aquify/server/internal/runtimepersist"
)

type runtimeSnapshot struct {
	Meets           map[string]Meet                `json:"meets"`
	Sessions        map[string]MeetSession         `json:"sessions"`
	Events          map[string]MeetEvent           `json:"events"`
	Commitments     map[string]Commitment          `json:"commitments"`
	Selections      map[string]EventSelection      `json:"selections"`
	Entries         map[string]Entry               `json:"entries"`
	EntryLegs       map[string]EntryLeg            `json:"entry_legs"`
	Swimmers        map[string]swimmerRef          `json:"swimmers"`
	Households      map[string]householdRef        `json:"households"`
	ParentHousehold map[string]map[string]struct{} `json:"parent_household"`
	AuditEvents     []MeetAuditEvent               `json:"audit_events"`
}

func (s *Service) ExportRuntimeSnapshot() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := runtimeSnapshot{
		Meets:           runtimepersist.CloneMap(s.meets),
		Sessions:        runtimepersist.CloneMap(s.sessions),
		Events:          runtimepersist.CloneMap(s.events),
		Commitments:     runtimepersist.CloneMap(s.commitments),
		Selections:      runtimepersist.CloneMap(s.selections),
		Entries:         runtimepersist.CloneMap(s.entries),
		EntryLegs:       runtimepersist.CloneMap(s.entryLegs),
		Swimmers:        runtimepersist.CloneMap(s.swimmers),
		Households:      runtimepersist.CloneMap(s.households),
		ParentHousehold: runtimepersist.CloneSetMap(s.parentHousehold),
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
	s.meets = runtimepersist.CloneMap(snapshot.Meets)
	s.sessions = runtimepersist.CloneMap(snapshot.Sessions)
	s.events = runtimepersist.CloneMap(snapshot.Events)
	s.commitments = runtimepersist.CloneMap(snapshot.Commitments)
	s.selections = runtimepersist.CloneMap(snapshot.Selections)
	s.entries = runtimepersist.CloneMap(snapshot.Entries)
	s.entryLegs = runtimepersist.CloneMap(snapshot.EntryLegs)
	s.swimmers = runtimepersist.CloneMap(snapshot.Swimmers)
	s.households = runtimepersist.CloneMap(snapshot.Households)
	s.parentHousehold = runtimepersist.CloneSetMap(snapshot.ParentHousehold)
	s.auditEvents = append([]MeetAuditEvent{}, snapshot.AuditEvents...)
	return nil
}

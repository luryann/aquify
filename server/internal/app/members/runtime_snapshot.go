package members

import (
	"encoding/json"

	"github.com/dareaquatics/aquify/server/internal/runtimepersist"
)

type runtimeSnapshot struct {
	Households           map[string]Household           `json:"households"`
	Contacts             map[string]Contact             `json:"contacts"`
	Swimmers             map[string]Swimmer             `json:"swimmers"`
	SwimmerGroupLinks    map[string]SwimmerGroupLink    `json:"swimmer_group_links"`
	ParentHouseholdIndex map[string]map[string]struct{} `json:"parent_household_index"`
	AuditEvents          []SwimmerAuditEvent            `json:"audit_events"`
}

func (s *Service) ExportRuntimeSnapshot() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := runtimeSnapshot{
		Households:           runtimepersist.CloneMap(s.households),
		Contacts:             runtimepersist.CloneMap(s.contacts),
		Swimmers:             runtimepersist.CloneMap(s.swimmers),
		SwimmerGroupLinks:    runtimepersist.CloneMap(s.swimmerGroupLinks),
		ParentHouseholdIndex: runtimepersist.CloneSetMap(s.parentHouseholdIndex),
		AuditEvents:          append([]SwimmerAuditEvent{}, s.auditEvents...),
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
	s.households = runtimepersist.CloneMap(snapshot.Households)
	s.contacts = runtimepersist.CloneMap(snapshot.Contacts)
	s.swimmers = runtimepersist.CloneMap(snapshot.Swimmers)
	s.swimmerGroupLinks = runtimepersist.CloneMap(snapshot.SwimmerGroupLinks)
	s.parentHouseholdIndex = runtimepersist.CloneSetMap(snapshot.ParentHouseholdIndex)
	s.auditEvents = append([]SwimmerAuditEvent{}, snapshot.AuditEvents...)
	return nil
}

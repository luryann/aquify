package members

import "encoding/json"

type runtimeSnapshot struct {
	Households           map[string]Household              `json:"households"`
	Contacts             map[string]Contact                `json:"contacts"`
	Swimmers             map[string]Swimmer                `json:"swimmers"`
	SwimmerGroupLinks    map[string]SwimmerGroupLink       `json:"swimmer_group_links"`
	ParentHouseholdIndex map[string]map[string]struct{}    `json:"parent_household_index"`
	AuditEvents          []SwimmerAuditEvent               `json:"audit_events"`
}

func (s *Service) ExportRuntimeSnapshot() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := runtimeSnapshot{
		Households:           cloneHouseholds(s.households),
		Contacts:             cloneContacts(s.contacts),
		Swimmers:             cloneSwimmers(s.swimmers),
		SwimmerGroupLinks:    cloneSwimmerGroupLinks(s.swimmerGroupLinks),
		ParentHouseholdIndex: cloneStringSetMap(s.parentHouseholdIndex),
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
	s.households = cloneHouseholds(snapshot.Households)
	s.contacts = cloneContacts(snapshot.Contacts)
	s.swimmers = cloneSwimmers(snapshot.Swimmers)
	s.swimmerGroupLinks = cloneSwimmerGroupLinks(snapshot.SwimmerGroupLinks)
	s.parentHouseholdIndex = cloneStringSetMap(snapshot.ParentHouseholdIndex)
	s.auditEvents = append([]SwimmerAuditEvent{}, snapshot.AuditEvents...)
	return nil
}

func cloneHouseholds(in map[string]Household) map[string]Household {
	out := make(map[string]Household, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneContacts(in map[string]Contact) map[string]Contact {
	out := make(map[string]Contact, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneSwimmers(in map[string]Swimmer) map[string]Swimmer {
	out := make(map[string]Swimmer, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneSwimmerGroupLinks(in map[string]SwimmerGroupLink) map[string]SwimmerGroupLink {
	out := make(map[string]SwimmerGroupLink, len(in))
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

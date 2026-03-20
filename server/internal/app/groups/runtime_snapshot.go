package groups

import "encoding/json"

type runtimeSnapshot struct {
	Groups          map[string]Group                  `json:"groups"`
	Rules           map[string]EligibilityRule        `json:"rules"`
	Registrations   map[string]Registration           `json:"registrations"`
	Waivers         map[string]WaiverAcceptance       `json:"waivers"`
	Swimmers        map[string]swimmerRef            `json:"swimmers"`
	Households      map[string]householdRef          `json:"households"`
	ParentHousehold map[string]map[string]struct{}   `json:"parent_household"`
	AuditEvents     []GroupAuditEvent                 `json:"audit_events"`
}

func (s *Service) ExportRuntimeSnapshot() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := runtimeSnapshot{
		Groups:          cloneGroups(s.groups),
		Rules:           cloneRules(s.rules),
		Registrations:   cloneRegistrations(s.registrations),
		Waivers:         cloneWaivers(s.waivers),
		Swimmers:        cloneSwimmerRefs(s.swimmers),
		Households:      cloneHouseholdRefs(s.households),
		ParentHousehold: cloneStringSetMap(s.parentHousehold),
		AuditEvents:     append([]GroupAuditEvent{}, s.auditEvents...),
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
	s.groups = cloneGroups(snapshot.Groups)
	s.rules = cloneRules(snapshot.Rules)
	s.registrations = cloneRegistrations(snapshot.Registrations)
	s.waivers = cloneWaivers(snapshot.Waivers)
	s.swimmers = cloneSwimmerRefs(snapshot.Swimmers)
	s.households = cloneHouseholdRefs(snapshot.Households)
	s.parentHousehold = cloneStringSetMap(snapshot.ParentHousehold)
	s.auditEvents = append([]GroupAuditEvent{}, snapshot.AuditEvents...)
	return nil
}

func cloneGroups(in map[string]Group) map[string]Group {
	out := make(map[string]Group, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneRules(in map[string]EligibilityRule) map[string]EligibilityRule {
	out := make(map[string]EligibilityRule, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneRegistrations(in map[string]Registration) map[string]Registration {
	out := make(map[string]Registration, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneWaivers(in map[string]WaiverAcceptance) map[string]WaiverAcceptance {
	out := make(map[string]WaiverAcceptance, len(in))
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

package groups

import (
	"encoding/json"

	"github.com/dareaquatics/aquify/server/internal/runtimepersist"
)

type runtimeSnapshot struct {
	Groups          map[string]Group               `json:"groups"`
	Rules           map[string]EligibilityRule     `json:"rules"`
	Registrations   map[string]Registration        `json:"registrations"`
	Waivers         map[string]WaiverAcceptance    `json:"waivers"`
	Swimmers        map[string]swimmerRef          `json:"swimmers"`
	Households      map[string]householdRef        `json:"households"`
	ParentHousehold map[string]map[string]struct{} `json:"parent_household"`
	AuditEvents     []GroupAuditEvent              `json:"audit_events"`
}

func (s *Service) ExportRuntimeSnapshot() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := runtimeSnapshot{
		Groups:          runtimepersist.CloneMap(s.groups),
		Rules:           runtimepersist.CloneMap(s.rules),
		Registrations:   runtimepersist.CloneMap(s.registrations),
		Waivers:         runtimepersist.CloneMap(s.waivers),
		Swimmers:        runtimepersist.CloneMap(s.swimmers),
		Households:      runtimepersist.CloneMap(s.households),
		ParentHousehold: runtimepersist.CloneSetMap(s.parentHousehold),
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
	s.groups = runtimepersist.CloneMap(snapshot.Groups)
	s.rules = runtimepersist.CloneMap(snapshot.Rules)
	s.registrations = runtimepersist.CloneMap(snapshot.Registrations)
	s.waivers = runtimepersist.CloneMap(snapshot.Waivers)
	s.swimmers = runtimepersist.CloneMap(snapshot.Swimmers)
	s.households = runtimepersist.CloneMap(snapshot.Households)
	s.parentHousehold = runtimepersist.CloneSetMap(snapshot.ParentHousehold)
	s.auditEvents = append([]GroupAuditEvent{}, snapshot.AuditEvents...)
	return nil
}

package performance

import (
	"encoding/json"

	"github.com/dareaquatics/aquify/server/internal/runtimepersist"
)

type runtimeSnapshot struct {
	Results     map[string]*Result       `json:"results"`
	Splits      map[string][]*Split      `json:"splits"`
	BestTimes   map[string]*BestTime     `json:"best_times"`
	Standards   map[string]*TimeStandard `json:"standards"`
	AuditEvents []*BestTimeAuditEvent    `json:"audit_events"`
}

func (s *Service) ExportRuntimeSnapshot() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := runtimeSnapshot{
		Results:     runtimepersist.ClonePtrMap(s.results),
		Splits:      runtimepersist.ClonePtrListMap(s.splits),
		BestTimes:   runtimepersist.ClonePtrMap(s.bestTimes),
		Standards:   runtimepersist.ClonePtrMap(s.standards),
		AuditEvents: runtimepersist.ClonePtrs(s.auditEvents),
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
	s.results = runtimepersist.ClonePtrMap(snapshot.Results)
	s.splits = runtimepersist.ClonePtrListMap(snapshot.Splits)
	s.bestTimes = runtimepersist.ClonePtrMap(snapshot.BestTimes)
	s.standards = runtimepersist.ClonePtrMap(snapshot.Standards)
	s.auditEvents = runtimepersist.ClonePtrs(snapshot.AuditEvents)
	s.btByKey = make(map[string]*BestTime, len(s.bestTimes))
	for _, bt := range s.bestTimes {
		if bt == nil {
			continue
		}
		s.btByKey[btKey(bt.OrgID, bt.SwimmerID, bt.EventDistance, bt.Stroke, bt.Course)] = bt
	}
	return nil
}

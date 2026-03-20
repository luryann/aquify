package performance

import "encoding/json"

type runtimeSnapshot struct {
	Results     map[string]*Result          `json:"results"`
	Splits      map[string][]*Split         `json:"splits"`
	BestTimes   map[string]*BestTime        `json:"best_times"`
	Standards   map[string]*TimeStandard    `json:"standards"`
	AuditEvents []*BestTimeAuditEvent       `json:"audit_events"`
}

func (s *Service) ExportRuntimeSnapshot() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := runtimeSnapshot{
		Results:     cloneResults(s.results),
		Splits:      cloneSplits(s.splits),
		BestTimes:   cloneBestTimes(s.bestTimes),
		Standards:   cloneTimeStandards(s.standards),
		AuditEvents: cloneBestTimeAuditEvents(s.auditEvents),
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
	s.results = cloneResults(snapshot.Results)
	s.splits = cloneSplits(snapshot.Splits)
	s.bestTimes = cloneBestTimes(snapshot.BestTimes)
	s.standards = cloneTimeStandards(snapshot.Standards)
	s.auditEvents = cloneBestTimeAuditEvents(snapshot.AuditEvents)
	s.btByKey = make(map[string]*BestTime, len(s.bestTimes))
	for _, bt := range s.bestTimes {
		if bt == nil {
			continue
		}
		s.btByKey[btKey(bt.OrgID, bt.SwimmerID, bt.EventDistance, bt.Stroke, bt.Course)] = bt
	}
	return nil
}

func cloneResults(in map[string]*Result) map[string]*Result {
	out := make(map[string]*Result, len(in))
	for k, v := range in {
		if v == nil {
			out[k] = nil
			continue
		}
		cp := *v
		out[k] = &cp
	}
	return out
}

func cloneSplits(in map[string][]*Split) map[string][]*Split {
	out := make(map[string][]*Split, len(in))
	for k, values := range in {
		cloned := make([]*Split, 0, len(values))
		for _, value := range values {
			if value == nil {
				cloned = append(cloned, nil)
				continue
			}
			cp := *value
			cloned = append(cloned, &cp)
		}
		out[k] = cloned
	}
	return out
}

func cloneBestTimes(in map[string]*BestTime) map[string]*BestTime {
	out := make(map[string]*BestTime, len(in))
	for k, v := range in {
		if v == nil {
			out[k] = nil
			continue
		}
		cp := *v
		out[k] = &cp
	}
	return out
}

func cloneTimeStandards(in map[string]*TimeStandard) map[string]*TimeStandard {
	out := make(map[string]*TimeStandard, len(in))
	for k, v := range in {
		if v == nil {
			out[k] = nil
			continue
		}
		cp := *v
		out[k] = &cp
	}
	return out
}

func cloneBestTimeAuditEvents(in []*BestTimeAuditEvent) []*BestTimeAuditEvent {
	out := make([]*BestTimeAuditEvent, 0, len(in))
	for _, value := range in {
		if value == nil {
			out = append(out, nil)
			continue
		}
		cp := *value
		out = append(out, &cp)
	}
	return out
}

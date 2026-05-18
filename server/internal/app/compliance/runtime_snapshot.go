package compliance

import "encoding/json"

type runtimeSnapshot struct {
	Exports   map[string]*DataExportRequest   `json:"exports"`
	Deletions map[string]*DataDeletionRequest `json:"deletions"`
}

func (s *Service) ExportRuntimeSnapshot() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := runtimeSnapshot{
		Exports:   cloneExportRequests(s.exports),
		Deletions: cloneDeletionRequests(s.deletions),
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
	s.exports = cloneExportRequests(snapshot.Exports)
	s.deletions = cloneDeletionRequests(snapshot.Deletions)
	return nil
}

func cloneExportRequests(in map[string]*DataExportRequest) map[string]*DataExportRequest {
	out := make(map[string]*DataExportRequest, len(in))
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

func cloneDeletionRequests(in map[string]*DataDeletionRequest) map[string]*DataDeletionRequest {
	out := make(map[string]*DataDeletionRequest, len(in))
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

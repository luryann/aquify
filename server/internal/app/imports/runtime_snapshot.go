package imports

import "encoding/json"

type runtimeSnapshot struct {
	Imports   map[string]*FileImport    `json:"imports"`
	Conflicts map[string]*MatchConflict `json:"conflicts"`
}

func (s *Service) ExportRuntimeSnapshot() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := runtimeSnapshot{
		Imports:   cloneImports(s.imports),
		Conflicts: cloneConflicts(s.conflicts),
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
	s.imports = cloneImports(snapshot.Imports)
	s.conflicts = cloneConflicts(snapshot.Conflicts)
	return nil
}

func cloneImports(in map[string]*FileImport) map[string]*FileImport {
	out := make(map[string]*FileImport, len(in))
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

func cloneConflicts(in map[string]*MatchConflict) map[string]*MatchConflict {
	out := make(map[string]*MatchConflict, len(in))
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

package compliance

import (
	"encoding/json"

	"github.com/dareaquatics/aquify/server/internal/runtimepersist"
)

type runtimeSnapshot struct {
	Exports   map[string]*DataExportRequest   `json:"exports"`
	Deletions map[string]*DataDeletionRequest `json:"deletions"`
}

func (s *Service) ExportRuntimeSnapshot() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := runtimeSnapshot{
		Exports:   runtimepersist.ClonePtrMap(s.exports),
		Deletions: runtimepersist.ClonePtrMap(s.deletions),
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
	s.exports = runtimepersist.ClonePtrMap(snapshot.Exports)
	s.deletions = runtimepersist.ClonePtrMap(snapshot.Deletions)
	return nil
}

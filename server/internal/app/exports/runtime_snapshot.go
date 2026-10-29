package exports

import "encoding/json"

type runtimeSnapshot struct {
	Exports map[string]fileExportSnapshot `json:"exports"`
}

type fileExportSnapshot struct {
	FileExport   FileExport   `json:"file_export"`
	PendingInput *ReportInput `json:"pending_input,omitempty"`
}

func (s *Service) ExportRuntimeSnapshot() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := runtimeSnapshot{
		Exports: make(map[string]fileExportSnapshot, len(s.exports)),
	}
	for id, export := range s.exports {
		if export == nil {
			continue
		}
		cp := *export
		cp.pendingInput = nil
		var pending *ReportInput
		if export.pendingInput != nil {
			input := *export.pendingInput
			pending = &input
		}
		snapshot.Exports[id] = fileExportSnapshot{
			FileExport:   cp,
			PendingInput: pending,
		}
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
	s.exports = make(map[string]*FileExport, len(snapshot.Exports))
	for id, export := range snapshot.Exports {
		cp := export.FileExport
		if export.PendingInput != nil {
			input := *export.PendingInput
			cp.pendingInput = &input
		}
		s.exports[id] = &cp
	}
	return nil
}

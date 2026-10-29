package comms

import (
	"encoding/json"

	"github.com/dareaquatics/aquify/server/internal/runtimepersist"
)

type runtimeSnapshot struct {
	Announcements map[string]*Announcement           `json:"announcements"`
	Campaigns     map[string]*EmailCampaign          `json:"campaigns"`
	Deliveries    map[string]*EmailDelivery          `json:"deliveries"`
	Suppression   map[string]*SuppressionEntry       `json:"suppression"`
	Preferences   map[string]*NotificationPreference `json:"preferences"`
}

func (s *Service) ExportRuntimeSnapshot() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := runtimeSnapshot{
		Announcements: runtimepersist.ClonePtrMap(s.announcements),
		Campaigns:     runtimepersist.ClonePtrMap(s.campaigns),
		Deliveries:    runtimepersist.ClonePtrMap(s.deliveries),
		Suppression:   runtimepersist.ClonePtrMap(s.suppression),
		Preferences:   runtimepersist.ClonePtrMap(s.preferences),
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
	s.announcements = runtimepersist.ClonePtrMap(snapshot.Announcements)
	s.campaigns = runtimepersist.ClonePtrMap(snapshot.Campaigns)
	s.deliveries = runtimepersist.ClonePtrMap(snapshot.Deliveries)
	s.suppression = runtimepersist.ClonePtrMap(snapshot.Suppression)
	s.preferences = runtimepersist.ClonePtrMap(snapshot.Preferences)
	return nil
}

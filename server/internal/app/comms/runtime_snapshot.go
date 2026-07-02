package comms

import "encoding/json"

type runtimeSnapshot struct {
	Announcements map[string]*Announcement      `json:"announcements"`
	Campaigns     map[string]*EmailCampaign     `json:"campaigns"`
	Deliveries    map[string]*EmailDelivery     `json:"deliveries"`
	Suppression   map[string]*SuppressionEntry  `json:"suppression"`
	Preferences   map[string]*NotificationPreference `json:"preferences"`
}

func (s *Service) ExportRuntimeSnapshot() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := runtimeSnapshot{
		Announcements: cloneAnnouncements(s.announcements),
		Campaigns:     cloneCampaigns(s.campaigns),
		Deliveries:    cloneDeliveries(s.deliveries),
		Suppression:   cloneSuppressionEntries(s.suppression),
		Preferences:   cloneNotificationPreferences(s.preferences),
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
	s.announcements = cloneAnnouncements(snapshot.Announcements)
	s.campaigns = cloneCampaigns(snapshot.Campaigns)
	s.deliveries = cloneDeliveries(snapshot.Deliveries)
	s.suppression = cloneSuppressionEntries(snapshot.Suppression)
	s.preferences = cloneNotificationPreferences(snapshot.Preferences)
	return nil
}

func cloneAnnouncements(in map[string]*Announcement) map[string]*Announcement {
	out := make(map[string]*Announcement, len(in))
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

func cloneCampaigns(in map[string]*EmailCampaign) map[string]*EmailCampaign {
	out := make(map[string]*EmailCampaign, len(in))
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

func cloneDeliveries(in map[string]*EmailDelivery) map[string]*EmailDelivery {
	out := make(map[string]*EmailDelivery, len(in))
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

func cloneSuppressionEntries(in map[string]*SuppressionEntry) map[string]*SuppressionEntry {
	out := make(map[string]*SuppressionEntry, len(in))
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

func cloneNotificationPreferences(in map[string]*NotificationPreference) map[string]*NotificationPreference {
	out := make(map[string]*NotificationPreference, len(in))
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

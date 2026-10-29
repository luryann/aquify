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

	Messages    map[string]*Message           `json:"messages"`
	Edits       map[string]*MessageEdit       `json:"message_edits"`
	Reactions   map[string]*MessageReaction   `json:"message_reactions"`
	Attachments map[string]*MessageAttachment `json:"message_attachments"`
	Reads       map[string]*MessageRead       `json:"message_reads"`
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

		Messages:    runtimepersist.ClonePtrMap(s.messages),
		Edits:       runtimepersist.ClonePtrMap(s.messageEdits),
		Reactions:   runtimepersist.ClonePtrMap(s.messageReactions),
		Attachments: runtimepersist.ClonePtrMap(s.messageAttachments),
		Reads:       runtimepersist.ClonePtrMap(s.messageReads),
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
	s.messages = nonNilMap(runtimepersist.ClonePtrMap(snapshot.Messages))
	s.messageEdits = nonNilMap(runtimepersist.ClonePtrMap(snapshot.Edits))
	s.messageReactions = nonNilMap(runtimepersist.ClonePtrMap(snapshot.Reactions))
	s.messageAttachments = nonNilMap(runtimepersist.ClonePtrMap(snapshot.Attachments))
	s.messageReads = nonNilMap(runtimepersist.ClonePtrMap(snapshot.Reads))
	return nil
}

// nonNilMap guards writes after loading an older snapshot that lacks a field.
func nonNilMap[V any](m map[string]V) map[string]V {
	if m == nil {
		return make(map[string]V)
	}
	return m
}

package members

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
)

// ErrSwimmerLinkRequestNotFound is returned when a pending link request
// cannot be found for the given org/id.
var ErrSwimmerLinkRequestNotFound = errors.New("swimmer link request not found")

// SwimmerLinkRequest records a swimmer-role user's self-service request to
// have their login linked to a swimmer roster record.
type SwimmerLinkRequest struct {
	ID                   string    `json:"id"`
	OrgID                string    `json:"org_id"`
	UserID               string    `json:"user_id"`
	RequestedSwimmerName string    `json:"requested_swimmer_name,omitempty"`
	Note                 string    `json:"note,omitempty"`
	Status               string    `json:"status"`
	CreatedAt            time.Time `json:"created_at"`
}

// CreateLinkRequest records a new pending swimmer-link request for a user.
func (s *Service) CreateLinkRequest(orgID, userID, requestedSwimmerName, note string) (SwimmerLinkRequest, error) {
	req := SwimmerLinkRequest{
		ID:                   s.newID("slr-"),
		OrgID:                orgID,
		UserID:               userID,
		RequestedSwimmerName: strings.TrimSpace(requestedSwimmerName),
		Note:                 strings.TrimSpace(note),
		Status:               "pending",
		CreatedAt:            time.Now().UTC(),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.repo != nil {
		if err := s.repo.InsertSwimmerLinkRequest(context.Background(), req); err != nil {
			return SwimmerLinkRequest{}, err
		}
	}
	if s.linkRequests == nil {
		s.linkRequests = make(map[string]SwimmerLinkRequest)
	}
	s.linkRequests[req.ID] = req
	return req, nil
}

// ListPendingLinkRequests returns pending swimmer-link requests for an org.
func (s *Service) ListPendingLinkRequests(orgID string) ([]SwimmerLinkRequest, error) {
	if s.repo != nil {
		return s.repo.ListPendingSwimmerLinkRequests(context.Background(), orgID)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []SwimmerLinkRequest{}
	for _, req := range s.linkRequests {
		if req.OrgID == orgID && req.Status == "pending" {
			out = append(out, req)
		}
	}
	// Match the repository's oldest-first order; map iteration is random.
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// ResolveLinkRequest marks a pending link request resolved. It does not
// itself perform the swimmer/user link; callers should invoke
// LinkSwimmerToUser separately and pass the resulting swimmer ID here.
func (s *Service) ResolveLinkRequest(orgID, requestID, resolvedBy, resolvedSwimmerID string) error {
	if s.repo != nil {
		return s.repo.ResolveSwimmerLinkRequest(context.Background(), orgID, requestID, resolvedBy, resolvedSwimmerID)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	req, ok := s.linkRequests[requestID]
	if !ok || req.OrgID != orgID || req.Status != "pending" {
		return ErrSwimmerLinkRequestNotFound
	}
	req.Status = "resolved"
	s.linkRequests[requestID] = req
	return nil
}

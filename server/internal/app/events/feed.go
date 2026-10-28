package events

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"time"
)

// FeedToken authorizes a household's read-only .ics calendar feed. Only the
// SHA-256 hash of the secret is stored; the plaintext is shown once.
type FeedToken struct {
	ID          string     `json:"id"`
	OrgID       string     `json:"org_id"`
	HouseholdID string     `json:"household_id"`
	TokenHash   string     `json:"-"`
	CreatedAt   time.Time  `json:"created_at"`
	CreatedBy   string     `json:"created_by"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
}

// HashFeedToken returns the stored hash for a plaintext feed token.
func HashFeedToken(plaintext string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(plaintext)))
	return hex.EncodeToString(sum[:])
}

func newFeedSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// RotateFeedToken revokes any live token for the household and issues a new
// one, returning the plaintext secret (never stored).
func (s *Service) RotateFeedToken(orgID, householdID, actorID string) (string, *FeedToken, error) {
	secret, err := newFeedSecret()
	if err != nil {
		return "", nil, err
	}
	now := s.now()
	tok := &FeedToken{
		ID:          newID(),
		OrgID:       orgID,
		HouseholdID: householdID,
		TokenHash:   HashFeedToken(secret),
		CreatedAt:   now,
		CreatedBy:   actorID,
	}
	if s.repo != nil {
		if err := s.repo.RotateFeedToken(context.Background(), tok, now); err != nil {
			return "", nil, err
		}
		return secret, tok, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.tokens {
		if t.OrgID == orgID && t.HouseholdID == householdID && t.RevokedAt == nil {
			revoked := now
			t.RevokedAt = &revoked
		}
	}
	s.tokens[tok.ID] = tok
	s.hashIdx[tok.TokenHash] = tok.ID
	cp := *tok
	return secret, &cp, nil
}

// ActiveFeedToken returns the household's live token metadata, or nil.
func (s *Service) ActiveFeedToken(orgID, householdID string) (*FeedToken, error) {
	if s.repo != nil {
		return s.repo.ActiveFeedToken(context.Background(), orgID, householdID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.tokens {
		if t.OrgID == orgID && t.HouseholdID == householdID && t.RevokedAt == nil {
			cp := *t
			return &cp, nil
		}
	}
	return nil, nil
}

// RevokeFeedToken revokes the household's live token.
func (s *Service) RevokeFeedToken(orgID, householdID string) error {
	now := s.now()
	if s.repo != nil {
		return s.repo.RevokeFeedToken(context.Background(), orgID, householdID, now)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	found := false
	for _, t := range s.tokens {
		if t.OrgID == orgID && t.HouseholdID == householdID && t.RevokedAt == nil {
			revoked := now
			t.RevokedAt = &revoked
			found = true
		}
	}
	if !found {
		return ErrFeedTokenMissing
	}
	return nil
}

// ResolveFeedToken maps a plaintext secret to its live token.
func (s *Service) ResolveFeedToken(plaintext string) (*FeedToken, error) {
	plaintext = strings.TrimSpace(plaintext)
	if len(plaintext) < 32 {
		return nil, ErrFeedTokenMissing
	}
	hash := HashFeedToken(plaintext)
	now := s.now()
	if s.repo != nil {
		return s.repo.ResolveFeedToken(context.Background(), hash, now)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.hashIdx[hash]
	if !ok {
		return nil, ErrFeedTokenMissing
	}
	t := s.tokens[id]
	if t == nil || t.RevokedAt != nil {
		return nil, ErrFeedTokenMissing
	}
	used := now
	t.LastUsedAt = &used
	cp := *t
	return &cp, nil
}

// ─── iCalendar rendering ─────────────────────────────────────────────────────

// CalendarItem is one VEVENT in a feed. All-day items use StartDate/EndDate
// (YYYY-MM-DD, EndDate inclusive); timed items use Start/End.
type CalendarItem struct {
	UID         string
	Title       string
	Description string
	Location    string
	Categories  string
	AllDay      bool
	StartDate   string
	EndDate     string
	Start       time.Time
	End         *time.Time
	Cancelled   bool
	Updated     time.Time
}

func icsEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`, "\r", `\n`)
	return r.Replace(s)
}

// foldLine folds a content line at 75 octets per RFC 5545 §3.1 without
// splitting UTF-8 sequences.
func foldLine(line string) string {
	if len(line) <= 75 {
		return line
	}
	var b strings.Builder
	limit := 75
	count := 0
	for _, r := range line {
		size := len(string(r))
		if count+size > limit {
			b.WriteString("\r\n ")
			count = 1
			limit = 75
		}
		b.WriteRune(r)
		count += size
	}
	return b.String()
}

func icsDate(date string) string {
	return strings.ReplaceAll(date, "-", "")
}

func nextDay(date string) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	return t.AddDate(0, 0, 1).Format("2006-01-02")
}

// RenderICS renders a VCALENDAR document.
func RenderICS(calendarName string, items []CalendarItem, now time.Time) string {
	var lines []string
	add := func(l string) { lines = append(lines, foldLine(l)) }
	add("BEGIN:VCALENDAR")
	add("VERSION:2.0")
	add("PRODID:-//DARE Platform//Club Calendar//EN")
	add("CALSCALE:GREGORIAN")
	add("METHOD:PUBLISH")
	add("X-WR-CALNAME:" + icsEscape(calendarName))
	add("X-PUBLISHED-TTL:PT1H")
	add("REFRESH-INTERVAL;VALUE=DURATION:PT1H")
	stamp := now.UTC().Format("20060102T150405Z")
	for _, it := range items {
		add("BEGIN:VEVENT")
		add("UID:" + icsEscape(it.UID))
		add("DTSTAMP:" + stamp)
		if !it.Updated.IsZero() {
			add("LAST-MODIFIED:" + it.Updated.UTC().Format("20060102T150405Z"))
		}
		if it.AllDay {
			end := it.EndDate
			if end == "" || end < it.StartDate {
				end = it.StartDate
			}
			add("DTSTART;VALUE=DATE:" + icsDate(it.StartDate))
			add("DTEND;VALUE=DATE:" + icsDate(nextDay(end)))
		} else {
			add("DTSTART:" + it.Start.UTC().Format("20060102T150405Z"))
			end := it.Start.Add(time.Hour)
			if it.End != nil && !it.End.Before(it.Start) {
				end = *it.End
			}
			add("DTEND:" + end.UTC().Format("20060102T150405Z"))
		}
		add("SUMMARY:" + icsEscape(it.Title))
		if it.Description != "" {
			add("DESCRIPTION:" + icsEscape(it.Description))
		}
		if it.Location != "" {
			add("LOCATION:" + icsEscape(it.Location))
		}
		if it.Categories != "" {
			add("CATEGORIES:" + icsEscape(it.Categories))
		}
		if it.Cancelled {
			add("STATUS:CANCELLED")
		} else {
			add("STATUS:CONFIRMED")
		}
		add("TRANSP:OPAQUE")
		add("END:VEVENT")
	}
	add("END:VCALENDAR")
	return strings.Join(lines, "\r\n") + "\r\n"
}

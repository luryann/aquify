// Package invites issues and redeems org-scoped sign-up invite codes.
package invites

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/tenantdb"
)

var (
	// ErrInvalidCode covers unknown, revoked, expired and exhausted codes alike so
	// a guesser learns nothing about which case applied.
	ErrInvalidCode = errors.New("invite code is not valid")
	ErrNotFound    = errors.New("invite not found")
	ErrInvalidRole = errors.New("role must be parent, swimmer or coach")
)

// Invite is the stored record (never the plaintext code).
type Invite struct {
	ID        string     `json:"id"`
	OrgID     string     `json:"org_id"`
	Label     string     `json:"label"`
	Role      string     `json:"role"`
	MaxUses   int        `json:"max_uses"`
	Uses      int        `json:"uses"`
	ExpiresAt time.Time  `json:"expires_at"`
	CreatedBy string     `json:"created_by,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	codeHash  string
}

// CreateInput describes a new invite.
type CreateInput struct {
	Label   string
	Role    string
	MaxUses int
	TTL     time.Duration
}

const (
	maxTTL         = 90 * 24 * time.Hour
	defaultTTL     = 14 * 24 * time.Hour
	maxUsesCeiling = 1000
	codeAlphabet   = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0/O/1/I
	codeLen        = 12
)

// Service stores invites in Postgres, or in memory without a pool.
type Service struct {
	pool *pgxpool.Pool
	now  func() time.Time
	mu   sync.Mutex
	mem  map[string]*Invite // by id
}

func NewService() *Service { return &Service{now: time.Now, mem: map[string]*Invite{}} }

// at binds orgID to every statement so row-level security on org_invites applies.
func (s *Service) at(orgID string) tenantdb.Scoped { return tenantdb.Pool{P: s.pool}.For(orgID) }

func NewServiceWithPool(pool *pgxpool.Pool) *Service {
	s := NewService()
	s.pool = pool
	return s
}

// NormalizeCode upper-cases and strips spaces/hyphens so users can type or paste
// the displayed "ABCD-EFGH-JKMN" form.
func NormalizeCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	return strings.NewReplacer("-", "", " ", "").Replace(code)
}

func hashCode(code string) string {
	sum := sha256.Sum256([]byte(NormalizeCode(code)))
	return hex.EncodeToString(sum[:])
}

func newCode() (string, error) {
	buf := make([]byte, codeLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, codeLen)
	for i, b := range buf {
		out[i] = codeAlphabet[int(b)%len(codeAlphabet)]
	}
	return string(out[0:4]) + "-" + string(out[4:8]) + "-" + string(out[8:12]), nil
}

// Create issues an invite and returns it with the plaintext code (shown once).
func (s *Service) Create(ctx context.Context, orgID, createdBy string, in CreateInput) (*Invite, string, error) {
	switch in.Role {
	case "parent", "swimmer", "coach":
	default:
		return nil, "", ErrInvalidRole
	}
	if in.MaxUses <= 0 {
		in.MaxUses = 1
	}
	if in.MaxUses > maxUsesCeiling {
		return nil, "", fmt.Errorf("max_uses must be at most %d", maxUsesCeiling)
	}
	if in.TTL <= 0 {
		in.TTL = defaultTTL
	}
	if in.TTL > maxTTL {
		return nil, "", fmt.Errorf("ttl must be at most %d days", int(maxTTL.Hours()/24))
	}
	code, err := newCode()
	if err != nil {
		return nil, "", err
	}
	now := s.now().UTC()
	inv := &Invite{ID: uuid.NewString(), OrgID: orgID, Label: strings.TrimSpace(in.Label), Role: in.Role, MaxUses: in.MaxUses,
		ExpiresAt: now.Add(in.TTL), CreatedBy: createdBy, CreatedAt: now, codeHash: hashCode(code)}
	if s.pool == nil {
		s.mu.Lock()
		s.mem[inv.ID] = inv
		s.mu.Unlock()
		cp := *inv
		return &cp, code, nil
	}
	var by any
	if strings.TrimSpace(createdBy) != "" {
		by = createdBy
	}
	if _, err := s.at(orgID).Exec(ctx, `INSERT INTO org_invites (id, org_id, code_hash, label, role, max_uses, expires_at, created_by, created_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9)`,
		inv.ID, orgID, inv.codeHash, inv.Label, inv.Role, inv.MaxUses, inv.ExpiresAt, by, inv.CreatedAt); err != nil {
		return nil, "", fmt.Errorf("create invite: %w", err)
	}
	return inv, code, nil
}

// List returns the org's invites, newest first.
func (s *Service) List(ctx context.Context, orgID string) ([]*Invite, error) {
	if s.pool == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		out := []*Invite{}
		for _, inv := range s.mem {
			if inv.OrgID == orgID {
				cp := *inv
				out = append(out, &cp)
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
		return out, nil
	}
	rows, err := s.at(orgID).Query(ctx, `SELECT id::text, org_id::text, label, role, max_uses, uses, expires_at, COALESCE(created_by::text, ''), created_at, revoked_at
		FROM org_invites WHERE org_id = $1::uuid ORDER BY created_at DESC, id`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Invite{}
	for rows.Next() {
		i := &Invite{}
		if err := rows.Scan(&i.ID, &i.OrgID, &i.Label, &i.Role, &i.MaxUses, &i.Uses, &i.ExpiresAt, &i.CreatedBy, &i.CreatedAt, &i.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// Revoke disables an invite of this org.
func (s *Service) Revoke(ctx context.Context, orgID, id string) error {
	now := s.now().UTC()
	if s.pool == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		inv, ok := s.mem[id]
		if !ok || inv.OrgID != orgID {
			return ErrNotFound
		}
		if inv.RevokedAt == nil {
			inv.RevokedAt = &now
		}
		return nil
	}
	if _, err := uuid.Parse(id); err != nil {
		return ErrNotFound
	}
	tag, err := s.at(orgID).Exec(ctx, `UPDATE org_invites SET revoked_at = COALESCE(revoked_at, $3) WHERE id = $1::uuid AND org_id = $2::uuid`, id, orgID, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Redeem consumes one use of the code if it is valid for orgID. It is atomic:
// concurrent redemptions can never exceed max_uses.
func (s *Service) Redeem(ctx context.Context, orgID, code string) (*Invite, error) {
	h := hashCode(code)
	if NormalizeCode(code) == "" {
		return nil, ErrInvalidCode
	}
	now := s.now().UTC()
	if s.pool == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, inv := range s.mem {
			if inv.OrgID == orgID && inv.codeHash == h {
				if inv.RevokedAt != nil || !now.Before(inv.ExpiresAt) || inv.Uses >= inv.MaxUses {
					return nil, ErrInvalidCode
				}
				inv.Uses++
				cp := *inv
				return &cp, nil
			}
		}
		return nil, ErrInvalidCode
	}
	i := &Invite{}
	err := s.at(orgID).QueryRow(ctx, `UPDATE org_invites SET uses = uses + 1
		WHERE org_id = $1::uuid AND code_hash = $2 AND revoked_at IS NULL AND expires_at > $3 AND uses < max_uses
		RETURNING id::text, org_id::text, label, role, max_uses, uses, expires_at, COALESCE(created_by::text, ''), created_at, revoked_at`,
		orgID, h, now).Scan(&i.ID, &i.OrgID, &i.Label, &i.Role, &i.MaxUses, &i.Uses, &i.ExpiresAt, &i.CreatedBy, &i.CreatedAt, &i.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidCode
	}
	return i, err
}

// Release returns a use taken by Redeem (the sign-up it was for failed).
func (s *Service) Release(ctx context.Context, orgID, id string) error {
	if s.pool == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		if inv, ok := s.mem[id]; ok && inv.OrgID == orgID && inv.Uses > 0 {
			inv.Uses--
		}
		return nil
	}
	_, err := s.at(orgID).Exec(ctx, `UPDATE org_invites SET uses = GREATEST(uses - 1, 0) WHERE id = $1::uuid AND org_id = $2::uuid`, id, orgID)
	return err
}

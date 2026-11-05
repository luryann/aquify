// Package payments stores which Stripe connected account belongs to which club.
package payments

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/dbutil"
	"github.com/luryann/aquify/server/internal/tenantdb"
)

var (
	ErrNotFound         = errors.New("payment account not found")
	ErrAlreadyConnected = errors.New("organization already has a payment account")
	ErrAccountTaken     = errors.New("stripe account already belongs to another organization")
)

// Account is a club's Stripe connected account and its capability flags.
type Account struct {
	OrgID            string    `json:"org_id"`
	StripeAccountID  string    `json:"stripe_account_id"`
	ChargesEnabled   bool      `json:"charges_enabled"`
	PayoutsEnabled   bool      `json:"payouts_enabled"`
	DetailsSubmitted bool      `json:"details_submitted"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Service is Postgres-backed with a pool, in-memory without.
type Service struct {
	pool *pgxpool.Pool
	mu   sync.Mutex
	mem  map[string]*Account // by org id
}

func NewService() *Service { return &Service{mem: map[string]*Account{}} }

// at binds orgID to every statement so row-level security on org_payment_accounts applies.
func (s *Service) at(orgID string) tenantdb.Scoped { return tenantdb.Pool{P: s.pool}.For(orgID) }

// forAccount binds a Stripe account id for the webhook paths, where the club is not yet
// known (policies account_lookup / account_status expose only that account's row).
func (s *Service) forAccount(stripeAccountID string) tenantdb.Scoped {
	return tenantdb.Pool{P: s.pool}.ForLookup(tenantdb.LookupStripeAccount, stripeAccountID)
}

func NewServiceWithPool(pool *pgxpool.Pool) *Service {
	s := NewService()
	s.pool = pool
	return s
}

const cols = `org_id::text, stripe_account_id, charges_enabled, payouts_enabled, details_submitted, created_at, updated_at`

func scan(row pgx.Row) (*Account, error) {
	a := &Account{}
	if err := row.Scan(&a.OrgID, &a.StripeAccountID, &a.ChargesEnabled, &a.PayoutsEnabled, &a.DetailsSubmitted, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return nil, err
	}
	return a, nil
}

// Get returns the org's account or ErrNotFound.
func (s *Service) Get(ctx context.Context, orgID string) (*Account, error) {
	orgID = strings.TrimSpace(orgID)
	if s.pool == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		if a, ok := s.mem[orgID]; ok {
			cp := *a
			return &cp, nil
		}
		return nil, ErrNotFound
	}
	a, err := scan(s.at(orgID).QueryRow(ctx, `SELECT `+cols+` FROM org_payment_accounts WHERE org_id = $1::uuid`, orgID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

// ByStripeAccount maps a Stripe account id back to its org.
func (s *Service) ByStripeAccount(ctx context.Context, stripeAccountID string) (*Account, error) {
	stripeAccountID = strings.TrimSpace(stripeAccountID)
	if stripeAccountID == "" {
		return nil, ErrNotFound
	}
	if s.pool == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, a := range s.mem {
			if a.StripeAccountID == stripeAccountID {
				cp := *a
				return &cp, nil
			}
		}
		return nil, ErrNotFound
	}
	a, err := scan(s.forAccount(stripeAccountID).QueryRow(ctx, `SELECT `+cols+` FROM org_payment_accounts WHERE stripe_account_id = $1`, stripeAccountID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

// Create records the org's connected account (flags start false).
func (s *Service) Create(ctx context.Context, orgID, stripeAccountID string) (*Account, error) {
	orgID, stripeAccountID = strings.TrimSpace(orgID), strings.TrimSpace(stripeAccountID)
	if orgID == "" || stripeAccountID == "" {
		return nil, errors.New("org id and stripe account id are required")
	}
	now := time.Now().UTC()
	if s.pool == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		if _, ok := s.mem[orgID]; ok {
			return nil, ErrAlreadyConnected
		}
		for _, a := range s.mem {
			if a.StripeAccountID == stripeAccountID {
				return nil, ErrAccountTaken
			}
		}
		a := &Account{OrgID: orgID, StripeAccountID: stripeAccountID, CreatedAt: now, UpdatedAt: now}
		s.mem[orgID] = a
		cp := *a
		return &cp, nil
	}
	_, err := s.at(orgID).Exec(ctx, `INSERT INTO org_payment_accounts (org_id, stripe_account_id) VALUES ($1::uuid, $2)`, orgID, stripeAccountID)
	if dbutil.IsUniqueViolation(err, "org_payment_accounts_account_uidx") {
		return nil, ErrAccountTaken
	}
	if dbutil.IsUniqueViolation(err, "") {
		return nil, ErrAlreadyConnected
	}
	if err != nil {
		return nil, fmt.Errorf("create payment account: %w", err)
	}
	return s.Get(ctx, orgID)
}

// UpdateStatus refreshes the capability flags of a connected account.
func (s *Service) UpdateStatus(ctx context.Context, stripeAccountID string, charges, payouts, details bool) (*Account, error) {
	if s.pool == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, a := range s.mem {
			if a.StripeAccountID == stripeAccountID {
				a.ChargesEnabled, a.PayoutsEnabled, a.DetailsSubmitted, a.UpdatedAt = charges, payouts, details, time.Now().UTC()
				cp := *a
				return &cp, nil
			}
		}
		return nil, ErrNotFound
	}
	a, err := scan(s.forAccount(stripeAccountID).QueryRow(ctx, `UPDATE org_payment_accounts SET charges_enabled = $2, payouts_enabled = $3, details_submitted = $4
		WHERE stripe_account_id = $1 RETURNING `+cols, stripeAccountID, charges, payouts, details))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

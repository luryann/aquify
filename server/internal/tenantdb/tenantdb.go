// Package tenantdb runs SQL with the tenant bound to the database session, so
// Postgres row-level security (policy: org_id = app.org_id) can act as a second
// line of defence behind the application's own org filters.
//
// Every statement runs in its own short transaction that first executes
// set_config('app.org_id', <org>, true) (transaction-local, so the setting can
// never leak to another request on a pooled connection). An empty org runs the
// statement unbound: under RLS that sees nothing, which is the safe failure.
package tenantdb

import (
	"context"
	"errors"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Setting is the session variable RLS policies read.
const Setting = "app.org_id"

// WriterSetting is the session variable naming the process that issued a statement. The
// change-counter triggers (org_data_versions) record it, so a cache can tell its own
// write-through changes from another instance's writes.
const WriterSetting = "app.writer"

// Pool binds orgs to statements on an underlying pool. Writer, when set, identifies this
// process to the change-counter triggers.
type Pool struct {
	P      *pgxpool.Pool
	Writer string
}

// For returns a handle whose statements run bound to orgID.
func (p Pool) For(orgID string) Scoped {
	return Scoped{pool: p.P, setting: Setting, value: orgID, writer: p.Writer}
}

// Lookup names a session setting that a pre-auth policy reads. Each one has a matching
// narrow policy in the migrations (the setting name here and in the SQL must agree).
type Lookup string

const (
	LookupFeedTokenHash Lookup = "app.feed_token_hash" // calendar_feed_tokens.feed_token_lookup
	LookupSessionHash   Lookup = "app.session_hash"    // sessions.session_lookup
	LookupPublicSlug    Lookup = "app.public_slug"     // org_settings.slug_lookup
	LookupDomain        Lookup = "app.domain"          // org_domains.domain_lookup
	LookupStripeAccount Lookup = "app.stripe_account"  // org_payment_accounts.account_lookup/account_status
	LookupKeycloakUser  Lookup = "app.keycloak_user"   // households/swimmers.keycloak_lookup
	LookupWebhookEvent  Lookup = "app.webhook_event"   // webhook_events.webhook_event
)

// ForLookup returns a handle bound to a credential or identifier instead of an org, for
// the few lookups made before any org is known (a session token's hash, a public slug, a
// verified domain, ...). The matching policy exposes only the row(s) that value identifies.
func (p Pool) ForLookup(key Lookup, value string) Scoped {
	return Scoped{pool: p.P, setting: string(key), value: value, writer: p.Writer}
}

// ForeignWrites returns how many change-counter bumps other writers (every writer except
// this pool's Writer) have committed for one org and domain. A cache whose own writes are
// write-through can skip a reload while this number is unchanged since it last loaded.
func (p Pool) ForeignWrites(ctx context.Context, orgID, domain string) (int64, error) {
	var n int64
	err := p.For(orgID).QueryRow(ctx, `SELECT COALESCE(SUM(version), 0)::bigint FROM org_data_versions
		WHERE org_id = $1::uuid AND domain = $2 AND writer <> $3`, orgID, domain, p.Writer).Scan(&n)
	return n, err
}

// Scoped is a pool handle bound to one org. It satisfies the subset of pgx's
// query API the repositories use (Exec, Query, QueryRow, Begin).
type Scoped struct {
	pool    *pgxpool.Pool
	setting string
	value   string
	writer  string
}

// bindSQL sets the binding and the writer id in one statement (an empty writer clears it).
const bindSQL = `SELECT set_config($1, $2, true), set_config($3, $4, true)`

func (s Scoped) bindArgs() []any { return []any{s.setting, s.value, WriterSetting, s.writer} }

func (s Scoped) begin(ctx context.Context) (pgx.Tx, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, bindSQL, s.bindArgs()...); err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}

// Begin starts a transaction bound to the org.
func (s Scoped) Begin(ctx context.Context) (pgx.Tx, error) { return s.begin(ctx) }

// batch sends set_config and the real statement together. A pgx batch is one
// implicit transaction, so the transaction-local binding applies to the statement
// and both cost a single round trip (no explicit BEGIN/COMMIT).
func (s Scoped) batch(ctx context.Context, sql string, args []any) (pgx.BatchResults, *pgxpool.Conn, error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, nil, err
	}
	b := &pgx.Batch{}
	b.Queue(bindSQL, s.bindArgs()...)
	b.Queue(sql, args...)
	br := conn.SendBatch(ctx, b)
	if _, err := br.Exec(); err != nil { // the set_config result
		_ = br.Close()
		conn.Release()
		return nil, nil, err
	}
	return br, conn, nil
}

// Exec runs one statement bound to the org.
func (s Scoped) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	br, conn, err := s.batch(ctx, sql, args)
	if err != nil {
		return pgconn.CommandTag{}, err
	}
	defer conn.Release()
	tag, err := br.Exec()
	if cerr := br.Close(); err == nil {
		err = cerr
	}
	return tag, err
}

// Query runs one query bound to the org. The connection is held until the rows are closed.
func (s Scoped) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	br, conn, err := s.batch(ctx, sql, args)
	if err != nil {
		return nil, err
	}
	rows, err := br.Query()
	if err != nil {
		_ = br.Close()
		conn.Release()
		return nil, err
	}
	return &scopedRows{Rows: rows, br: br, conn: conn}, nil
}

// QueryRow runs a single-row query bound to the org.
func (s Scoped) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	rows, err := s.Query(ctx, sql, args...)
	if err != nil {
		return errRow{err}
	}
	return &scopedRow{rows: rows}
}

type scopedRows struct {
	pgx.Rows
	br   pgx.BatchResults
	conn *pgxpool.Conn
	once sync.Once
}

func (r *scopedRows) Close() {
	r.once.Do(func() {
		r.Rows.Close()
		_ = r.br.Close()
		r.conn.Release()
	})
}

type scopedRow struct{ rows pgx.Rows }

func (r *scopedRow) Scan(dest ...any) error {
	defer r.rows.Close()
	if !r.rows.Next() {
		if err := r.rows.Err(); err != nil {
			return err
		}
		return pgx.ErrNoRows
	}
	return r.rows.Scan(dest...)
}

type errRow struct{ err error }

func (e errRow) Scan(...any) error { return e.err }

// WithOrgTx runs fn inside one transaction bound to orgID.
func WithOrgTx(ctx context.Context, pool *pgxpool.Pool, orgID string, fn func(tx pgx.Tx) error) error {
	if orgID == "" {
		return errors.New("tenantdb: org id is required")
	}
	tx, err := Pool{P: pool}.For(orgID).Begin(ctx)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

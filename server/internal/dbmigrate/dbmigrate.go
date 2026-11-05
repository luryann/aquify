// Package dbmigrate applies the goose SQL migrations from inside the API
// binary, so a deployment needs nothing but the release image to initialise or
// upgrade its database (the runtime image is distroless: no goose, no psql).
//
// The SQL files are embedded from ./sql. That directory is populated at image
// build time from the repository's db/migrations (see server/Dockerfile and
// `make embed-migrations`); db/migrations remains the single source of truth and
// nothing under ./sql except .gitkeep is committed. An empty embed is a hard
// error, never a silent no-op.
package dbmigrate

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

//go:embed all:sql
var embedded embed.FS

// ErrNoMigrations means the binary was built without the SQL files.
var ErrNoMigrations = errors.New("dbmigrate: no migrations embedded; build the image from the repository root so db/migrations is copied into server/internal/dbmigrate/sql")

// Embedded returns the migrations compiled into the binary.
func Embedded() fs.FS {
	sub, err := fs.Sub(embedded, "sql")
	if err != nil { // cannot happen: "sql" is a literal embedded directory
		panic(err)
	}
	return sub
}

// Applied describes one migration touched by Up.
type Applied struct {
	Version int64
	Source  string
}

// State is one migration's status.
type State struct {
	Version int64
	Source  string
	Applied bool
}

func open(ctx context.Context, dsn string, fsys fs.FS) (*goose.Provider, *sql.DB, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, nil, errors.New("dbmigrate: database URL is empty")
	}
	if !hasSQL(fsys) {
		return nil, nil, ErrNoMigrations
	}
	cfg, err := parseConfig(dsn)
	if err != nil {
		return nil, nil, err
	}
	db := stdlib.OpenDB(*cfg)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("dbmigrate: connect: %w", scrub(err, dsn))
	}
	// A session-level advisory lock serialises concurrent runners (two
	// containers, or a retry racing a slow first run).
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		_ = db.Close()
		return nil, nil, err
	}
	p, err := goose.NewProvider(goose.DialectPostgres, db, fsys, goose.WithSessionLocker(locker))
	if err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("dbmigrate: %w", err)
	}
	return p, db, nil
}

func hasSQL(fsys fs.FS) bool {
	matches, err := fs.Glob(fsys, "*.sql")
	return err == nil && len(matches) > 0
}

// Up applies every pending migration in order. It returns the migrations it
// applied; a failing migration stops the run and is not recorded as applied.
func Up(ctx context.Context, dsn string, fsys fs.FS) ([]Applied, error) {
	p, db, err := open(ctx, dsn, fsys)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	results, err := p.Up(ctx)
	out := make([]Applied, 0, len(results))
	for _, r := range results {
		if r.Error == nil {
			out = append(out, Applied{Version: r.Source.Version, Source: r.Source.Path})
		}
	}
	if err != nil {
		// A mid-run failure reports the earlier successes inside the error.
		var partial *goose.PartialError
		if errors.As(err, &partial) {
			for _, r := range partial.Applied {
				out = append(out, Applied{Version: r.Source.Version, Source: r.Source.Path})
			}
		}
		return out, fmt.Errorf("dbmigrate: up: %w", scrub(err, dsn))
	}
	return out, nil
}

// Status reports every known migration and whether it is applied.
func Status(ctx context.Context, dsn string, fsys fs.FS) ([]State, error) {
	p, db, err := open(ctx, dsn, fsys)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := p.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("dbmigrate: status: %w", scrub(err, dsn))
	}
	out := make([]State, 0, len(rows))
	for _, r := range rows {
		out = append(out, State{Version: r.Source.Version, Source: r.Source.Path, Applied: r.State == goose.StateApplied})
	}
	return out, nil
}

// scrub removes the connection string (which carries the password) from an
// error before it is logged.
func scrub(err error, dsn string) error {
	if err == nil || dsn == "" {
		return err
	}
	msg := strings.ReplaceAll(err.Error(), dsn, "[redacted-dsn]")
	if pw := dsnPassword(dsn); pw != "" {
		msg = strings.ReplaceAll(msg, pw, "[redacted]")
	}
	if msg == err.Error() {
		return err
	}
	return errors.New(msg)
}

var errParse = errors.New("dbmigrate: database URL is not a valid postgres connection string")

package dbutil

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// CollectRows reads every row with scan, closes rows, and returns the results.
// A scan failure is wrapped as "<scanContext>: <err>"; a nil slice means no rows.
func CollectRows[T any](rows pgx.Rows, scanContext string, scan func(pgx.Rows) (T, error)) ([]T, error) {
	defer rows.Close()
	var out []T
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", scanContext, err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// CollectRowsNonNil is CollectRows for callers that must return an empty,
// non-nil slice (for example to encode as [] rather than null).
func CollectRowsNonNil[T any](rows pgx.Rows, scanContext string, scan func(pgx.Rows) (T, error)) ([]T, error) {
	out, err := CollectRows(rows, scanContext, scan)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []T{}
	}
	return out, nil
}

// ListOrgIDs returns every registered club id. The orgs registry is not tenant data, so
// this needs no binding; all-club loads use it to read each club under its own binding.
func ListOrgIDs(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT id::text FROM orgs ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list orgs: %w", err)
	}
	return CollectRows(rows, "scan org", func(rows pgx.Rows) (string, error) {
		var id string
		return id, rows.Scan(&id)
	})
}

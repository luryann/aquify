package db

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

var expectedColumns = map[string][]string{
	"announcements":          {"deleted_at"},
	"data_deletion_requests": {"deleted_at", "expires_at"},
	"data_export_requests":   {"deleted_at"},
	"email_campaigns":        {"deleted_at"},
	"entries":                {"entry_time_ms", "notes"},
	"eligibility_rules":      {"rule_value"},
	"file_exports":           {"expires_at", "filename", "last_error", "size_bytes"},
	"meets":                  {"venue"},
	"meet_sessions":          {"warm_up_time"},
	"payments":               {"household_id", "payment_method"},
	"waiver_acceptances":     {"accepted_by"},
}

// MissingExpectedColumns compares the discovered DB schema to the set of
// repository-required columns and returns a stable, sorted list of missing
// table.column references.
func MissingExpectedColumns(actual map[string]map[string]bool) []string {
	var missing []string
	for table, columns := range expectedColumns {
		tableColumns := actual[table]
		for _, column := range columns {
			if tableColumns == nil || !tableColumns[column] {
				missing = append(missing, fmt.Sprintf("%s.%s", table, column))
			}
		}
	}
	sort.Strings(missing)
	return missing
}

// ValidateExpectedSchema checks that columns referenced by DB-backed
// repositories exist in the connected database.
func ValidateExpectedSchema(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return nil
	}

	rows, err := pool.Query(ctx, `
		SELECT table_name, column_name
		FROM information_schema.columns
		WHERE table_schema = ANY(current_schemas(false))`)
	if err != nil {
		return fmt.Errorf("schema validation query failed: %w", err)
	}
	defer rows.Close()

	actual := make(map[string]map[string]bool)
	for rows.Next() {
		var tableName, columnName string
		if err := rows.Scan(&tableName, &columnName); err != nil {
			return fmt.Errorf("schema validation scan failed: %w", err)
		}
		if actual[tableName] == nil {
			actual[tableName] = make(map[string]bool)
		}
		actual[tableName][columnName] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("schema validation iteration failed: %w", err)
	}

	missing := MissingExpectedColumns(actual)
	if len(missing) == 0 {
		return nil
	}

	return fmt.Errorf(
		"schema drift detected; missing expected columns: %s. Run the latest DB migrations before starting the API",
		strings.Join(missing, ", "),
	)
}

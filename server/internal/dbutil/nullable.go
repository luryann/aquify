// Package dbutil holds small helpers shared by the DB-backed repositories.
package dbutil

// NullableString maps the empty string to SQL NULL and passes other values
// through unchanged, for use as a query argument.
func NullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

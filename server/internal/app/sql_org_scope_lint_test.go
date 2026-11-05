package app

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Tenancy tracker item 0.4. Scans production Go sources for SQL string literals
// that read or mutate a tenant table by primary key without an org_id predicate.
// A new offender fails the test; an intentional exception must be added to
// sqlOrgScopeAllowlist with a reason.
//
// Heuristic, not a parser: it looks at backtick-quoted literals only, and only
// flags `WHERE id = $n` / `AND id = $n` forms. It exists to stop regressions of
// the pattern fixed in item 4.1, not to prove isolation (RLS, item 4.4, does).
var sqlOrgScopeAllowlist = map[string]string{
	"compliance/repository.go|FROM data_export_requests WHERE id = $1":   "by-ID cache sync; every service read re-checks org",
	"compliance/repository.go|FROM data_deletion_requests WHERE id = $1": "by-ID cache sync; every service read re-checks org",
	"members/repository.go|UPDATE households SET deleted_at":             "soft delete after the service verified org; revisit under RLS",
	"members/repository.go|UPDATE swimmers":                              "soft delete after the service verified org; revisit under RLS",
	"members/repository.go|FROM households WHERE id":                     "reviewed: caller holds verified record",
	"members/repository.go|FROM swimmers WHERE id":                       "reviewed: caller holds verified record",
	"authn/service.go|UPDATE mfa_factors":                                "user-scoped secret rows",
	"authn/service.go|FROM mfa_factors":                                  "user-scoped secret rows",
}

var (
	sqlLiteralRe = regexp.MustCompile("(?s)`([^`]*)`")
	sqlByIDRe    = regexp.MustCompile(`(?is)\b(WHERE|AND)\s+(\w+\.)?id\s*=\s*\$\d`)
	sqlVerbRe    = regexp.MustCompile(`(?is)\b(UPDATE\s+(\w+)|DELETE\s+FROM\s+(\w+)|FROM\s+(\w+))`)
	createTblRe  = regexp.MustCompile(`(?is)CREATE TABLE(?: IF NOT EXISTS)?\s+(\w+)\s*\((.*?)\n\);`)
)

func tenantTablesFromMigrations(t *testing.T) map[string]bool {
	t.Helper()
	paths, err := filepath.Glob("../../../db/migrations/*.sql")
	if err != nil || len(paths) == 0 {
		t.Fatalf("migrations not found: %v", err)
	}
	tables := map[string]bool{}
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		up := strings.SplitN(string(raw), "-- +goose Down", 2)[0]
		for _, m := range createTblRe.FindAllStringSubmatch(up, -1) {
			if strings.Contains(m[2], "org_id") {
				tables[strings.ToLower(m[1])] = true
			}
		}
	}
	return tables
}

func TestSQLByIDStatementsAreOrgScoped(t *testing.T) {
	tenant := tenantTablesFromMigrations(t)
	if len(tenant) < 50 {
		t.Fatalf("only %d tenant tables found; migration parsing is broken", len(tenant))
	}
	var offenders []string
	used := map[string]bool{}
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, lit := range sqlLiteralRe.FindAllStringSubmatch(string(src), -1) {
			sql := lit[1]
			if !sqlByIDRe.MatchString(sql) || strings.Contains(strings.ToLower(sql), "org_id") {
				continue
			}
			for _, v := range sqlVerbRe.FindAllStringSubmatch(sql, -1) {
				table := strings.ToLower(v[2] + v[3] + v[4])
				if !tenant[table] {
					continue
				}
				oneLine := strings.Join(strings.Fields(sql), " ")
				allowed := false
				for key := range sqlOrgScopeAllowlist {
					parts := strings.SplitN(key, "|", 2)
					if strings.HasSuffix(filepath.ToSlash(path), parts[0]) && strings.Contains(oneLine, parts[1]) {
						allowed, used[key] = true, true
					}
				}
				if !allowed {
					snippet := oneLine
					if len(snippet) > 110 {
						snippet = snippet[:110]
					}
					offenders = append(offenders, filepath.ToSlash(path)+": "+table+": "+snippet)
				}
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(offenders)
	if len(offenders) > 0 {
		t.Fatalf("SQL on tenant tables selects/mutates by id without org_id (add `AND org_id = $n`, or allowlist with a reason):\n  %s", strings.Join(offenders, "\n  "))
	}
}

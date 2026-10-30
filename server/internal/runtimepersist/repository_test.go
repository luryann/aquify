package runtimepersist

import (
	"sort"
	"strings"
	"testing"
)

func TestPsqlEnvFromURLKeepsPasswordOutOfArgv(t *testing.T) {
	env, ok := psqlEnvFromURL("postgres://app:p%40ss%2Fw@db.internal:6432/aquify?sslmode=require")
	if !ok {
		t.Fatal("url should convert")
	}
	sort.Strings(env)
	got := strings.Join(env, " ")
	for _, want := range []string{"PGHOST=db.internal", "PGPORT=6432", "PGUSER=app", "PGPASSWORD=p@ss/w", "PGDATABASE=aquify", "PGSSLMODE=require"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}

func TestPsqlEnvFromURLFallsBackForUnsupportedInput(t *testing.T) {
	for _, dsn := range []string{"host=localhost dbname=x", "postgres://u@h/db?unknown_param=1", "mysql://u@h/db"} {
		if _, ok := psqlEnvFromURL(dsn); ok {
			t.Errorf("%q should fall back to -d", dsn)
		}
	}
}

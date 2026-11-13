package runtimepersist

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// cov1xFakePsql installs a fake psql executable on PATH (the true external
// boundary). It appends each call's argv/stdin/PG env to files and replies
// with the configured stdout and exit status.
type cov1xFakePsql struct {
	dir string
}

func cov1xInstallPsql(t *testing.T, stdout string, exitCode int) *cov1xFakePsql {
	t.Helper()
	dir := t.TempDir()
	f := &cov1xFakePsql{dir: dir}
	script := "#!/bin/sh\n" +
		"D='" + dir + "'\n" +
		"printf '%s\\n' \"$@\" >> \"$D/args\"\n" +
		"cat >> \"$D/stdin\"\n" +
		"printf '\\n----\\n' >> \"$D/stdin\"\n" +
		"env | grep '^PG' >> \"$D/env\"\n" +
		"cat \"$D/out\"\n" +
		"exit " + string(rune('0'+exitCode)) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "psql"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "out"), []byte(stdout), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	return f
}

func (f *cov1xFakePsql) read(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(f.dir, name))
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatal(err)
	}
	return string(b)
}

func TestCov1xNewRepositoryTrimsAndEnabled(t *testing.T) {
	r := NewRepository("  postgres://u@h/db \n")
	if r.DatabaseURL != "postgres://u@h/db" {
		t.Fatalf("url not trimmed: %q", r.DatabaseURL)
	}
	if !r.Enabled() {
		t.Fatal("repo with url should be enabled")
	}
	if NewRepository("   ").Enabled() {
		t.Fatal("blank url must be disabled")
	}
	var nilRepo *Repository
	if nilRepo.Enabled() {
		t.Fatal("nil repo must be disabled")
	}
}

func TestCov1xAvailableRequiresPsqlOnPath(t *testing.T) {
	var nilRepo *Repository
	if nilRepo.Available() {
		t.Fatal("nil repo cannot be available")
	}
	t.Setenv("PATH", t.TempDir())
	if NewRepository("postgres://u@h/db").Available() {
		t.Fatal("no psql on PATH must be unavailable")
	}
	cov1xInstallPsql(t, "", 0)
	if !NewRepository("postgres://u@h/db").Available() {
		t.Fatal("psql on PATH must be available")
	}
	if NewRepository("").Available() {
		t.Fatal("disabled repo is unavailable even with psql")
	}
}

func TestCov1xNoPsqlIsNoop(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	r := NewRepository("postgres://u@h/db")
	if err := r.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	got, err := r.LoadAll()
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("LoadAll = %v, %v; want empty map", got, err)
	}
	if err := r.SaveAll(map[string]json.RawMessage{"a": json.RawMessage(`{}`)}); err != nil {
		t.Fatalf("SaveAll: %v", err)
	}
}

func TestCov1xEnsureSchemaRunsCreateTableWithEnv(t *testing.T) {
	f := cov1xInstallPsql(t, "", 0)
	r := NewRepository("postgres://app:s3cret@db.internal:6432/aquify?sslmode=require")
	if err := r.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.read(t, "stdin"), "CREATE TABLE IF NOT EXISTS runtime_snapshots") {
		t.Fatalf("stdin missing DDL: %q", f.read(t, "stdin"))
	}
	args := f.read(t, "args")
	if strings.Contains(args, "s3cret") {
		t.Fatalf("password leaked into argv: %q", args)
	}
	if !strings.Contains(args, "ON_ERROR_STOP=1") || strings.Contains(args, "-d\n") {
		t.Fatalf("unexpected argv: %q", args)
	}
	env := f.read(t, "env")
	for _, want := range []string{"PGHOST=db.internal", "PGPORT=6432", "PGUSER=app", "PGPASSWORD=s3cret", "PGDATABASE=aquify", "PGSSLMODE=require"} {
		if !strings.Contains(env, want) {
			t.Errorf("env missing %q: %q", want, env)
		}
	}
}

func TestCov1xRunSQLFallsBackToDashDForNonURL(t *testing.T) {
	f := cov1xInstallPsql(t, "hello\n", 0)
	t.Setenv("PGHOST", "ambient-host")
	r := NewRepository("host=localhost dbname=x")
	out, err := r.runSQL("SELECT 1;")
	if err != nil || out != "hello" {
		t.Fatalf("runSQL = %q, %v", out, err)
	}
	args := f.read(t, "args")
	if !strings.HasPrefix(args, "-d\nhost=localhost dbname=x\n") {
		t.Fatalf("expected -d fallback, got %q", args)
	}
	if strings.Contains(f.read(t, "env"), "PGHOST=localhost") || !strings.Contains(f.read(t, "env"), "PGHOST=ambient-host") {
		t.Fatal("fallback must not set PG env from DSN")
	}
}

func TestCov1xRunSQLReportsFailureOutput(t *testing.T) {
	cov1xInstallPsql(t, "ERROR: boom\n", 3)
	r := NewRepository("postgres://u@h/db")
	out, err := r.runSQL("SELECT 1;")
	if err == nil || out != "" {
		t.Fatalf("want error, got %q, %v", out, err)
	}
	if !strings.Contains(err.Error(), "runtime snapshot psql failed") || !strings.Contains(err.Error(), "ERROR: boom") {
		t.Fatalf("error lacks context: %v", err)
	}
	if e := r.EnsureSchema(); e == nil {
		t.Fatal("EnsureSchema must propagate failure")
	}
	if _, e := r.LoadAll(); e == nil {
		t.Fatal("LoadAll must propagate EnsureSchema failure")
	}
	if e := r.SaveAll(map[string]json.RawMessage{"a": json.RawMessage(`{}`)}); e == nil {
		t.Fatal("SaveAll must propagate EnsureSchema failure")
	}
}

func cov1xRow(domain, payload string) string {
	return domain + "\t" + hex.EncodeToString([]byte(payload))
}

func TestCov1xLoadAllParsesRows(t *testing.T) {
	out := strings.Join([]string{
		cov1xRow("members", `{"a":1}`),
		"",
		"  " + cov1xRow("billing", `{"b":"x\ty"}`) + "  ",
	}, "\n") + "\n"
	f := cov1xInstallPsql(t, out, 0)
	got, err := NewRepository("postgres://u@h/db").LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]json.RawMessage{
		"members": json.RawMessage(`{"a":1}`),
		"billing": json.RawMessage(`{"b":"x\ty"}`),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if !strings.Contains(f.read(t, "stdin"), "SELECT domain") {
		t.Fatal("select query not issued")
	}
}

func TestCov1xLoadAllRejectsMalformedRows(t *testing.T) {
	cov1xInstallPsql(t, "no-tab-here\n", 0)
	_, err := NewRepository("postgres://u@h/db").LoadAll()
	if err == nil || !strings.Contains(err.Error(), `invalid runtime snapshot row "no-tab-here"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestCov1xLoadAllRejectsBadHex(t *testing.T) {
	cov1xInstallPsql(t, "members\tzz\n", 0)
	_, err := NewRepository("postgres://u@h/db").LoadAll()
	if err == nil || !strings.Contains(err.Error(), "decode runtime snapshot members") {
		t.Fatalf("err = %v", err)
	}
}

func TestCov1xSaveAllEmptyDoesNotWrite(t *testing.T) {
	f := cov1xInstallPsql(t, "", 0)
	if err := NewRepository("postgres://u@h/db").SaveAll(nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(f.read(t, "stdin"), "INSERT") {
		t.Fatal("no insert expected for empty snapshots")
	}
}

func TestCov1xSaveAllBuildsEscapedUpsertTransaction(t *testing.T) {
	f := cov1xInstallPsql(t, "", 0)
	payload := json.RawMessage(`{"name":"O'Brien"}`)
	err := NewRepository("postgres://u@h/db").SaveAll(map[string]json.RawMessage{"it's": payload})
	if err != nil {
		t.Fatal(err)
	}
	stdin := f.read(t, "stdin")
	enc := base64.StdEncoding.EncodeToString(payload)
	for _, want := range []string{"BEGIN;", "COMMIT;", "VALUES ('it''s'", "decode('" + enc + "', 'base64')", "ON CONFLICT(domain) DO UPDATE"} {
		if !strings.Contains(stdin, want) {
			t.Errorf("stdin missing %q:\n%s", want, stdin)
		}
	}
	if strings.Contains(stdin, "O'Brien") {
		t.Fatal("payload must travel base64-encoded, not raw")
	}
}

func TestCov1xEscapeLiteral(t *testing.T) {
	if got := escapeLiteral("a'b''c"); got != "a''b''''c" {
		t.Fatalf("got %q", got)
	}
}

func TestCov1xLoadAllPropagatesSelectFailure(t *testing.T) {
	f := cov1xInstallPsql(t, "", 0)
	// Succeed for the schema DDL, fail for the SELECT.
	script := "#!/bin/sh\n" +
		"in=$(cat)\n" +
		"case \"$in\" in *'SELECT domain'*) echo 'ERROR: select failed'; exit 2;; esac\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(f.dir, "psql"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := NewRepository("postgres://u@h/db").LoadAll()
	if err == nil || !strings.Contains(err.Error(), "select failed") || got != nil {
		t.Fatalf("got %v, %v", got, err)
	}
}

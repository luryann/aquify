package app

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/members"
	"github.com/dareaquatics/aquify/server/internal/runtimepersist"
	"github.com/dareaquatics/aquify/server/internal/testutil"
)

// The runtime-snapshot bridge is a backward-compat, best-effort mirror of state;
// the DB pool is canonical. A snapshot save failure must NOT turn an
// already-committed mutation into a 500 — the successful response is returned
// and the failure is logged. (Previously this returned 500, which broke writes
// on Windows once the snapshot exceeded the command-line length limit.)
func TestRuntimePersistenceMiddlewareReturnsSuccessWhenSnapshotSaveFails(t *testing.T) {
	t.Setenv("PATH", prependFakePSQL(t, fakePSQLBehavior{
		failOnBegin: true,
	}))

	repo := runtimepersist.NewRepository("postgres://runtime-test")
	participants := []runtimeParticipant{{
		domain:  "members",
		service: snapshotStub{payload: []byte(`{"ok":true}`)},
	}}

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	handler := runtimePersistenceMiddleware(logger, repo, participants)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test", "buffered")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"created":true}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/members/households", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected committed mutation to still return 201 despite snapshot failure, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"created":true`) {
		t.Fatalf("expected successful response body to be flushed, got %s", rec.Body.String())
	}
	if got := rec.Header().Get("X-Test"); got != "buffered" {
		t.Fatalf("expected buffered success headers to be flushed, got %q", got)
	}
	if strings.Contains(rec.Body.String(), "runtime_persistence_failed") {
		t.Fatalf("snapshot failure should not surface as an error payload, got %s", rec.Body.String())
	}
	if !strings.Contains(logs.String(), "runtime snapshot persistence failed") {
		t.Fatalf("expected snapshot failure to be logged, logs=%s", logs.String())
	}
}

func TestLoadRuntimeSnapshotsKeepsExistingStateWhenSnapshotPayloadIsInvalid(t *testing.T) {
	t.Setenv("PATH", prependFakePSQL(t, fakePSQLBehavior{
		selectOutput: "members\t6e6f742d6a736f6e",
	}))

	svc := members.NewService()
	parent := testutil.ParentPrincipal("parent-runtime", "")
	if _, err := svc.CreateHousehold(parent, members.CreateHouseholdInput{
		OrgID:         testutil.DefaultOrgID,
		HouseholdName: "Existing Household",
		PrimaryEmail:  "existing@example.com",
	}); err != nil {
		t.Fatalf("seed household: %v", err)
	}

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	repo := runtimepersist.NewRepository("postgres://runtime-test")

	loadRuntimeSnapshots(logger, repo, []runtimeParticipant{{
		domain:  "members",
		service: svc,
	}})

	if got := len(svc.SnapshotHouseholds(testutil.DefaultOrgID)); got != 1 {
		t.Fatalf("expected invalid snapshot hydrate to preserve existing state, got %d households", got)
	}
	if !strings.Contains(logs.String(), "runtime snapshot hydrate failed") {
		t.Fatalf("expected hydrate failure to be logged, logs=%s", logs.String())
	}
}

type snapshotStub struct {
	payload []byte
}

func (s snapshotStub) ExportRuntimeSnapshot() ([]byte, error) {
	return append([]byte{}, s.payload...), nil
}

func (snapshotStub) LoadRuntimeSnapshot([]byte) error {
	return nil
}

type fakePSQLBehavior struct {
	selectOutput string
	failOnBegin  bool
}

func prependFakePSQL(t *testing.T, behavior fakePSQLBehavior) string {
	t.Helper()

	dir := t.TempDir()

	if runtime.GOOS == "windows" {
		// runSQL now feeds SQL via stdin (`psql -f -`), not the -c argument, so
		// the fake reads the first stdin line (BEGIN;/SELECT domain/CREATE TABLE
		// each begin their statement on their own line) to decide behavior.
		// Capture stdin to a temp file, then findstr the file. Searching the SQL
		// via a file (not by echoing/piping %sql%) avoids cmd mangling BEGIN;/
		// SELECT lines that contain shell metacharacters like `||` and `|`.
		path := filepath.Join(dir, "psql.cmd")
		script := "@echo off\r\n" +
			"set \"tmp=%TEMP%\\fakepsql_%RANDOM%%RANDOM%.sql\"\r\n" +
			"findstr \"^\" > \"%tmp%\"\r\n"
		if behavior.failOnBegin {
			script += "findstr /C:\"BEGIN;\" \"%tmp%\" >nul\r\n" +
				"if not errorlevel 1 (\r\n" +
				"  del \"%tmp%\"\r\n" +
				"  echo simulated save failure 1>&2\r\n" +
				"  exit /b 1\r\n" +
				")\r\n"
		}
		if strings.TrimSpace(behavior.selectOutput) != "" {
			script += "findstr /C:\"SELECT domain\" \"%tmp%\" >nul\r\n" +
				"if not errorlevel 1 (\r\n" +
				"  del \"%tmp%\"\r\n" +
				fmt.Sprintf("  echo %s\r\n", behavior.selectOutput) +
				"  exit /b 0\r\n" +
				")\r\n"
		}
		script += "del \"%tmp%\"\r\n" +
			"exit /b 0\r\n"
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatalf("write fake psql: %v", err)
		}
	} else {
		path := filepath.Join(dir, "psql")
		var script strings.Builder
		script.WriteString("#!/bin/sh\n")
		// runSQL feeds SQL via stdin (`psql -f -`), not -c; read it all.
		script.WriteString("sql=\"$(cat)\"\n")
		if behavior.failOnBegin {
			script.WriteString("case \"$sql\" in\n")
			script.WriteString("  *\"BEGIN;\"*)\n")
			script.WriteString("    echo \"simulated save failure\" >&2\n")
			script.WriteString("    exit 1\n")
			script.WriteString("    ;;\n")
			script.WriteString("esac\n")
		}
		if strings.TrimSpace(behavior.selectOutput) != "" {
			script.WriteString("case \"$sql\" in\n")
			script.WriteString("  *\"SELECT domain\"*)\n")
			script.WriteString(fmt.Sprintf("    printf '%s\\n'\n", strings.ReplaceAll(behavior.selectOutput, "'", "'\\''")))
			script.WriteString("    exit 0\n")
			script.WriteString("    ;;\n")
			script.WriteString("esac\n")
		}
		script.WriteString("exit 0\n")
		if err := os.WriteFile(path, []byte(script.String()), 0o755); err != nil {
			t.Fatalf("write fake psql: %v", err)
		}
	}

	current := os.Getenv("PATH")
	if current == "" {
		return dir
	}
	return fmt.Sprintf("%s%c%s", dir, os.PathListSeparator, current)
}

type exportCounter struct{ exports *int }

func (c exportCounter) ExportRuntimeSnapshot() ([]byte, error) {
	*c.exports++
	return []byte(`{}`), nil
}
func (exportCounter) LoadRuntimeSnapshot([]byte) error { return nil }

func TestRuntimePersistenceSnapshotsOnlyTouchedDomainAndSkipsNonSnapshotRoutes(t *testing.T) {
	t.Setenv("PATH", prependFakePSQL(t, fakePSQLBehavior{}))
	repo := runtimepersist.NewRepository("postgres://runtime-test")
	var members, comms int
	participants := []runtimeParticipant{
		{domain: "members", service: exportCounter{&members}},
		{domain: "comms", service: exportCounter{&comms}},
	}
	handler := runtimePersistenceMiddleware(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), repo, participants)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	do := func(path string) {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, path, nil))
	}

	do("/api/v1/announcements")
	if comms != 1 || members != 0 {
		t.Fatalf("announcement mutation exported members=%d comms=%d, want only comms", members, comms)
	}
	do("/api/v1/volunteer/jobs")
	do("/api/v1/club-events")
	if comms != 1 || members != 0 {
		t.Fatalf("non-snapshot routes must skip the bridge: members=%d comms=%d", members, comms)
	}
	do("/api/v1/something-unmapped")
	if comms != 2 || members != 1 {
		t.Fatalf("unmapped routes snapshot every domain: members=%d comms=%d", members, comms)
	}
}

func TestRuntimePersistenceDoesNotBufferStreamingResponses(t *testing.T) {
	t.Setenv("PATH", prependFakePSQL(t, fakePSQLBehavior{}))
	repo := runtimepersist.NewRepository("postgres://runtime-test")
	var n int
	handler := runtimePersistenceMiddleware(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), repo,
		[]runtimeParticipant{{domain: "comms", service: exportCounter{&n}}})(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("chunk-1"))
			w.(http.Flusher).Flush()
			if rec := w.(interface{ Unwrap() http.ResponseWriter }).Unwrap().(*httptest.ResponseRecorder); rec.Body.String() != "chunk-1" || !rec.Flushed {
				t.Errorf("first chunk was buffered until the handler returned (body=%q flushed=%v)", rec.Body.String(), rec.Flushed)
			}
		}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/announcements", nil))
	if n != 1 {
		t.Fatalf("snapshot exports = %d", n)
	}
}

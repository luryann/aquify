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

func TestRuntimePersistenceMiddlewareReturns500WhenSnapshotSaveFails(t *testing.T) {
	t.Setenv("PATH", prependFakePSQL(t, fakePSQLBehavior{
		failOnBegin: true,
	}))

	repo := runtimepersist.NewRepository("postgres://runtime-test")
	participants := []runtimeParticipant{{
		domain:  "members",
		service: snapshotStub{payload: []byte(`{"ok":true}`)},
	}}

	handler := runtimePersistenceMiddleware(discardLogger(), repo, participants)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test", "buffered")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"created":true}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/members/households", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when persistence fails, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), `"created":true`) {
		t.Fatalf("success body leaked despite persistence failure: %s", rec.Body.String())
	}
	if got := rec.Header().Get("X-Test"); got != "" {
		t.Fatalf("buffered success headers should not be flushed on failure, got %q", got)
	}
	if !strings.Contains(rec.Body.String(), "runtime_persistence_failed") {
		t.Fatalf("expected runtime persistence error payload, got %s", rec.Body.String())
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
		path := filepath.Join(dir, "psql.cmd")
		script := "@echo off\r\n" +
			"setlocal EnableDelayedExpansion\r\n" +
			"set \"sql=\"\r\n" +
			":loop\r\n" +
			"if \"%~1\"==\"\" goto after\r\n" +
			"if \"%~1\"==\"-c\" (\r\n" +
			"  set \"sql=%~2\"\r\n" +
			"  goto after\r\n" +
			")\r\n" +
			"shift\r\n" +
			"goto loop\r\n" +
			":after\r\n"
		if behavior.failOnBegin {
			script += "echo(!sql!| findstr /C:\"BEGIN;\" >nul && (\r\n" +
				"  echo simulated save failure 1>&2\r\n" +
				"  exit /b 1\r\n" +
				")\r\n"
		}
		if strings.TrimSpace(behavior.selectOutput) != "" {
			script += "echo(!sql!| findstr /C:\"SELECT domain\" >nul && (\r\n" +
				fmt.Sprintf("  echo %s\r\n", behavior.selectOutput) +
				"  exit /b 0\r\n" +
				")\r\n"
		}
		script += "exit /b 0\r\n"
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatalf("write fake psql: %v", err)
		}
	} else {
		path := filepath.Join(dir, "psql")
		var script strings.Builder
		script.WriteString("#!/bin/sh\n")
		script.WriteString("sql=\"\"\n")
		script.WriteString("prev=\"\"\n")
		script.WriteString("for arg in \"$@\"; do\n")
		script.WriteString("  if [ \"$prev\" = \"-c\" ]; then\n")
		script.WriteString("    sql=\"$arg\"\n")
		script.WriteString("    break\n")
		script.WriteString("  fi\n")
		script.WriteString("  prev=\"$arg\"\n")
		script.WriteString("done\n")
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

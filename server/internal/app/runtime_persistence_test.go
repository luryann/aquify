package app

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/members"
	"github.com/dareaquatics/aquify/server/internal/runtimepersist"
	"github.com/dareaquatics/aquify/server/internal/testutil"
)

func TestRuntimePersistenceMiddlewareReturns500WhenSnapshotSaveFails(t *testing.T) {
	t.Setenv("PATH", prependFakePSQL(t, `#!/bin/sh
sql=""
prev=""
for arg in "$@"; do
  if [ "$prev" = "-c" ]; then
    sql="$arg"
    break
  fi
  prev="$arg"
done
case "$sql" in
  *"BEGIN;"*)
    echo "simulated save failure" >&2
    exit 1
    ;;
  *)
    exit 0
    ;;
esac
`))

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
	t.Setenv("PATH", prependFakePSQL(t, `#!/bin/sh
sql=""
prev=""
for arg in "$@"; do
  if [ "$prev" = "-c" ]; then
    sql="$arg"
    break
  fi
  prev="$arg"
done
case "$sql" in
  *"SELECT domain"* )
    printf 'members\t6e6f742d6a736f6e\n'
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`))

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

func prependFakePSQL(t *testing.T, script string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "psql")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake psql: %v", err)
	}

	current := os.Getenv("PATH")
	if current == "" {
		return dir
	}
	return fmt.Sprintf("%s%c%s", dir, os.PathListSeparator, current)
}

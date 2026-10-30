package app

import (
	"context"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/storage"
	"github.com/dareaquatics/aquify/server/internal/workers"
)

func TestReadinessTypedNilWorkerServerIsDisabled(t *testing.T) {
	var typedNil *workers.Server
	checker := buildReadinessChecker(Config{}, nil, storage.NewStubStorage(""), nil, nil, typedNil, nil)
	report := checker.CheckReadiness(context.Background())
	if got := report.Checks["queue"]; got != "disabled" {
		t.Fatalf("queue = %q, want disabled", got)
	}
}

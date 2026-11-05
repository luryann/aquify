package app

import (
	"context"
	"testing"

	"github.com/luryann/aquify/server/internal/app/orgs"
	"github.com/luryann/aquify/server/internal/workers"
)

type fanOutRegistrar struct{ jobs []workers.ScheduledJob }

func (c *fanOutRegistrar) Register(j workers.ScheduledJob) { c.jobs = append(c.jobs, j) }

// Tenancy tracker item 3.4: org-scoped jobs fan out to the registry's active orgs
// at each firing (a newly provisioned org is picked up; a suspended one is not).
func TestOrgScopedJobsFanOutToActiveOrgs(t *testing.T) {
	ctx := context.Background()
	reg := orgs.NewService()
	mk := func(name string) string {
		o, _ := reg.Create(ctx, orgs.CreateInput{Name: name})
		_, _ = reg.SetStatus(ctx, o.ID, orgs.StatusActive)
		return o.ID
	}
	a := mk("A")
	cfg := Config{AppEnv: "production"}
	cfg.Worker.BillingAutomationInterval = "1h"
	cfg.Worker.ReconcileInterval = "1h"
	cfg.Worker.IntegrityInterval = "1h"
	cap := &fanOutRegistrar{}
	registerScheduledJobs(cap, cfg, OrgListerFor(cfg, reg))

	var fan *workers.ScheduledJob
	for i := range cap.jobs {
		if cap.jobs[i].FanOut != nil {
			fan = &cap.jobs[i]
			break
		}
	}
	if fan == nil {
		t.Fatal("expected org-scoped jobs to register a fan-out")
	}
	keys := func() map[string]bool {
		items, err := fan.FanOut(ctx)
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]bool{}
		for _, it := range items {
			m[it.Key] = true
		}
		return m
	}
	if k := keys(); len(k) != 1 || !k[a] {
		t.Fatalf("initial: %v", k)
	}
	b := mk("B")
	if k := keys(); len(k) != 2 || !k[b] {
		t.Fatalf("new org must be picked up without a restart: %v", k)
	}
	_, _ = reg.SetStatus(ctx, a, orgs.StatusSuspended)
	if k := keys(); len(k) != 1 || k[a] {
		t.Fatalf("suspended org must stop receiving jobs: %v", k)
	}
}

func TestOrgListerOperatorOverrideWins(t *testing.T) {
	cfg := Config{AppEnv: "production"}
	cfg.Worker.ScheduledOrgIDs = []string{"pinned"}
	ids, err := OrgListerFor(cfg, orgs.NewService())(context.Background())
	if err != nil || len(ids) != 1 || ids[0] != "pinned" {
		t.Fatalf("override: %v %v", ids, err)
	}
}

func TestValidateWorkerConfigAcceptsRegistryInsteadOfStaticList(t *testing.T) {
	cfg := Config{AppEnv: "production", DatabaseURL: "postgres://x"}
	cfg.Worker.RedisURL = "redis://localhost:6379"
	if err := ValidateWorkerConfig(cfg); err != nil {
		t.Fatalf("a configured registry replaces WORKER_SCHEDULED_ORG_IDS: %v", err)
	}
}

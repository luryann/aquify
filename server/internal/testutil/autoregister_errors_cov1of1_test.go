package testutil

import (
	"context"
	"testing"
)

func TestCov1xInstallOrgAutoRegisterFailsWhenFunctionExists(t *testing.T) {
	pool := IsolatedSchemaPool(t) // already installed the trigger function
	fake := cov1xRunFake(func(ft *testing.T) { installOrgAutoRegister(ft, pool) })
	if !fake.Failed() {
		t.Fatal("installing the auto-register function twice must fail")
	}
}

func TestCov1xInstallOrgAutoRegisterFailsWhenTriggerAlreadyPresent(t *testing.T) {
	pool := IsolatedSchemaPoolStrict(t)
	ctx := context.Background()
	for _, q := range []string{
		`CREATE FUNCTION cov1x_noop() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END $$`,
		`CREATE TRIGGER aaa_test_autoregister_org BEFORE INSERT ON households FOR EACH ROW EXECUTE FUNCTION cov1x_noop()`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	fake := cov1xRunFake(func(ft *testing.T) { installOrgAutoRegister(ft, pool) })
	if !fake.Failed() {
		t.Fatal("expected failure when a trigger of the same name already exists")
	}
}

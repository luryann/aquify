package main

import "testing"

func TestValidateResetTargetRequiresExplicitOptIn(t *testing.T) {
	err := validateResetTarget("postgres://postgres:postgres@127.0.0.1:5432/aquify?sslmode=disable", "")
	if err == nil {
		t.Fatal("expected missing opt-in to be rejected")
	}
}

func TestValidateResetTargetAcceptsLocalTestDatabase(t *testing.T) {
	err := validateResetTarget("postgres://postgres:postgres@127.0.0.1:5432/aquify_e2e?sslmode=disable", "true")
	if err != nil {
		t.Fatalf("expected local e2e database to pass: %v", err)
	}
}

func TestValidateResetTargetRejectsRemoteAndProductionNames(t *testing.T) {
	for _, databaseURL := range []string{
		"postgres://postgres:postgres@db.example.com:5432/aquify_e2e?sslmode=disable",
		"postgres://postgres:postgres@127.0.0.1:5432/aquify_prod?sslmode=disable",
		"postgres://postgres:postgres@127.0.0.1:5432/aquify?sslmode=disable",
		"postgres://postgres:postgres@127.0.0.1:5432/postgres?sslmode=disable",
	} {
		if err := validateResetTarget(databaseURL, "true"); err == nil {
			t.Fatalf("expected %q to be rejected", databaseURL)
		}
	}
}

func TestValidateResetTargetRejectsSubstringOnlyMatches(t *testing.T) {
	for _, name := range []string{"contest_results", "latest", "aquify_attested", "e2eish"} {
		if err := validateResetTarget("postgres://postgres:postgres@127.0.0.1:5432/"+name+"?sslmode=disable", "true"); err == nil {
			t.Fatalf("expected database %q to be rejected", name)
		}
	}
	for _, name := range []string{"aquify_e2e", "aquify-test", "test_aquify", "e2e"} {
		if err := validateResetTarget("postgres://postgres:postgres@127.0.0.1:5432/"+name+"?sslmode=disable", "true"); err != nil {
			t.Fatalf("expected database %q to pass: %v", name, err)
		}
	}
}

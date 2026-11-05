package app

import "testing"

func TestValidateAllowedOriginsRejectsNonBareOrigins(t *testing.T) {
	for _, origin := range []string{"https://admin.example.com/", "https://admin.example.com/app", "https://admin.example.com?x=1"} {
		cfg := Config{AppEnv: "production", AllowedOrigins: []string{origin}}
		if err := ValidateAllowedOrigins(cfg); err == nil {
			t.Fatalf("origin %q must be rejected", origin)
		}
	}
	if err := ValidateAllowedOrigins(Config{AppEnv: "production", AllowedOrigins: []string{"https://admin.example.com", "https://admin.example.com:8443"}}); err != nil {
		t.Fatalf("bare origins must pass: %v", err)
	}
}

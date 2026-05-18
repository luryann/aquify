package app

import "testing"

func TestValidateProviderConfigAllowsDevStubFallback(t *testing.T) {
	t.Parallel()

	cfg := Config{AppEnv: "development"}
	if err := ValidateProviderConfig(cfg); err != nil {
		t.Fatalf("expected development config to allow stub fallback, got %v", err)
	}
}

func TestValidateProviderConfigRequiresRealProvidersOutsideDev(t *testing.T) {
	t.Parallel()

	cfg := Config{
		AppEnv: "production",
		Stripe: StripeConfig{},
		Postmark: PostmarkConfig{
			ServerToken: "",
			FromEmail:   "",
		},
	}
	if err := ValidateProviderConfig(cfg); err == nil {
		t.Fatal("expected provider validation to fail in production without Stripe/Postmark config")
	}
}

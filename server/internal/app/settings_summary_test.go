package app

import "testing"

func TestSettingsSummaryWebhookAuthNeedsNameAndValue(t *testing.T) {
	var cfg Config
	cfg.Postmark.WebhookAuthHeaderName = "X-Hook"
	pm := settingsSummary(cfg)["postmark"].(map[string]any)
	if pm["webhook_auth"].(bool) {
		t.Fatal("header name alone must not report webhook auth configured")
	}
	cfg.Postmark.WebhookAuthHeaderValue = "secret"
	pm = settingsSummary(cfg)["postmark"].(map[string]any)
	if !pm["webhook_auth"].(bool) {
		t.Fatal("name+value should report configured")
	}
}

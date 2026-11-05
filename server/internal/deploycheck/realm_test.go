package deploycheck

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func python(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not installed")
	}
	return p
}

func writeAppEnv(t *testing.T, dir string, over map[string]string) string {
	t.Helper()
	vals := map[string]string{
		"API_ALLOWED_ORIGINS":          "https://app.club.test",
		"OIDC_CLIENT_SECRET":           "oidcclientsecretvalue0123456789",
		"KEYCLOAK_ADMIN_CLIENT_SECRET": "adminclientsecretvalue0123456789",
		"POSTMARK_SERVER_TOKEN":        "pm-token-synthetic-0123456789",
		"POSTMARK_FROM_EMAIL":          "noreply@club.test",
	}
	for k, v := range over {
		vals[k] = v
	}
	var b strings.Builder
	for k, v := range vals {
		b.WriteString(k + "=" + v + "\n")
	}
	p := filepath.Join(dir, ".env.app")
	if err := os.WriteFile(p, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func render(t *testing.T, py, envFile, out string, extra ...string) ([]byte, error) {
	t.Helper()
	args := append([]string{repoFile(t, "deploy/keycloak/render_realm.py"), "--app-env", envFile, "--out", out}, extra...)
	return exec.Command(py, args...).CombinedOutput()
}

func TestProductionRealmIsRenderedWithoutSeedUsersOrDevSecrets(t *testing.T) {
	py := python(t)
	dir := t.TempDir()
	envFile := writeAppEnv(t, dir, nil)
	out := filepath.Join(dir, "production-realm.json")
	if b, err := render(t, py, envFile, out); err != nil {
		t.Fatalf("render failed: %v\n%s", err, b)
	}
	raw, _ := os.ReadFile(out)
	var realm map[string]any
	if err := json.Unmarshal(raw, &realm); err != nil {
		t.Fatal(err)
	}
	users := realm["users"].([]any)
	if len(users) != 1 || users[0].(map[string]any)["serviceAccountClientId"] != "aquify-admin" {
		t.Fatalf("users = %v, want only the admin client's service account", users)
	}
	for _, bad := range []string{"localhost", "127.0.0.1", "aquify.local", "dev-secret", "local-admin-secret", `"credentials"`} {
		if strings.Contains(string(raw), bad) {
			t.Errorf("rendered realm still contains %q", bad)
		}
	}
	for _, want := range []string{"https://app.club.test/auth/callback", "oidcclientsecretvalue0123456789", "adminclientsecretvalue0123456789", "smtp.postmarkapp.com"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("rendered realm lacks %q", want)
		}
	}
	if realm["sslRequired"] != "external" || realm["verifyEmail"] != true || realm["registrationAllowed"] != false {
		t.Errorf("TLS/verification/registration settings wrong: %v %v %v", realm["sslRequired"], realm["verifyEmail"], realm["registrationAllowed"])
	}
	// The rendered file passes its own check, and the roles/clients/mappers survived.
	if b, err := render(t, py, envFile, out, "--check-only"); err != nil {
		t.Fatalf("check-only failed on a freshly rendered realm: %v\n%s", err, b)
	}
	if len(realm["clients"].([]any)) < 2 || len(realm["roles"].(map[string]any)["realm"].([]any)) != 4 {
		t.Error("clients or realm roles were lost in rendering")
	}
}

func TestRealmRendererRefusesBadInput(t *testing.T) {
	py := python(t)
	for name, over := range map[string]map[string]string{
		"missing client secret": {"OIDC_CLIENT_SECRET": ""},
		"weak admin secret":     {"KEYCLOAK_ADMIN_CLIENT_SECRET": "short"},
		"http origin":           {"API_ALLOWED_ORIGINS": "http://app.club.test"},
		"no smtp token":         {"POSTMARK_SERVER_TOKEN": ""},
		"no origin":             {"API_ALLOWED_ORIGINS": ""},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			out := filepath.Join(dir, "r.json")
			if b, err := render(t, py, writeAppEnv(t, dir, over), out); err == nil {
				t.Fatalf("expected a failure\n%s", b)
			}
			if _, err := os.Stat(out); err == nil {
				t.Error("a realm file was written despite the failure")
			}
		})
	}
}

func TestRealmCheckRejectsATamperedFile(t *testing.T) {
	py := python(t)
	dir := t.TempDir()
	envFile := writeAppEnv(t, dir, nil)
	good := filepath.Join(dir, "good.json")
	if b, err := render(t, py, envFile, good); err != nil {
		t.Fatalf("%v\n%s", err, b)
	}
	raw, _ := os.ReadFile(good)
	var realm map[string]any
	_ = json.Unmarshal(raw, &realm)

	tamper := map[string]func(){
		"seed user": func() {
			realm["users"] = append(realm["users"].([]any), map[string]any{"username": "admin@aquify.local", "credentials": []any{map[string]any{"value": "x"}}})
		},
		"no TLS requirement": func() { realm["sslRequired"] = "none" },
		"registration on":    func() { realm["registrationAllowed"] = true },
		"dev redirect": func() {
			realm["clients"].([]any)[0].(map[string]any)["redirectUris"] = []any{"http://localhost:3000/auth/callback"}
		},
	}
	for name, mutate := range tamper {
		t.Run(name, func(t *testing.T) {
			var copyRealm map[string]any
			_ = json.Unmarshal(raw, &copyRealm)
			realm = copyRealm
			mutate()
			b, _ := json.Marshal(realm)
			bad := filepath.Join(dir, strings.ReplaceAll(name, " ", "_")+".json")
			_ = os.WriteFile(bad, b, 0o600)
			if out, err := render(t, py, envFile, bad, "--check-only"); err == nil {
				t.Fatalf("tampered realm (%s) passed the check\n%s", name, out)
			}
		})
	}
}

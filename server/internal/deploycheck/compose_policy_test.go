package deploycheck

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func readFile(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(repoFile(t, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

func services(t *testing.T) map[string]map[string]any {
	t.Helper()
	out := map[string]map[string]any{}
	for name, v := range loadYAML(t, "deploy/docker-compose.production.yml")["services"].(map[string]any) {
		out[name] = v.(map[string]any)
	}
	return out
}

// envKeys returns the KEY names assigned (not commented) in an env template.
func envKeys(t *testing.T, rel string) []string {
	t.Helper()
	var keys []string
	sc := bufio.NewScanner(strings.NewReader(readFile(t, rel)))
	re := regexp.MustCompile(`^([A-Z][A-Z0-9_]*)=`)
	for sc.Scan() {
		if m := re.FindStringSubmatch(sc.Text()); m != nil {
			keys = append(keys, m[1])
		}
	}
	return keys
}

func TestMigrateServiceIsAOneShotToolsJob(t *testing.T) {
	svc, ok := services(t)["aquify-migrate"]
	if !ok {
		t.Fatal("aquify-migrate service is missing: promote.sh has nothing to run")
	}
	if p, _ := svc["profiles"].([]any); len(p) != 1 || p[0] != "tools" {
		t.Errorf("profiles = %v, want [tools] (it must not start with the stack)", svc["profiles"])
	}
	if svc["restart"] != "no" {
		t.Errorf("restart = %v, want \"no\"", svc["restart"])
	}
	cmd, _ := svc["command"].([]any)
	if len(cmd) != 2 || cmd[0] != "migrate" || cmd[1] != "up" {
		t.Errorf("command = %v, want [migrate up]", svc["command"])
	}
	if _, has := svc["env_file"]; has {
		t.Error("aquify-migrate must not use env_file: it gets only the owner DSN")
	}
	env := svc["environment"].(map[string]any)
	if v, _ := env["MIGRATE_DATABASE_URL"].(string); !strings.Contains(v, "AQUIFY_OWNER_DB_") {
		t.Errorf("MIGRATE_DATABASE_URL = %q, want the owner role", v)
	}
	if _, has := env["DATABASE_URL"]; has {
		t.Error("aquify-migrate must not be given the application DATABASE_URL")
	}
}

func TestOnlyTheAppContainersReceiveAnEnvFile(t *testing.T) {
	for name, svc := range services(t) {
		ef, has := svc["env_file"]
		switch name {
		case "aquify-api", "aquify-worker":
			l, _ := ef.([]any)
			if !has || len(l) != 1 || l[0] != ".env.app" {
				t.Errorf("%s env_file = %v, want [.env.app]", name, ef)
			}
		default:
			if has {
				t.Errorf("%s has env_file %v; give it explicit environment entries so it cannot read unrelated secrets", name, ef)
			}
		}
	}
	if strings.Contains(readFile(t, "deploy/docker-compose.production.yml"), "env_file:\n      - .env.production") {
		t.Error("no service may load the infrastructure secrets file wholesale")
	}
}

func TestAppContainersPinProductionAndDisableDevShortcuts(t *testing.T) {
	for _, name := range []string{"aquify-api", "aquify-worker"} {
		env := services(t)[name]["environment"].(map[string]any)
		if env["APP_ENV"] != "production" {
			t.Errorf("%s APP_ENV = %v", name, env["APP_ENV"])
		}
		if env["AUTH_ENABLE_DEV_STUB_TOKENS"] != "false" {
			t.Errorf("%s must pin AUTH_ENABLE_DEV_STUB_TOKENS=false", name)
		}
	}
	if env := services(t)["aquify-api"]["environment"].(map[string]any); env["AUTH_ALLOW_DEV_SESSION_ISSUE"] != "false" {
		t.Error("aquify-api must pin AUTH_ALLOW_DEV_SESSION_ISSUE=false")
	}
}

// Redis is the Asynq broker. Any eviction policy can silently drop queued jobs.
func TestRedisNeverEvictsJobData(t *testing.T) {
	for _, rel := range []string{"deploy/docker-compose.production.yml", "deploy/docker-compose.staging.yml"} {
		text := readFile(t, rel)
		if strings.Contains(text, "allkeys-") || strings.Contains(text, "volatile-") {
			t.Errorf("%s configures a Redis eviction policy; the broker must use noeviction", rel)
		}
	}
	text := readFile(t, "deploy/docker-compose.production.yml")
	if !strings.Contains(text, "--maxmemory-policy noeviction") {
		t.Error("production redis must set --maxmemory-policy noeviction")
	}
	if strings.Contains(text, "${REDIS_PASSWORD}\n        --appendonly") || strings.Contains(text, "-a\", \"${REDIS_PASSWORD}") {
		t.Error("the Redis password must not be placed on a command line")
	}
}

var interpolation = regexp.MustCompile(`\$\{([A-Z][A-Z0-9_]*)(:?[-?][^}]*)?\}`)

// Every ${VAR} the compose file needs (no default) must be documented in the
// infrastructure template (or come from release.env), and every key in that
// template must actually be used. Keeps the example and the stack in step.
func TestInfraTemplateMatchesComposeInterpolation(t *testing.T) {
	compose := readFile(t, "deploy/docker-compose.production.yml")
	required, used := map[string]bool{}, map[string]bool{}
	for _, m := range interpolation.FindAllStringSubmatch(compose, -1) {
		used[m[1]] = true
		if m[2] == "" || strings.HasPrefix(m[2], ":?") || strings.HasPrefix(m[2], "?") {
			required[m[1]] = true
		}
	}
	fromRelease := map[string]bool{"IMAGE_TAG": true, "GHCR_NAMESPACE": true}
	tmpl := map[string]bool{}
	for _, k := range envKeys(t, "deploy/env/production.env.example") {
		tmpl[k] = true
	}
	var missing, dead []string
	for k := range required {
		if !tmpl[k] && !fromRelease[k] {
			missing = append(missing, k)
		}
	}
	for k := range tmpl {
		if !used[k] {
			dead = append(dead, k)
		}
	}
	sort.Strings(missing)
	sort.Strings(dead)
	if len(missing) > 0 {
		t.Errorf("compose requires variables absent from deploy/env/production.env.example: %v", missing)
	}
	if len(dead) > 0 {
		t.Errorf("deploy/env/production.env.example documents variables compose never uses: %v", dead)
	}
}

// Every key in the app template must be read by the server, so documentation
// cannot list secrets that do nothing (SESSION_SIGNING_KEY was once such a key).
func TestAppTemplateKeysAreReadByTheServer(t *testing.T) {
	var src strings.Builder
	root := repoFile(t, "server")
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		b, _ := os.ReadFile(p)
		src.Write(b)
		return nil
	})
	code := src.String()
	for _, k := range envKeys(t, "deploy/env/app.env.example") {
		if !strings.Contains(code, `"`+k+`"`) {
			t.Errorf("app.env.example lists %s but no server code reads it", k)
		}
	}
}

// Keys startup requires outside development must be in the template, so a copy
// of it can actually boot.
func TestAppTemplateCoversWhatStartupRequires(t *testing.T) {
	have := map[string]bool{}
	for _, k := range envKeys(t, "deploy/env/app.env.example") {
		have[k] = true
	}
	for _, k := range []string{
		"DATABASE_URL", "REDIS_URL", "API_ALLOWED_ORIGINS", "STORAGE_ENDPOINT", "STORAGE_ACCESS_KEY", "STORAGE_SECRET_KEY",
		"STORAGE_BUCKET", "INTEGRATIONS_ENCRYPTION_KEY", "OIDC_EXPECTED_ISSUER", "OIDC_END_SESSION_URL",
		"OIDC_POST_LOGOUT_REDIRECT_URL", "OIDC_STATE_SIGNING_KEY", "KEYCLOAK_ADMIN_URL", "KEYCLOAK_REALM",
		"KEYCLOAK_ADMIN_CLIENT_ID", "KEYCLOAK_ADMIN_CLIENT_SECRET", "STRIPE_SECRET_KEY", "STRIPE_WEBHOOK_SECRET",
		"STRIPE_CONNECT_WEBHOOK_SECRET", "POSTMARK_SERVER_TOKEN", "POSTMARK_FROM_EMAIL", "SECURITY_TRUSTED_PROXY_CIDRS",
		"METRICS_ADDR",
	} {
		if !have[k] {
			t.Errorf("app.env.example is missing %s", k)
		}
	}
	for _, banned := range []string{"AUTH_ENABLE_DEV_STUB_TOKENS", "AUTH_ALLOW_DEV_SESSION_ISSUE", "SESSION_COOKIE_SECURE", "APP_ENV", "SESSION_SIGNING_KEY", "SESSION_ENCRYPTION_KEY"} {
		if have[banned] {
			t.Errorf("app.env.example must not set %s", banned)
		}
	}
}

func TestPostgresRoleBootstrapMatchesCompose(t *testing.T) {
	pg := services(t)["postgres"]
	env := pg["environment"].(map[string]any)
	// Variables 10-bootstrap.sh requires must all reach the container.
	sh := readFile(t, "deploy/postgres/init/10-bootstrap.sh")
	m := regexp.MustCompile(`(?s)for v in (POSTGRES_USER.*?); do\n  require`).FindStringSubmatch(sh)
	if m == nil {
		t.Fatal("cannot find the required-variable list in 10-bootstrap.sh")
	}
	for _, v := range strings.Fields(strings.ReplaceAll(m[1], "\\\n", " ")) {
		if v == "POSTGRES_USER" || v == "POSTGRES_DB" || v == "POSTGRES_PASSWORD" {
			if _, ok := env[v]; !ok {
				t.Errorf("postgres service does not set %s", v)
			}
			continue
		}
		if _, ok := env[v]; !ok {
			t.Errorf("10-bootstrap.sh requires %s but the postgres service does not provide it", v)
		}
	}
	vols, _ := pg["volumes"].([]any)
	found := false
	for _, v := range vols {
		if s, _ := v.(string); strings.HasPrefix(s, "./postgres/init:/docker-entrypoint-initdb.d") {
			found = true
		}
	}
	if !found {
		t.Error("postgres does not mount ./postgres/init into /docker-entrypoint-initdb.d")
	}
	kc := services(t)["keycloak"]["environment"].(map[string]any)
	if v, _ := kc["KC_DB_URL"].(string); strings.Contains(v, "${POSTGRES_DB") {
		t.Errorf("keycloak must use its own database, got %s", v)
	}
	if v, _ := kc["KC_DB_USERNAME"].(string); strings.Contains(v, "POSTGRES_USER") || strings.Contains(v, "AQUIFY_") {
		t.Errorf("keycloak must use its own role, got %s", v)
	}
}

// Only the ingress may be reachable from the internet. A published port without a
// 127.0.0.1 prefix is exposed on every interface of the host.
func TestOnlyTheIngressPublishesPublicPorts(t *testing.T) {
	for name, svc := range services(t) {
		ports, _ := svc["ports"].([]any)
		for _, p := range ports {
			s, _ := p.(string)
			if name == "caddy" {
				continue
			}
			if !strings.HasPrefix(s, "127.0.0.1:") {
				t.Errorf("%s publishes %q on all interfaces; bind it to 127.0.0.1 or route it through the ingress", name, s)
			}
		}
	}
	caddy, ok := services(t)["caddy"]
	if !ok {
		t.Fatal("no ingress service: nothing can serve the documented public hostnames")
	}
	got := map[string]bool{}
	for _, p := range caddy["ports"].([]any) {
		got[p.(string)] = true
	}
	for _, want := range []string{"80:80", "443:443", "443:443/udp"} {
		if !got[want] {
			t.Errorf("caddy does not publish %s", want)
		}
	}
	if strings.Contains(readFile(t, "deploy/docker-compose.production.yml"), "9001:9001") {
		t.Error("the MinIO console must not be published")
	}
}

func TestIngressRoutesAndKeycloakExposure(t *testing.T) {
	cf := readFile(t, "deploy/caddy/Caddyfile")
	for _, host := range []string{"app.{$AQUIFY_DOMAIN}", "api.{$AQUIFY_DOMAIN}", "auth.{$AQUIFY_DOMAIN}", "files.{$AQUIFY_DOMAIN}"} {
		if !strings.Contains(cf, host+" {") {
			t.Errorf("Caddyfile has no site block for %s", host)
		}
	}
	for _, upstream := range []string{"aquify-admin:3000", "aquify-api:8080", "keycloak:8081", "minio:9000"} {
		if !strings.Contains(cf, "reverse_proxy "+upstream) {
			t.Errorf("Caddyfile does not proxy to %s", upstream)
		}
	}
	// Keycloak's admin console / master realm / metrics must not be public.
	for _, blocked := range []string{"/admin", "/realms/master", "/metrics"} {
		if !strings.Contains(cf, blocked) {
			t.Errorf("Caddyfile does not block Keycloak path %s", blocked)
		}
	}
	kc := services(t)["keycloak"]["environment"].(map[string]any)
	if _, legacy := kc["KC_PROXY"]; legacy {
		t.Error("KC_PROXY was removed in Keycloak 26; use KC_PROXY_HEADERS")
	}
	if kc["KC_PROXY_HEADERS"] != "xforwarded" {
		t.Errorf("KC_PROXY_HEADERS = %v, want xforwarded", kc["KC_PROXY_HEADERS"])
	}
	if h, _ := kc["KC_HOSTNAME"].(string); !strings.HasPrefix(h, "https://auth.") {
		t.Errorf("KC_HOSTNAME = %q, want the https auth hostname", h)
	}
	// The API container reaches files.<domain> through the ingress via a network alias.
	caddyNets := services(t)["caddy"]["networks"].(map[string]any)["aquify-net"].(map[string]any)
	aliases, _ := caddyNets["aliases"].([]any)
	if len(aliases) == 0 || !strings.HasPrefix(aliases[0].(string), "files.") {
		t.Errorf("caddy needs a files.<domain> alias on aquify-net, got %v", aliases)
	}
}

// Behind the ingress every visitor would share one address unless the API trusts the
// compose network to set X-Forwarded-For, so the template must name that network.
func TestTrustedProxyCIDRMatchesTheComposeNetwork(t *testing.T) {
	doc := loadYAML(t, "deploy/docker-compose.production.yml")
	nets := doc["networks"].(map[string]any)["aquify-net"].(map[string]any)
	subnet := nets["ipam"].(map[string]any)["config"].([]any)[0].(map[string]any)["subnet"].(string)
	if !strings.Contains(readFile(t, "deploy/env/app.env.example"), "SECURITY_TRUSTED_PROXY_CIDRS="+subnet) {
		t.Errorf("app.env.example SECURITY_TRUSTED_PROXY_CIDRS must equal the compose network %s", subnet)
	}
}

// Every host path the production compose file bind-mounts must exist in the deploy
// bundle (deploy/ plus docs/observability copied to <deploy dir>/docs/observability),
// otherwise Docker silently creates an empty directory in its place (this is how
// Prometheus lost its alert rules). The rendered Keycloak realm is generated on the host.
func TestBindMountSourcesExistInTheDeployBundle(t *testing.T) {
	generatedOnHost := map[string]bool{"./keycloak/production-realm.json": true}
	for name, svc := range services(t) {
		vols, _ := svc["volumes"].([]any)
		for _, v := range vols {
			spec, _ := v.(string)
			src := strings.SplitN(spec, ":", 2)[0]
			if !strings.HasPrefix(src, "./") || generatedOnHost[src] {
				continue
			}
			rel := "deploy/" + strings.TrimPrefix(src, "./")
			if strings.HasPrefix(src, "./docs/observability") {
				rel = strings.TrimPrefix(src, "./")
			}
			if _, err := os.Stat(repoFile(t, rel)); err != nil {
				t.Errorf("%s mounts %s but %s is not in the repository: %v", name, src, rel, err)
			}
		}
	}
}

// admin/Dockerfile does `COPY . .`; without these exclusions the host's node_modules
// and any local env file are sent into the image build.
func TestAdminBuildContextExcludesDependenciesAndSecrets(t *testing.T) {
	text := readFile(t, "admin/.dockerignore")
	for _, want := range []string{"node_modules", ".next", ".env", ".env.*", "*.pem"} {
		found := false
		for _, line := range strings.Split(text, "\n") {
			if strings.TrimSpace(line) == want {
				found = true
			}
		}
		if !found {
			t.Errorf("admin/.dockerignore does not exclude %q", want)
		}
	}
}

// Keycloak only imports a file named <realm>-realm.json; anything else makes the
// container exit at startup ("File name / realm name mismatch"), found by running the
// real image.
func TestKeycloakRealmImportFileIsNamedAfterTheRealm(t *testing.T) {
	svc := services(t)["keycloak"]
	vols, _ := svc["volumes"].([]any)
	found := false
	for _, v := range vols {
		spec, _ := v.(string)
		if i := strings.Index(spec, "/opt/keycloak/data/import/"); i >= 0 {
			found = true
			name := strings.SplitN(spec[i+len("/opt/keycloak/data/import/"):], ":", 2)[0]
			if name != "aquify-realm.json" {
				t.Errorf("realm import is mounted as %q; Keycloak requires aquify-realm.json for realm aquify", name)
			}
		}
	}
	if !found {
		t.Error("keycloak has no realm import mount")
	}
}

// Floating tags make a redeploy a silent upgrade and a rollback impossible. Every
// third-party image in both compose files must be pinned by digest; images built from
// this repo are selected by ${IMAGE_TAG}, and images built on the host (a build:
// context, e.g. minio) carry a dated tag.
func TestEveryImageIsPinned(t *testing.T) {
	for _, file := range []string{"deploy/docker-compose.production.yml", "deploy/docker-compose.staging.yml"} {
		svcs, _ := loadYAML(t, file)["services"].(map[string]any)
		for name, raw := range svcs {
			svc, _ := raw.(map[string]any)
			image, _ := svc["image"].(string)
			if image == "" || strings.Contains(image, "${IMAGE_TAG") {
				continue
			}
			if build, ok := svc["build"]; ok {
				ctx, _ := build.(string)
				if _, err := os.Stat(repoFile(t, "deploy/"+ctx)); err != nil {
					t.Errorf("%s %s: build context %q missing", file, name, ctx)
				}
				if strings.HasSuffix(image, ":latest") || !strings.Contains(image, ":") {
					t.Errorf("%s %s: locally built image %q needs an explicit tag", file, name, image)
				}
				continue
			}
			if !strings.Contains(image, "@sha256:") {
				t.Errorf("%s %s: image %q is not pinned by digest", file, name, image)
			}
		}
	}
}

// The Loki logging driver runs inside the host's Docker daemon, outside the compose
// network, so the push URL cannot use the service name "loki" (found by running the
// stack: nothing ever reached Loki). It must be the loopback port loki publishes.
func TestLokiDriverPushesToThePublishedLoopbackPort(t *testing.T) {
	for _, file := range []string{"deploy/docker-compose.production.yml", "deploy/docker-compose.staging.yml"} {
		text := readFile(t, file)
		if !strings.Contains(text, `loki-url: "http://127.0.0.1:3100/loki/api/v1/push"`) {
			t.Errorf("%s: loki-url must be http://127.0.0.1:3100/... (the driver cannot resolve compose service names)", file)
		}
		if !strings.Contains(text, `"127.0.0.1:3100:3100"`) {
			t.Errorf("%s: loki must publish 127.0.0.1:3100 for the logging driver", file)
		}
	}
}

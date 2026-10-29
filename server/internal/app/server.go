package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/authn"
	"github.com/dareaquatics/aquify/server/internal/app/billing"
	"github.com/dareaquatics/aquify/server/internal/app/comms"
	"github.com/dareaquatics/aquify/server/internal/app/compliance"
	"github.com/dareaquatics/aquify/server/internal/app/documents"
	"github.com/dareaquatics/aquify/server/internal/app/exports"
	"github.com/dareaquatics/aquify/server/internal/app/groups"
	"github.com/dareaquatics/aquify/server/internal/app/handlers"
	"github.com/dareaquatics/aquify/server/internal/app/meets"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/app/orgsettings"
	"github.com/dareaquatics/aquify/server/internal/app/performance"
	"github.com/dareaquatics/aquify/server/internal/app/reports"
	"github.com/dareaquatics/aquify/server/internal/app/seasons"
	"github.com/dareaquatics/aquify/server/internal/app/training"
	"github.com/dareaquatics/aquify/server/internal/db"
	"github.com/dareaquatics/aquify/server/internal/keycloak"
	"github.com/dareaquatics/aquify/server/internal/postmark"
	"github.com/dareaquatics/aquify/server/internal/runtimepersist"
	"github.com/dareaquatics/aquify/server/internal/storage"
	stripelib "github.com/dareaquatics/aquify/server/internal/stripe"
	"github.com/dareaquatics/aquify/server/internal/telemetry"
	"github.com/dareaquatics/aquify/server/internal/workers"
	"github.com/dareaquatics/aquify/server/internal/workers/jobhandlers"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewRouter is a convenience wrapper for tests and simple deployments.
// It builds and starts the worker server but does NOT start the scheduler,
// because the scheduler goroutine cannot be stopped from this call site.
// Production deployments use NewRouterWithServices directly (see main.go).
func NewRouter(cfg Config, logger *slog.Logger) http.Handler {
	return NewRouterWithServices(cfg, logger, RouterDeps{})
}

// RouterDeps names every injectable dependency of the router. Zero (nil)
// fields are replaced by defaults: in-memory/stub services in dev and test
// environments, real clients (or startup failures) elsewhere. Named fields
// replace the old 17-19 positional parameters, where same-typed nils could be
// transposed without a compile error.
type RouterDeps struct {
	Auth        *authn.Service
	Members     *members.Service
	Groups      *groups.Service
	Meets       *meets.Service
	Exports     *exports.Service
	Performance *performance.Service
	Billing     *billing.Service
	Stripe      stripelib.Client
	Comms       *comms.Service
	Audit       *audit.Service
	Compliance  *compliance.Service
	Documents   *documents.Service
	Training    *training.Service
	// Workers may be nil; when nil a new in-memory worker server is created,
	// registered and started automatically.
	Workers workers.Backend
	// Keycloak may be nil; when nil a StubClient is used in dev/test and a real
	// AdminClient is constructed from cfg.Keycloak in non-dev environments.
	Keycloak keycloak.Client
	// Metrics is the Prometheus registry; nil creates a private one.
	Metrics *telemetry.Metrics
	// ExposePrometheus mounts the scrape endpoint on the returned router.
	ExposePrometheus bool
}

func allowsProviderStubFallback(appEnv string) bool {
	return isExplicitDevelopmentEnv(appEnv)
}

func allowsKeycloakAdminStubFallback(cfg Config) bool {
	return allowsProviderStubFallback(cfg.AppEnv) && !requiresKeycloakAdminConfig(cfg)
}

func canonicalUserManagementRole(role string) string {
	return strings.TrimSpace(strings.ToLower(role))
}

// seedsLocalDevFixtures reports whether local-dev fixtures (the stub parent's
// household) should be seeded. The test env is excluded so suites keep
// starting from empty state.
func seedsLocalDevFixtures(appEnv string) bool {
	switch NormalizeAppEnv(appEnv) {
	case "development", "local":
		return true
	}
	return false
}

func seedLocalDevKeycloakStub(stub *keycloak.StubClient) {
	if stub == nil {
		return
	}

	for _, user := range authn.LocalDevUsers() {
		stub.AddUser(keycloak.User{
			ID:            user.UserID,
			Username:      strings.ToLower(strings.TrimSpace(user.Email)),
			Email:         strings.TrimSpace(user.Email),
			FirstName:     strings.TrimSpace(user.Name),
			LastName:      "",
			Enabled:       true,
			EmailVerified: true,
			Attributes: map[string][]string{
				"org_id": {authn.LocalDevOrgID},
			},
		})

		for _, role := range user.Roles {
			_ = stub.AssignRealmRole(context.Background(), user.UserID, canonicalUserManagementRole(role))
		}
	}
}

// ValidateKeycloakStartup verifies Keycloak connectivity when KEYCLOAK_REALM is set.
// It checks that the OIDC JWKS endpoint is reachable and ensures required realm roles exist.
// Returns an error if JWKS is unreachable; logs a warning if realm roles cannot be verified.
func ValidateKeycloakStartup(cfg Config, logger *slog.Logger) error {
	if cfg.Keycloak.Realm == "" {
		return nil
	}
	if logger == nil {
		logger = slog.Default()
	}

	// 1. Verify JWKS endpoint is reachable.
	if cfg.Auth.OIDC.JWKSURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.Auth.OIDC.JWKSURL, nil)
		if err != nil {
			return fmt.Errorf("keycloak startup: invalid JWKS URL %q: %w", cfg.Auth.OIDC.JWKSURL, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("keycloak startup: JWKS endpoint unreachable at %q — verify OIDC_JWKS_URL and Keycloak is running: %w", cfg.Auth.OIDC.JWKSURL, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("keycloak startup: JWKS endpoint at %q returned HTTP %d (expected 200)", cfg.Auth.OIDC.JWKSURL, resp.StatusCode)
		}
		logger.Info("keycloak startup: JWKS endpoint reachable", "url", cfg.Auth.OIDC.JWKSURL)
	}

	// 2. Ensure required realm roles exist (idempotent — creates any that are missing).
	if cfg.Keycloak.AdminURL != "" && cfg.Keycloak.ClientID != "" && cfg.Keycloak.ClientSecret != "" {
		kcClient, err := keycloak.NewAdminClient(keycloak.AdminConfig{
			AdminURL:     cfg.Keycloak.AdminURL,
			Realm:        cfg.Keycloak.Realm,
			ClientID:     cfg.Keycloak.ClientID,
			ClientSecret: cfg.Keycloak.ClientSecret,
		})
		if err != nil {
			return fmt.Errorf("keycloak startup: failed to create admin client: %w", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := kcClient.EnsureRealmRolesExist(ctx, keycloak.DefaultRealmRoles); err != nil {
			if requiresKeycloakAdminConfig(cfg) {
				return fmt.Errorf(
					"keycloak startup: admin client could not verify/create realm roles; verify KEYCLOAK_ADMIN_CLIENT_ID, KEYCLOAK_ADMIN_CLIENT_SECRET, and the service account's realm-management roles (manage-users, view-users, query-users, manage-realm): %w",
					err,
				)
			}
			logger.Warn("keycloak startup: could not verify/create realm roles — role assignment features may be degraded",
				"error", err,
				"roles", keycloak.DefaultRealmRoles,
			)
		} else {
			logger.Info("keycloak startup: realm roles verified", "roles", keycloak.DefaultRealmRoles)
		}
	}

	return nil
}

// ValidateProviderConfig enforces that non-dev environments cannot boot
// without working Stripe and Postmark provider clients, and that Postmark
// webhook authentication is explicitly configured.
func ValidateProviderConfig(cfg Config) error {
	if allowsProviderStubFallback(cfg.AppEnv) {
		return nil
	}

	var errs []string
	if _, err := stripelib.NewSDKClient(cfg.Stripe.SecretKey); err != nil {
		errs = append(errs, fmt.Sprintf("stripe: %v", err))
	}
	if strings.TrimSpace(cfg.Stripe.SecretKey) != "" && strings.TrimSpace(cfg.Stripe.WebhookSecret) == "" {
		errs = append(errs, "stripe: STRIPE_WEBHOOK_SECRET must be set in non-development environments when Stripe is enabled, otherwise webhook signature verification cannot be enforced")
	}
	if _, err := postmark.NewHTTPClient(postmark.Config{
		ServerToken: cfg.Postmark.ServerToken,
		FromName:    cfg.Postmark.FromName,
		FromEmail:   cfg.Postmark.FromEmail,
	}); err != nil {
		errs = append(errs, fmt.Sprintf("postmark: %v", err))
	}
	// Postmark webhook auth must be explicitly configured in non-dev environments.
	// Static shared-secret header or HTTP basic auth is the minimum requirement.
	pmWebhookAuthConfigured := (cfg.Postmark.WebhookAuthHeaderName != "" && cfg.Postmark.WebhookAuthHeaderValue != "") ||
		(cfg.Postmark.WebhookBasicUser != "" && cfg.Postmark.WebhookBasicPass != "")
	if !pmWebhookAuthConfigured {
		errs = append(errs, "postmark webhook: auth must be configured via POSTMARK_WEBHOOK_AUTH_HEADER_NAME+VALUE or POSTMARK_WEBHOOK_BASIC_USER+PASS in non-development environments")
	}
	// OIDC issuer validation must be mandatory outside dev/test — without it
	// a compromised or misconfigured discovery endpoint could allow a
	// different Keycloak realm/provider to mint accepted ID tokens.
	if cfg.Auth.OIDC.ClientID != "" && strings.TrimSpace(cfg.Auth.OIDC.ExpectedIssuer) == "" {
		errs = append(errs, "oidc: OIDC_EXPECTED_ISSUER must be set in non-development environments so ID token issuer is validated")
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("provider startup validation failed: %s", strings.Join(errs, "; "))
}

// ValidateWorkerConfig enforces Redis-backed durable queues outside explicit
// development/test environments.
func ValidateWorkerConfig(cfg Config) error {
	if allowsProviderStubFallback(cfg.AppEnv) {
		return nil
	}
	if strings.TrimSpace(cfg.Worker.RedisURL) == "" {
		return fmt.Errorf("worker startup validation failed: REDIS_URL must be configured in non-development environments")
	}
	return nil
}

type unavailableStripeClient struct {
	err error
}

func (c unavailableStripeClient) Ready(_ context.Context) error {
	return c.err
}

func (c unavailableStripeClient) CreatePaymentIntent(_ context.Context, _ stripelib.PaymentIntentParams) (*stripelib.PaymentIntentResult, error) {
	return nil, c.err
}

func (c unavailableStripeClient) CreateCheckoutSession(_ context.Context, _ stripelib.CheckoutSessionParams) (*stripelib.CheckoutSessionResult, error) {
	return nil, c.err
}

func (c unavailableStripeClient) ParseWebhookEvent(_ []byte, _, _ string) (*stripelib.WebhookEventData, error) {
	return nil, c.err
}

func (c unavailableStripeClient) FetchBalanceCents(_ context.Context) (int, error) {
	return 0, c.err
}

func (c unavailableStripeClient) CreateRefund(_ context.Context, _ stripelib.RefundParams) (*stripelib.RefundResult, error) {
	return nil, c.err
}

type unavailablePostmarkClient struct {
	err error
}

func (c unavailablePostmarkClient) Ready(_ context.Context) error {
	return c.err
}

func (c unavailablePostmarkClient) SendEmail(_ context.Context, _ postmark.SendEmailRequest) (postmark.SendEmailResponse, error) {
	return postmark.SendEmailResponse{}, c.err
}

var (
	openDBPool       = db.Open
	validateDBSchema = db.ValidateExpectedSchema
	closeDBPool      = func(pool *pgxpool.Pool) { pool.Close() }
)

func openValidatedDBPool(ctx context.Context, cfg Config, logger *slog.Logger) *pgxpool.Pool {
	if cfg.DatabaseURL == "" {
		return nil
	}

	dbPool, err := openDBPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("db pool init failed; services will run in-memory only", "error", err)
		return nil
	}
	if dbPool == nil {
		return nil
	}
	if err := validateDBSchema(ctx, dbPool); err != nil {
		logger.Error("database schema validation failed; DB-backed services disabled", "error", err)
		closeDBPool(dbPool)
		return nil
	}
	return dbPool
}

func isNilWorkerBackend(backend workers.Backend) bool {
	if backend == nil {
		return true
	}
	value := reflect.ValueOf(backend)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// NewRouterWithServices constructs the HTTP router and wires all services from
// the named dependencies in deps (see RouterDeps for the nil defaults).
func NewRouterWithServices(cfg Config, logger *slog.Logger, deps RouterDeps) http.Handler {
	metrics := deps.Metrics
	if metrics == nil {
		metrics = telemetry.NewMetrics()
	}
	exposePrometheus := deps.ExposePrometheus
	authService, membershipService, groupsService := deps.Auth, deps.Members, deps.Groups
	meetsService, exportsService, performanceService := deps.Meets, deps.Exports, deps.Performance
	billingService, stripeClient, commsService := deps.Billing, deps.Stripe, deps.Comms
	auditService, complianceService, documentsService := deps.Audit, deps.Compliance, deps.Documents
	trainingService, workerBackend, kcClient := deps.Training, deps.Workers, deps.Keycloak
	if logger == nil {
		logger = slog.Default()
	}
	appEnv := effectiveAppEnv(cfg)
	allowDevSessionIssue := cfg.Auth.AllowDevSessionIssue
	if !allowDevSessionIssue && isExplicitDevelopmentEnv(appEnv) {
		allowDevSessionIssue = true
	}

	router := chi.NewRouter()

	// Initialise audit service first — it is injected into multiple handlers.
	if auditService == nil {
		auditService = audit.NewService()
	}
	objectStore, err := newObjectStorageClient(cfg.Storage.ClientConfig())
	if err != nil {
		logger.Error("object storage init failed; falling back to stub storage", "error", err)
		objectStore = storage.NewStubStorage(cfg.Storage.BaseURL())
	}
	// With the stub in local dev (no or unreachable S3), serve presigned URLs
	// from the API itself so uploads and downloads work without MinIO (see
	// devstorage.go).
	var devStorage *storage.StubStorage
	if _, isStub := objectStore.(*storage.StubStorage); isStub && seedsLocalDevFixtures(cfg.AppEnv) {
		devStorage = storage.NewStubStorage(devStoragePath)
		objectStore = devStorage
	}

	// Open a pgxpool when DatabaseURL is configured. The pool is used to back
	// all service repositories; a nil pool falls back to in-memory only.
	dbPool := openValidatedDBPool(context.Background(), cfg, logger)
	auditService.SetPool(dbPool)

	if authService == nil {
		authService = authn.NewService(authn.Config{
			EnableDevStubTokens: cfg.Auth.EnableDevStubTokens,
			SeedLocalUsers:      isExplicitDevelopmentEnv(appEnv),
			Logger:              logger,
			OIDC: authn.OIDCConfig{
				AuthorizeURL:          cfg.Auth.OIDC.AuthorizeURL,
				TokenURL:              cfg.Auth.OIDC.TokenURL,
				JWKSURL:               cfg.Auth.OIDC.JWKSURL,
				EndSessionURL:         cfg.Auth.OIDC.EndSessionURL,
				RedirectURL:           cfg.Auth.OIDC.RedirectURL,
				PostLogoutRedirectURL: cfg.Auth.OIDC.PostLogoutRedirectURL,
				ClientID:              cfg.Auth.OIDC.ClientID,
				ClientSecret:          cfg.Auth.OIDC.ClientSecret,
				ExpectedIssuer:        cfg.Auth.OIDC.ExpectedIssuer,
				StateSigningKey:       cfg.Auth.OIDC.StateSigningKey,
				HTTPTimeout:           cfg.Auth.OIDC.HTTPTimeout,
			},
		})
	}
	// Persist MFA factors to DB when a pool is available so they survive restarts.
	authService.SetPool(dbPool)
	authHandlers := handlers.NewAuthHandlersWithAudit(authService, auditService, allowDevSessionIssue)
	authHandlers.SetSessionCookies(appmiddleware.SessionCookies{
		// Defence in depth: non-Secure cookies are honoured only in explicit dev-like envs.
		Insecure:     cfg.Auth.AllowInsecureSessionCookies && isExplicitDevelopmentEnv(appEnv),
		NewCSRFToken: cfg.Auth.CSRFTokenFactory,
	})
	if membershipService == nil {
		membershipService = members.NewServiceWithPool(dbPool)
	}
	if cfg.Auth.EnableDevStubTokens && seedsLocalDevFixtures(cfg.AppEnv) {
		// Dev stub parent/swimmer principals are pinned to a fixed household;
		// make sure it exists so household-scoped portal pages resolve.
		if err := membershipService.EnsureSeedHousehold(members.Household{
			ID:            authn.LocalDevHouseholdID,
			OrgID:         authn.LocalDevOrgID,
			HouseholdName: "Dev Household",
			PrimaryEmail:  "parent@dare.local",
			CreatedBy:     authn.LocalDevAdminUserID,
			UpdatedBy:     authn.LocalDevAdminUserID,
		}, authn.LocalDevParentUserID); err != nil {
			logger.Warn("local dev household seed failed", "error", err)
		}
	}
	membershipHandlers := handlers.NewMembershipHandlersWithAudit(membershipService, auditService)
	if groupsService == nil {
		groupsService = groups.NewServiceWithPool(dbPool)
	}
	groupsHandlers := handlers.NewGroupsHandlers(groupsService)
	groupsHandlers.SetClientAddressResolver(appmiddleware.NewClientAddressResolver(cfg.Security.RateLimit.TrustedProxyCIDRs))
	if meetsService == nil {
		meetsService = meets.NewServiceWithPool(dbPool)
	}
	meetsHandlers := handlers.NewMeetsHandlersWithAudit(meetsService, auditService)
	if exportsService == nil {
		exportsService = exports.NewServiceWithPool(dbPool, objectStore)
	}
	orgSettingsService := orgsettings.NewServiceWithPool(dbPool)
	exportsHandlers := handlers.NewExportsHandlersWithAudit(exportsService, meetsService, membershipService, orgSettingsService, auditService)
	if documentsService == nil {
		documentsService = documents.NewServiceWithPool(dbPool)
	}
	if performanceService == nil {
		performanceService = performance.NewServiceWithPool(dbPool)
	}
	// Wire meets service into performance service so that best-time recomputes
	// automatically back-fill seed times on future draft/submitted entries.
	performanceService.SetEntrySeedTimeUpdater(meetsService)
	// Wire members service as the household checker so that per-swimmer
	// performance reads enforce parent household scoping at the service layer.
	performanceService.SetHouseholdChecker(membershipService)
	// Swimmer/meet facts for team records, top times, relays and progress reports.
	performanceService.SetDirectory(newPerformanceDirectory(membershipService, meetsService))
	performanceHandlers := handlers.NewPerformanceHandlersWithAuditAndContext(
		performanceService,
		membershipService,
		groupsService,
		meetsService,
		documentsService,
		auditService,
	)
	if billingService == nil {
		billingService = billing.NewServiceWithPool(dbPool)
	}
	stripeClient = resolveStripeClient(cfg, logger, appEnv, stripeClient)
	billingHandlers := handlers.NewBillingHandlersWithAuditAndStripe(billingService, auditService, stripeClient)
	if metrics == nil {
		metrics = telemetry.NewMetrics()
	}
	stripeHandlers := handlers.NewStripeHandlers(
		stripeClient,
		billingService,
		cfg.Stripe.WebhookSecret,
		cfg.Stripe.Currency,
		cfg.Stripe.ReconciliationAlertThresholdCents,
		logger,
	)
	stripeHandlers.SetAllowedReturnOrigins(cfg.AllowedOrigins)
	stripeHandlers.SetMetrics(metrics)
	// Apply a tighter per-IP rate limiter to cost-driving Stripe write routes.
	stripeHandlers.SetWriteLimiter(appmiddleware.NewRateLimiter(
		cfg.Security.RateLimit.StripeRequests,
		cfg.Security.RateLimit.StripeWindow,
		cfg.Security.RateLimit.TrustedProxyCIDRs,
	))
	var postmarkClient postmark.Client
	if commsService == nil {
		postmarkClient = resolvePostmarkClient(cfg, logger, appEnv)
		if dbPool != nil {
			commsService = comms.NewServiceWithPool(dbPool)
			commsService.SetPostmarkClient(postmarkClient)
		} else {
			commsService = comms.NewServiceWithClient(postmarkClient)
		}
	} else {
		postmarkClient = commsService.PostmarkClient()
	}
	performanceHandlers.SetCommsService(commsService) // "new team record" announcement drafts
	commsHandlers := handlers.NewCommsHandlersWithAuditAndWebhookAuth(
		commsService,
		auditService,
		membershipService,
		groupsService,
		cfg.Postmark.WebhookAuthHeaderName,
		cfg.Postmark.WebhookAuthHeaderValue,
		cfg.Postmark.WebhookBasicUser,
		cfg.Postmark.WebhookBasicPass,
		appEnv,
	)
	commsHandlers.SetStorage(objectStore)
	commsService.SetStorage(objectStore)
	if strings.TrimSpace(cfg.Worker.RedisURL) != "" {
		if broadcaster, err := comms.NewMessageBroadcaster(cfg.Worker.RedisURL, logger); err == nil {
			commsService.SetBroadcaster(broadcaster)
		} else {
			logger.Warn("comms: realtime broadcaster unavailable, message streaming disabled", "err", err)
		}
	}
	auditHandlers := handlers.NewAuditHandlers(auditService)
	if complianceService == nil {
		complianceService = compliance.NewServiceWithPool(dbPool, cfg.Compliance.ExportTTLDays, cfg.Compliance.DeletionReviewDays)
	}
	complianceHandlers := handlers.NewComplianceHandlers(complianceService, auditService)
	documentHandlers := handlers.NewDocumentHandlers(documentsService, auditService, objectStore)
	if trainingService == nil {
		trainingService = training.NewServiceWithPool(dbPool)
	}
	trainingHandlers := handlers.NewTrainingHandlersWithMembers(trainingService, membershipService, groupsService, auditService)
	var seasonsService *seasons.Service
	if dbPool != nil {
		seasonsService = seasons.NewServiceWithPool(dbPool)
	} else {
		seasonsService = seasons.NewService()
	}
	seasonsHandlers := handlers.NewSeasonsHandlers(seasonsService, auditService)
	engagement := buildEngagementHandlers(dbPool, logger, membershipService, groupsService, meetsService, trainingService, billingService, auditService)
	trainingHandlers.SetSeasonsService(seasonsService)
	orgSettingsHandlers := handlers.NewOrgSettingsHandlers(orgSettingsService, auditService)
	publicSiteHandlers := buildPublicSiteHandlers(cfg, orgSettingsService, commsService, meetsService, engagement.eventsService, performanceService, kcClient, nil)

	kcClient = resolveKeycloakClient(cfg, logger, kcClient)
	userHandlers := handlers.NewUserHandlersWithMembers(kcClient, auditService, membershipService)

	wireCrossDomainHooks(logger, authService, membershipService, groupsService, meetsService)

	billingService.SetRosterSource(newBillingRosterSource(membershipService, groupsService))
	reportsService := reports.NewService(membershipService, groupsService, meetsService, billingService)
	reportsService.SetStorage(objectStore)
	reportsHandlers := handlers.NewReportsHandlers(reportsService, auditService)
	settingsHandlers := handlers.NewSettingsHandlers(settingsSummary(cfg))
	runtimeRepo := runtimepersist.NewRepository(cfg.DatabaseURL)
	runtimeParticipants := buildRuntimeParticipants(
		membershipService,
		groupsService,
		meetsService,
		exportsService,
		performanceService,
		billingService,
		commsService,
		complianceService,
		reportsService,
	)
	// Skip snapshot hydration when a DB pool backs these services: each service
	// already loaded canonical, current state from Postgres above, and restoring
	// a legacy snapshot here would silently overwrite it with stale data (the
	// snapshot bridge exists only for pre-DB in-memory mode).
	if dbPool == nil {
		loadRuntimeSnapshots(logger, runtimeRepo, runtimeParticipants)
	}

	// ── Workers ────────────────────────────────────────────────────────────────
	if isNilWorkerBackend(workerBackend) {
		workerServer, _ := BuildWorkersWithMetrics(cfg, logger, metrics, exportsService, reportsService, billingService, stripeClient, commsService, complianceService, membershipService, documentsService, objectStore, kcClient)
		workerServer.Start()
		workerBackend = workerServer
	} else {
		registerWorkerHandlers(withJobMetrics(workerBackend, metrics), logger, exportsService, reportsService, billingService, stripeClient, commsService, complianceService, membershipService, documentsService, objectStore, kcClient, cfg.Stripe.ReconciliationAlertThresholdCents)
	}
	readinessChecker := buildReadinessChecker(cfg, dbPool, objectStore, stripeClient, postmarkClient, workerBackend, kcClient)
	exportsService.SetQueue(workerBackend)
	complianceHandlers.SetJobQueue(workerBackend)
	jobsHandlers := handlers.NewJobsHandlers(workerBackend)
	volunteerHandlers, volunteerService := wireVolunteer(volunteerDeps{
		pool: dbPool, logger: logger, audit: auditService, members: membershipService, meets: meetsService,
		seasons: seasonsService, billing: billingService, postmark: postmarkClient, workerBack: workerBackend,
	})
	// Right-of-access export: members, groups and billing are read directly;
	// every other domain holding household records contributes via a source.
	exportSources := buildHouseholdExportSources(householdExportDeps{
		meets: meetsService, volunteer: volunteerService, wellness: engagement.wellnessService,
		events: engagement.eventsService, documents: documentsService, comms: commsService,
	})
	complianceService.SetHouseholdDataCollector(func(orgID, householdID string) (map[string]any, error) {
		return collectHouseholdExportData(orgID, householdID, membershipService, groupsService, billingService, exportSources...)
	})

	router.Use(chimiddleware.RequestID)
	router.Use(appmiddleware.TrustedProxy(cfg.Security.RateLimit.TrustedProxyCIDRs))
	router.Use(appmiddleware.OTelTracing(cfg.Telemetry.ServiceName))
	router.Use(appmiddleware.PrometheusMetrics(metrics))
	router.Use(appmiddleware.StructuredLogger(logger))
	router.Use(appmiddleware.Recoverer(logger))
	router.Use(appmiddleware.SecurityHeaders)
	router.Use(appmiddleware.NewRateLimiter(cfg.Security.RateLimit.GlobalRequests, cfg.Security.RateLimit.GlobalWindow, cfg.Security.RateLimit.TrustedProxyCIDRs))
	// Like hydration above, the legacy snapshot mirror only applies to pre-DB
	// in-memory mode; with a pool every service already writes through to
	// Postgres, and re-serialising all state via psql per mutation is waste.
	snapshotRepo := runtimeRepo
	if dbPool != nil {
		snapshotRepo = nil
	}
	router.Use(runtimePersistenceMiddleware(logger, snapshotRepo, runtimeParticipants))
	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   allowedCORSHeaders(cfg),
		ExposedHeaders:   []string{"X-Request-Id"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	if exposePrometheus {
		router.Get("/metrics", metrics.Handler().ServeHTTP)
	}

	router.Route("/api/v1", func(api chi.Router) {
		api.Route("/health", func(healthRouter chi.Router) {
			handlers.RegisterHealthRoutes(healthRouter, readinessChecker)
		})
		api.Get("/openapi", handlers.OpenAPIStub)

		// Public routes: no auth middleware required.
		if devStorage != nil {
			api.Handle("/dev-storage/*", devStorageHandler(devStorage))
		}
		stripeHandlers.RegisterPublicRoutes(api)
		commsHandlers.RegisterPublicRoutes(api)
		userHandlers.RegisterPublicRoutes(api)
		engagement.clubEvents.RegisterPublicRoutes(api)
		publicSiteHandlers.RegisterPublicRoutes(api)

		api.Route("/auth", func(auth chi.Router) {
			// The strict auth limiter guards credential-bearing writes (OIDC
			// callback, session issue/refresh, MFA challenge/enroll). Reads such
			// as GET /auth/session run on every page load and stay under the
			// global limiter, or a club's shared NAT would lock families out.
			authLimiter := appmiddleware.NewRateLimiter(cfg.Security.RateLimit.AuthRequests, cfg.Security.RateLimit.AuthWindow, cfg.Security.RateLimit.TrustedProxyCIDRs)
			auth.Use(func(next http.Handler) http.Handler {
				limited := authLimiter(next)
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.Method {
					case http.MethodGet, http.MethodHead, http.MethodOptions:
						next.ServeHTTP(w, r)
					default:
						limited.ServeHTTP(w, r)
					}
				})
			})
			// OIDC endpoints are exempt from CSRF: the signed state parameter
			// already provides CSRF protection for the authorization code flow.
			auth.Group(func(oidc chi.Router) {
				oidc.Get("/oidc/authorize", authHandlers.OIDCAuthorize)
				oidc.Post("/oidc/callback", authHandlers.OIDCCallback)
			})
			// Logout is intentionally CSRF-exempt because it only terminates
			// browser session state (no privilege gain or data mutation).
			auth.Group(func(noCSRF chi.Router) {
				authHandlers.RegisterNoCSRFRoutes(noCSRF)
			})
			// All other auth endpoints require CSRF protection.
			auth.Group(func(csrf chi.Router) {
				csrf.Use(appmiddleware.CSRFProtection(cfg.Security.CSRFHeaderName))
				authHandlers.RegisterPublicRoutes(csrf)
				csrf.Group(func(protected chi.Router) {
					protected.Use(appmiddleware.Auth(authService))
					protected.Use(appmiddleware.RequireOrgScope)
					authHandlers.RegisterProtectedRoutes(protected)
				})
			})
		})

		api.Group(func(protected chi.Router) {
			protected.Use(appmiddleware.CSRFProtection(cfg.Security.CSRFHeaderName))
			protected.Use(appmiddleware.Auth(authService))
			protected.Use(appmiddleware.RequireOrgScope)
			protected.Use(appmiddleware.RejectImpersonationHeaders())
			protected.With(appmiddleware.RBACStub("admin", "coach")).Get("/authz/ping", handlers.AuthzPing)

			protected.Route("/members", func(membersRouter chi.Router) {
				membershipHandlers.RegisterProtectedRoutes(membersRouter)
			})

			groupsHandlers.RegisterRoutes(protected)
			meetsHandlers.RegisterRoutes(protected)
			exportsHandlers.RegisterRoutes(protected)
			performanceHandlers.RegisterRoutes(protected)
			billingHandlers.RegisterRoutes(protected)
			billingHandlers.RegisterFinanceRoutes(protected)
			stripeHandlers.RegisterProtectedRoutes(protected)
			commsHandlers.RegisterRoutes(protected)
			auditHandlers.RegisterRoutes(protected)
			complianceHandlers.RegisterRoutes(protected)
			documentHandlers.RegisterRoutes(protected)
			trainingHandlers.RegisterRoutes(protected)
			seasonsHandlers.RegisterRoutes(protected)
			engagement.clubEvents.RegisterRoutes(protected)
			engagement.wellness.RegisterRoutes(protected)
			engagement.attendanceSummary.RegisterRoutes(protected)
			volunteerHandlers.RegisterRoutes(protected)
			orgSettingsHandlers.RegisterRoutes(protected)
			reportsHandlers.RegisterRoutes(protected)
			jobsHandlers.RegisterRoutes(protected)
			settingsHandlers.RegisterRoutes(protected)
			userHandlers.RegisterRoutes(protected)
		})
	})

	return router
}

func allowedCORSHeaders(cfg Config) []string {
	csrfHeaderName := strings.TrimSpace(cfg.Security.CSRFHeaderName)
	if csrfHeaderName == "" {
		csrfHeaderName = "X-CSRF-Token"
	}

	headers := []string{"Accept", "Authorization", "Content-Type", "X-Org-Id", "X-Request-Id"}
	for _, header := range headers {
		if strings.EqualFold(header, csrfHeaderName) {
			return headers
		}
	}
	return append(headers, csrfHeaderName)
}

// BuildWorkers creates and configures the worker server and scheduler without
// starting either. Callers must invoke ws.Start() and sched.Start() and call
// the corresponding Stop() methods on shutdown.
func BuildWorkers(cfg Config, logger *slog.Logger, exportsService *exports.Service, reportsService *reports.Service, billingService *billing.Service, stripeClient stripelib.Client, commsService *comms.Service, complianceService *compliance.Service, membershipService *members.Service, documentsService *documents.Service, objectStore storage.ObjectStorage, kcClient keycloak.Client) (*workers.Server, *workers.Scheduler) {
	return BuildWorkersWithMetrics(cfg, logger, nil, exportsService, reportsService, billingService, stripeClient, commsService, complianceService, membershipService, documentsService, objectStore, kcClient)
}

// BuildWorkersWithMetrics is BuildWorkers that also records job executions into
// metrics (nil disables recording).
func BuildWorkersWithMetrics(cfg Config, logger *slog.Logger, metrics *telemetry.Metrics, exportsService *exports.Service, reportsService *reports.Service, billingService *billing.Service, stripeClient stripelib.Client, commsService *comms.Service, complianceService *compliance.Service, membershipService *members.Service, documentsService *documents.Service, objectStore storage.ObjectStorage, kcClient keycloak.Client) (*workers.Server, *workers.Scheduler) {
	if logger == nil {
		logger = slog.Default()
	}
	ws := workers.NewServer(workers.ServerConfig{
		Concurrency: cfg.Worker.ConcurrencyPerQueue,
		Logger:      logger,
	})
	registerWorkerHandlers(withJobMetrics(ws, metrics), logger, exportsService, reportsService, billingService, stripeClient, commsService, complianceService, membershipService, documentsService, objectStore, kcClient, cfg.Stripe.ReconciliationAlertThresholdCents)

	sched := workers.NewScheduler(ws, logger)
	registerScheduledJobs(sched, cfg)

	return ws, sched
}

// RegisterDurableScheduledJobs wires the configured periodic jobs into the
// Redis-backed durable backend used by the dedicated worker process.
func RegisterDurableScheduledJobs(backend *workers.RedisBackend, cfg Config) {
	if backend == nil {
		return
	}
	registerScheduledJobs(backend, cfg)
}

// registerScheduledJobs wires periodic jobs into the scheduler.
// Intervals are parsed from config; a zero or invalid duration disables the job.
type scheduledJobRegistrar interface {
	Register(workers.ScheduledJob)
}

type workerHandlerRegistrar interface {
	RegisterHandler(taskType string, h workers.Handler)
}

func registerWorkerHandlers(registrar workerHandlerRegistrar, logger *slog.Logger, exportsService *exports.Service, reportsService *reports.Service, billingService *billing.Service, stripeClient stripelib.Client, commsService *comms.Service, complianceService *compliance.Service, membershipService *members.Service, documentsService *documents.Service, objectStore storage.ObjectStorage, kcClient keycloak.Client, reconciliationAlertThresholdCents int) {
	if registrar == nil {
		return
	}
	registrar.RegisterHandler(workers.TypeGenerateExport, jobhandlers.GenerateExportHandler(logger, exportsService))
	registrar.RegisterHandler(workers.TypeBillingReconcile, jobhandlers.BillingReconcileHandler(logger, billingService, stripeClient, reconciliationAlertThresholdCents))
	registrar.RegisterHandler(workers.TypeNightlyIntegrity, jobhandlers.NightlyIntegrityHandler(logger, billingService))
	registrar.RegisterHandler(workers.TypeBillingRecurringRun, jobhandlers.BillingRecurringRunHandler(logger, billingService))
	registrar.RegisterHandler(workers.TypeBillingLateFees, jobhandlers.BillingLateFeesHandler(logger, billingService))
	registrar.RegisterHandler(workers.TypePurgeExpiredFiles, jobhandlers.PurgeExpiredFilesHandler(logger, exportsService, reportsService))
	registrar.RegisterHandler(workers.TypeProcessDataExport, jobhandlers.ProcessDataExportHandler(logger, complianceService, objectStore))
	registrar.RegisterHandler(workers.TypeProcessDataDeletion, jobhandlers.ProcessDataDeletionHandler(logger, complianceService, membershipService, commsService, documentsService, objectStore, kcClient))
}

func registerScheduledJobs(s scheduledJobRegistrar, cfg Config) {
	type entry struct {
		name      string
		taskType  string
		queue     string
		interval  string
		orgScoped bool
	}
	jobs := []entry{
		{
			name:      "billing-reconcile",
			taskType:  workers.TypeBillingReconcile,
			queue:     workers.QueueReconcile,
			interval:  cfg.Worker.ReconcileInterval,
			orgScoped: true,
		},
		{
			name:      "nightly-integrity",
			taskType:  workers.TypeNightlyIntegrity,
			queue:     workers.QueueBilling,
			interval:  cfg.Worker.IntegrityInterval,
			orgScoped: true,
		},
		{
			name:      "billing-recurring-run",
			taskType:  workers.TypeBillingRecurringRun,
			queue:     workers.QueueBilling,
			interval:  cfg.Worker.BillingAutomationInterval,
			orgScoped: true,
		},
		{
			name:      "billing-late-fees",
			taskType:  workers.TypeBillingLateFees,
			queue:     workers.QueueBilling,
			interval:  cfg.Worker.BillingAutomationInterval,
			orgScoped: true,
		},
		{
			name:     "purge-expired-files",
			taskType: workers.TypePurgeExpiredFiles,
			queue:    workers.QueueDefault,
			interval: cfg.Worker.CleanupInterval,
		},
	}
	for _, j := range jobs {
		d := parseDurationSafe(j.interval)
		if d <= 0 {
			continue
		}
		if j.orgScoped {
			for _, orgID := range scheduledOrgIDs(cfg) {
				s.Register(workers.ScheduledJob{
					Name:     j.name + ":" + orgID,
					TaskType: j.taskType,
					Queue:    j.queue,
					Interval: d,
					Payload:  map[string]string{"triggered_by": "scheduler", "org_id": orgID},
				})
			}
			continue
		}
		s.Register(workers.ScheduledJob{
			Name:     j.name,
			TaskType: j.taskType,
			Queue:    j.queue,
			Interval: d,
			Payload:  map[string]string{"triggered_by": "scheduler"},
		})
	}
}

func scheduledOrgIDs(cfg Config) []string {
	seen := make(map[string]struct{}, len(cfg.Worker.ScheduledOrgIDs))
	orgIDs := make([]string, 0, len(cfg.Worker.ScheduledOrgIDs))
	for _, orgID := range cfg.Worker.ScheduledOrgIDs {
		orgID = strings.TrimSpace(orgID)
		if orgID == "" {
			continue
		}
		if _, ok := seen[orgID]; ok {
			continue
		}
		seen[orgID] = struct{}{}
		orgIDs = append(orgIDs, orgID)
	}
	appEnv := effectiveAppEnv(cfg)
	if len(orgIDs) == 0 && allowsProviderStubFallback(appEnv) {
		return []string{"org-demo"}
	}
	return orgIDs
}

// parseDurationSafe parses a Go duration string and returns 0 on any error.
func parseDurationSafe(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d
}

// settingsSummary is the non-secret configuration snapshot shown on the admin settings screen.
func settingsSummary(cfg Config) map[string]any {
	return map[string]any{
		"app_env": cfg.AppEnv,
		"auth": map[string]any{
			"enable_dev_stub_tokens":  cfg.Auth.EnableDevStubTokens,
			"allow_dev_session_issue": cfg.Auth.AllowDevSessionIssue,
			"oidc_client_id":          cfg.Auth.OIDC.ClientID,
			"oidc_redirect_url":       cfg.Auth.OIDC.RedirectURL,
			"oidc_expected_issuer":    cfg.Auth.OIDC.ExpectedIssuer,
		},
		"workers": map[string]any{
			"redis_configured":      cfg.Worker.RedisURL != "",
			"concurrency_per_queue": cfg.Worker.ConcurrencyPerQueue,
			"reconcile_interval":    cfg.Worker.ReconcileInterval,
			"integrity_interval":    cfg.Worker.IntegrityInterval,
			"cleanup_interval":      cfg.Worker.CleanupInterval,
		},
		"compliance": map[string]any{
			"audit_retention_days":     cfg.Compliance.AuditRetentionDays,
			"export_ttl_days":          cfg.Compliance.ExportTTLDays,
			"deletion_review_days":     cfg.Compliance.DeletionReviewDays,
			"csrf_header_name":         cfg.Security.CSRFHeaderName,
			"impersonation_enabled":    cfg.Security.Impersonation.Enabled,
			"global_rate_limit":        cfg.Security.RateLimit.GlobalRequests,
			"global_rate_limit_window": cfg.Security.RateLimit.GlobalWindow.String(),
			"auth_rate_limit":          cfg.Security.RateLimit.AuthRequests,
			"auth_rate_limit_window":   cfg.Security.RateLimit.AuthWindow.String(),
		},
		"storage": map[string]any{
			"endpoint": cfg.Storage.Endpoint,
			"bucket":   cfg.Storage.Bucket,
			"use_ssl":  cfg.Storage.UseSSL,
			"region":   cfg.Storage.Region,
		},
		"stripe": map[string]any{
			"currency":                             cfg.Stripe.Currency,
			"reconciliation_alert_threshold_cents": cfg.Stripe.ReconciliationAlertThresholdCents,
		},
		"postmark": map[string]any{
			"from_name":    cfg.Postmark.FromName,
			"from_email":   cfg.Postmark.FromEmail,
			"webhook_auth": cfg.Postmark.WebhookAuthHeaderName != "" || cfg.Postmark.WebhookBasicUser != "" || cfg.Postmark.WebhookBasicPass != "",
		},
		"telemetry": map[string]any{
			"service_name":    cfg.Telemetry.ServiceName,
			"service_version": cfg.Telemetry.ServiceVersion,
			"otlp_endpoint":   cfg.Telemetry.OTLPEndpoint,
			"sample_rate":     cfg.Telemetry.SampleRate,
			"metrics_addr":    cfg.Telemetry.MetricsAddr,
		},
	}
}

// resolveKeycloakClient prefers an injected client, then a real admin client
// built from config, then (dev/test only) an in-memory stub; any other
// environment gets an unavailable client that fails every call loudly.
func resolveKeycloakClient(cfg Config, logger *slog.Logger, injected keycloak.Client) keycloak.Client {
	if injected != nil {
		return injected
	}
	realKC, kcErr := keycloak.NewAdminClient(keycloak.AdminConfig{
		AdminURL:     cfg.Keycloak.AdminURL,
		Realm:        cfg.Keycloak.Realm,
		ClientID:     cfg.Keycloak.ClientID,
		ClientSecret: cfg.Keycloak.ClientSecret,
	})
	if kcErr == nil {
		return realKC
	}
	if allowsKeycloakAdminStubFallback(cfg) {
		if errors.Is(kcErr, keycloak.ErrInvalidConfig) {
			logger.Info("keycloak admin client not configured; using stub client (dev mode)")
		} else {
			logger.Info("keycloak init skipped; falling back to stub client (dev mode)", "error", kcErr)
		}
		stubKC := keycloak.NewStubClient()
		seedLocalDevKeycloakStub(stubKC)
		return stubKC
	}
	logger.Error("keycloak init failed; admin user management is unavailable", "error", kcErr)
	return keycloak.NewUnavailableClient(fmt.Errorf("keycloak admin client unavailable: %w", kcErr))
}

// wireCrossDomainHooks connects services that must react to each other's
// events without importing one another (roster, meets, groups, auth lookups).
func wireCrossDomainHooks(logger *slog.Logger, authService *authn.Service, membershipService *members.Service, groupsService *groups.Service, meetsService *meets.Service) {
	// Wire household lookup: after OIDC login, attach the session's household_id
	// from the keycloak_user_id stored on the household record.
	// Wire swimmer gender/DOB lookup so the meets domain can gate entry creation
	// on event eligibility (gender + age) without importing the members package.
	if groupsService != nil && membershipService != nil {
		groupsService.SetRegistrationActivatedHook(func(reg groups.Registration, groupCode, actorUserID string) {
			// Approval is staff-only, but the roster write runs as a system
			// admin action on the approver's behalf.
			actor := &appmiddleware.Principal{UserID: actorUserID, OrgID: reg.OrgID, Roles: []string{"admin"}}
			if _, err := membershipService.AssignSwimmerToGroup(actor, reg.OrgID, reg.SwimmerID, members.AssignSwimmerGroupInput{
				GroupCode: groupCode,
				IsPrimary: !membershipService.HasPrimaryGroup(reg.OrgID, reg.SwimmerID),
			}); err != nil {
				logger.Warn("add registered swimmer to group roster failed", "registration_id", reg.ID, "error", err)
			}
		})
	}
	if membershipService != nil {
		membershipService.SetSwimmerCreatedHook(func(swimmer members.Swimmer) {
			// Keep the groups scope index current in in-memory mode (DB mode
			// reloads it from the shared swimmers table on every sync).
			if groupsService != nil {
				groupsService.SeedSwimmer(swimmer.OrgID, swimmer.ID, swimmer.HouseholdID, swimmer.DateOfBirth, swimmer.Gender, swimmer.RegistrationStatus)
			}
			if meetsService != nil {
				if _, err := meetsService.InviteSwimmerToOpenMeets(swimmer.OrgID, swimmer.ID, swimmer.HouseholdID, swimmer.CreatedBy); err != nil {
					logger.Warn("invite new swimmer to open meets failed", "swimmer_id", swimmer.ID, "error", err)
				}
			}
		})
	}
	if meetsService != nil && membershipService != nil {
		meetsService.SetSwimmerEligibilityLookup(func(orgID, swimmerID string) (meets.SwimmerEligibilityView, bool) {
			info, ok := membershipService.SwimmerEligibility(orgID, swimmerID)
			if !ok {
				return meets.SwimmerEligibilityView{}, false
			}
			return meets.SwimmerEligibilityView{Gender: info.Gender, DateOfBirth: info.DateOfBirth}, true
		})
	}

	authService.SetHouseholdLookup(func(keycloakUserID string) string {
		if membershipService == nil {
			return ""
		}
		h, found := membershipService.GetHouseholdByKeycloakUserID(keycloakUserID)
		if !found {
			return ""
		}
		return h.ID
	})

	authService.SetSwimmerLookup(func(keycloakUserID string) string {
		if membershipService == nil {
			return ""
		}
		sw, found := membershipService.GetSwimmerByKeycloakUserID(keycloakUserID)
		if !found {
			return ""
		}
		return sw.ID
	})
}

// resolveStripeClient prefers an injected client, then the real SDK client,
// then (dev/test only) a stub; other environments get an unavailable client.
func resolveStripeClient(cfg Config, logger *slog.Logger, appEnv string, injected stripelib.Client) stripelib.Client {
	if injected != nil {
		return injected
	}
	realClient, err := stripelib.NewSDKClient(cfg.Stripe.SecretKey)
	if err == nil {
		return realClient
	}
	if allowsProviderStubFallback(appEnv) {
		logger.Info("stripe init skipped; falling back to stub client (dev mode)", "error", err)
		return stripelib.NewStubClient()
	}
	logger.Error("stripe init failed; provider unavailable in non-dev environment", "error", err)
	return unavailableStripeClient{err: fmt.Errorf("stripe provider unavailable: %w", err)}
}

// resolvePostmarkClient builds the real Postmark client when configured.
// Non-dev environments fail closed instead of silently downgrading to a stub.
func resolvePostmarkClient(cfg Config, logger *slog.Logger, appEnv string) postmark.Client {
	realClient, err := postmark.NewHTTPClient(postmark.Config{
		ServerToken: cfg.Postmark.ServerToken,
		FromName:    cfg.Postmark.FromName,
		FromEmail:   cfg.Postmark.FromEmail,
	})
	if err == nil {
		return realClient
	}
	if allowsProviderStubFallback(appEnv) {
		logger.Info("postmark init skipped; falling back to stub client (dev mode)", "error", err)
		return postmark.NewStubClient()
	}
	logger.Error("postmark init failed; provider unavailable in non-dev environment", "error", err)
	return unavailablePostmarkClient{err: fmt.Errorf("postmark provider unavailable: %w", err)}
}

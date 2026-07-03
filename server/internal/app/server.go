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

	"github.com/dareaquatics/aquify/meetfiles"
	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/authn"
	"github.com/dareaquatics/aquify/server/internal/app/billing"
	"github.com/dareaquatics/aquify/server/internal/app/comms"
	"github.com/dareaquatics/aquify/server/internal/app/compliance"
	"github.com/dareaquatics/aquify/server/internal/app/documents"
	"github.com/dareaquatics/aquify/server/internal/app/exports"
	"github.com/dareaquatics/aquify/server/internal/app/groups"
	"github.com/dareaquatics/aquify/server/internal/app/handlers"
	"github.com/dareaquatics/aquify/server/internal/app/imports"
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
	return NewRouterWithServices(cfg, logger, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
}

// NewRouterWithServicesAndMetrics constructs the HTTP router with a caller-
// supplied Prometheus registry. exposePrometheus controls whether the scrape
// endpoint itself is mounted on the returned router.
func NewRouterWithServicesAndMetrics(cfg Config, logger *slog.Logger, metrics *telemetry.Metrics, exposePrometheus bool, authService *authn.Service, membershipService *members.Service, groupsService *groups.Service, meetsService *meets.Service, importsService *imports.Service, exportsService *exports.Service, performanceService *performance.Service, billingService *billing.Service, stripeClient stripelib.Client, commsService *comms.Service, auditService *audit.Service, complianceService *compliance.Service, documentsService *documents.Service, trainingService *training.Service, workerBackend workers.Backend, kcClient keycloak.Client) http.Handler {
	return newRouterWithServices(cfg, logger, metrics, exposePrometheus, authService, membershipService, groupsService, meetsService, importsService, exportsService, performanceService, billingService, stripeClient, commsService, auditService, complianceService, documentsService, trainingService, workerBackend, kcClient)
}

func allowsProviderStubFallback(appEnv string) bool {
	return isExplicitDevelopmentEnv(appEnv)
}

func allowsKeycloakAdminStubFallback(cfg Config) bool {
	return allowsProviderStubFallback(cfg.AppEnv) && !requiresKeycloakAdminConfig(cfg)
}

func canonicalUserManagementRole(role string) string {
	switch strings.TrimSpace(strings.ToLower(role)) {
	case "read-only", "read_only_staff":
		return "read_only_staff"
	default:
		return strings.TrimSpace(strings.ToLower(role))
	}
}

func normalizedImportGender(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "male", "m":
		return "M"
	case "female", "f":
		return "F"
	case "mixed", "x":
		return "X"
	default:
		return strings.ToUpper(strings.TrimSpace(raw))
	}
}

func importStrokeFromMeetEvent(raw string, isRelay bool) meetfiles.Stroke {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "free", "freestyle":
		if isRelay {
			return meetfiles.StrokeFreestyleRelay
		}
		return meetfiles.StrokeFreestyle
	case "back", "backstroke":
		return meetfiles.StrokeBackstroke
	case "breast", "breaststroke":
		return meetfiles.StrokeBreaststroke
	case "fly", "butterfly":
		return meetfiles.StrokeButterfly
	case "im", "medley":
		if isRelay {
			return meetfiles.StrokeMedleyRelay
		}
		return meetfiles.StrokeIM
	default:
		return meetfiles.StrokeUnknown
	}
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

// NewRouterWithServices constructs the HTTP router and wires all services.
// workerBackend may be nil; when nil a new in-memory worker server is
// created, registered, and started automatically.
// kcClient may be nil; when nil a StubClient is used in dev/test and a real
// AdminClient is constructed from cfg.Keycloak in non-dev environments.
func NewRouterWithServices(cfg Config, logger *slog.Logger, authService *authn.Service, membershipService *members.Service, groupsService *groups.Service, meetsService *meets.Service, importsService *imports.Service, exportsService *exports.Service, performanceService *performance.Service, billingService *billing.Service, stripeClient stripelib.Client, commsService *comms.Service, auditService *audit.Service, complianceService *compliance.Service, documentsService *documents.Service, trainingService *training.Service, workerBackend workers.Backend, kcClient keycloak.Client) http.Handler {
	return newRouterWithServices(cfg, logger, telemetry.NewMetrics(), false, authService, membershipService, groupsService, meetsService, importsService, exportsService, performanceService, billingService, stripeClient, commsService, auditService, complianceService, documentsService, trainingService, workerBackend, kcClient)
}

func newRouterWithServices(cfg Config, logger *slog.Logger, metrics *telemetry.Metrics, exposePrometheus bool, authService *authn.Service, membershipService *members.Service, groupsService *groups.Service, meetsService *meets.Service, importsService *imports.Service, exportsService *exports.Service, performanceService *performance.Service, billingService *billing.Service, stripeClient stripelib.Client, commsService *comms.Service, auditService *audit.Service, complianceService *compliance.Service, documentsService *documents.Service, trainingService *training.Service, workerBackend workers.Backend, kcClient keycloak.Client) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	appEnv := strings.ToLower(strings.TrimSpace(cfg.AppEnv))
	if appEnv == "" {
		appEnv = "test"
	}
	allowDevSessionIssue := cfg.Auth.AllowDevSessionIssue
	if !allowDevSessionIssue && (appEnv == "development" || appEnv == "local" || appEnv == "test") {
		allowDevSessionIssue = true
	}

	router := chi.NewRouter()

	// Initialise audit service first — it is injected into multiple handlers.
	if auditService == nil {
		auditService = audit.NewService()
	}
	objectStore, err := newObjectStorageClient(storage.Config{
		Endpoint:  cfg.Storage.Endpoint,
		AccessKey: cfg.Storage.AccessKey,
		SecretKey: cfg.Storage.SecretKey,
		Bucket:    cfg.Storage.Bucket,
		UseSSL:    cfg.Storage.UseSSL,
		Region:    cfg.Storage.Region,
	})
	if err != nil {
		logger.Error("object storage init failed; falling back to stub storage", "error", err)
		objectStore = storage.NewStubStorage(cfg.Storage.BaseURL())
	}

	// Open a pgxpool when DatabaseURL is configured. The pool is used to back
	// all service repositories; a nil pool falls back to in-memory only.
	dbPool := openValidatedDBPool(context.Background(), cfg, logger)
	auditService.SetPool(dbPool)

	if authService == nil {
		authService = authn.NewService(authn.Config{
			EnableDevStubTokens: cfg.Auth.EnableDevStubTokens,
			SeedLocalUsers:      appEnv == "development" || appEnv == "local" || appEnv == "test",
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
	if membershipService == nil {
		if dbPool != nil {
			membershipService = members.NewServiceWithPool(dbPool)
		} else {
			membershipService = members.NewService()
		}
	}
	membershipHandlers := handlers.NewMembershipHandlersWithAudit(membershipService, auditService)
	if groupsService == nil {
		if dbPool != nil {
			groupsService = groups.NewServiceWithPool(dbPool)
		} else {
			groupsService = groups.NewService()
		}
	}
	groupsHandlers := handlers.NewGroupsHandlers(groupsService)
	if meetsService == nil {
		if dbPool != nil {
			meetsService = meets.NewServiceWithPool(dbPool)
		} else {
			meetsService = meets.NewService()
		}
	}
	meetsHandlers := handlers.NewMeetsHandlersWithAudit(meetsService, auditService)
	if importsService == nil {
		if dbPool != nil {
			importsService = imports.NewServiceWithPool(dbPool, objectStore)
		} else {
			importsService = imports.NewServiceWithStorage(objectStore)
		}
	}
	if cfg.Security.Uploads.MaxBytes > 0 || len(cfg.Security.Uploads.AllowedMIMEs) > 0 || cfg.Security.Uploads.RequireMalwareScanHook {
		importsService.ConfigureSecurity(imports.SecurityConfig{
			MaxBytes:               cfg.Security.Uploads.MaxBytes,
			AllowedMIMEs:           cfg.Security.Uploads.AllowedMIMEs,
			RequireMalwareScanHook: cfg.Security.Uploads.RequireMalwareScanHook,
		})
	}
	importsService.SetRosterLoader(func(orgID string) []meetfiles.RosterSwimmer {
		if membershipService == nil || strings.TrimSpace(orgID) == "" {
			return nil
		}
		principal := &appmiddleware.Principal{UserID: "system-imports", OrgID: orgID, Roles: []string{"admin"}}
		swimmers, err := membershipService.ListSwimmers(principal, orgID, members.ListSwimmersInput{})
		if err != nil {
			logger.Warn("imports: failed to load swimmer roster for matching", "org_id", orgID, "error", err)
			return nil
		}
		roster := make([]meetfiles.RosterSwimmer, 0, len(swimmers))
		for _, swimmer := range swimmers {
			var dob *time.Time
			if parsedDOB, err := time.Parse("2006-01-02", strings.TrimSpace(swimmer.DateOfBirth)); err == nil {
				parsedDOB = parsedDOB.UTC()
				dob = &parsedDOB
			}
			roster = append(roster, meetfiles.RosterSwimmer{
				ID:        swimmer.ID,
				USSNumber: strings.TrimSpace(swimmer.SwimmerExternalID),
				LastName:  strings.TrimSpace(swimmer.LastName),
				FirstName: strings.TrimSpace(swimmer.FirstName),
				DOB:       dob,
				Gender:    normalizedImportGender(swimmer.Gender),
			})
		}
		return roster
	})
	importsService.SetKnownEventsLoader(func(orgID, meetID string) []meetfiles.KnownEvent {
		if meetsService == nil || strings.TrimSpace(orgID) == "" || strings.TrimSpace(meetID) == "" {
			return nil
		}
		principal := &appmiddleware.Principal{UserID: "system-imports", OrgID: orgID, Roles: []string{"admin"}}
		events, err := meetsService.ListMeetEvents(principal, orgID, meetID)
		if err != nil {
			logger.Warn("imports: failed to load meet events for validation", "org_id", orgID, "meet_id", meetID, "error", err)
			return nil
		}
		known := make([]meetfiles.KnownEvent, 0, len(events))
		for _, event := range events {
			known = append(known, meetfiles.KnownEvent{
				EventNumber: strings.TrimSpace(event.EventNumber),
				Distance:    event.Distance,
				Stroke:      importStrokeFromMeetEvent(event.Stroke, event.IsRelay),
				EventGender: normalizedImportGender(event.Gender),
			})
		}
		return known
	})
	importsHandlers := handlers.NewImportsHandlersWithAudit(importsService, auditService)
	if exportsService == nil {
		if dbPool != nil {
			exportsService = exports.NewServiceWithPool(dbPool, objectStore)
		} else {
			exportsService = exports.NewServiceWithStorage(objectStore)
		}
	}
	var orgSettingsService *orgsettings.Service
	if dbPool != nil {
		orgSettingsService = orgsettings.NewServiceWithPool(dbPool)
	} else {
		orgSettingsService = orgsettings.NewService()
	}
	exportsHandlers := handlers.NewExportsHandlersWithMeetAccess(exportsService, meetsService, membershipService, orgSettingsService)
	if documentsService == nil {
		if dbPool != nil {
			documentsService = documents.NewServiceWithPool(dbPool)
		} else {
			documentsService = documents.NewService()
		}
	}
	if performanceService == nil {
		if dbPool != nil {
			performanceService = performance.NewServiceWithPool(dbPool)
		} else {
			performanceService = performance.NewService()
		}
	}
	// Wire meets service into performance service so that best-time recomputes
	// automatically back-fill seed times on future draft/submitted entries.
	performanceService.SetEntrySeedTimeUpdater(meetsService)
	// Wire members service as the household checker so that per-swimmer
	// performance reads enforce parent household scoping at the service layer.
	performanceService.SetHouseholdChecker(membershipService)
	performanceHandlers := handlers.NewPerformanceHandlersWithAuditAndContext(
		performanceService,
		membershipService,
		groupsService,
		meetsService,
		documentsService,
		auditService,
	)
	if billingService == nil {
		if dbPool != nil {
			billingService = billing.NewServiceWithPool(dbPool)
		} else {
			billingService = billing.NewService()
		}
	}
	if stripeClient == nil {
		realStripeClient, err := stripelib.NewSDKClient(cfg.Stripe.SecretKey)
		if err == nil {
			stripeClient = realStripeClient
		} else if allowsProviderStubFallback(appEnv) {
			logger.Info("stripe init skipped; falling back to stub client (dev mode)", "error", err)
			stripeClient = stripelib.NewStubClient()
		} else {
			logger.Error("stripe init failed; provider unavailable in non-dev environment", "error", err)
			stripeClient = unavailableStripeClient{err: fmt.Errorf("stripe provider unavailable: %w", err)}
		}
	}
	billingHandlers := handlers.NewBillingHandlersWithAuditAndStripe(billingService, auditService, stripeClient)
	stripeHandlers := handlers.NewStripeHandlers(
		stripeClient,
		billingService,
		cfg.Stripe.WebhookSecret,
		cfg.Stripe.Currency,
		cfg.Stripe.ReconciliationAlertThresholdCents,
		logger,
	)
	stripeHandlers.SetAllowedReturnOrigins(cfg.AllowedOrigins)
	// Apply a tighter per-IP rate limiter to cost-driving Stripe write routes.
	stripeHandlers.SetWriteLimiter(appmiddleware.NewRateLimiter(
		cfg.Security.RateLimit.StripeRequests,
		cfg.Security.RateLimit.StripeWindow,
		cfg.Security.RateLimit.TrustedProxyCIDRs,
	))
	var postmarkClient postmark.Client
	if commsService == nil {
		// Wire real Postmark client when configured. Non-dev environments must
		// fail closed instead of silently downgrading to a stub provider.
		realClient, err := postmark.NewHTTPClient(postmark.Config{
			ServerToken: cfg.Postmark.ServerToken,
			FromName:    cfg.Postmark.FromName,
			FromEmail:   cfg.Postmark.FromEmail,
		})
		if err == nil {
			postmarkClient = realClient
		} else if allowsProviderStubFallback(appEnv) {
			logger.Info("postmark init skipped; falling back to stub client (dev mode)", "error", err)
		} else {
			logger.Error("postmark init failed; provider unavailable in non-dev environment", "error", err)
			postmarkClient = unavailablePostmarkClient{err: fmt.Errorf("postmark provider unavailable: %w", err)}
		}
		if postmarkClient == nil {
			postmarkClient = postmark.NewStubClient()
		}
		if dbPool != nil {
			commsService = comms.NewServiceWithPool(dbPool)
			commsService.SetPostmarkClient(postmarkClient)
		} else {
			commsService = comms.NewServiceWithClient(postmarkClient)
		}
	} else {
		postmarkClient = commsService.PostmarkClient()
	}
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
	auditHandlers := handlers.NewAuditHandlers(auditService)
	if complianceService == nil {
		if dbPool != nil {
			complianceService = compliance.NewServiceWithPool(dbPool, cfg.Compliance.ExportTTLDays, cfg.Compliance.DeletionReviewDays)
		} else {
			complianceService = compliance.NewServiceWithConfig(cfg.Compliance.ExportTTLDays, cfg.Compliance.DeletionReviewDays)
		}
	}
	complianceHandlers := handlers.NewComplianceHandlers(complianceService, auditService)
	documentHandlers := handlers.NewDocumentHandlers(documentsService, auditService, objectStore)
	if trainingService == nil {
		if dbPool != nil {
			trainingService = training.NewServiceWithPool(dbPool)
		} else {
			trainingService = training.NewService()
		}
	}
	trainingHandlers := handlers.NewTrainingHandlersWithMembers(trainingService, membershipService, groupsService, auditService)
	var seasonsService *seasons.Service
	if dbPool != nil {
		seasonsService = seasons.NewServiceWithPool(dbPool)
	} else {
		seasonsService = seasons.NewService()
	}
	seasonsHandlers := handlers.NewSeasonsHandlers(seasonsService, auditService)
	orgSettingsHandlers := handlers.NewOrgSettingsHandlers(orgSettingsService, auditService)

	// Keycloak client — prefer injected, then real from config, then stub fallback.
	if kcClient == nil {
		realKC, kcErr := keycloak.NewAdminClient(keycloak.AdminConfig{
			AdminURL:     cfg.Keycloak.AdminURL,
			Realm:        cfg.Keycloak.Realm,
			ClientID:     cfg.Keycloak.ClientID,
			ClientSecret: cfg.Keycloak.ClientSecret,
		})
		if kcErr == nil {
			kcClient = realKC
		} else if allowsKeycloakAdminStubFallback(cfg) {
			if errors.Is(kcErr, keycloak.ErrInvalidConfig) {
				logger.Info("keycloak admin client not configured; using stub client (dev mode)")
			} else {
				logger.Info("keycloak init skipped; falling back to stub client (dev mode)", "error", kcErr)
			}
			stubKC := keycloak.NewStubClient()
			seedLocalDevKeycloakStub(stubKC)
			kcClient = stubKC
		} else {
			logger.Error("keycloak init failed; admin user management is unavailable", "error", kcErr)
			kcClient = keycloak.NewUnavailableClient(fmt.Errorf("keycloak admin client unavailable: %w", kcErr))
		}
	}
	userHandlers := handlers.NewUserHandlersWithMembers(kcClient, auditService, membershipService)

	// Wire household lookup: after OIDC login, attach the session's household_id
	// from the keycloak_user_id stored on the household record.
	// Wire swimmer gender/DOB lookup so the meets domain can gate entry creation
	// on event eligibility (gender + age) without importing the members package.
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

	reportsService := reports.NewService(membershipService, groupsService, meetsService, billingService)
	reportsService.SetStorage(objectStore)
	reportsHandlers := handlers.NewReportsHandlers(reportsService, auditService)
	settingsHandlers := handlers.NewSettingsHandlers(map[string]any{
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
	})
	runtimeRepo := runtimepersist.NewRepository(cfg.DatabaseURL)
	runtimeParticipants := buildRuntimeParticipants(
		membershipService,
		groupsService,
		meetsService,
		importsService,
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
		workerServer, _ := BuildWorkers(cfg, logger, importsService, exportsService, reportsService, billingService, stripeClient, commsService, complianceService, membershipService, objectStore)
		workerServer.Start()
		workerBackend = workerServer
	} else {
		registerWorkerHandlers(workerBackend, logger, importsService, exportsService, reportsService, billingService, stripeClient, commsService, complianceService, membershipService, objectStore, cfg.Stripe.ReconciliationAlertThresholdCents)
	}
	readinessChecker := buildReadinessChecker(cfg, dbPool, objectStore, stripeClient, postmarkClient, workerBackend, kcClient)
	exportsService.SetQueue(workerBackend)
	jobsHandlers := handlers.NewJobsHandlers(workerBackend)

	if metrics == nil {
		metrics = telemetry.NewMetrics()
	}

	router.Use(chimiddleware.RequestID)
	router.Use(appmiddleware.OTelTracing(cfg.Telemetry.ServiceName))
	router.Use(appmiddleware.PrometheusMetrics(metrics))
	router.Use(appmiddleware.StructuredLogger(logger))
	router.Use(appmiddleware.Recoverer(logger))
	router.Use(appmiddleware.SecurityHeaders)
	router.Use(appmiddleware.NewRateLimiter(cfg.Security.RateLimit.GlobalRequests, cfg.Security.RateLimit.GlobalWindow, cfg.Security.RateLimit.TrustedProxyCIDRs))
	router.Use(runtimePersistenceMiddleware(logger, runtimeRepo, runtimeParticipants))
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
		stripeHandlers.RegisterPublicRoutes(api)
		commsHandlers.RegisterPublicRoutes(api)
		userHandlers.RegisterPublicRoutes(api)

		api.Route("/auth", func(auth chi.Router) {
			auth.Use(appmiddleware.NewRateLimiter(cfg.Security.RateLimit.AuthRequests, cfg.Security.RateLimit.AuthWindow, cfg.Security.RateLimit.TrustedProxyCIDRs))
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
			protected.Use(appmiddleware.Auth(authService))
			protected.Use(appmiddleware.RequireOrgScope)
			protected.Use(appmiddleware.RejectImpersonationHeaders(cfg.Security.Impersonation.Enabled))
			protected.With(appmiddleware.RBACStub("owner", "admin", "coach")).Get("/authz/ping", handlers.AuthzPing)

			protected.Route("/members", func(membersRouter chi.Router) {
				membershipHandlers.RegisterProtectedRoutes(membersRouter)
			})

			groupsHandlers.RegisterRoutes(protected)
			meetsHandlers.RegisterRoutes(protected)
			importsHandlers.RegisterRoutes(protected)
			exportsHandlers.RegisterRoutes(protected)
			performanceHandlers.RegisterRoutes(protected)
			billingHandlers.RegisterRoutes(protected)
			stripeHandlers.RegisterProtectedRoutes(protected)
			commsHandlers.RegisterRoutes(protected)
			auditHandlers.RegisterRoutes(protected)
			complianceHandlers.RegisterRoutes(protected)
			documentHandlers.RegisterRoutes(protected)
			trainingHandlers.RegisterRoutes(protected)
			seasonsHandlers.RegisterRoutes(protected)
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
func BuildWorkers(cfg Config, logger *slog.Logger, importsService *imports.Service, exportsService *exports.Service, reportsService *reports.Service, billingService *billing.Service, stripeClient stripelib.Client, commsService *comms.Service, complianceService *compliance.Service, membershipService *members.Service, objectStore storage.ObjectStorage) (*workers.Server, *workers.Scheduler) {
	if logger == nil {
		logger = slog.Default()
	}
	ws := workers.NewServer(workers.ServerConfig{
		Concurrency: cfg.Worker.ConcurrencyPerQueue,
		Logger:      logger,
	})
	registerWorkerHandlers(ws, logger, importsService, exportsService, reportsService, billingService, stripeClient, commsService, complianceService, membershipService, objectStore, cfg.Stripe.ReconciliationAlertThresholdCents)

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

func registerWorkerHandlers(registrar workerHandlerRegistrar, logger *slog.Logger, importsService *imports.Service, exportsService *exports.Service, reportsService *reports.Service, billingService *billing.Service, stripeClient stripelib.Client, commsService *comms.Service, complianceService *compliance.Service, membershipService *members.Service, objectStore storage.ObjectStorage, reconciliationAlertThresholdCents int) {
	if registrar == nil {
		return
	}
	registrar.RegisterHandler(workers.TypeParseImport, jobhandlers.ParseImportHandler(logger, importsService))
	registrar.RegisterHandler(workers.TypeGenerateExport, jobhandlers.GenerateExportHandler(logger, exportsService))
	registrar.RegisterHandler(workers.TypeSendCampaignBatch, jobhandlers.SendCampaignBatchHandler(logger, commsService))
	registrar.RegisterHandler(workers.TypeBillingReconcile, jobhandlers.BillingReconcileHandler(logger, billingService, stripeClient, reconciliationAlertThresholdCents))
	registrar.RegisterHandler(workers.TypeNightlyIntegrity, jobhandlers.NightlyIntegrityHandler(logger, billingService))
	registrar.RegisterHandler(workers.TypePurgeExpiredFiles, jobhandlers.PurgeExpiredFilesHandler(logger, importsService, exportsService, reportsService))
	registrar.RegisterHandler(workers.TypeProcessDataExport, jobhandlers.ProcessDataExportHandler(logger, complianceService, objectStore))
	registrar.RegisterHandler(workers.TypeProcessDataDeletion, jobhandlers.ProcessDataDeletionHandler(logger, complianceService, membershipService))
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
	appEnv := strings.TrimSpace(cfg.AppEnv)
	if appEnv == "" {
		appEnv = "test"
	}
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

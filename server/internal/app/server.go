package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/authn"
	"github.com/dareaquatics/aquify/server/internal/app/billing"
	"github.com/dareaquatics/aquify/server/internal/app/comms"
	"github.com/dareaquatics/aquify/server/internal/app/compliance"
	"github.com/dareaquatics/aquify/server/internal/app/exports"
	"github.com/dareaquatics/aquify/server/internal/app/groups"
	"github.com/dareaquatics/aquify/server/internal/app/handlers"
	"github.com/dareaquatics/aquify/server/internal/app/imports"
	"github.com/dareaquatics/aquify/server/internal/app/meets"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/app/performance"
	"github.com/dareaquatics/aquify/server/internal/app/reports"
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
)

// NewRouter is a convenience wrapper for tests and simple deployments.
// It builds and starts the worker server but does NOT start the scheduler,
// because the scheduler goroutine cannot be stopped from this call site.
// Production deployments use NewRouterWithServices directly (see main.go).
func NewRouter(cfg Config, logger *slog.Logger) http.Handler {
	return NewRouterWithServices(cfg, logger, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
}

func allowsProviderStubFallback(appEnv string) bool {
	switch strings.ToLower(strings.TrimSpace(appEnv)) {
	case "", "development", "local", "test":
		return true
	default:
		return false
	}
}

// ValidateProviderConfig enforces that non-dev environments cannot boot
// without working Stripe and Postmark provider clients.
func ValidateProviderConfig(cfg Config) error {
	if allowsProviderStubFallback(cfg.AppEnv) {
		return nil
	}

	var errs []string
	if _, err := stripelib.NewSDKClient(cfg.Stripe.SecretKey); err != nil {
		errs = append(errs, fmt.Sprintf("stripe: %v", err))
	}
	if _, err := postmark.NewHTTPClient(postmark.Config{
		ServerToken: cfg.Postmark.ServerToken,
		FromName:    cfg.Postmark.FromName,
		FromEmail:   cfg.Postmark.FromEmail,
	}); err != nil {
		errs = append(errs, fmt.Sprintf("postmark: %v", err))
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("provider startup validation failed: %s", strings.Join(errs, "; "))
}

type unavailableStripeClient struct {
	err error
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

func (c unavailablePostmarkClient) SendEmail(_ context.Context, _ postmark.SendEmailRequest) (postmark.SendEmailResponse, error) {
	return postmark.SendEmailResponse{}, c.err
}

// NewRouterWithServices constructs the HTTP router and wires all services.
// workerServer may be nil; when nil a new in-memory worker server is
// created, registered, and started automatically.
func NewRouterWithServices(cfg Config, logger *slog.Logger, authService *authn.Service, membershipService *members.Service, groupsService *groups.Service, meetsService *meets.Service, importsService *imports.Service, exportsService *exports.Service, performanceService *performance.Service, billingService *billing.Service, stripeClient stripelib.Client, commsService *comms.Service, auditService *audit.Service, complianceService *compliance.Service, workerServer *workers.Server) http.Handler {
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
	objectStore, err := storage.NewClient(storage.Config{
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

	if authService == nil {
		authService = authn.NewService(authn.Config{
			EnableDevStubTokens: cfg.Auth.EnableDevStubTokens,
			OIDC: authn.OIDCConfig{
				AuthorizeURL:    cfg.Auth.OIDC.AuthorizeURL,
				TokenURL:        cfg.Auth.OIDC.TokenURL,
				JWKSURL:         cfg.Auth.OIDC.JWKSURL,
				RedirectURL:     cfg.Auth.OIDC.RedirectURL,
				ClientID:        cfg.Auth.OIDC.ClientID,
				ClientSecret:    cfg.Auth.OIDC.ClientSecret,
				ExpectedIssuer:  cfg.Auth.OIDC.ExpectedIssuer,
				StateSigningKey: cfg.Auth.OIDC.StateSigningKey,
				HTTPTimeout:     cfg.Auth.OIDC.HTTPTimeout,
			},
		})
	}
	authHandlers := handlers.NewAuthHandlersWithAudit(authService, auditService, allowDevSessionIssue)
	if membershipService == nil {
		membershipService = members.NewService()
	}
	membershipHandlers := handlers.NewMembershipHandlersWithAudit(membershipService, auditService)
	if groupsService == nil {
		groupsService = groups.NewService()
	}
	groupsHandlers := handlers.NewGroupsHandlers(groupsService)
	if meetsService == nil {
		meetsService = meets.NewService()
	}
	meetsHandlers := handlers.NewMeetsHandlersWithAudit(meetsService, auditService)
	if importsService == nil {
		importsService = imports.NewServiceWithStorage(objectStore)
	}
	if cfg.Security.Uploads.MaxBytes > 0 || len(cfg.Security.Uploads.AllowedMIMEs) > 0 || cfg.Security.Uploads.RequireMalwareScanHook {
		importsService.ConfigureSecurity(imports.SecurityConfig{
			MaxBytes:               cfg.Security.Uploads.MaxBytes,
			AllowedMIMEs:           cfg.Security.Uploads.AllowedMIMEs,
			RequireMalwareScanHook: cfg.Security.Uploads.RequireMalwareScanHook,
		})
	}
	importsHandlers := handlers.NewImportsHandlersWithAudit(importsService, auditService)
	if exportsService == nil {
		exportsService = exports.NewServiceWithStorage(objectStore)
	}
	exportsHandlers := handlers.NewExportsHandlers(exportsService)
	if performanceService == nil {
		performanceService = performance.NewService()
	}
	// Wire meets service into performance service so that best-time recomputes
	// automatically back-fill seed times on future draft/submitted entries.
	performanceService.SetEntrySeedTimeUpdater(meetsService)
	performanceHandlers := handlers.NewPerformanceHandlersWithAudit(performanceService, auditService)
	if billingService == nil {
		billingService = billing.NewService()
	}
	if stripeClient == nil {
		realStripeClient, err := stripelib.NewSDKClient(cfg.Stripe.SecretKey)
		if err == nil {
			stripeClient = realStripeClient
		} else if allowsProviderStubFallback(appEnv) {
			logger.Warn("stripe init failed; falling back to stub client", "error", err)
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
	if commsService == nil {
		// Wire real Postmark client when configured. Non-dev environments must
		// fail closed instead of silently downgrading to a stub provider.
		var pmClient postmark.Client
		realClient, err := postmark.NewHTTPClient(postmark.Config{
			ServerToken: cfg.Postmark.ServerToken,
			FromName:    cfg.Postmark.FromName,
			FromEmail:   cfg.Postmark.FromEmail,
		})
		if err == nil {
			pmClient = realClient
		} else if allowsProviderStubFallback(appEnv) {
			logger.Warn("postmark init failed; falling back to stub client", "error", err)
		} else {
			logger.Error("postmark init failed; provider unavailable in non-dev environment", "error", err)
			pmClient = unavailablePostmarkClient{err: fmt.Errorf("postmark provider unavailable: %w", err)}
		}
		if pmClient == nil {
			pmClient = postmark.NewStubClient()
		}
		commsService = comms.NewServiceWithClient(pmClient)
	}
	commsHandlers := handlers.NewCommsHandlersWithWebhookAuth(
		commsService,
		cfg.Postmark.WebhookAuthHeaderName,
		cfg.Postmark.WebhookAuthHeaderValue,
		cfg.Postmark.WebhookBasicUser,
		cfg.Postmark.WebhookBasicPass,
		appEnv,
	)
	auditHandlers := handlers.NewAuditHandlers(auditService)
	if complianceService == nil {
		complianceService = compliance.NewServiceWithConfig(cfg.Compliance.ExportTTLDays, cfg.Compliance.DeletionReviewDays)
	}
	complianceHandlers := handlers.NewComplianceHandlers(complianceService, auditService)
	reportsService := reports.NewService(membershipService, groupsService, meetsService, billingService)
	reportsService.SetStorage(objectStore)
	reportsHandlers := handlers.NewReportsHandlers(reportsService, auditService)
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
	loadRuntimeSnapshots(logger, runtimeRepo, runtimeParticipants)

	// ── Workers ────────────────────────────────────────────────────────────────
	if workerServer == nil {
		workerServer, _ = BuildWorkers(cfg, logger, importsService, exportsService, reportsService)
		workerServer.Start()
	}
	workerServer.RegisterHandler(workers.TypeGenerateExport, jobhandlers.GenerateExportHandler(logger, exportsService))
	workerServer.RegisterHandler(workers.TypePurgeExpiredFiles, jobhandlers.PurgeExpiredFilesHandler(logger, importsService, exportsService, reportsService))
	exportsService.SetQueue(workerServer)
	jobsHandlers := handlers.NewJobsHandlers(workerServer)

	// Initialise Prometheus metrics. A no-op instance is used in tests when
	// the metrics address is blank.
	metrics := telemetry.NewMetrics()

	router.Use(chimiddleware.RequestID)
	router.Use(appmiddleware.OTelTracing(cfg.Telemetry.ServiceName))
	router.Use(appmiddleware.PrometheusMetrics(metrics))
	router.Use(appmiddleware.StructuredLogger(logger))
	router.Use(appmiddleware.Recoverer(logger))
	router.Use(appmiddleware.SecurityHeaders)
	router.Use(appmiddleware.NewRateLimiter(cfg.Security.RateLimit.GlobalRequests, cfg.Security.RateLimit.GlobalWindow))
	router.Use(runtimePersistenceMiddleware(logger, runtimeRepo, runtimeParticipants))
	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Org-Id", "X-Request-Id"},
		ExposedHeaders:   []string{"X-Request-Id"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Prometheus metrics endpoint — served on the API port at /metrics.
	// This endpoint is intentionally unauthenticated; in production it should
	// be firewalled to only allow the Prometheus scraper IP range.
	router.Get("/metrics", metrics.Handler().ServeHTTP)

	router.Route("/api/v1", func(api chi.Router) {
		api.Route("/health", handlers.RegisterHealthRoutes)
		api.Get("/openapi", handlers.OpenAPIStub)

		// Public routes: no auth middleware required.
		stripeHandlers.RegisterPublicRoutes(api)
		commsHandlers.RegisterPublicRoutes(api)

		api.Route("/auth", func(auth chi.Router) {
			auth.Use(appmiddleware.NewRateLimiter(cfg.Security.RateLimit.AuthRequests, cfg.Security.RateLimit.AuthWindow))
			auth.Use(appmiddleware.CSRFProtection(cfg.Security.CSRFHeaderName))
			authHandlers.RegisterPublicRoutes(auth)
			auth.Group(func(protected chi.Router) {
				protected.Use(appmiddleware.Auth(authService))
				authHandlers.RegisterProtectedRoutes(protected)
			})
		})

		api.Group(func(protected chi.Router) {
			protected.Use(appmiddleware.Auth(authService))
			protected.Use(appmiddleware.OrgScopeStub)
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
			reportsHandlers.RegisterRoutes(protected)
			jobsHandlers.RegisterRoutes(protected)
		})
	})

	return router
}

// BuildWorkers creates and configures the worker server and scheduler without
// starting either. Callers must invoke ws.Start() and sched.Start() and call
// the corresponding Stop() methods on shutdown.
func BuildWorkers(cfg Config, logger *slog.Logger, importsService *imports.Service, exportsService *exports.Service, reportsService *reports.Service) (*workers.Server, *workers.Scheduler) {
	if logger == nil {
		logger = slog.Default()
	}
	ws := workers.NewServer(workers.ServerConfig{
		Concurrency: cfg.Worker.ConcurrencyPerQueue,
		Logger:      logger,
	})
	ws.RegisterHandler(workers.TypeParseImport, jobhandlers.ParseImportHandler(logger))
	ws.RegisterHandler(workers.TypeGenerateExport, jobhandlers.GenerateExportHandler(logger, exportsService))
	ws.RegisterHandler(workers.TypeSendCampaignBatch, jobhandlers.SendCampaignBatchHandler(logger))
	ws.RegisterHandler(workers.TypeBillingReconcile, jobhandlers.BillingReconcileHandler(logger))
	ws.RegisterHandler(workers.TypeNightlyIntegrity, jobhandlers.NightlyIntegrityHandler(logger))
	ws.RegisterHandler(workers.TypePurgeExpiredFiles, jobhandlers.PurgeExpiredFilesHandler(logger, importsService, exportsService, reportsService))
	ws.RegisterHandler(workers.TypeProcessDataExport, jobhandlers.ProcessDataExportHandler(logger))
	ws.RegisterHandler(workers.TypeProcessDataDeletion, jobhandlers.ProcessDataDeletionHandler(logger))

	sched := workers.NewScheduler(ws, logger)
	registerScheduledJobs(sched, cfg)

	return ws, sched
}

// registerScheduledJobs wires periodic jobs into the scheduler.
// Intervals are parsed from config; a zero or invalid duration disables the job.
func registerScheduledJobs(s *workers.Scheduler, cfg Config) {
	type entry struct {
		name     string
		taskType string
		queue    string
		interval string
	}
	jobs := []entry{
		{
			name:     "billing-reconcile",
			taskType: workers.TypeBillingReconcile,
			queue:    workers.QueueReconcile,
			interval: cfg.Worker.ReconcileInterval,
		},
		{
			name:     "nightly-integrity",
			taskType: workers.TypeNightlyIntegrity,
			queue:    workers.QueueBilling,
			interval: cfg.Worker.IntegrityInterval,
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
		s.Register(workers.ScheduledJob{
			Name:     j.name,
			TaskType: j.taskType,
			Queue:    j.queue,
			Interval: d,
			Payload:  map[string]string{"triggered_by": "scheduler"},
		})
	}
}

// parseDurationSafe parses a Go duration string and returns 0 on any error.
func parseDurationSafe(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d
}

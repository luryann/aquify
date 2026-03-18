package app

import (
	"log/slog"
	"net/http"
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
	stripelib "github.com/dareaquatics/aquify/server/internal/stripe"
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
	ws, _ := BuildWorkers(cfg, logger)
	ws.Start()
	return NewRouterWithServices(cfg, logger, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, ws)
}

// NewRouterWithServices constructs the HTTP router and wires all services.
// workerServer may be nil; when nil a new in-memory worker server is
// created, registered, and started automatically.
func NewRouterWithServices(cfg Config, logger *slog.Logger, authService *authn.Service, membershipService *members.Service, groupsService *groups.Service, meetsService *meets.Service, importsService *imports.Service, exportsService *exports.Service, performanceService *performance.Service, billingService *billing.Service, stripeClient stripelib.Client, commsService *comms.Service, auditService *audit.Service, complianceService *compliance.Service, workerServer *workers.Server) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	router := chi.NewRouter()

	// Initialise audit service first — it is injected into multiple handlers.
	if auditService == nil {
		auditService = audit.NewService()
	}

	if authService == nil {
		authService = authn.NewService(authn.Config{
			EnableDevStubTokens: cfg.Auth.EnableDevStubTokens,
			OIDC: authn.OIDCConfig{
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
	authHandlers := handlers.NewAuthHandlersWithAudit(authService, auditService)
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
		importsService = imports.NewService()
	}
	importsHandlers := handlers.NewImportsHandlersWithAudit(importsService, auditService)
	if exportsService == nil {
		exportsService = exports.NewService()
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
	billingHandlers := handlers.NewBillingHandlersWithAudit(billingService, auditService)
	if stripeClient == nil {
		stripeClient = stripelib.NewStubClient()
	}
	stripeHandlers := handlers.NewStripeHandlers(
		stripeClient,
		billingService,
		cfg.Stripe.WebhookSecret,
		cfg.Stripe.Currency,
		cfg.Stripe.ReconciliationAlertThresholdCents,
		logger,
	)
	if commsService == nil {
		// Wire real Postmark client when server token is configured; fall back to stub.
		var pmClient postmark.Client
		if cfg.Postmark.ServerToken != "" {
			realClient, err := postmark.NewHTTPClient(postmark.Config{
				ServerToken: cfg.Postmark.ServerToken,
				FromName:    cfg.Postmark.FromName,
				FromEmail:   cfg.Postmark.FromEmail,
			})
			if err == nil {
				pmClient = realClient
			}
		}
		if pmClient == nil {
			pmClient = postmark.NewStubClient()
		}
		commsService = comms.NewServiceWithClient(pmClient)
	}
	commsHandlers := handlers.NewCommsHandlers(commsService)
	auditHandlers := handlers.NewAuditHandlers(auditService)
	if complianceService == nil {
		complianceService = compliance.NewServiceWithConfig(cfg.Compliance.ExportTTLDays, cfg.Compliance.DeletionReviewDays)
	}
	complianceHandlers := handlers.NewComplianceHandlers(complianceService, auditService)
	reportsService := reports.NewService(membershipService, groupsService, meetsService, billingService)
	reportsHandlers := handlers.NewReportsHandlers(reportsService, auditService)

	// ── Workers ────────────────────────────────────────────────────────────────
	if workerServer == nil {
		workerServer, _ = BuildWorkers(cfg, logger)
		// Note: Start() is intentionally NOT called here. Callers (main.go) are
		// responsible for starting the worker server and scheduler so that
		// constructors remain side-effect-free and tests do not leak goroutines.
	}
	jobsHandlers := handlers.NewJobsHandlers(workerServer)

	router.Use(chimiddleware.RequestID)
	router.Use(appmiddleware.StructuredLogger(logger))
	router.Use(appmiddleware.Recoverer(logger))
	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Org-Id", "X-Request-Id"},
		ExposedHeaders:   []string{"X-Request-Id"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	router.Route("/api/v1", func(api chi.Router) {
		api.Route("/health", handlers.RegisterHealthRoutes)
		api.Get("/openapi", handlers.OpenAPIStub)

		// Public routes: no auth middleware required.
		stripeHandlers.RegisterPublicRoutes(api)

		api.Route("/auth", func(auth chi.Router) {
			authHandlers.RegisterPublicRoutes(auth)
			auth.Group(func(protected chi.Router) {
				protected.Use(appmiddleware.Auth(authService))
				protected.Use(appmiddleware.OrgScopeStub)
				authHandlers.RegisterProtectedRoutes(protected)
			})
		})

		api.Group(func(protected chi.Router) {
			protected.Use(appmiddleware.Auth(authService))
			protected.Use(appmiddleware.OrgScopeStub)
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
func BuildWorkers(cfg Config, logger *slog.Logger) (*workers.Server, *workers.Scheduler) {
	if logger == nil {
		logger = slog.Default()
	}
	ws := workers.NewServer(workers.ServerConfig{
		Concurrency: cfg.Worker.ConcurrencyPerQueue,
		Logger:      logger,
	})
	ws.RegisterHandler(workers.TypeParseImport, jobhandlers.ParseImportHandler(logger))
	ws.RegisterHandler(workers.TypeGenerateExport, jobhandlers.GenerateExportHandler(logger))
	ws.RegisterHandler(workers.TypeSendCampaignBatch, jobhandlers.SendCampaignBatchHandler(logger))
	ws.RegisterHandler(workers.TypeBillingReconcile, jobhandlers.BillingReconcileHandler(logger))
	ws.RegisterHandler(workers.TypeNightlyIntegrity, jobhandlers.NightlyIntegrityHandler(logger))
	ws.RegisterHandler(workers.TypePurgeExpiredFiles, jobhandlers.PurgeExpiredFilesHandler(logger))
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

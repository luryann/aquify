package migration

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/billing"
	"github.com/dareaquatics/aquify/server/internal/app/groups"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/app/performance"
)

const (
	StepHouseholds          = "households"
	StepSwimmers            = "swimmers"
	StepGroups              = "groups"
	StepHistoricalBestTimes = "historical_best_times"
	StepOpenInvoices        = "open_invoices"
	StepPaymentHistory      = "payment_history"
)

var RequiredFiles = []string{
	"households.csv",
	"swimmers.csv",
	"groups.csv",
	"historical_best_times.csv",
	"open_invoices.csv",
	"payment_history.csv",
}

var requiredHeadersByFile = map[string][]string{
	"households.csv": {
		"legacy_household_id",
		"household_name",
	},
	"swimmers.csv": {
		"legacy_swimmer_id",
		"legacy_household_id",
		"first_name",
		"last_name",
		"date_of_birth",
	},
	"groups.csv": {
		"legacy_group_code",
		"name",
	},
	"historical_best_times.csv": {
		"legacy_best_time_key",
		"legacy_swimmer_id",
		"distance",
		"stroke",
		"course",
		"best_time_ms",
	},
	"open_invoices.csv": {
		"legacy_invoice_id",
		"invoice_number",
		"legacy_household_id",
		"due_date",
		"outstanding_amount_cents",
	},
	"payment_history.csv": {
		"legacy_payment_id",
		"legacy_invoice_id",
		"amount_cents",
		"method",
	},
}

type Config struct {
	OrgID                   string
	ActorID                 string
	SourceCounts            map[string]int
	SourceBillingTotalCents *int
}

type Runner struct {
	cfg         Config
	principal   *appmiddleware.Principal
	membersSvc  *members.Service
	groupsSvc   *groups.Service
	perfSvc     *performance.Service
	billingSvc  *billing.Service
	idempotency map[string]map[string]struct{}
	legacyIDs   struct {
		households map[string]string
		swimmers   map[string]string
		groups     map[string]string
		invoices   map[string]string
	}
}

type Report struct {
	GeneratedAt time.Time          `json:"generated_at"`
	OrgID       string             `json:"org_id"`
	ActorID     string             `json:"actor_id"`
	InputDir    string             `json:"input_dir"`
	Required    []string           `json:"required_files"`
	Steps       []StepReport       `json:"steps"`
	Comparisons []ComparisonReport `json:"comparisons,omitempty"`
	Warnings    []string           `json:"warnings,omitempty"`
	Errors      []string           `json:"errors,omitempty"`
}

type StepReport struct {
	Name        string   `json:"name"`
	SourceFile  string   `json:"source_file"`
	Processed   int      `json:"processed"`
	Imported    int      `json:"imported"`
	Skipped     int      `json:"skipped"`
	Duplicates  int      `json:"duplicates"`
	Warnings    []string `json:"warnings,omitempty"`
	Errors      []string `json:"errors,omitempty"`
	StartedAt   string   `json:"started_at"`
	CompletedAt string   `json:"completed_at"`
}

type ComparisonReport struct {
	Name        string `json:"name"`
	SourceValue int    `json:"source_value"`
	TargetValue int    `json:"target_value"`
	Delta       int    `json:"delta"`
	Status      string `json:"status"`
}

type HouseholdRow struct {
	LegacyHouseholdID string
	HouseholdName     string
	PrimaryEmail      string
	PrimaryPhone      string
	Status            string
	Notes             string
}

type SwimmerRow struct {
	LegacySwimmerID    string
	LegacyHouseholdID  string
	FirstName          string
	LastName           string
	PreferredName      string
	DateOfBirth        string
	Gender             string
	SwimmerExternalID  string
	USASwimmingID      string
	RegistrationStatus string
	Notes              string
}

type GroupRow struct {
	LegacyGroupCode string
	Name            string
	Description     string
	Type            string
	CoachUserID     string
	MinAge          string
	MaxAge          string
}

type HistoricalBestTimeRow struct {
	LegacyBestTimeKey string
	LegacySwimmerID   string
	Distance          string
	Stroke            string
	Course            string
	BestTimeMS        string
	RecordedAt        string
}

type OpenInvoiceRow struct {
	LegacyInvoiceID        string
	InvoiceNumber          string
	LegacyHouseholdID      string
	DueDate                string
	TotalAmountCents       string
	OutstandingAmountCents string
	Notes                  string
}

type PaymentHistoryRow struct {
	LegacyPaymentID string
	LegacyInvoiceID string
	AmountCents     string
	Method          string
	PaidAt          string
	ExternalRef     string
	Notes           string
}

func NewRunner(cfg Config) *Runner {
	orgID := strings.TrimSpace(cfg.OrgID)
	if orgID == "" {
		orgID = "org-migration"
	}
	actorID := strings.TrimSpace(cfg.ActorID)
	if actorID == "" {
		actorID = "migration-cli"
	}

	principal := &appmiddleware.Principal{
		UserID:           actorID,
		OrgID:            orgID,
		Roles:            []string{"admin"},
		MFAAuthenticated: true,
	}

	return &Runner{
		cfg: Config{
			OrgID:                   orgID,
			ActorID:                 actorID,
			SourceCounts:            copySourceCounts(cfg.SourceCounts),
			SourceBillingTotalCents: cfg.SourceBillingTotalCents,
		},
		principal:  principal,
		membersSvc: members.NewService(),
		groupsSvc:  groups.NewService(),
		perfSvc:    performance.NewService(),
		billingSvc: billing.NewService(),
		idempotency: map[string]map[string]struct{}{
			StepHouseholds:          {},
			StepSwimmers:            {},
			StepGroups:              {},
			StepHistoricalBestTimes: {},
			StepOpenInvoices:        {},
			StepPaymentHistory:      {},
		},
		legacyIDs: struct {
			households map[string]string
			swimmers   map[string]string
			groups     map[string]string
			invoices   map[string]string
		}{
			households: map[string]string{},
			swimmers:   map[string]string{},
			groups:     map[string]string{},
			invoices:   map[string]string{},
		},
	}
}

func (r *Runner) DryRun(ctx context.Context, inputDir string) (Report, error) {
	report := Report{
		GeneratedAt: time.Now().UTC(),
		OrgID:       r.cfg.OrgID,
		ActorID:     r.cfg.ActorID,
		InputDir:    inputDir,
		Required:    append([]string{}, RequiredFiles...),
	}

	if err := ctx.Err(); err != nil {
		return report, err
	}

	for _, file := range RequiredFiles {
		if _, err := os.Stat(filepath.Join(inputDir, file)); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("missing required input file: %s", file))
		}
	}
	if len(report.Errors) > 0 {
		return report, errors.New("required migration input files are missing")
	}

	steps := []struct {
		name string
		file string
		run  func(context.Context, string) StepReport
	}{
		{name: StepHouseholds, file: "households.csv", run: r.importHouseholds},
		{name: StepSwimmers, file: "swimmers.csv", run: r.importSwimmers},
		{name: StepGroups, file: "groups.csv", run: r.importGroups},
		{name: StepHistoricalBestTimes, file: "historical_best_times.csv", run: r.importHistoricalBestTimes},
		{name: StepOpenInvoices, file: "open_invoices.csv", run: r.importOpenInvoices},
		{name: StepPaymentHistory, file: "payment_history.csv", run: r.importPaymentHistory},
	}

	for _, step := range steps {
		if err := ctx.Err(); err != nil {
			report.Errors = append(report.Errors, err.Error())
			return report, err
		}
		stepReport := step.run(ctx, filepath.Join(inputDir, step.file))
		report.Steps = append(report.Steps, stepReport)
		report.Errors = append(report.Errors, stepReport.Errors...)
		for _, warning := range stepReport.Warnings {
			report.Warnings = append(report.Warnings, fmt.Sprintf("%s: %s", step.name, warning))
		}
	}

	report.Comparisons = r.buildComparisons(report.Steps)
	for _, comparison := range report.Comparisons {
		if comparison.Status != "match" {
			report.Errors = append(report.Errors, fmt.Sprintf("%s mismatch: source=%d target=%d delta=%d", comparison.Name, comparison.SourceValue, comparison.TargetValue, comparison.Delta))
		}
	}

	if len(report.Errors) > 0 {
		return report, errors.New("migration dry-run completed with validation errors")
	}

	return report, nil
}

func (r *Runner) WriteReport(path string, report Report) error {
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o600)
}

func (r *Runner) WriteRemediationLog(path string, report Report) error {
	var b strings.Builder
	b.WriteString("# Migration Dry-Run Remediation Log\n\n")
	b.WriteString(fmt.Sprintf("- Generated at: %s\n", report.GeneratedAt.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("- Org ID: %s\n", report.OrgID))
	b.WriteString(fmt.Sprintf("- Input directory: %s\n\n", report.InputDir))

	if len(report.Comparisons) > 0 {
		b.WriteString("## Source Comparisons\n\n")
		b.WriteString("| Check | Source | Target | Delta | Status |\n")
		b.WriteString("|---|---:|---:|---:|---|\n")
		for _, comparison := range report.Comparisons {
			b.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %s |\n", comparison.Name, comparison.SourceValue, comparison.TargetValue, comparison.Delta, comparison.Status))
		}
		b.WriteString("\n")
	}

	b.WriteString("## Manual Remediation Items\n\n")
	hasItems := false
	for _, step := range report.Steps {
		for _, warning := range step.Warnings {
			hasItems = true
			b.WriteString(fmt.Sprintf("- [ ] `%s` warning: %s\n", step.Name, warning))
		}
		for _, stepErr := range step.Errors {
			hasItems = true
			b.WriteString(fmt.Sprintf("- [ ] `%s` error: %s\n", step.Name, stepErr))
		}
	}
	for _, comparison := range report.Comparisons {
		if comparison.Status == "match" {
			continue
		}
		hasItems = true
		b.WriteString(fmt.Sprintf("- [ ] `%s` source parity mismatch: source=%d target=%d delta=%d\n", comparison.Name, comparison.SourceValue, comparison.TargetValue, comparison.Delta))
	}
	if !hasItems {
		b.WriteString("No manual remediation items were generated by this dry-run.\n")
	}

	return os.WriteFile(path, []byte(b.String()), 0o600)
}

func (r *Runner) Plan() string {
	return strings.Join([]string{
		"Step 24 migration CLI plan",
		"",
		"Expected input files:",
		"- households.csv",
		"- swimmers.csv",
		"- groups.csv",
		"- historical_best_times.csv",
		"- open_invoices.csv",
		"- payment_history.csv",
		"",
		"Implemented steps:",
		"- households",
		"- swimmers",
		"- groups",
		"- historical best times",
		"- open invoices",
		"- payment history",
		"",
		"Current output:",
		"- JSON validation report with processed/imported/skipped/duplicate counts",
		"- deterministic idempotency skips per migration step",
		"- optional source row-count and billing-total comparisons",
		"- optional markdown remediation log for warnings, errors, and parity mismatches",
		"",
		"Real staging dry-runs still require actual legacy exports and staging execution evidence.",
	}, "\n")
}

func (r *Runner) MembersService() *members.Service {
	return r.membersSvc
}

func (r *Runner) GroupsService() *groups.Service {
	return r.groupsSvc
}

func (r *Runner) PerformanceService() *performance.Service {
	return r.perfSvc
}

func (r *Runner) BillingService() *billing.Service {
	return r.billingSvc
}

func (r *Runner) importHouseholds(ctx context.Context, path string) StepReport {
	step := newStepReport(StepHouseholds, filepath.Base(path))
	rows, err := loadRows(path, requiredHeadersByFile[filepath.Base(path)], func(record map[string]string) HouseholdRow {
		return HouseholdRow{
			LegacyHouseholdID: field(record, "legacy_household_id"),
			HouseholdName:     field(record, "household_name"),
			PrimaryEmail:      field(record, "primary_email"),
			PrimaryPhone:      firstNonEmpty(field(record, "primary_phone"), field(record, "primary_phone_e164")),
			Status:            field(record, "status"),
			Notes:             field(record, "notes"),
		}
	})
	if err != nil {
		step.Errors = append(step.Errors, err.Error())
		return finishStepReport(step)
	}

	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			step.Errors = append(step.Errors, err.Error())
			break
		}
		step.Processed++
		key := strings.TrimSpace(row.LegacyHouseholdID)
		if key == "" {
			step.Errors = append(step.Errors, "row missing legacy_household_id")
			continue
		}
		if r.isDuplicate(StepHouseholds, key) {
			step.Skipped++
			step.Duplicates++
			continue
		}

		household, err := r.membersSvc.CreateHousehold(r.principal, members.CreateHouseholdInput{
			OrgID:            r.cfg.OrgID,
			HouseholdName:    row.HouseholdName,
			PrimaryEmail:     row.PrimaryEmail,
			PrimaryPhoneE164: row.PrimaryPhone,
			Status:           normalizeStatus(row.Status, "active"),
			Notes:            row.Notes,
		})
		if err != nil {
			step.Errors = append(step.Errors, fmt.Sprintf("household %s: %v", key, err))
			continue
		}

		r.idempotency[StepHouseholds][key] = struct{}{}
		r.legacyIDs.households[key] = household.ID
		step.Imported++
	}

	return finishStepReport(step)
}

func (r *Runner) importSwimmers(ctx context.Context, path string) StepReport {
	step := newStepReport(StepSwimmers, filepath.Base(path))
	rows, err := loadRows(path, requiredHeadersByFile[filepath.Base(path)], func(record map[string]string) SwimmerRow {
		return SwimmerRow{
			LegacySwimmerID:    field(record, "legacy_swimmer_id"),
			LegacyHouseholdID:  field(record, "legacy_household_id"),
			FirstName:          field(record, "first_name"),
			LastName:           field(record, "last_name"),
			PreferredName:      field(record, "preferred_name"),
			DateOfBirth:        field(record, "date_of_birth"),
			Gender:             field(record, "gender"),
			SwimmerExternalID:  field(record, "swimmer_external_id"),
			USASwimmingID:      field(record, "usa_swimming_id"),
			RegistrationStatus: field(record, "registration_status"),
			Notes:              field(record, "notes"),
		}
	})
	if err != nil {
		step.Errors = append(step.Errors, err.Error())
		return finishStepReport(step)
	}

	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			step.Errors = append(step.Errors, err.Error())
			break
		}
		step.Processed++
		key := strings.TrimSpace(row.LegacySwimmerID)
		if key == "" {
			step.Errors = append(step.Errors, "row missing legacy_swimmer_id")
			continue
		}
		if r.isDuplicate(StepSwimmers, key) {
			step.Skipped++
			step.Duplicates++
			continue
		}

		householdID, ok := r.legacyIDs.households[strings.TrimSpace(row.LegacyHouseholdID)]
		if !ok {
			step.Errors = append(step.Errors, fmt.Sprintf("swimmer %s: unknown legacy_household_id %q", key, row.LegacyHouseholdID))
			continue
		}

		swimmer, _, err := r.membersSvc.CreateSwimmer(r.principal, r.cfg.OrgID, members.CreateSwimmerInput{
			HouseholdID:        householdID,
			FirstName:          row.FirstName,
			LastName:           row.LastName,
			PreferredName:      row.PreferredName,
			DateOfBirth:        row.DateOfBirth,
			Gender:             normalizeGender(row.Gender),
			SwimmerExternalID:  row.SwimmerExternalID,
			USASwimmingID:      row.USASwimmingID,
			RegistrationStatus: normalizeStatus(row.RegistrationStatus, "active"),
			Notes:              row.Notes,
		})
		if err != nil {
			step.Errors = append(step.Errors, fmt.Sprintf("swimmer %s: %v", key, err))
			continue
		}

		r.idempotency[StepSwimmers][key] = struct{}{}
		r.legacyIDs.swimmers[key] = swimmer.ID
		r.groupsSvc.SeedSwimmer(swimmer.OrgID, swimmer.ID, swimmer.HouseholdID, swimmer.DateOfBirth, swimmer.Gender, swimmer.RegistrationStatus)
		step.Imported++
	}

	return finishStepReport(step)
}

func (r *Runner) importGroups(ctx context.Context, path string) StepReport {
	step := newStepReport(StepGroups, filepath.Base(path))
	rows, err := loadRows(path, requiredHeadersByFile[filepath.Base(path)], func(record map[string]string) GroupRow {
		return GroupRow{
			LegacyGroupCode: field(record, "legacy_group_code"),
			Name:            field(record, "name"),
			Description:     field(record, "description"),
			Type:            field(record, "type"),
			CoachUserID:     field(record, "coach_user_id"),
			MinAge:          field(record, "min_age"),
			MaxAge:          field(record, "max_age"),
		}
	})
	if err != nil {
		step.Errors = append(step.Errors, err.Error())
		return finishStepReport(step)
	}

	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			step.Errors = append(step.Errors, err.Error())
			break
		}
		step.Processed++
		key := strings.TrimSpace(row.LegacyGroupCode)
		if key == "" {
			step.Errors = append(step.Errors, "row missing legacy_group_code")
			continue
		}
		if r.isDuplicate(StepGroups, key) {
			step.Skipped++
			step.Duplicates++
			continue
		}

		minAge, err := parseOptionalInt(row.MinAge)
		if err != nil {
			step.Errors = append(step.Errors, fmt.Sprintf("group %s: invalid min_age: %v", key, err))
			continue
		}
		maxAge, err := parseOptionalInt(row.MaxAge)
		if err != nil {
			step.Errors = append(step.Errors, fmt.Sprintf("group %s: invalid max_age: %v", key, err))
			continue
		}

		group, _, err := r.groupsSvc.CreateGroup(r.principal, r.cfg.OrgID, groups.CreateGroupInput{
			Code:        row.LegacyGroupCode,
			Name:        row.Name,
			Description: row.Description,
			Type:        normalizeGroupType(row.Type),
			CoachUserID: row.CoachUserID,
			MinAge:      minAge,
			MaxAge:      maxAge,
		})
		if err != nil {
			step.Errors = append(step.Errors, fmt.Sprintf("group %s: %v", key, err))
			continue
		}

		r.idempotency[StepGroups][key] = struct{}{}
		r.legacyIDs.groups[key] = group.ID
		step.Imported++
	}

	return finishStepReport(step)
}

func (r *Runner) importHistoricalBestTimes(ctx context.Context, path string) StepReport {
	step := newStepReport(StepHistoricalBestTimes, filepath.Base(path))
	rows, err := loadRows(path, requiredHeadersByFile[filepath.Base(path)], func(record map[string]string) HistoricalBestTimeRow {
		return HistoricalBestTimeRow{
			LegacyBestTimeKey: field(record, "legacy_best_time_key"),
			LegacySwimmerID:   field(record, "legacy_swimmer_id"),
			Distance:          field(record, "distance"),
			Stroke:            field(record, "stroke"),
			Course:            field(record, "course"),
			BestTimeMS:        firstNonEmpty(field(record, "best_time_ms"), field(record, "best_time_millis")),
			RecordedAt:        field(record, "recorded_at"),
		}
	})
	if err != nil {
		step.Errors = append(step.Errors, err.Error())
		return finishStepReport(step)
	}

	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			step.Errors = append(step.Errors, err.Error())
			break
		}
		step.Processed++
		key := strings.TrimSpace(row.LegacyBestTimeKey)
		if key == "" {
			step.Errors = append(step.Errors, "row missing legacy_best_time_key")
			continue
		}
		if r.isDuplicate(StepHistoricalBestTimes, key) {
			step.Skipped++
			step.Duplicates++
			continue
		}

		swimmerID, ok := r.legacyIDs.swimmers[strings.TrimSpace(row.LegacySwimmerID)]
		if !ok {
			step.Errors = append(step.Errors, fmt.Sprintf("best_time %s: unknown legacy_swimmer_id %q", key, row.LegacySwimmerID))
			continue
		}

		distance, err := strconv.Atoi(strings.TrimSpace(row.Distance))
		if err != nil || distance <= 0 {
			step.Errors = append(step.Errors, fmt.Sprintf("best_time %s: invalid distance %q", key, row.Distance))
			continue
		}
		bestTimeMS, err := strconv.Atoi(strings.TrimSpace(row.BestTimeMS))
		if err != nil || bestTimeMS < 0 {
			step.Errors = append(step.Errors, fmt.Sprintf("best_time %s: invalid best_time_ms %q", key, row.BestTimeMS))
			continue
		}
		recordedAt, err := normalizeOptionalDate(row.RecordedAt)
		if err != nil {
			step.Errors = append(step.Errors, fmt.Sprintf("best_time %s: invalid recorded_at %q", key, row.RecordedAt))
			continue
		}

		_, _, err = r.perfSvc.OverrideBestTime(r.cfg.OrgID, r.cfg.ActorID, r.principal, performance.OverrideBestTimeInput{
			SwimmerID:     swimmerID,
			EventDistance: distance,
			Stroke:        normalizeBestTimeStroke(row.Stroke),
			Course:        normalizeCourse(row.Course),
			BestTimeMs:    intPtr(bestTimeMS),
			Reason:        "migration historical best time import",
			RecordedAt:    recordedAt,
		})
		if err != nil {
			step.Errors = append(step.Errors, fmt.Sprintf("best_time %s: %v", key, err))
			continue
		}

		r.idempotency[StepHistoricalBestTimes][key] = struct{}{}
		step.Imported++
	}

	return finishStepReport(step)
}

func (r *Runner) importOpenInvoices(ctx context.Context, path string) StepReport {
	step := newStepReport(StepOpenInvoices, filepath.Base(path))
	rows, err := loadRows(path, requiredHeadersByFile[filepath.Base(path)], func(record map[string]string) OpenInvoiceRow {
		return OpenInvoiceRow{
			LegacyInvoiceID:        field(record, "legacy_invoice_id"),
			InvoiceNumber:          field(record, "invoice_number"),
			LegacyHouseholdID:      field(record, "legacy_household_id"),
			DueDate:                field(record, "due_date"),
			TotalAmountCents:       firstNonEmpty(field(record, "total_amount_cents"), field(record, "invoice_total_cents")),
			OutstandingAmountCents: firstNonEmpty(field(record, "outstanding_amount_cents"), field(record, "amount_due_cents")),
			Notes:                  field(record, "notes"),
		}
	})
	if err != nil {
		step.Errors = append(step.Errors, err.Error())
		return finishStepReport(step)
	}

	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			step.Errors = append(step.Errors, err.Error())
			break
		}
		step.Processed++
		key := strings.TrimSpace(row.LegacyInvoiceID)
		if key == "" {
			step.Errors = append(step.Errors, "row missing legacy_invoice_id")
			continue
		}
		if r.isDuplicate(StepOpenInvoices, key) {
			step.Skipped++
			step.Duplicates++
			continue
		}

		householdID, ok := r.legacyIDs.households[strings.TrimSpace(row.LegacyHouseholdID)]
		if !ok {
			step.Errors = append(step.Errors, fmt.Sprintf("invoice %s: unknown legacy_household_id %q", key, row.LegacyHouseholdID))
			continue
		}

		totalAmount, err := strconv.Atoi(firstNonEmpty(row.TotalAmountCents, "0"))
		if err != nil || totalAmount < 0 {
			step.Errors = append(step.Errors, fmt.Sprintf("invoice %s: invalid total_amount_cents %q", key, row.TotalAmountCents))
			continue
		}
		outstandingAmount, err := strconv.Atoi(strings.TrimSpace(row.OutstandingAmountCents))
		if err != nil || outstandingAmount < 0 {
			step.Errors = append(step.Errors, fmt.Sprintf("invoice %s: invalid outstanding_amount_cents %q", key, row.OutstandingAmountCents))
			continue
		}
		if totalAmount == 0 {
			totalAmount = outstandingAmount
		}
		if totalAmount < outstandingAmount {
			step.Errors = append(step.Errors, fmt.Sprintf("invoice %s: total_amount_cents must be >= outstanding_amount_cents", key))
			continue
		}
		dueDate, err := normalizeRequiredDate(row.DueDate)
		if err != nil {
			step.Errors = append(step.Errors, fmt.Sprintf("invoice %s: invalid due_date %q", key, row.DueDate))
			continue
		}

		invoice, err := r.billingSvc.CreateInvoice(r.cfg.OrgID, r.cfg.ActorID, r.principal, billing.CreateInvoiceInput{
			HouseholdID:   householdID,
			InvoiceNumber: row.InvoiceNumber,
			DueDate:       dueDate,
			Notes:         row.Notes,
			ExternalRef:   key,
		})
		if err != nil {
			step.Errors = append(step.Errors, fmt.Sprintf("invoice %s: %v", key, err))
			continue
		}

		_, err = r.billingSvc.AddInvoiceItem(r.cfg.OrgID, r.cfg.ActorID, r.principal, invoice.ID, billing.AddInvoiceItemInput{
			Description:     "Legacy open balance migration",
			Quantity:        1,
			UnitAmountCents: totalAmount,
			Category:        "manual",
		})
		if err != nil {
			step.Errors = append(step.Errors, fmt.Sprintf("invoice %s: %v", key, err))
			continue
		}

		if _, _, err := r.billingSvc.IssueInvoice(r.cfg.OrgID, r.cfg.ActorID, r.principal, invoice.ID); err != nil {
			step.Errors = append(step.Errors, fmt.Sprintf("invoice %s: %v", key, err))
			continue
		}

		r.idempotency[StepOpenInvoices][key] = struct{}{}
		r.legacyIDs.invoices[key] = invoice.ID
		if totalAmount != outstandingAmount {
			step.Warnings = append(step.Warnings, fmt.Sprintf("invoice %s imported with total=%d and outstanding=%d; payment history must supply the difference", key, totalAmount, outstandingAmount))
		}
		step.Imported++
	}

	return finishStepReport(step)
}

func (r *Runner) importPaymentHistory(ctx context.Context, path string) StepReport {
	step := newStepReport(StepPaymentHistory, filepath.Base(path))
	rows, err := loadRows(path, requiredHeadersByFile[filepath.Base(path)], func(record map[string]string) PaymentHistoryRow {
		return PaymentHistoryRow{
			LegacyPaymentID: field(record, "legacy_payment_id"),
			LegacyInvoiceID: field(record, "legacy_invoice_id"),
			AmountCents:     field(record, "amount_cents"),
			Method:          field(record, "method"),
			PaidAt:          field(record, "paid_at"),
			ExternalRef:     field(record, "external_ref"),
			Notes:           field(record, "notes"),
		}
	})
	if err != nil {
		step.Errors = append(step.Errors, err.Error())
		return finishStepReport(step)
	}

	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			step.Errors = append(step.Errors, err.Error())
			break
		}
		step.Processed++
		key := strings.TrimSpace(row.LegacyPaymentID)
		if key == "" {
			step.Errors = append(step.Errors, "row missing legacy_payment_id")
			continue
		}
		if r.isDuplicate(StepPaymentHistory, key) {
			step.Skipped++
			step.Duplicates++
			continue
		}

		invoiceID, ok := r.legacyIDs.invoices[strings.TrimSpace(row.LegacyInvoiceID)]
		if !ok {
			step.Errors = append(step.Errors, fmt.Sprintf("payment %s: unknown legacy_invoice_id %q", key, row.LegacyInvoiceID))
			continue
		}

		amount, err := strconv.Atoi(strings.TrimSpace(row.AmountCents))
		if err != nil || amount <= 0 {
			step.Errors = append(step.Errors, fmt.Sprintf("payment %s: invalid amount_cents %q", key, row.AmountCents))
			continue
		}
		paidAt, err := normalizeOptionalDate(row.PaidAt)
		if err != nil {
			step.Errors = append(step.Errors, fmt.Sprintf("payment %s: invalid paid_at %q", key, row.PaidAt))
			continue
		}

		_, _, err = r.billingSvc.RecordPayment(r.cfg.OrgID, r.cfg.ActorID, r.principal, invoiceID, billing.RecordPaymentInput{
			AmountCents: amount,
			Method:      normalizePaymentMethod(row.Method),
			ExternalRef: firstNonEmpty(row.ExternalRef, key),
			Notes:       strings.TrimSpace(row.Notes),
			PaidAt:      paidAt,
		})
		if err != nil {
			step.Errors = append(step.Errors, fmt.Sprintf("payment %s: %v", key, err))
			continue
		}

		r.idempotency[StepPaymentHistory][key] = struct{}{}
		step.Imported++
	}

	return finishStepReport(step)
}

func newStepReport(name, sourceFile string) StepReport {
	now := time.Now().UTC().Format(time.RFC3339)
	return StepReport{
		Name:       name,
		SourceFile: sourceFile,
		StartedAt:  now,
	}
}

func finishStepReport(step StepReport) StepReport {
	step.CompletedAt = time.Now().UTC().Format(time.RFC3339)
	return step
}

func (r *Runner) isDuplicate(step, key string) bool {
	_, exists := r.idempotency[step][key]
	return exists
}

func loadRows[T any](path string, requiredHeaders []string, mapper func(map[string]string) T) ([]T, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1

	headers, err := reader.Read()
	if err != nil {
		return nil, err
	}

	for i := range headers {
		headers[i] = normalizeHeader(headers[i])
	}
	if missing := missingHeaders(headers, requiredHeaders); len(missing) > 0 {
		return nil, fmt.Errorf("%s missing required headers: %s", filepath.Base(path), strings.Join(missing, ", "))
	}

	var rows []T
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if isBlankCSVRecord(record) {
			continue
		}

		row := make(map[string]string, len(headers))
		for idx, header := range headers {
			if idx < len(record) {
				row[header] = strings.TrimSpace(record[idx])
			}
		}
		rows = append(rows, mapper(row))
	}

	return rows, nil
}

func missingHeaders(headers, required []string) []string {
	if len(required) == 0 {
		return nil
	}
	headerSet := make(map[string]struct{}, len(headers))
	for _, header := range headers {
		headerSet[normalizeHeader(header)] = struct{}{}
	}
	var missing []string
	for _, requiredHeader := range required {
		if _, ok := headerSet[normalizeHeader(requiredHeader)]; !ok {
			missing = append(missing, normalizeHeader(requiredHeader))
		}
	}
	return missing
}

func isBlankCSVRecord(record []string) bool {
	for _, value := range record {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}

func normalizeHeader(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "-", "_")
	return value
}

func field(record map[string]string, key string) string {
	return strings.TrimSpace(record[normalizeHeader(key)])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func normalizeStatus(value, fallback string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return fallback
	}
	return value
}

func normalizeGender(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "f", "female", "girl", "woman":
		return "female"
	case "m", "male", "boy", "man":
		return "male"
	case "x", "non_binary", "non-binary", "nonbinary":
		return "non_binary"
	default:
		return "undisclosed"
	}
}

func normalizeGroupType(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(strings.ToLower(value)), "-", "_")
	switch value {
	case "competitive", "recreational", "developmental", "masters", "other":
		return value
	case "pre_competitive", "precompetitive":
		return "developmental"
	case "lesson", "lessons", "learn_to_swim":
		return "recreational"
	default:
		return "competitive"
	}
}

func normalizeBestTimeStroke(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "freestyle", "free":
		return "free"
	case "back", "backstroke":
		return "back"
	case "breast", "breaststroke":
		return "breast"
	case "fly", "butterfly":
		return "fly"
	case "im", "individual_medley", "individual medley":
		return "im"
	default:
		return strings.TrimSpace(strings.ToLower(value))
	}
}

func normalizeCourse(value string) string {
	return strings.TrimSpace(strings.ToUpper(value))
}

func normalizePaymentMethod(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "stripe", "card", "credit_card", "payment_intent":
		return "stripe"
	case "check", "cheque":
		return "check"
	case "cash":
		return "cash"
	case "ach", "bank", "bank_transfer":
		return "ach"
	default:
		return "other"
	}
}

func normalizeDate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	layouts := []string{
		"2006-01-02",
		time.RFC3339,
		"2006/01/02",
		"01/02/2006",
		"1/2/2006",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.Format("2006-01-02")
		}
	}
	return value
}

func normalizeRequiredDate(value string) (string, error) {
	normalized, err := normalizeOptionalDate(value)
	if err != nil {
		return "", err
	}
	if normalized == "" {
		return "", errors.New("date is required")
	}
	return normalized, nil
}

func normalizeOptionalDate(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}

	normalized := normalizeDate(value)
	if _, err := time.Parse("2006-01-02", normalized); err != nil {
		return "", err
	}
	return normalized, nil
}

func parseOptionalInt(value string) (*int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func intPtr(v int) *int {
	return &v
}

func copySourceCounts(in map[string]int) map[string]int {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]int, len(in))
	for key, value := range in {
		out[normalizeComparisonName(key)] = value
	}
	return out
}

func normalizeComparisonName(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimSuffix(value, ".csv")
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.ReplaceAll(value, " ", "_")
	return value
}

func (r *Runner) buildComparisons(steps []StepReport) []ComparisonReport {
	var comparisons []ComparisonReport
	for _, step := range steps {
		sourceValue, ok := r.cfg.SourceCounts[normalizeComparisonName(step.Name)]
		if !ok {
			sourceValue, ok = r.cfg.SourceCounts[normalizeComparisonName(step.SourceFile)]
		}
		if !ok {
			continue
		}
		comparisons = append(comparisons, newComparison("row_count_"+step.Name, sourceValue, step.Processed))
	}

	if r.cfg.SourceBillingTotalCents != nil {
		targetTotal := 0
		for _, invoice := range r.billingSvc.SnapshotInvoices(r.cfg.OrgID) {
			targetTotal += invoice.AmountDueCents
		}
		comparisons = append(comparisons, newComparison("billing_outstanding_total_cents", *r.cfg.SourceBillingTotalCents, targetTotal))
	}
	return comparisons
}

func newComparison(name string, sourceValue, targetValue int) ComparisonReport {
	delta := targetValue - sourceValue
	status := "match"
	if delta != 0 {
		status = "mismatch"
	}
	return ComparisonReport{
		Name:        name,
		SourceValue: sourceValue,
		TargetValue: targetValue,
		Delta:       delta,
		Status:      status,
	}
}

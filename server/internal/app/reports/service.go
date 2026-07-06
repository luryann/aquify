package reports

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/billing"
	"github.com/dareaquatics/aquify/server/internal/app/groups"
	"github.com/dareaquatics/aquify/server/internal/app/meets"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/storage"
)

var (
	ErrForbidden         = errors.New("forbidden")
	ErrInvoiceIDRequired = errors.New("invoice_id is required")
	ErrInvoiceNotFound   = errors.New("invoice not found")
)

type Service struct {
	mu      sync.RWMutex
	members *members.Service
	groups  *groups.Service
	meets   *meets.Service
	billing *billing.Service
	store   storage.ObjectStorage
	nowFunc func() time.Time
	ttl     time.Duration
	urlTTL  time.Duration
	files   map[string]StoredArtifact
}

func NewService(memberSvc *members.Service, groupSvc *groups.Service, meetSvc *meets.Service, billingSvc *billing.Service) *Service {
	return &Service{
		members: memberSvc,
		groups:  groupSvc,
		meets:   meetSvc,
		billing: billingSvc,
		store:   storage.NewStubStorage(""),
		nowFunc: func() time.Time { return time.Now().UTC() },
		ttl:     7 * 24 * time.Hour,
		urlTTL:  15 * time.Minute,
		files:   make(map[string]StoredArtifact),
	}
}

func (s *Service) SetStorage(store storage.ObjectStorage) {
	if store == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store = store
}

func (s *Service) ConfigureArtifactTTL(ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ttl = ttl
}

type StoredArtifact struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"org_id"`
	Kind        string    `json:"kind"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	SizeBytes   int       `json:"size_bytes"`
	StorageKey  string    `json:"storage_key"`
	DownloadURL string    `json:"download_url,omitempty"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
}

func (s *Service) StoreCSVArtifact(orgID, kind string, generatedAt time.Time, doc *CSVDocument) (*StoredArtifact, error) {
	if doc == nil {
		return nil, fmt.Errorf("csv document is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	id := fmt.Sprintf("report_%d", generatedAt.UnixNano())
	storageKey := fmt.Sprintf("reports/%s/%s/%s", orgID, kind, doc.Filename)
	if err := s.store.PutObject(context.Background(), storageKey, "text/csv; charset=utf-8", doc.Data); err != nil {
		return nil, fmt.Errorf("store report artifact: %w", err)
	}

	artifact := StoredArtifact{
		ID:          id,
		OrgID:       orgID,
		Kind:        kind,
		Filename:    doc.Filename,
		ContentType: "text/csv; charset=utf-8",
		SizeBytes:   len(doc.Data),
		StorageKey:  storageKey,
		ExpiresAt:   generatedAt.Add(s.ttl),
		CreatedAt:   generatedAt,
	}
	s.files[id] = artifact
	response := s.cloneArtifactForResponse(artifact)
	return &response, nil
}

func (s *Service) ExpireArtifacts(cutoff time.Time) []StoredArtifact {
	s.mu.Lock()
	defer s.mu.Unlock()

	var expired []StoredArtifact
	for id, artifact := range s.files {
		if artifact.ExpiresAt.After(cutoff) {
			continue
		}
		expired = append(expired, artifact)
		delete(s.files, id)
	}
	return expired
}

func (s *Service) DeleteArtifact(storageKey string) error {
	if strings.TrimSpace(storageKey) == "" {
		return nil
	}
	s.mu.RLock()
	store := s.store
	s.mu.RUnlock()
	return store.Delete(storageKey)
}

func (s *Service) cloneArtifactForResponse(artifact StoredArtifact) StoredArtifact {
	if artifact.StorageKey == "" || (!artifact.ExpiresAt.IsZero() && !artifact.ExpiresAt.After(s.nowFunc())) {
		return artifact
	}
	if url, err := s.store.PresignedGetURL(artifact.StorageKey, s.urlTTL); err == nil {
		artifact.DownloadURL = url
	}
	return artifact
}

func (s *Service) ExportRuntimeSnapshot() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return json.Marshal(struct {
		Files map[string]StoredArtifact `json:"files"`
	}{
		Files: s.files,
	})
}

func (s *Service) LoadRuntimeSnapshot(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	var snapshot struct {
		Files map[string]StoredArtifact `json:"files"`
	}
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files = make(map[string]StoredArtifact, len(snapshot.Files))
	for id, artifact := range snapshot.Files {
		s.files[id] = artifact
	}
	return nil
}

func (s *Service) requireReportAccess(principal *appmiddleware.Principal) error {
	if principal == nil {
		return ErrForbidden
	}
	for _, role := range principal.Roles {
		switch strings.ToLower(role) {
		case "admin":
			return nil
		}
	}
	return ErrForbidden
}

type MembershipStatusRow struct {
	HouseholdID         string    `json:"household_id"`
	HouseholdName       string    `json:"household_name"`
	HouseholdStatus     string    `json:"household_status"`
	PrimaryEmail        string    `json:"primary_email,omitempty"`
	PrimaryPhoneE164    string    `json:"primary_phone_e164,omitempty"`
	SwimmerID           string    `json:"swimmer_id"`
	SwimmerName         string    `json:"swimmer_name"`
	PreferredName       string    `json:"preferred_name,omitempty"`
	RegistrationStatus  string    `json:"registration_status"`
	PrimaryGroupCode    string    `json:"primary_group_code,omitempty"`
	PrimaryGroupName    string    `json:"primary_group_name,omitempty"`
	GroupCount          int       `json:"group_count"`
	LastProfileUpdateAt time.Time `json:"last_profile_update_at"`
}

type MembershipStatusReport struct {
	GeneratedAt time.Time             `json:"generated_at"`
	RowCount    int                   `json:"row_count"`
	Rows        []MembershipStatusRow `json:"rows"`
}

func (s *Service) RunMembershipStatusReport(orgID string, principal *appmiddleware.Principal) (*MembershipStatusReport, error) {
	if err := s.requireReportAccess(principal); err != nil {
		return nil, err
	}

	households := s.members.SnapshotHouseholds(orgID)
	contacts := s.members.SnapshotContacts(orgID)
	swimmers := s.members.SnapshotSwimmers(orgID)
	links := s.members.SnapshotSwimmerGroupLinks(orgID)
	groupDefs := s.groups.SnapshotGroups(orgID)

	householdByID := make(map[string]members.Household, len(households))
	for _, household := range households {
		householdByID[household.ID] = household
	}

	primaryContact := make(map[string]members.Contact)
	for _, contact := range contacts {
		if existing, ok := primaryContact[contact.HouseholdID]; ok {
			if existing.IsPrimary {
				continue
			}
		}
		if contact.IsPrimary || primaryContact[contact.HouseholdID].ID == "" {
			primaryContact[contact.HouseholdID] = contact
		}
	}

	groupByCode := make(map[string]groups.Group, len(groupDefs))
	for _, group := range groupDefs {
		groupByCode[strings.ToLower(group.Code)] = group
	}

	linksBySwimmer := make(map[string][]members.SwimmerGroupLink)
	for _, link := range links {
		linksBySwimmer[link.SwimmerID] = append(linksBySwimmer[link.SwimmerID], link)
	}

	rows := make([]MembershipStatusRow, 0, len(swimmers))
	for _, swimmer := range swimmers {
		household := householdByID[swimmer.HouseholdID]
		contact := primaryContact[swimmer.HouseholdID]
		swimmerLinks := linksBySwimmer[swimmer.ID]
		row := MembershipStatusRow{
			HouseholdID:         household.ID,
			HouseholdName:       household.HouseholdName,
			HouseholdStatus:     household.Status,
			PrimaryEmail:        firstNonEmpty(contact.Email, household.PrimaryEmail),
			PrimaryPhoneE164:    firstNonEmpty(contact.PhoneE164, household.PrimaryPhoneE164),
			SwimmerID:           swimmer.ID,
			SwimmerName:         strings.TrimSpace(swimmer.FirstName + " " + swimmer.LastName),
			PreferredName:       swimmer.PreferredName,
			RegistrationStatus:  swimmer.RegistrationStatus,
			GroupCount:          len(swimmerLinks),
			LastProfileUpdateAt: swimmer.UpdatedAt,
		}
		for _, link := range swimmerLinks {
			if !link.IsPrimary && row.PrimaryGroupCode != "" {
				continue
			}
			row.PrimaryGroupCode = link.GroupCode
			if group, ok := groupByCode[strings.ToLower(link.GroupCode)]; ok {
				row.PrimaryGroupName = group.Name
			}
			if link.IsPrimary {
				break
			}
		}
		rows = append(rows, row)
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].HouseholdName != rows[j].HouseholdName {
			return rows[i].HouseholdName < rows[j].HouseholdName
		}
		return rows[i].SwimmerName < rows[j].SwimmerName
	})

	return &MembershipStatusReport{
		GeneratedAt: s.nowFunc(),
		RowCount:    len(rows),
		Rows:        rows,
	}, nil
}

type MeetFeesRow struct {
	InvoiceID       string     `json:"invoice_id"`
	InvoiceNumber   string     `json:"invoice_number"`
	InvoiceStatus   string     `json:"invoice_status"`
	HouseholdID     string     `json:"household_id,omitempty"`
	MeetID          string     `json:"meet_id,omitempty"`
	MeetName        string     `json:"meet_name,omitempty"`
	SwimmerID       string     `json:"swimmer_id,omitempty"`
	SwimmerName     string     `json:"swimmer_name,omitempty"`
	LineItemID      string     `json:"line_item_id"`
	LineDescription string     `json:"line_description"`
	Quantity        int        `json:"quantity"`
	UnitAmountCents int        `json:"unit_amount_cents"`
	TotalCents      int        `json:"total_cents"`
	DueDate         string     `json:"due_date,omitempty"`
	IssuedAt        *time.Time `json:"issued_at,omitempty"`
}

type MeetFeesReport struct {
	GeneratedAt    time.Time     `json:"generated_at"`
	RowCount       int           `json:"row_count"`
	TotalFeesCents int           `json:"total_fees_cents"`
	Rows           []MeetFeesRow `json:"rows"`
}

func (s *Service) RunMeetFeesReport(orgID string, principal *appmiddleware.Principal) (*MeetFeesReport, error) {
	if err := s.requireReportAccess(principal); err != nil {
		return nil, err
	}

	invoices := s.billing.SnapshotInvoices(orgID)
	items := s.billing.SnapshotInvoiceItems(orgID)
	meetDefs := s.meets.SnapshotMeets(orgID)
	swimmers := s.members.SnapshotSwimmers(orgID)

	invoiceByID := make(map[string]billing.Invoice, len(invoices))
	for _, invoice := range invoices {
		invoiceByID[invoice.ID] = invoice
	}
	meetByID := make(map[string]meets.Meet, len(meetDefs))
	for _, meet := range meetDefs {
		meetByID[meet.ID] = meet
	}
	swimmerByID := make(map[string]members.Swimmer, len(swimmers))
	for _, swimmer := range swimmers {
		swimmerByID[swimmer.ID] = swimmer
	}

	rows := make([]MeetFeesRow, 0)
	total := 0
	for _, item := range items {
		if strings.ToLower(item.Category) != "meet_fee" {
			continue
		}
		invoice := invoiceByID[item.InvoiceID]
		meet := meetByID[item.MeetID]
		swimmer := swimmerByID[item.SwimmerID]
		row := MeetFeesRow{
			InvoiceID:       invoice.ID,
			InvoiceNumber:   invoice.InvoiceNumber,
			InvoiceStatus:   invoice.Status,
			HouseholdID:     invoice.HouseholdID,
			MeetID:          item.MeetID,
			MeetName:        meet.Name,
			SwimmerID:       item.SwimmerID,
			SwimmerName:     strings.TrimSpace(swimmer.FirstName + " " + swimmer.LastName),
			LineItemID:      item.ID,
			LineDescription: item.Description,
			Quantity:        item.Quantity,
			UnitAmountCents: item.UnitAmountCents,
			TotalCents:      item.TotalCents,
			DueDate:         invoice.DueDate,
			IssuedAt:        invoice.IssuedAt,
		}
		total += item.TotalCents
		rows = append(rows, row)
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].MeetName != rows[j].MeetName {
			return rows[i].MeetName < rows[j].MeetName
		}
		if rows[i].InvoiceNumber != rows[j].InvoiceNumber {
			return rows[i].InvoiceNumber < rows[j].InvoiceNumber
		}
		return rows[i].LineItemID < rows[j].LineItemID
	})

	return &MeetFeesReport{
		GeneratedAt:    s.nowFunc(),
		RowCount:       len(rows),
		TotalFeesCents: total,
		Rows:           rows,
	}, nil
}

type InvoiceAgingRow struct {
	InvoiceID       string `json:"invoice_id"`
	InvoiceNumber   string `json:"invoice_number"`
	HouseholdID     string `json:"household_id,omitempty"`
	Status          string `json:"status"`
	DueDate         string `json:"due_date,omitempty"`
	DaysPastDue     int    `json:"days_past_due"`
	AgingBucket     string `json:"aging_bucket"`
	TotalCents      int    `json:"total_cents"`
	AmountPaidCents int    `json:"amount_paid_cents"`
	AmountDueCents  int    `json:"amount_due_cents"`
	IsOutstanding   bool   `json:"is_outstanding"`
}

type InvoiceAgingReport struct {
	GeneratedAt             time.Time         `json:"generated_at"`
	AsOfDate                string            `json:"as_of_date"`
	RowCount                int               `json:"row_count"`
	OutstandingInvoiceCount int               `json:"outstanding_invoice_count"`
	OutstandingCents        int               `json:"outstanding_cents"`
	Rows                    []InvoiceAgingRow `json:"rows"`
}

func (s *Service) RunInvoiceAgingReport(orgID string, principal *appmiddleware.Principal, asOf time.Time) (*InvoiceAgingReport, error) {
	if err := s.requireReportAccess(principal); err != nil {
		return nil, err
	}

	if asOf.IsZero() {
		asOf = s.nowFunc()
	}
	asOf = asOf.UTC()

	invoices := s.billing.SnapshotInvoices(orgID)
	rows := make([]InvoiceAgingRow, 0, len(invoices))
	outstandingCount := 0
	outstandingCents := 0

	for _, invoice := range invoices {
		daysPastDue := 0
		bucket := "current"
		if dueAt, ok := parseDate(invoice.DueDate); ok {
			daysPastDue = int(asOf.Sub(dueAt).Hours() / 24)
			if daysPastDue < 0 {
				daysPastDue = 0
			}
			bucket = agingBucket(daysPastDue, invoice.AmountDueCents)
		} else if invoice.AmountDueCents > 0 {
			bucket = "no_due_date"
		}
		outstanding := invoice.AmountDueCents > 0 && invoice.Status != "voided" && invoice.Status != "written_off"
		if outstanding {
			outstandingCount++
			outstandingCents += invoice.AmountDueCents
		}
		rows = append(rows, InvoiceAgingRow{
			InvoiceID:       invoice.ID,
			InvoiceNumber:   invoice.InvoiceNumber,
			HouseholdID:     invoice.HouseholdID,
			Status:          invoice.Status,
			DueDate:         invoice.DueDate,
			DaysPastDue:     daysPastDue,
			AgingBucket:     bucket,
			TotalCents:      invoice.TotalCents,
			AmountPaidCents: invoice.AmountPaidCents,
			AmountDueCents:  invoice.AmountDueCents,
			IsOutstanding:   outstanding,
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].DaysPastDue != rows[j].DaysPastDue {
			return rows[i].DaysPastDue > rows[j].DaysPastDue
		}
		return rows[i].InvoiceNumber < rows[j].InvoiceNumber
	})

	return &InvoiceAgingReport{
		GeneratedAt:             s.nowFunc(),
		AsOfDate:                asOf.Format("2006-01-02"),
		RowCount:                len(rows),
		OutstandingInvoiceCount: outstandingCount,
		OutstandingCents:        outstandingCents,
		Rows:                    rows,
	}, nil
}

type InvoiceDetailRow struct {
	RecordType  string `json:"record_type"` // item | payment | refund
	Description string `json:"description"`
	SwimmerName string `json:"swimmer_name,omitempty"`
	Quantity    int    `json:"quantity,omitempty"`
	AmountCents int    `json:"amount_cents"`
	Status      string `json:"status,omitempty"`
	Date        string `json:"date,omitempty"`
}

type InvoiceDetailReport struct {
	GeneratedAt   time.Time          `json:"generated_at"`
	InvoiceID     string             `json:"invoice_id"`
	InvoiceNumber string             `json:"invoice_number"`
	HouseholdID   string             `json:"household_id,omitempty"`
	RowCount      int                `json:"row_count"`
	Rows          []InvoiceDetailRow `json:"rows"`
}

func (s *Service) RunInvoiceDetailReport(orgID string, principal *appmiddleware.Principal, invoiceID string) (*InvoiceDetailReport, error) {
	if err := s.requireReportAccess(principal); err != nil {
		return nil, err
	}
	invoiceID = strings.TrimSpace(invoiceID)
	if invoiceID == "" {
		return nil, ErrInvoiceIDRequired
	}

	var invoice *billing.Invoice
	for _, inv := range s.billing.SnapshotInvoices(orgID) {
		if inv.ID == invoiceID {
			candidate := inv
			invoice = &candidate
			break
		}
	}
	if invoice == nil {
		return nil, ErrInvoiceNotFound
	}

	swimmers := s.members.SnapshotSwimmers(orgID)
	swimmerByID := make(map[string]members.Swimmer, len(swimmers))
	for _, swimmer := range swimmers {
		swimmerByID[swimmer.ID] = swimmer
	}

	rows := make([]InvoiceDetailRow, 0)
	for _, item := range s.billing.SnapshotInvoiceItems(orgID) {
		if item.InvoiceID != invoiceID {
			continue
		}
		swimmerName := ""
		if item.SwimmerID != "" {
			if swimmer, ok := swimmerByID[item.SwimmerID]; ok {
				swimmerName = strings.TrimSpace(swimmer.FirstName + " " + swimmer.LastName)
			}
		}
		rows = append(rows, InvoiceDetailRow{
			RecordType:  "item",
			Description: item.Description,
			SwimmerName: swimmerName,
			Quantity:    item.Quantity,
			AmountCents: item.TotalCents,
			Status:      item.Category,
			Date:        item.CreatedAt.Format("2006-01-02"),
		})
	}
	for _, payment := range s.billing.SnapshotPayments(orgID) {
		if payment.InvoiceID != invoiceID {
			continue
		}
		rows = append(rows, InvoiceDetailRow{
			RecordType:  "payment",
			Description: payment.Method,
			AmountCents: payment.AmountCents,
			Status:      payment.Status,
			Date:        payment.CreatedAt.Format("2006-01-02"),
		})
	}
	for _, refund := range s.billing.SnapshotRefunds(orgID) {
		if refund.InvoiceID != invoiceID {
			continue
		}
		rows = append(rows, InvoiceDetailRow{
			RecordType:  "refund",
			Description: refund.Reason,
			AmountCents: refund.AmountCents,
			Status:      refund.Status,
			Date:        refund.CreatedAt.Format("2006-01-02"),
		})
	}

	return &InvoiceDetailReport{
		GeneratedAt:   s.nowFunc(),
		InvoiceID:     invoice.ID,
		InvoiceNumber: invoice.InvoiceNumber,
		HouseholdID:   invoice.HouseholdID,
		RowCount:      len(rows),
		Rows:          rows,
	}, nil
}

type PaymentReconciliationRow struct {
	InvoiceID           string `json:"invoice_id"`
	InvoiceNumber       string `json:"invoice_number"`
	PaymentID           string `json:"payment_id"`
	Method              string `json:"method"`
	PaymentStatus       string `json:"payment_status"`
	PaymentAmountCents  int    `json:"payment_amount_cents"`
	RefundedCents       int    `json:"refunded_cents"`
	NetCollectedCents   int    `json:"net_collected_cents"`
	LedgerPaymentCents  int    `json:"ledger_payment_cents"`
	LedgerRefundCents   int    `json:"ledger_refund_cents"`
	IsReconciled        bool   `json:"is_reconciled"`
	ReconciliationNotes string `json:"reconciliation_notes,omitempty"`
}

type PaymentReconciliationReport struct {
	GeneratedAt          time.Time                  `json:"generated_at"`
	RowCount             int                        `json:"row_count"`
	PaymentTotalCents    int                        `json:"payment_total_cents"`
	RefundTotalCents     int                        `json:"refund_total_cents"`
	NetCollectedCents    int                        `json:"net_collected_cents"`
	UnreconciledRowCount int                        `json:"unreconciled_row_count"`
	StripeReportedCents  *int                       `json:"stripe_reported_cents,omitempty"`
	StripeMismatchCents  *int                       `json:"stripe_mismatch_cents,omitempty"`
	StripeAlertTriggered *bool                      `json:"stripe_alert_triggered,omitempty"`
	Rows                 []PaymentReconciliationRow `json:"rows"`
}

func (s *Service) RunPaymentReconciliationReport(orgID string, principal *appmiddleware.Principal, stripeReportedCents *int) (*PaymentReconciliationReport, error) {
	if err := s.requireReportAccess(principal); err != nil {
		return nil, err
	}

	invoices := s.billing.SnapshotInvoices(orgID)
	payments := s.billing.SnapshotPayments(orgID)
	refunds := s.billing.SnapshotRefunds(orgID)
	ledger := s.billing.SnapshotLedgerEvents(orgID)

	invoiceByID := make(map[string]billing.Invoice, len(invoices))
	for _, invoice := range invoices {
		invoiceByID[invoice.ID] = invoice
	}

	refundsByPayment := make(map[string][]billing.Refund)
	for _, refund := range refunds {
		refundsByPayment[refund.PaymentID] = append(refundsByPayment[refund.PaymentID], refund)
	}

	paymentLedger := make(map[string]int)
	refundLedger := make(map[string]int)
	for _, event := range ledger {
		switch event.EventType {
		case "payment_succeeded":
			if event.PaymentID != "" {
				paymentLedger[event.PaymentID] += -event.AmountCents
			}
		case "refund_processed":
			if event.PaymentID != "" {
				refundLedger[event.PaymentID] += event.AmountCents
			}
		}
	}

	rows := make([]PaymentReconciliationRow, 0, len(payments))
	paymentTotal := 0
	refundTotal := 0
	netTotal := 0
	unreconciled := 0

	for _, payment := range payments {
		invoice := invoiceByID[payment.InvoiceID]
		paymentRefunds := refundsByPayment[payment.ID]
		refundedCents := 0
		for _, refund := range paymentRefunds {
			refundedCents += refund.AmountCents
		}
		ledgerPayment := paymentLedger[payment.ID]
		ledgerRefund := refundLedger[payment.ID]
		reconciled := ledgerPayment == payment.AmountCents && ledgerRefund == refundedCents
		notes := []string{}
		if ledgerPayment != payment.AmountCents {
			notes = append(notes, fmt.Sprintf("payment ledger %d != payment amount %d", ledgerPayment, payment.AmountCents))
		}
		if ledgerRefund != refundedCents {
			notes = append(notes, fmt.Sprintf("refund ledger %d != refund total %d", ledgerRefund, refundedCents))
		}

		row := PaymentReconciliationRow{
			InvoiceID:           invoice.ID,
			InvoiceNumber:       invoice.InvoiceNumber,
			PaymentID:           payment.ID,
			Method:              payment.Method,
			PaymentStatus:       payment.Status,
			PaymentAmountCents:  payment.AmountCents,
			RefundedCents:       refundedCents,
			NetCollectedCents:   payment.AmountCents - refundedCents,
			LedgerPaymentCents:  ledgerPayment,
			LedgerRefundCents:   ledgerRefund,
			IsReconciled:        reconciled,
			ReconciliationNotes: strings.Join(notes, "; "),
		}
		if !reconciled {
			unreconciled++
		}
		paymentTotal += payment.AmountCents
		refundTotal += refundedCents
		netTotal += row.NetCollectedCents
		rows = append(rows, row)
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].IsReconciled != rows[j].IsReconciled {
			return !rows[i].IsReconciled
		}
		return rows[i].InvoiceNumber < rows[j].InvoiceNumber
	})

	report := &PaymentReconciliationReport{
		GeneratedAt:          s.nowFunc(),
		RowCount:             len(rows),
		PaymentTotalCents:    paymentTotal,
		RefundTotalCents:     refundTotal,
		NetCollectedCents:    netTotal,
		UnreconciledRowCount: unreconciled,
		Rows:                 rows,
	}

	if stripeReportedCents != nil {
		recon, err := s.billing.GetReconciliationReport(orgID, principal, *stripeReportedCents, 100)
		if err == nil {
			report.StripeReportedCents = intPtr(recon.StripeReportedCents)
			report.StripeMismatchCents = intPtr(recon.MismatchCents)
			report.StripeAlertTriggered = boolPtr(recon.AlertTriggered)
		}
	}

	return report, nil
}

type RegistrationStatusRow struct {
	RegistrationID       string     `json:"registration_id"`
	Season               string     `json:"season"`
	Status               string     `json:"status"`
	RegistrationDate     string     `json:"registration_date"`
	GroupID              string     `json:"group_id"`
	GroupCode            string     `json:"group_code,omitempty"`
	GroupName            string     `json:"group_name,omitempty"`
	HouseholdID          string     `json:"household_id"`
	HouseholdName        string     `json:"household_name,omitempty"`
	SwimmerID            string     `json:"swimmer_id"`
	SwimmerName          string     `json:"swimmer_name,omitempty"`
	WaiverCount          int        `json:"waiver_count"`
	LastWaiverAcceptedAt *time.Time `json:"last_waiver_accepted_at,omitempty"`
}

type RegistrationStatusReport struct {
	GeneratedAt time.Time               `json:"generated_at"`
	RowCount    int                     `json:"row_count"`
	Rows        []RegistrationStatusRow `json:"rows"`
}

func (s *Service) RunRegistrationStatusReport(orgID string, principal *appmiddleware.Principal) (*RegistrationStatusReport, error) {
	if err := s.requireReportAccess(principal); err != nil {
		return nil, err
	}

	registrations := s.groups.SnapshotRegistrations(orgID)
	waivers := s.groups.SnapshotWaivers(orgID)
	groupDefs := s.groups.SnapshotGroups(orgID)
	households := s.members.SnapshotHouseholds(orgID)
	swimmers := s.members.SnapshotSwimmers(orgID)

	groupByID := make(map[string]groups.Group, len(groupDefs))
	for _, group := range groupDefs {
		groupByID[group.ID] = group
	}
	householdByID := make(map[string]members.Household, len(households))
	for _, household := range households {
		householdByID[household.ID] = household
	}
	swimmerByID := make(map[string]members.Swimmer, len(swimmers))
	for _, swimmer := range swimmers {
		swimmerByID[swimmer.ID] = swimmer
	}

	waiverCountByRegistration := make(map[string]int)
	lastWaiverByRegistration := make(map[string]time.Time)
	for _, waiver := range waivers {
		waiverCountByRegistration[waiver.RegistrationID]++
		if waiver.AcceptedAt.After(lastWaiverByRegistration[waiver.RegistrationID]) {
			lastWaiverByRegistration[waiver.RegistrationID] = waiver.AcceptedAt
		}
	}

	rows := make([]RegistrationStatusRow, 0, len(registrations))
	for _, registration := range registrations {
		group := groupByID[registration.GroupID]
		household := householdByID[registration.HouseholdID]
		swimmer := swimmerByID[registration.SwimmerID]
		var lastWaiver *time.Time
		if ts := lastWaiverByRegistration[registration.ID]; !ts.IsZero() {
			cp := ts
			lastWaiver = &cp
		}
		rows = append(rows, RegistrationStatusRow{
			RegistrationID:       registration.ID,
			Season:               registration.Season,
			Status:               registration.Status,
			RegistrationDate:     registration.RegistrationDate,
			GroupID:              registration.GroupID,
			GroupCode:            group.Code,
			GroupName:            group.Name,
			HouseholdID:          registration.HouseholdID,
			HouseholdName:        household.HouseholdName,
			SwimmerID:            registration.SwimmerID,
			SwimmerName:          strings.TrimSpace(swimmer.FirstName + " " + swimmer.LastName),
			WaiverCount:          waiverCountByRegistration[registration.ID],
			LastWaiverAcceptedAt: lastWaiver,
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Season != rows[j].Season {
			return rows[i].Season < rows[j].Season
		}
		if rows[i].GroupName != rows[j].GroupName {
			return rows[i].GroupName < rows[j].GroupName
		}
		return rows[i].SwimmerName < rows[j].SwimmerName
	})

	return &RegistrationStatusReport{
		GeneratedAt: s.nowFunc(),
		RowCount:    len(rows),
		Rows:        rows,
	}, nil
}

func agingBucket(daysPastDue, amountDueCents int) string {
	if amountDueCents <= 0 {
		return "settled"
	}
	switch {
	case daysPastDue <= 0:
		return "current"
	case daysPastDue <= 30:
		return "1_30"
	case daysPastDue <= 60:
		return "31_60"
	case daysPastDue <= 90:
		return "61_90"
	default:
		return "90_plus"
	}
}

func parseDate(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, false
	}
	return parsed.UTC(), true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func ParseStripeReportedCents(raw string) (*int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return nil, fmt.Errorf("stripe_reported_cents must be a non-negative integer")
	}
	return &value, nil
}

func intPtr(value int) *int {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}

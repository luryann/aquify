package handlers

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

func TestRecordAuditLogsAndReturnsAppendFailure(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	p := &appmiddleware.Principal{UserID: "admin-1", Roles: []string{"admin"}}
	// An unmarshalable "after" state makes Append fail before it records.
	err := recordAudit(audit.NewService(), p, "org-1", "billing.invoice.void", "invoice", "inv-1", nil, make(chan int), nil)
	if err == nil {
		t.Fatal("expected the append error to be returned")
	}
	out := buf.String()
	if !strings.Contains(out, "audit append failed") || !strings.Contains(out, "billing.invoice.void") || !strings.Contains(out, "inv-1") {
		t.Fatalf("append failure not logged with context: %q", out)
	}
	if err := recordAudit(nil, p, "org-1", "x", "y", "z", nil, nil, nil); err != nil {
		t.Fatalf("nil service must be a no-op, got %v", err)
	}
}

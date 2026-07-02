package reports

import (
	"strings"
	"testing"
)

func TestBuildCSVQuotesHeadersAndRows(t *testing.T) {
	t.Parallel()

	doc, err := BuildCSV("export.csv", []string{"Name", "Note"}, [][]string{
		{"Lane, Sky", "Line one\nLine two"},
	})
	if err != nil {
		t.Fatalf("BuildCSV() error = %v", err)
	}
	if doc.Filename != "export.csv" {
		t.Fatalf("BuildCSV() filename = %q, want export.csv", doc.Filename)
	}
	got := string(doc.Data)
	if !strings.Contains(got, "\"Lane, Sky\"") || !strings.Contains(got, "\"Line one\nLine two\"") {
		t.Fatalf("BuildCSV() data = %q, want quoted comma and newline fields", got)
	}
}

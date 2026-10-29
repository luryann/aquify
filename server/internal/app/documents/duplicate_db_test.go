package documents

import (
	"errors"
	"sync"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/testutil"
	"github.com/google/uuid"
)

// Two instances can both pass the in-memory name check; the database's unique
// index must decide and its violation must surface as ErrDuplicateName.
func TestDocumentTypeDuplicateNameArbitratedByDatabase(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	org := uuid.NewString()
	a, b := NewServiceWithPool(pool), NewServiceWithPool(pool)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, svc := range []*Service{a, b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = svc.CreateDocumentType(org, "Liability Waiver", "waiver", "", "household", true, true, nil, uuid.NewString())
		}()
	}
	wg.Wait()
	ok, dup := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrDuplicateName):
			dup++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if ok != 1 || dup != 1 {
		t.Fatalf("want one success and one ErrDuplicateName, got ok=%d dup=%d", ok, dup)
	}

	// A stale instance renaming onto a name created elsewhere also maps cleanly.
	other, err := a.CreateDocumentType(org, "Medical", "medical", "", "swimmer", true, true, nil, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	c := NewServiceWithPool(pool)
	if _, err := c.CreateDocumentType(org, "Photo Release", "release", "", "swimmer", true, true, nil, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if _, err := a.UpdateDocumentType(org, other.ID, map[string]interface{}{"name": "photo release"}, uuid.NewString()); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("rename onto name created by another instance: want ErrDuplicateName, got %v", err)
	}
}

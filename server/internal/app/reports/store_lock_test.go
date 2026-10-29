package reports

import (
	"context"
	"testing"
	"time"

	"github.com/dareaquatics/aquify/server/internal/storage"
)

// blockingStore stalls PutObject until released.
type blockingStore struct {
	storage.ObjectStorage
	entered chan struct{}
	release chan struct{}
}

func (b *blockingStore) PutObject(ctx context.Context, key, contentType string, data []byte) error {
	close(b.entered)
	<-b.release
	return b.ObjectStorage.PutObject(ctx, key, contentType, data)
}

func TestStoreCSVArtifactDoesNotHoldServiceLockDuringUpload(t *testing.T) {
	svc := NewService(nil, nil, nil, nil)
	store := &blockingStore{ObjectStorage: storage.NewStubStorage(""), entered: make(chan struct{}), release: make(chan struct{})}
	svc.SetStorage(store)

	done := make(chan error, 1)
	go func() {
		_, err := svc.StoreCSVArtifact("org", "aging", time.Now().UTC(), &CSVDocument{Filename: "a.csv", Data: []byte("x")})
		done <- err
	}()
	<-store.entered

	// While the upload is stalled, other service operations must not block.
	got := make(chan struct{})
	go func() {
		_, _ = svc.ExportRuntimeSnapshot()
		svc.ExpireArtifacts(time.Now())
		close(got)
	}()
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("service lock is held across PutObject")
	}
	close(store.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(svc.files) != 1 {
		t.Fatalf("artifact not recorded: %d", len(svc.files))
	}
}

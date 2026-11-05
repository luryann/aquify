package dbmigrate

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// db/migrations is the source of truth; the embedded copy (made at image build
// time) must be identical. When nothing is embedded (a plain `go test` checkout)
// this is skipped, and CI runs `make embed-migrations` first so it is enforced.
func TestEmbeddedCopyMatchesSourceMigrations(t *testing.T) {
	emb := Embedded()
	files, err := fs.Glob(emb, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Skip("no migrations embedded (run `make embed-migrations`)")
	}
	src, _ := filepath.Glob(filepath.Join(sourceDir(), "*.sql"))
	if len(files) != len(src) {
		t.Fatalf("embedded %d migrations, source has %d", len(files), len(src))
	}
	for _, p := range src {
		want, _ := os.ReadFile(p)
		got, err := fs.ReadFile(emb, filepath.Base(p))
		if err != nil || string(got) != string(want) {
			t.Fatalf("embedded %s differs from db/migrations (err=%v)", filepath.Base(p), err)
		}
	}
}

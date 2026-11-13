package main

import (
	"io/fs"
	"strings"
	"testing"
)

// Production runs migrations from the files embedded in the binary. Tests normally replace
// migrationSource, so this checks the real default: it must expose the goose migration files.
// (Not parallel: other tests in this package swap the package-level variable.)
func TestDefaultMigrationSourceIsTheEmbeddedMigrationSet(t *testing.T) {
	fsys := migrationSource()
	files, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("the embedded migration set is empty; the binary could not migrate a database")
	}
	for _, name := range files {
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			t.Fatalf("read embedded migration %s: %v", name, err)
		}
		if !strings.Contains(string(data), "-- +goose Up") {
			t.Errorf("embedded migration %s has no goose Up section", name)
		}
	}
}

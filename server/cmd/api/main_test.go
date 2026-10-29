package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestShouldLoadLocalEnvFilesOnlyForExplicitLocalDevelopmentModes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		appEnv string
		want   bool
	}{
		{appEnv: "development", want: true},
		{appEnv: "local", want: true},
		{appEnv: "test", want: false},
		{appEnv: "staging", want: false},
		{appEnv: "production", want: false},
		{appEnv: "", want: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.appEnv, func(t *testing.T) {
			t.Parallel()
			if got := shouldLoadLocalEnvFiles(tc.appEnv); got != tc.want {
				t.Fatalf("shouldLoadLocalEnvFiles(%q) = %v, want %v", tc.appEnv, got, tc.want)
			}
		})
	}
}

func TestLoadLocalEnvFilesSkipsNonLocalRuntimes(t *testing.T) {
	// Not parallel: swaps the package-level loadDotEnvFile hook.

	originalLoader := loadDotEnvFile
	t.Cleanup(func() {
		loadDotEnvFile = originalLoader
	})

	called := false
	loadDotEnvFile = func(path string) {
		called = true
	}

	for _, appEnv := range []string{"", "test", "staging", "production"} {
		loadLocalEnvFiles(appEnv)
	}

	if called {
		t.Fatal("expected non-local runtimes to skip .env.local loading")
	}
}

func TestLoadLocalEnvFilesLoadsExpectedPathsForDevelopment(t *testing.T) {
	// Not parallel: swaps the package-level loadDotEnvFile hook.

	originalLoader := loadDotEnvFile
	t.Cleanup(func() {
		loadDotEnvFile = originalLoader
	})

	var paths []string
	loadDotEnvFile = func(path string) {
		paths = append(paths, path)
	}

	loadLocalEnvFiles("development")

	want := []string{"../.env.local", ".env.local"}
	if !slices.Equal(paths, want) {
		t.Fatalf("loadLocalEnvFiles loaded %v, want %v", paths, want)
	}
}

func TestLoadDotEnvPreservesQuotedCommentsAndExistingEmptyValues(t *testing.T) {
	t.Setenv("AQUIFY_DOTENV_EMPTY", "")
	for _, key := range []string{"AQUIFY_DOTENV_DOUBLE", "AQUIFY_DOTENV_SINGLE", "AQUIFY_DOTENV_PLAIN"} {
		os.Unsetenv(key)
		t.Cleanup(func() { os.Unsetenv(key) })
	}
	path := filepath.Join(t.TempDir(), "test.env")
	content := "AQUIFY_DOTENV_EMPTY=override\nAQUIFY_DOTENV_DOUBLE=\"secret # value\" # comment\nAQUIFY_DOTENV_SINGLE='another # value' # comment\nAQUIFY_DOTENV_PLAIN=value # comment\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	loadDotEnv(path)
	for key, want := range map[string]string{"AQUIFY_DOTENV_EMPTY": "", "AQUIFY_DOTENV_DOUBLE": "secret # value", "AQUIFY_DOTENV_SINGLE": "another # value", "AQUIFY_DOTENV_PLAIN": "value"} {
		if got := os.Getenv(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestLoadDotEnvSupportsExportPrefixAndEscapedNewlines(t *testing.T) {
	keys := []string{"AQUIFY_DOTENV_EXPORTED", "AQUIFY_DOTENV_NEWLINE"}
	for _, key := range keys {
		os.Unsetenv(key)
		t.Cleanup(func() { os.Unsetenv(key) })
	}
	path := filepath.Join(t.TempDir(), "test.env")
	content := "export AQUIFY_DOTENV_EXPORTED=yes\nAQUIFY_DOTENV_NEWLINE=\"line1\\nline2\"\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	loadDotEnv(path)
	if got := os.Getenv("AQUIFY_DOTENV_EXPORTED"); got != "yes" {
		t.Errorf("export-prefixed key = %q, want yes", got)
	}
	if got := os.Getenv("AQUIFY_DOTENV_NEWLINE"); got != "line1\nline2" {
		t.Errorf("escaped newline = %q", got)
	}
}

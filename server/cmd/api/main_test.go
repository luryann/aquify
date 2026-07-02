package main

import (
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
	t.Parallel()

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
	t.Parallel()

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

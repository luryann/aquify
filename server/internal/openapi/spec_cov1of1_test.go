package openapi

import (
	"bytes"
	"errors"
	"testing"
)

// cov1xopenapiSwapSpec replaces the embedded spec for the duration of a test.
// Tests using it are deliberately not parallel: parallel tests only resume
// after all serial tests (and their cleanups) have finished.
func cov1xopenapiSwapSpec(t *testing.T, replacement []byte) {
	t.Helper()
	orig := rawSpec
	rawSpec = replacement
	t.Cleanup(func() { rawSpec = orig })
}

func TestCov1xopenapiBytesReturnsDefensiveCopy(t *testing.T) {
	got := Bytes()
	if len(got) == 0 {
		t.Fatal("expected packaged spec bytes")
	}
	if !bytes.Equal(got, rawSpec) || string(got) != String() {
		t.Fatal("Bytes content differs from the packaged spec")
	}
	orig := rawSpec[0]
	got[0] ^= 0xff
	if rawSpec[0] != orig {
		t.Fatal("mutating Bytes result changed the packaged spec")
	}
	if again := Bytes(); again[0] != orig {
		t.Fatal("second Bytes call observed caller mutation")
	}
}

func TestCov1xopenapiBytesNilWhenSpecEmpty(t *testing.T) {
	cov1xopenapiSwapSpec(t, nil)
	if got := Bytes(); got != nil {
		t.Fatalf("expected nil bytes for empty spec, got %q", got)
	}
}

func TestCov1xopenapiValidateRejectsBlankSpec(t *testing.T) {
	for name, blank := range map[string][]byte{
		"nil":        nil,
		"empty":      {},
		"whitespace": []byte(" \n\t\r\n"),
	} {
		t.Run(name, func(t *testing.T) {
			cov1xopenapiSwapSpec(t, blank)
			if err := Validate(); !errors.Is(err, errSpecUnavailable) {
				t.Fatalf("expected errSpecUnavailable, got %v", err)
			}
		})
	}
}

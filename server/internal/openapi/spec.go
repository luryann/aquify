package openapi

import (
	"bytes"
	_ "embed"
	"errors"
)

var (
	//go:embed spec/openapi.yaml
	rawSpec []byte

	errSpecUnavailable = errors.New("openapi: packaged OpenAPI spec is unavailable")
)

// Bytes returns a defensive copy of the packaged OpenAPI document.
func Bytes() []byte {
	if len(rawSpec) == 0 {
		return nil
	}
	return append([]byte(nil), rawSpec...)
}

// String returns the packaged OpenAPI document as UTF-8 text.
func String() string {
	return string(rawSpec)
}

// Validate reports whether the packaged OpenAPI document is present.
func Validate() error {
	if len(bytes.TrimSpace(rawSpec)) == 0 {
		return errSpecUnavailable
	}
	return nil
}

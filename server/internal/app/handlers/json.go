package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	"github.com/go-playground/validator/v10"
)

const defaultJSONBodyLimit int64 = 1 << 20

var errSingleJSONObject = errors.New("request body must contain a single JSON object")

type decodeJSONOptions struct {
	AllowEmpty bool
	MaxBytes   int64
}

type decodeAndValidateOptions struct {
	AllowEmpty        bool
	MaxBytes          int64
	InvalidStatus     int
	InvalidCode       string
	InvalidMessage    string
	ValidationStatus  int
	ValidationCode    string
	ValidationMessage string
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) error {
	return decodeJSONBodyWithOptions(w, r, dst, decodeJSONOptions{})
}

func decodeOptionalJSONBody(w http.ResponseWriter, r *http.Request, dst any) error {
	return decodeJSONBodyWithOptions(w, r, dst, decodeJSONOptions{AllowEmpty: true})
}

func decodeJSONBodyWithOptions(w http.ResponseWriter, r *http.Request, dst any, opts decodeJSONOptions) error {
	limit := opts.MaxBytes
	if limit <= 0 {
		limit = defaultJSONBodyLimit
	}

	defer r.Body.Close()
	r.Body = http.MaxBytesReader(w, r.Body, limit)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		if opts.AllowEmpty && errors.Is(err, io.EOF) {
			return nil
		}
		return normalizeJSONDecodeError(err, limit)
	}

	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errSingleJSONObject
	}

	return nil
}

func decodeAndValidateJSON(w http.ResponseWriter, r *http.Request, dst any, validate *validator.Validate, opts decodeAndValidateOptions) bool {
	if err := decodeJSONBodyWithOptions(w, r, dst, decodeJSONOptions{
		AllowEmpty: opts.AllowEmpty,
		MaxBytes:   opts.MaxBytes,
	}); err != nil {
		status := opts.InvalidStatus
		if status == 0 {
			status = http.StatusBadRequest
		}
		code := opts.InvalidCode
		if code == "" {
			code = "invalid_request_body"
		}
		message := opts.InvalidMessage
		if message == "" {
			message = err.Error()
		}
		httpx.WriteError(w, r, status, code, message, nil)
		return false
	}

	if validate == nil {
		return true
	}
	if err := validate.Struct(dst); err != nil {
		status := opts.ValidationStatus
		if status == 0 {
			status = http.StatusUnprocessableEntity
		}
		code := opts.ValidationCode
		if code == "" {
			code = "validation_error"
		}
		message := opts.ValidationMessage
		if message == "" {
			message = "Request validation failed."
		}
		httpx.WriteError(w, r, status, code, message, httpx.FormatValidationErrors(err))
		return false
	}

	return true
}

func normalizeJSONDecodeError(err error, limit int64) error {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return fmt.Errorf("request body must not exceed %d bytes", limit)
	}

	if errors.Is(err, io.ErrUnexpectedEOF) {
		return errors.New("request body contains malformed JSON")
	}

	if syntaxErr := new(json.SyntaxError); errors.As(err, &syntaxErr) {
		return errors.New("request body contains malformed JSON")
	}

	if strings.HasPrefix(err.Error(), "json: unknown field ") {
		return err
	}

	return err
}

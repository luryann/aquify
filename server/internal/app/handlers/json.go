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

// bodyTooLargeError marks a request body over the configured limit; it maps to 413.
type bodyTooLargeError struct{ limit int64 }

func (e bodyTooLargeError) Error() string {
	return fmt.Sprintf("request body must not exceed %d bytes", e.limit)
}

func isBodyTooLarge(err error) bool {
	var tooLarge bodyTooLargeError
	return errors.As(err, &tooLarge)
}

// writeJSONDecodeError answers a failed body decode: 413 for an oversized body,
// otherwise 400 with the caller's code and message (or the decoder's safe
// message when message is empty).
func writeJSONDecodeError(w http.ResponseWriter, r *http.Request, err error, code, message string) {
	if isBodyTooLarge(err) {
		httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, "request_body_too_large", err.Error(), nil)
		return
	}
	if message == "" {
		message = err.Error()
	}
	httpx.WriteError(w, r, http.StatusBadRequest, code, message, nil)
}

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
		if isBodyTooLarge(err) {
			status, code, message = http.StatusRequestEntityTooLarge, "request_body_too_large", err.Error()
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

// normalizeJSONDecodeError maps decoder errors to client-safe errors: no Go
// struct or field names, no reflection detail.
func normalizeJSONDecodeError(err error, limit int64) error {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return bodyTooLargeError{limit: limit}
	}

	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return errors.New("request body contains malformed JSON")
	}

	if syntaxErr := new(json.SyntaxError); errors.As(err, &syntaxErr) {
		return errors.New("request body contains malformed JSON")
	}

	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		if typeErr.Field != "" {
			return fmt.Errorf("request body field %q has the wrong type", typeErr.Field)
		}
		return errors.New("request body has the wrong type")
	}

	// DisallowUnknownFields reports `json: unknown field "x"`, which only
	// echoes a name the client sent.
	if strings.HasPrefix(err.Error(), "json: unknown field ") {
		return err
	}

	return errors.New("request body is invalid")
}

// decodeAndValidateBadRequest decodes and validates a body, answering validation
// failures with 400 validation_failed.
func decodeAndValidateBadRequest(w http.ResponseWriter, r *http.Request, dst any, validate *validator.Validate) bool {
	return decodeAndValidateJSON(w, r, dst, validate, decodeAndValidateOptions{
		InvalidStatus:     http.StatusBadRequest,
		InvalidCode:       "invalid_request_body",
		ValidationStatus:  http.StatusBadRequest,
		ValidationCode:    "validation_failed",
		ValidationMessage: "One or more request fields are invalid.",
	})
}

// decodeAndValidateUnprocessable decodes and validates a body, answering
// validation failures with 422 validation_error.
func decodeAndValidateUnprocessable(w http.ResponseWriter, r *http.Request, dst any, validate *validator.Validate) bool {
	return decodeAndValidateJSON(w, r, dst, validate, decodeAndValidateOptions{
		InvalidStatus:     http.StatusBadRequest,
		InvalidCode:       "invalid_request_body",
		ValidationStatus:  http.StatusUnprocessableEntity,
		ValidationCode:    "validation_error",
		ValidationMessage: "Request validation failed.",
	})
}

// decodeAndValidateLegacy decodes and validates a body for the older handlers whose
// contract is 400 bad_request on malformed JSON and 422 validation_error with the given message.
func decodeAndValidateLegacy(w http.ResponseWriter, r *http.Request, dst any, validate *validator.Validate, validationMessage string) bool {
	return decodeAndValidateJSON(w, r, dst, validate, decodeAndValidateOptions{
		InvalidCode:       "bad_request",
		InvalidMessage:    "Invalid JSON body.",
		ValidationMessage: validationMessage,
	})
}

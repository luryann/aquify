package httpx

import (
	"errors"
	"strings"

	"github.com/go-playground/validator/v10"
)

type ValidationIssue struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Rule    string `json:"rule,omitempty"`
}

func FormatValidationErrors(err error) []ValidationIssue {
	if err == nil {
		return nil
	}

	var validationErrors validator.ValidationErrors
	if errors.As(err, &validationErrors) {
		issues := make([]ValidationIssue, 0, len(validationErrors))
		for _, validationError := range validationErrors {
			issues = append(issues, ValidationIssue{
				Field:   strings.ToLower(validationError.Field()),
				Message: validationMessage(validationError),
				Rule:    validationError.Tag(),
			})
		}
		return issues
	}

	return []ValidationIssue{{
		Field:   "request",
		Message: err.Error(),
	}}
}

func validationMessage(validationError validator.FieldError) string {
	switch validationError.Tag() {
	case "required":
		return "This field is required."
	case "email":
		return "Must be a valid email address."
	case "min":
		return "Value is below the minimum allowed."
	case "max":
		return "Value exceeds the maximum allowed."
	default:
		return "Invalid value."
	}
}

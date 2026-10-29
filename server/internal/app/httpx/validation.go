package httpx

import (
	"errors"
	"reflect"
	"strings"
	"unicode"

	"github.com/go-playground/validator/v10"
)

type ValidationIssue struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Rule    string `json:"rule,omitempty"`
}

// NewValidator returns a validator whose field names are the JSON tag names, so
// validation errors line up with the keys clients actually send
// (sleep_quality, not SleepQuality).
func NewValidator() *validator.Validate {
	v := validator.New()
	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		name := strings.SplitN(f.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})
	return v
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
				Field:   validationFieldName(validationError),
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

// validationFieldName prefers the JSON tag name registered by NewValidator and
// only lowercases bare Go field names from validators built without it.
func validationFieldName(validationError validator.FieldError) string {
	name := validationError.Field()
	if name != "" && unicode.IsUpper(rune(name[0])) {
		return strings.ToLower(name)
	}
	return name
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
	case "gte":
		return "Must be at least " + validationError.Param() + "."
	case "lte":
		return "Must be at most " + validationError.Param() + "."
	case "gt":
		return "Must be greater than " + validationError.Param() + "."
	case "lt":
		return "Must be less than " + validationError.Param() + "."
	case "len":
		return "Must have length " + validationError.Param() + "."
	case "oneof":
		return "Must be one of: " + validationError.Param() + "."
	default:
		return "Invalid value."
	}
}

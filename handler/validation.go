
package handler

import (
	"fmt"
	"strings"
)

// Validator provides request validation utilities
type Validator struct {
	errors []string
}

// NewValidator creates a new validator
func NewValidator() *Validator {
	return &Validator{
		errors: make([]string, 0),
	}
}

// RequireNonEmpty validates that a string field is not empty
func (v *Validator) RequireNonEmpty(field, value string) {
	if strings.TrimSpace(value) == "" {
		v.errors = append(v.errors, fmt.Sprintf("%s is required", field))
	}
}

// RequireNoPathTraversal validates that a path doesn't contain ..
func (v *Validator) RequireNoPathTraversal(field, value string) {
	if strings.Contains(value, "..") {
		v.errors = append(v.errors, fmt.Sprintf("%s contains invalid path traversal", field))
	}
}

// RequireValidFormat validates that format is one of the allowed formats
func (v *Validator) RequireValidFormat(format string, allowedFormats []string) {
	if format == "" {
		return // Empty is OK, will use default
	}

	for _, allowed := range allowedFormats {
		if format == allowed {
			return
		}
	}

	v.errors = append(v.errors, fmt.Sprintf("format must be one of: %s", strings.Join(allowedFormats, ", ")))
}

// IsValid returns true if there are no validation errors
func (v *Validator) IsValid() bool {
	return len(v.errors) == 0
}

// Errors returns all validation errors
func (v *Validator) Errors() []string {
	return v.errors
}

// Error returns a single string with all errors
func (v *Validator) Error() string {
	return strings.Join(v.errors, "; ")
}

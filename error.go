package bencode

import (
	"fmt"
	"strings"
)

// Error represents an error that occurred during bencode processing.
type Error struct {
	// Type categorizes the error.
	Type ErrorType
	// Msg provides a human-readable description of the error.
	Msg string
	// FieldName is relevant for struct field errors or map key errors.
	FieldName string
	// WrappedErr holds the underlying error, if any.
	WrappedErr error
}

// Error returns a string representation of the bencode error.
// It includes the error type, field name (if applicable), message, and any wrapped error.
func (e *Error) Error() string {
	var sb strings.Builder
	sb.WriteString("bencode: ")
	if e.FieldName != "" {
		sb.WriteString(fmt.Sprintf("field %q: ", e.FieldName))
	}
	sb.WriteString(e.Msg)
	if e.WrappedErr != nil {
		sb.WriteString(": ")
		sb.WriteString(e.WrappedErr.Error())
	}
	return sb.String()
}

// Unwrap returns the underlying error, if any, to support error chaining.
func (e *Error) Unwrap() error {
	return e.WrappedErr
}

// ErrorType defines the category of a bencode error.
type ErrorType string

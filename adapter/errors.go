package adapter

import (
	"errors"
	"fmt"
)

// ErrUnsupportedOperation identifies a provider operation that is not
// declared by the configured adapter.
var ErrUnsupportedOperation = errors.New("unsupported provider operation")

// UnsupportedOperationError carries the provider and operation that rejected
// a call while preserving errors.Is compatibility with ErrUnsupportedOperation.
type UnsupportedOperationError struct {
	Provider  string
	Operation Operation
}

func (e *UnsupportedOperationError) Error() string {
	if e == nil {
		return ErrUnsupportedOperation.Error()
	}
	if e.Provider == "" {
		return fmt.Sprintf("%s: %s", ErrUnsupportedOperation, e.Operation)
	}
	return fmt.Sprintf("%s: provider=%s operation=%s", ErrUnsupportedOperation, e.Provider, e.Operation)
}

func (e *UnsupportedOperationError) Unwrap() error {
	return ErrUnsupportedOperation
}

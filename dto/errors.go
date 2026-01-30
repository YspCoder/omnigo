// Package dto defines standardized request and response payloads.
package dto

import "fmt"

// LLMError represents a unified error structure across providers.
type LLMError struct {
	Code     int    `json:"code"`
	Message  string `json:"message"`
	Provider string `json:"provider"`
}

func (e *LLMError) Error() string {
	if e == nil {
		return ""
	}
	if e.Provider == "" {
		return fmt.Sprintf("%s (code=%d)", e.Message, e.Code)
	}
	return fmt.Sprintf("%s (code=%d, provider=%s)", e.Message, e.Code, e.Provider)
}

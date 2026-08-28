package dto

import (
	"errors"
	"fmt"
	"strings"
)

const (
	TaskStatusQueued     = "queued"
	TaskStatusInProgress = "in_progress"
	TaskStatusSucceeded  = "succeeded"
	TaskStatusFailed     = "failed"
)

// ErrUnsupportedTaskStatus indicates that a provider returned an unknown task status.
var ErrUnsupportedTaskStatus = errors.New("unsupported task status")

// NormalizeTaskStatus maps provider-specific status aliases to a stable task status.
// Unknown statuses are returned unchanged together with ErrUnsupportedTaskStatus.
func NormalizeTaskStatus(value string) (string, error) {
	original := strings.TrimSpace(value)
	normalized := strings.ToLower(original)
	normalized = strings.NewReplacer("-", "_", " ", "_").Replace(normalized)

	switch normalized {
	case "queued", "pending", "submitted", "not_start":
		return TaskStatusQueued, nil
	case "in_progress", "running", "processing":
		return TaskStatusInProgress, nil
	case "succeeded", "success", "completed":
		return TaskStatusSucceeded, nil
	case "failed", "failure", "error", "rejected", "cancelled", "canceled":
		return TaskStatusFailed, nil
	default:
		return original, fmt.Errorf("%w: %q", ErrUnsupportedTaskStatus, original)
	}
}

// IsPending reports whether a task is queued or in progress.
func IsPending(value string) bool {
	status, err := NormalizeTaskStatus(value)
	return err == nil && (status == TaskStatusQueued || status == TaskStatusInProgress)
}

// IsSucceeded reports whether a task completed successfully.
func IsSucceeded(value string) bool {
	status, err := NormalizeTaskStatus(value)
	return err == nil && status == TaskStatusSucceeded
}

// IsFailed reports whether a task reached a failed terminal state.
func IsFailed(value string) bool {
	status, err := NormalizeTaskStatus(value)
	return err == nil && status == TaskStatusFailed
}

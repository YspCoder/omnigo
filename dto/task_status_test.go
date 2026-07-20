package dto

import (
	"errors"
	"testing"
)

func TestNormalizeTaskStatus(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "queued", raw: "queued", want: TaskStatusQueued},
		{name: "pending", raw: "PENDING", want: TaskStatusQueued},
		{name: "submitted", raw: "submitted", want: TaskStatusQueued},
		{name: "in progress", raw: "in_progress", want: TaskStatusInProgress},
		{name: "hyphenated in progress", raw: "in-progress", want: TaskStatusInProgress},
		{name: "running", raw: "RUNNING", want: TaskStatusInProgress},
		{name: "processing", raw: "processing", want: TaskStatusInProgress},
		{name: "succeeded", raw: "succeeded", want: TaskStatusSucceeded},
		{name: "success", raw: "SUCCESS", want: TaskStatusSucceeded},
		{name: "completed", raw: "completed", want: TaskStatusSucceeded},
		{name: "failed", raw: "failed", want: TaskStatusFailed},
		{name: "failure", raw: "failure", want: TaskStatusFailed},
		{name: "error", raw: "error", want: TaskStatusFailed},
		{name: "rejected", raw: "rejected", want: TaskStatusFailed},
		{name: "cancelled", raw: "cancelled", want: TaskStatusFailed},
		{name: "canceled", raw: "canceled", want: TaskStatusFailed},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := NormalizeTaskStatus(test.raw)
			if err != nil {
				t.Fatalf("NormalizeTaskStatus(%q) error = %v", test.raw, err)
			}
			if got != test.want {
				t.Fatalf("NormalizeTaskStatus(%q) = %q, want %q", test.raw, got, test.want)
			}
		})
	}
}

func TestTaskStatusPredicates(t *testing.T) {
	tests := []struct {
		status    string
		pending   bool
		succeeded bool
		failed    bool
	}{
		{status: "queued", pending: true},
		{status: "in_progress", pending: true},
		{status: "processing", pending: true},
		{status: "completed", succeeded: true},
		{status: "success", succeeded: true},
		{status: "failed", failed: true},
		{status: "rejected", failed: true},
	}

	for _, test := range tests {
		if got := IsPending(test.status); got != test.pending {
			t.Errorf("IsPending(%q) = %v, want %v", test.status, got, test.pending)
		}
		if got := IsSucceeded(test.status); got != test.succeeded {
			t.Errorf("IsSucceeded(%q) = %v, want %v", test.status, got, test.succeeded)
		}
		if got := IsFailed(test.status); got != test.failed {
			t.Errorf("IsFailed(%q) = %v, want %v", test.status, got, test.failed)
		}
	}
}

func TestNormalizeTaskStatusPreservesUnknownValue(t *testing.T) {
	const raw = "vendor_waiting"

	got, err := NormalizeTaskStatus(raw)
	if got != raw {
		t.Fatalf("NormalizeTaskStatus(%q) = %q, want original value", raw, got)
	}
	if !errors.Is(err, ErrUnsupportedTaskStatus) {
		t.Fatalf("error = %v, want ErrUnsupportedTaskStatus", err)
	}
	if IsPending(raw) || IsSucceeded(raw) || IsFailed(raw) {
		t.Fatal("unknown status must not be classified as pending, succeeded, or failed")
	}
}

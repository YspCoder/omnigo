package llm

import (
	"strings"
	"testing"
)

func TestSSEDecoderHandlesCRLFEventDelimiters(t *testing.T) {
	decoder := NewSSEDecoder(strings.NewReader("event: message\r\ndata: hello\r\n\r\n"))

	if !decoder.Next() {
		t.Fatalf("Next() = false, want event; err = %v", decoder.Err())
	}
	event := decoder.Event()
	if event.Type != "message" {
		t.Fatalf("event type = %q, want message", event.Type)
	}
	if string(event.Data) != "hello\n" {
		t.Fatalf("event data = %q, want %q", event.Data, "hello\n")
	}
	if decoder.Next() {
		t.Fatal("Next() = true after the only event, want false")
	}
	if err := decoder.Err(); err != nil {
		t.Fatalf("decoder error = %v", err)
	}
}

func TestSSEDecoderFlushesEventAtEOF(t *testing.T) {
	decoder := NewSSEDecoder(strings.NewReader("event: done\ndata: final\n"))

	if !decoder.Next() {
		t.Fatalf("Next() = false, want final event; err = %v", decoder.Err())
	}
	event := decoder.Event()
	if event.Type != "done" || string(event.Data) != "final\n" {
		t.Fatalf("event = %#v, want type done and data %q", event, "final\n")
	}
	if decoder.Next() {
		t.Fatal("Next() = true after EOF event was flushed, want false")
	}
}

func TestSSEDecoderAcceptsLongDataLines(t *testing.T) {
	data := strings.Repeat("x", 128*1024)
	decoder := NewSSEDecoder(strings.NewReader("data: " + data + "\n\n"))

	if !decoder.Next() {
		t.Fatalf("Next() = false for long event; err = %v", decoder.Err())
	}
	if got := string(decoder.Event().Data); got != data+"\n" {
		t.Fatalf("data length = %d, want %d", len(got), len(data)+1)
	}
	if err := decoder.Err(); err != nil {
		t.Fatalf("decoder error = %v", err)
	}
}

package utils

import (
	"bufio"
	"bytes"
	"io"
)

// SSEDecoder handles Server-Sent Events (SSE) streaming
type SSEDecoder struct {
	reader  *bufio.Scanner
	current SSEEvent
	err     error
	done    bool
}

type SSEEvent struct {
	Type string
	Data []byte
}

func NewSSEDecoder(reader io.Reader) *SSEDecoder {
	scanner := bufio.NewScanner(reader)
	// Image events can carry base64 data on a single long JSON line.
	scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
	return &SSEDecoder{reader: scanner}
}

func (d *SSEDecoder) Next() bool {
	if d.done || d.err != nil {
		return false
	}

	event := ""
	data := bytes.NewBuffer(nil)

	for d.reader.Scan() {
		line := d.reader.Bytes()
		// Scanner strips '\n' but leaves the '\r' from CRLF responses.
		// Normalize it so blank lines delimit events consistently across
		// providers and transports.
		line = bytes.TrimSuffix(line, []byte{'\r'})

		// Dispatch event on empty line
		if len(line) == 0 {
			if data.Len() == 0 {
				event = ""
				continue
			}
			d.current = SSEEvent{
				Type: event,
				Data: data.Bytes(),
			}
			return true
		}

		// Split "event: value" into parts
		name, value, _ := bytes.Cut(line, []byte(":"))

		// Remove optional space after colon
		if len(value) > 0 && value[0] == ' ' {
			value = value[1:]
		}

		switch string(name) {
		case "":
			continue // Skip comments
		case "event":
			event = string(value)
		case "data":
			data.Write(value)
			data.WriteRune('\n')
		}
	}

	if err := d.reader.Err(); err != nil {
		d.err = err
		d.done = true
		return false
	}
	// SSE permits the final event to end at EOF without a blank delimiter.
	// Deliver it once before reporting end-of-stream.
	if data.Len() > 0 {
		d.current = SSEEvent{Type: event, Data: data.Bytes()}
		d.done = true
		return true
	}
	d.done = true
	return false
}

func (d *SSEDecoder) Event() SSEEvent {
	return d.current
}

func (d *SSEDecoder) Err() error {
	return d.err
}

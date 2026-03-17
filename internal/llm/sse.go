package llm

import (
	"bufio"
	"context"
	"io"
	"strings"
)

// SSEEvent represents a single Server-Sent Event.
type SSEEvent struct {
	Event string // event type (empty = "message")
	Data  string // concatenated data lines
	ID    string // last event ID
	Err   error  // non-nil on parse error or stream end
}

// ParseSSE reads Server-Sent Events from an io.Reader per the SSE spec.
// It yields events on the returned channel, which is closed on io.EOF,
// context cancellation, or encountering the [DONE] sentinel.
//
// Per the SSE specification:
//   - Lines starting with ':' are comments (ignored)
//   - 'event:' sets the event type
//   - 'data:' appends to data buffer (multiple data lines joined with \n)
//   - 'id:' sets the event ID
//   - An empty line dispatches the accumulated event
//   - The [DONE] sentinel ends the stream (used by OpenAI/GLM APIs)
func ParseSSE(ctx context.Context, r io.Reader) <-chan SSEEvent {
	ch := make(chan SSEEvent, 16)

	go func() {
		defer close(ch)

		scanner := bufio.NewScanner(r)
		// Increase buffer size for large SSE data lines.
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		var (
			eventType string
			dataBuf   strings.Builder
			eventID   string
			hasData   bool
		)

		resetEvent := func() {
			eventType = ""
			dataBuf.Reset()
			eventID = ""
			hasData = false
		}

		for scanner.Scan() {
			// Check for context cancellation.
			select {
			case <-ctx.Done():
				emit(ch, SSEEvent{Err: ctx.Err()})
				return
			default:
			}

			line := scanner.Text()

			// Empty line → dispatch event.
			if line == "" {
				if hasData {
					data := dataBuf.String()

					// Check for [DONE] sentinel.
					if strings.TrimSpace(data) == "[DONE]" {
						resetEvent()
						return
					}

					select {
					case ch <- SSEEvent{
						Event: eventType,
						Data:  data,
						ID:    eventID,
					}:
					case <-ctx.Done():
						emit(ch, SSEEvent{Err: ctx.Err()})
						return
					}
				}
				resetEvent()
				continue
			}

			// Comment line.
			if strings.HasPrefix(line, ":") {
				continue
			}

			// Parse field: value.
			field, value := parseSSELine(line)

			switch field {
			case "event":
				eventType = value
			case "data":
				if hasData {
					dataBuf.WriteByte('\n')
				}
				dataBuf.WriteString(value)
				hasData = true
			case "id":
				eventID = value
			case "retry":
				// Retry is specified in the SSE spec but we don't use it.
			}
		}

		// Scanner finished — emit any error from the underlying reader.
		if err := scanner.Err(); err != nil {
			emit(ch, SSEEvent{Err: err})
		}
	}()

	return ch
}

// parseSSELine splits a line into field and value per the SSE spec.
// If the line contains a colon, field is before the first colon and value
// is after (with a single leading space stripped if present).
// If no colon, the entire line is the field and value is empty.
func parseSSELine(line string) (field, value string) {
	idx := strings.IndexByte(line, ':')
	if idx < 0 {
		return line, ""
	}
	field = line[:idx]
	value = line[idx+1:]
	// Strip a single leading space after the colon, per SSE spec.
	if len(value) > 0 && value[0] == ' ' {
		value = value[1:]
	}
	return field, value
}

// emit sends an SSEEvent on the channel, non-blocking if the channel is full
// (should not happen given channel buffer, but prevents goroutine leaks).
func emit(ch chan<- SSEEvent, ev SSEEvent) {
	select {
	case ch <- ev:
	default:
	}
}

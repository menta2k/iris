package logstream

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrLineTooLong = errors.New("logstream: line exceeds LineMaxBytes")
	ErrTooDeep     = errors.New("logstream: JSON nested deeper than ParseMaxDepth")
	ErrTooManyKeys = errors.New("logstream: JSON has more than ParseMaxKeys keys")
	ErrInvalidJSON = errors.New("logstream: invalid JSON")
)

// bufferPool is used to assemble the parsed event without per-call alloc on
// the hot path. The caller MUST NOT retain the returned LogEvent past the
// next call on the same parser.
var bufferPool = sync.Pool{New: func() any { return new(LogEvent) }}

// ReleaseEvent returns a LogEvent to the pool. Callers who persist the value
// (e.g., copy fields into a DB write) should call this after use.
func ReleaseEvent(e *LogEvent) {
	if e == nil {
		return
	}
	*e = LogEvent{}
	bufferPool.Put(e)
}

// AcquireEvent borrows a fresh LogEvent.
func AcquireEvent() *LogEvent {
	return bufferPool.Get().(*LogEvent)
}

// Parse decodes one kumomta log line. The returned *LogEvent is borrowed
// from a sync.Pool — call ReleaseEvent when done.
//
// Allocation discipline: we use json.Decoder with a depth-counting tokenizer
// to bound memory. A second Decode pass populates the typed fields.
func Parse(line []byte) (*LogEvent, error) {
	if len(line) > LineMaxBytes {
		return nil, ErrLineTooLong
	}
	if !looksLikeJSON(line) {
		return nil, ErrInvalidJSON
	}
	if err := boundedScan(line); err != nil {
		return nil, err
	}

	ev := AcquireEvent()
	ev.ParsedAt = time.Now().UTC()

	dec := json.NewDecoder(bytes.NewReader(line))
	dec.UseNumber()
	if err := dec.Decode(ev); err != nil {
		ReleaseEvent(ev)
		return nil, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	return ev, nil
}

// looksLikeJSON does a cheap pre-flight to skip obvious garbage.
func looksLikeJSON(line []byte) bool {
	for _, b := range line {
		if b == ' ' || b == '\t' || b == '\r' || b == '\n' {
			continue
		}
		return b == '{'
	}
	return false
}

// boundedScan walks the JSON token stream once, enforcing depth and key
// limits without unmarshalling into a map[string]any.
func boundedScan(line []byte) error {
	dec := json.NewDecoder(bytes.NewReader(line))
	depth := 0
	keys := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, errEOF()) {
				break
			}
			// Distinguish "EOF" from real syntax errors via the decoder API.
			// json/Token returns io.EOF when stream ends cleanly.
			if err.Error() == "EOF" {
				break
			}
			return fmt.Errorf("%w: %v", ErrInvalidJSON, err)
		}
		switch t := tok.(type) {
		case json.Delim:
			if t == '{' || t == '[' {
				depth++
				if depth > ParseMaxDepth {
					return ErrTooDeep
				}
			} else if t == '}' || t == ']' {
				depth--
			}
		case string:
			// crude: each string token at "key" position counts; structurally
			// json.Decoder yields key strings at object positions. We just
			// count strings as keys/values combined — the cap applies to the
			// total token count which is an over-approximation that still
			// bounds memory.
			keys++
			if keys > ParseMaxKeys {
				return ErrTooManyKeys
			}
		}
	}
	return nil
}

// errEOF gives a typed sentinel without importing io into the public API.
func errEOF() error { return errEOFSentinel }

var errEOFSentinel = errors.New("EOF")

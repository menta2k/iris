package logstream

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseHappyPath(t *testing.T) {
	line := []byte(`{"type":"Reception","timestamp":"2026-01-01T00:00:00Z","queue":"q1","sender":"s@a","recipient":"r@b","id":"abc","response_code":250}`)
	ev, err := Parse(line)
	require.NoError(t, err)
	defer ReleaseEvent(ev)
	require.Equal(t, "Reception", ev.Type)
	require.Equal(t, "q1", ev.QueueName)
	require.Equal(t, "r@b", ev.Recipient)
	require.Equal(t, int32(250), ev.ResponseCode)
}

func TestParseRejectsLineTooLong(t *testing.T) {
	big := bytes.Repeat([]byte("x"), LineMaxBytes+1)
	_, err := Parse(big)
	require.ErrorIs(t, err, ErrLineTooLong)
}

func TestParseRejectsNonJSON(t *testing.T) {
	_, err := Parse([]byte(`hello world`))
	require.ErrorIs(t, err, ErrInvalidJSON)
}

func TestParseRejectsTooDeep(t *testing.T) {
	deep := strings.Repeat(`{"a":`, ParseMaxDepth+1) + "1" + strings.Repeat("}", ParseMaxDepth+1)
	_, err := Parse([]byte(deep))
	require.ErrorIs(t, err, ErrTooDeep)
}

func TestParseRejectsBrokenJSON(t *testing.T) {
	_, err := Parse([]byte(`{"type":`))
	require.Error(t, err)
}

func TestEventPoolReuse(t *testing.T) {
	a, err := Parse([]byte(`{"type":"A"}`))
	require.NoError(t, err)
	require.Equal(t, "A", a.Type)
	ReleaseEvent(a)

	b, err := Parse([]byte(`{"type":"B"}`))
	require.NoError(t, err)
	defer ReleaseEvent(b)
	require.Equal(t, "B", b.Type)
}

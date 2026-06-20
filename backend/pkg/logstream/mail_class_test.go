package logstream

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRedisLogRecordMailClass(t *testing.T) {
	tests := []struct {
		name string
		rec  *RedisLogRecord
		want string
	}{
		{
			name: "meta is the source of truth",
			rec:  &RedisLogRecord{Meta: map[string]any{"mailclass": "homesbg"}},
			want: "homesbg",
		},
		{
			name: "meta wins over legacy header",
			rec: &RedisLogRecord{
				Meta:    map[string]any{"mailclass": "fromMeta"},
				Headers: map[string]any{"X-Kumo-Mail-Class": "fromHeader"},
			},
			want: "fromMeta",
		},
		{
			name: "falls back to legacy header (string)",
			rec:  &RedisLogRecord{Headers: map[string]any{"X-Kumo-Mail-Class": "tx"}},
			want: "tx",
		},
		{
			name: "falls back to legacy header (list shape)",
			rec:  &RedisLogRecord{Headers: map[string]any{"X-Kumo-Mail-Class": []any{"tx", "dup"}}},
			want: "tx",
		},
		{
			name: "blank meta falls through to header",
			rec: &RedisLogRecord{
				Meta:    map[string]any{"mailclass": "  "},
				Headers: map[string]any{"X-Kumo-Mail-Class": "tx"},
			},
			want: "tx",
		},
		{
			name: "unclassified",
			rec:  &RedisLogRecord{},
			want: "",
		},
		{
			name: "nil record",
			rec:  nil,
			want: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.rec.MailClass())
		})
	}
}

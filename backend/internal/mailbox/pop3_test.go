package mailbox

import (
	"testing"

	"github.com/menta2k/iris/backend/internal/biz"
)

func TestHeaderBlock(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"crlf", "From: a@b\r\nSubject: hi\r\n\r\nbody here", "From: a@b\r\nSubject: hi"},
		{"lf", "From: a@b\nSubject: hi\n\nbody", "From: a@b\nSubject: hi"},
		{"headers only", "From: a@b\r\nSubject: hi", "From: a@b\r\nSubject: hi"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := headerBlock([]byte(c.raw)); got != c.want {
				t.Errorf("headerBlock() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestFoldersFor(t *testing.T) {
	// No configured folders → default to INBOX.
	if got := foldersFor(&biz.MonitoringAccount{}); len(got) != 1 || got[0] != "INBOX" {
		t.Errorf("foldersFor(empty) = %v, want [INBOX]", got)
	}
	custom := []string{"INBOX", "Spam"}
	if got := foldersFor(&biz.MonitoringAccount{CheckFolders: custom}); len(got) != 2 {
		t.Errorf("foldersFor(custom) = %v, want 2 folders", got)
	}
}

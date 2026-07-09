package biz

import (
	"strings"
	"testing"
)

func TestParseHeaderSignalsAuthPass(t *testing.T) {
	raw := "Authentication-Results: mx.google.com;\r\n" +
		"       spf=pass smtp.mailfrom=probe@monitor.example.com;\r\n" +
		"       dkim=pass header.d=monitor.example.com;\r\n" +
		"       dmarc=pass header.from=monitor.example.com\r\n" +
		"Subject: [iris-probe] ipabc"
	a := ParseHeaderSignals(raw)
	if a.SPF != "pass" || a.DKIM != "pass" || a.DMARC != "pass" {
		t.Fatalf("auth = spf:%q dkim:%q dmarc:%q", a.SPF, a.DKIM, a.DMARC)
	}
	if a.Verdict != VerdictClean {
		t.Errorf("verdict = %q, want clean", a.Verdict)
	}
	if a.Source != AnalysisSourceHeuristic {
		t.Errorf("source = %q, want heuristic", a.Source)
	}
}

func TestParseHeaderSignalsBothFailIsSpam(t *testing.T) {
	raw := "Authentication-Results: mx.example.com; spf=fail; dkim=fail; dmarc=fail\r\n" +
		"Subject: hi"
	a := ParseHeaderSignals(raw)
	if a.Verdict != VerdictSpam {
		t.Errorf("verdict = %q, want spam", a.Verdict)
	}
}

func TestParseHeaderSignalsSpamFlag(t *testing.T) {
	raw := "X-Spam-Flag: YES\r\nX-Spam-Score: 9.4\r\nSubject: cheap pills"
	a := ParseHeaderSignals(raw)
	if a.SpamFlag == nil || !*a.SpamFlag {
		t.Error("expected spam flag true")
	}
	if a.SpamScore == nil || *a.SpamScore != 9.4 {
		t.Errorf("spam score = %v, want 9.4", a.SpamScore)
	}
	if a.Verdict != VerdictSpam {
		t.Errorf("verdict = %q, want spam", a.Verdict)
	}
}

func TestParseHeaderSignalsSpamStatusScore(t *testing.T) {
	raw := "X-Spam-Status: Yes, score=6.1 required=5.0 tests=BAYES_99\r\nSubject: x"
	a := ParseHeaderSignals(raw)
	if a.SpamScore == nil || *a.SpamScore != 6.1 {
		t.Errorf("score = %v, want 6.1", a.SpamScore)
	}
	if a.SpamFlag == nil || !*a.SpamFlag {
		t.Error("expected spam flag from X-Spam-Status Yes")
	}
}

func TestParseHeaderSignalsDmarcFailSuspicious(t *testing.T) {
	raw := "Authentication-Results: mx; spf=pass; dkim=pass; dmarc=fail\r\nSubject: x"
	a := ParseHeaderSignals(raw)
	if a.Verdict != VerdictSuspicious {
		t.Errorf("verdict = %q, want suspicious", a.Verdict)
	}
}

func TestParseHeaderSignalsNoAuth(t *testing.T) {
	a := ParseHeaderSignals("Subject: hello\r\nFrom: a@b.com")
	if a.Verdict != VerdictClean || a.Confidence > 0.4 {
		t.Errorf("verdict=%q conf=%v, want clean low-confidence", a.Verdict, a.Confidence)
	}
	if len(a.Factors) == 0 || !strings.Contains(strings.Join(a.Factors, " "), "no authentication") {
		t.Errorf("expected a no-auth factor, got %v", a.Factors)
	}
}

func TestNormalizeVerdict(t *testing.T) {
	cases := map[string]string{
		"clean": VerdictClean, "SPAM": VerdictSpam, "Suspicious": VerdictSuspicious,
		"garbage": VerdictSuspicious, "": VerdictSuspicious,
	}
	for in, want := range cases {
		if got := normalizeVerdict(in); got != want {
			t.Errorf("normalizeVerdict(%q) = %q, want %q", in, got, want)
		}
	}
}

package biz

import (
	"bufio"
	"context"
	"net/textproto"
	"strconv"
	"strings"

	"github.com/emersion/go-msgauth/authres"
)

// Spam-risk verdicts assigned by phase-3 header analysis.
const (
	VerdictClean      = "clean"
	VerdictSuspicious = "suspicious"
	VerdictSpam       = "spam"
)

// Analysis source labels.
const (
	AnalysisSourceHeuristic = "heuristic"
	AnalysisSourceLLM       = "llm"
)

// ProbeAnalysis is the phase-3 deliverability assessment of a probe's headers.
// It is serialized to JSON and stored on the probe's analysis field. The auth
// fields and spam signals come from deterministic header parsing; the verdict is
// either the heuristic fallback or an LLM refinement.
type ProbeAnalysis struct {
	SPF        string   `json:"spf,omitempty"`
	DKIM       string   `json:"dkim,omitempty"`
	DMARC      string   `json:"dmarc,omitempty"`
	SpamScore  *float64 `json:"spam_score,omitempty"`
	SpamFlag   *bool    `json:"spam_flag,omitempty"`
	Verdict    string   `json:"verdict"`              // clean|suspicious|spam
	Confidence float64  `json:"confidence"`           // 0..1
	Summary    string   `json:"summary,omitempty"`    // short explanation
	Factors    []string `json:"factors,omitempty"`    // notable signals
	Source     string   `json:"source"`               // heuristic|llm
}

// LLMHeaderVerdict is the structured reply from an LLM header analyzer.
type LLMHeaderVerdict struct {
	Verdict    string   `json:"verdict"`
	Confidence float64  `json:"confidence"`
	Summary    string   `json:"summary"`
	Factors    []string `json:"factors"`
}

// ProbeHeaderAnalyzer refines a deliverability verdict from raw headers using an
// LLM. Implemented by OpenAIHeaderAnalyzer; nil disables the LLM layer (the
// deterministic heuristic verdict is used instead).
type ProbeHeaderAnalyzer interface {
	AnalyzeHeaders(ctx context.Context, headers string) (LLMHeaderVerdict, error)
}

// ParseHeaderSignals extracts SPF/DKIM/DMARC and spam signals from a raw header
// block and derives a heuristic verdict. This always runs (no network), so an
// analysis exists even when the LLM layer is unavailable.
func ParseHeaderSignals(rawHeaders string) ProbeAnalysis {
	a := ProbeAnalysis{Source: AnalysisSourceHeuristic}
	hdr := readHeaders(rawHeaders)
	if hdr == nil {
		a.Verdict = VerdictSuspicious
		a.Confidence = 0.3
		a.Factors = []string{"headers could not be parsed"}
		return a
	}

	// Authentication-Results may appear multiple times (one per hop). Record the
	// first value seen for each mechanism (topmost = receiving provider).
	for _, v := range hdr.Values("Authentication-Results") {
		_, results, err := authres.Parse(v)
		if err != nil {
			continue
		}
		for _, r := range results {
			switch res := r.(type) {
			case *authres.SPFResult:
				if a.SPF == "" {
					a.SPF = string(res.Value)
				}
			case *authres.DKIMResult:
				if a.DKIM == "" {
					a.DKIM = string(res.Value)
				}
			case *authres.DMARCResult:
				if a.DMARC == "" {
					a.DMARC = string(res.Value)
				}
			}
		}
	}

	parseSpamSignals(hdr, &a)
	applyHeuristicVerdict(&a)
	return a
}

// parseSpamSignals reads common spam-scoring headers (SpamAssassin, rspamd).
func parseSpamSignals(hdr textproto.MIMEHeader, a *ProbeAnalysis) {
	if flag := strings.TrimSpace(hdr.Get("X-Spam-Flag")); flag != "" {
		yes := strings.EqualFold(flag, "yes")
		a.SpamFlag = &yes
	}
	if score, ok := parseFloatField(hdr.Get("X-Spam-Score")); ok {
		a.SpamScore = &score
	}
	// X-Spam-Status: "Yes, score=5.2 required=5.0 ..." (SpamAssassin).
	if status := hdr.Get("X-Spam-Status"); status != "" {
		if a.SpamFlag == nil {
			yes := strings.HasPrefix(strings.ToLower(strings.TrimSpace(status)), "yes")
			a.SpamFlag = &yes
		}
		if a.SpamScore == nil {
			if score, ok := parseKeyedFloat(status, "score="); ok {
				a.SpamScore = &score
			}
		}
	}
}

// applyHeuristicVerdict derives a verdict from the parsed signals. Auth failures
// and an explicit spam flag are the strongest signals.
func applyHeuristicVerdict(a *ProbeAnalysis) {
	var factors []string
	spam := false
	suspicious := false

	if a.SpamFlag != nil && *a.SpamFlag {
		spam = true
		factors = append(factors, "spam flag set")
	}
	if a.SpamScore != nil && *a.SpamScore >= 5 {
		spam = true
		factors = append(factors, "high spam score")
	}
	if isFail(a.DMARC) {
		suspicious = true
		factors = append(factors, "DMARC "+a.DMARC)
	}
	if isFail(a.SPF) && isFail(a.DKIM) {
		spam = true
		factors = append(factors, "SPF and DKIM both failed")
	} else if isFail(a.SPF) || isFail(a.DKIM) {
		suspicious = true
	}

	switch {
	case spam:
		a.Verdict = VerdictSpam
		a.Confidence = 0.7
	case suspicious:
		a.Verdict = VerdictSuspicious
		a.Confidence = 0.5
	default:
		a.Verdict = VerdictClean
		a.Confidence = 0.6
		if a.SPF == "" && a.DKIM == "" && a.DMARC == "" {
			// No auth results present at all — low-confidence clean.
			a.Confidence = 0.3
			factors = append(factors, "no authentication results present")
		}
	}
	a.Factors = factors
}

func isFail(v string) bool {
	switch strings.ToLower(v) {
	case "fail", "softfail", "permerror", "temperror":
		return true
	default:
		return false
	}
}

// readHeaders parses a raw header block into a MIMEHeader, returning nil on
// failure. A trailing blank line is appended so ReadMIMEHeader terminates.
func readHeaders(raw string) textproto.MIMEHeader {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	r := textproto.NewReader(bufio.NewReader(strings.NewReader(raw + "\n\n")))
	hdr, err := r.ReadMIMEHeader()
	if err != nil && len(hdr) == 0 {
		return nil
	}
	return hdr
}

func parseFloatField(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	return f, err == nil
}

// parseKeyedFloat extracts the float following key (e.g. "score=") in s.
func parseKeyedFloat(s, key string) (float64, bool) {
	i := strings.Index(strings.ToLower(s), key)
	if i < 0 {
		return 0, false
	}
	rest := s[i+len(key):]
	end := strings.IndexFunc(rest, func(r rune) bool {
		return !(r == '-' || r == '+' || r == '.' || (r >= '0' && r <= '9'))
	})
	if end >= 0 {
		rest = rest[:end]
	}
	return parseFloatField(rest)
}

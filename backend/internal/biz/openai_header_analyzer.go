package biz

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// analyzeSystemPrompt frames the deliverability-analysis task. The model must
// reply with a strict JSON object so the response parses without post-processing.
const analyzeSystemPrompt = "You are an email deliverability analyst. " +
	"Given the raw headers of a message that was delivered to a seed mailbox, " +
	"assess whether the receiving provider likely treated it as spam. " +
	"Weigh SPF/DKIM/DMARC results (Authentication-Results), any spam-score or " +
	"spam-flag headers, provider anti-spam headers (e.g. X-Microsoft-Antispam SCL, " +
	"X-Spamd-Result), and header hygiene. " +
	"Reply ONLY with a JSON object of this exact shape: " +
	`{"verdict":"clean|suspicious|spam","confidence":0.0,"summary":"one sentence","factors":["short signal", "..."]}. ` +
	"verdict must be one of clean, suspicious, spam. confidence is 0..1. " +
	"Keep summary under 200 characters and factors to at most 5 short strings."

// maxAnalyzeHeaderBytes caps how many header bytes are sent to the model.
const maxAnalyzeHeaderBytes = 6000

// OpenAIHeaderAnalyzer implements ProbeHeaderAnalyzer via an OpenAI-compatible
// /chat/completions endpoint. It mirrors OpenAIClassifier's injectable-HTTPDoer
// pattern and is gated by IRIS_OPENAI_API_KEY at construction in main.
type OpenAIHeaderAnalyzer struct {
	apiKey  string
	model   string
	apiBase string
	client  HTTPDoer
}

// NewOpenAIHeaderAnalyzer constructs the analyzer. A nil client defaults to a
// 20s http.Client; empty model/apiBase fall back to OpenAI defaults.
func NewOpenAIHeaderAnalyzer(apiKey, model, apiBase string, client HTTPDoer) *OpenAIHeaderAnalyzer {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = "gpt-4o-mini"
	}
	apiBase = strings.TrimRight(strings.TrimSpace(apiBase), "/")
	if apiBase == "" {
		apiBase = "https://api.openai.com/v1"
	}
	return &OpenAIHeaderAnalyzer{apiKey: strings.TrimSpace(apiKey), model: model, apiBase: apiBase, client: client}
}

// AnalyzeHeaders asks the model for a structured spam verdict over the headers.
func (o *OpenAIHeaderAnalyzer) AnalyzeHeaders(ctx context.Context, headers string) (LLMHeaderVerdict, error) {
	var out LLMHeaderVerdict
	if o.apiKey == "" {
		return out, fmt.Errorf("openai: no api key configured")
	}
	if len(headers) > maxAnalyzeHeaderBytes {
		headers = headers[:maxAnalyzeHeaderBytes]
	}

	reqBody := map[string]any{
		"model": o.model,
		"messages": []map[string]string{
			{"role": "system", "content": analyzeSystemPrompt},
			{"role": "user", "content": headers},
		},
		"response_format": map[string]string{"type": "json_object"},
		"max_tokens":      300,
		"temperature":     0,
	}
	buf, err := json.Marshal(reqBody)
	if err != nil {
		return out, fmt.Errorf("openai: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.apiBase+"/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return out, fmt.Errorf("openai: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+o.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return out, fmt.Errorf("openai: request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("openai: returned HTTP %d", resp.StatusCode)
	}

	var pr struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return out, fmt.Errorf("openai: decode response: %w", err)
	}
	if len(pr.Choices) == 0 {
		return out, fmt.Errorf("openai: empty response")
	}
	if err := json.Unmarshal([]byte(pr.Choices[0].Message.Content), &out); err != nil {
		return out, fmt.Errorf("openai: parse verdict json: %w", err)
	}
	out.Verdict = normalizeVerdict(out.Verdict)
	if out.Confidence < 0 {
		out.Confidence = 0
	} else if out.Confidence > 1 {
		out.Confidence = 1
	}
	return out, nil
}

// normalizeVerdict maps a model verdict to a known value, defaulting to
// suspicious for anything unrecognized.
func normalizeVerdict(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case VerdictClean:
		return VerdictClean
	case VerdictSpam:
		return VerdictSpam
	default:
		return VerdictSuspicious
	}
}

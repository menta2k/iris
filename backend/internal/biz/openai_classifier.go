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

// classifySystemPrompt instructs the model to return a terse category. Kept
// deliberately strict so the reply needs minimal post-processing.
const classifySystemPrompt = "You classify an email by its subject line. " +
	"Reply with a category of at most two words, lowercase, no punctuation, " +
	"no quotes, and no explanation. Examples: \"order confirmation\", " +
	"\"password reset\", \"newsletter\", \"invoice\", \"shipping update\"."

// OpenAIClassifier labels subjects via an OpenAI-compatible /chat/completions
// endpoint (works with OpenAI, Azure OpenAI, or a local gateway via apiBase).
// It mirrors the injectable-HTTPDoer pattern used by MetricsUsecase.
type OpenAIClassifier struct {
	apiKey string
	client HTTPDoer
}

// NewOpenAIClassifier constructs the client. A nil client defaults to a 15s
// http.Client. The apiKey comes from IRIS_OPENAI_API_KEY.
func NewOpenAIClassifier(apiKey string, client HTTPDoer) *OpenAIClassifier {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &OpenAIClassifier{apiKey: strings.TrimSpace(apiKey), client: client}
}

// ClassifySubject asks the model for a ≤2-word category. Returns the raw model
// text (the caller normalizes it). apiBase/model fall back to OpenAI defaults.
func (o *OpenAIClassifier) ClassifySubject(ctx context.Context, subject, model, apiBase string) (string, error) {
	if o.apiKey == "" {
		return "", fmt.Errorf("openai: no api key configured")
	}
	base := strings.TrimRight(strings.TrimSpace(apiBase), "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	if strings.TrimSpace(model) == "" {
		model = "gpt-4o-mini"
	}

	reqBody := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": classifySystemPrompt},
			{"role": "user", "content": subject},
		},
		"max_tokens":  10,
		"temperature": 0,
	}
	buf, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("openai: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return "", fmt.Errorf("openai: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+o.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai: request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openai: returned HTTP %d", resp.StatusCode)
	}

	var pr struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return "", fmt.Errorf("openai: decode response: %w", err)
	}
	if len(pr.Choices) == 0 {
		return "", nil
	}
	return strings.TrimSpace(pr.Choices[0].Message.Content), nil
}

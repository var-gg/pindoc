package changegroup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type SummaryConfig struct {
	Endpoint      string
	APIKey        string
	Model         string
	Timeout       time.Duration
	DailyTokenCap int
	GroupCap      int
}

func (c SummaryConfig) effectiveTimeout() time.Duration {
	if c.Timeout <= 0 {
		return 15 * time.Second
	}
	return c.Timeout
}

func (c SummaryConfig) effectiveGroupCap() int {
	if c.GroupCap <= 0 {
		return 20
	}
	return c.GroupCap
}

func EstimateSummaryTokens(groups []Group, groupCap int) int {
	payload, _ := json.Marshal(CompactLLMInput(groups, groupCap))
	return len(payload)/4 + 400
}

func RequestLLMBrief(ctx context.Context, cfg SummaryConfig, groups []Group, locale string) (Brief, int, error) {
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		return Brief{}, 0, errors.New("summary LLM endpoint not configured")
	}
	groupCap := cfg.effectiveGroupCap()
	prompt := SourceBoundPrompt(groups, locale, groupCap)
	reqBody := map[string]any{
		"model": strings.TrimSpace(cfg.Model),
		"messages": []map[string]string{
			{"role": "system", "content": "You write concise project briefing summaries from source-bound metadata only."},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.2,
	}
	if strings.TrimSpace(cfg.Model) == "" {
		delete(reqBody, "model")
	}
	body, _ := json.Marshal(reqBody)
	reqCtx, cancel := context.WithTimeout(ctx, cfg.effectiveTimeout())
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Brief{}, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Brief{}, EstimateSummaryTokens(groups, groupCap), err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Brief{}, EstimateSummaryTokens(groups, groupCap), fmt.Errorf("summary LLM status %d", resp.StatusCode)
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Text string `json:"text"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return Brief{}, EstimateSummaryTokens(groups, groupCap), err
	}
	if len(parsed.Choices) == 0 {
		return Brief{}, EstimateSummaryTokens(groups, groupCap), errors.New("summary LLM returned no choices")
	}
	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if content == "" {
		content = strings.TrimSpace(parsed.Choices[0].Text)
	}
	brief, err := parseBriefContent(content)
	if err != nil {
		return Brief{}, EstimateSummaryTokens(groups, groupCap), err
	}
	brief.Source = "llm"
	brief.AIHint = "AI-generated"
	brief.CreatedAt = time.Now().UTC()
	return brief, EstimateSummaryTokens(groups, groupCap), nil
}

func parseBriefContent(content string) (Brief, error) {
	content = strings.TrimSpace(strings.Trim(content, "`"))
	if strings.HasPrefix(content, "json") {
		content = strings.TrimSpace(strings.TrimPrefix(content, "json"))
	}
	var parsed struct {
		Headline string   `json:"headline"`
		Bullets  []string `json:"bullets"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return Brief{}, err
	}
	parsed.Headline = strings.TrimSpace(parsed.Headline)
	clean := make([]string, 0, 3)
	for _, b := range parsed.Bullets {
		b = strings.TrimSpace(b)
		if b != "" {
			clean = append(clean, b)
		}
		if len(clean) == 3 {
			break
		}
	}
	if parsed.Headline == "" || len(clean) == 0 {
		return Brief{}, errors.New("summary LLM JSON missing headline or bullets")
	}
	return Brief{Headline: parsed.Headline, Bullets: clean}, nil
}

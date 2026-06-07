package extractor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ashleyfullero/scrapeowl/internal/config"
)

// aiClient is an HTTP client for AI API calls
var aiClient = &http.Client{Timeout: 60 * time.Second}

// callOpenAI sends a prompt to the OpenAI chat completions API
func callOpenAI(cfg config.AIConfig, systemPrompt, userContent string) (string, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	model := cfg.Model
	if model == "" {
		model = "gpt-4o"
	}

	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type request struct {
		Model       string    `json:"model"`
		Messages    []message `json:"messages"`
		Temperature float64   `json:"temperature"`
		MaxTokens   int       `json:"max_tokens"`
	}

	reqBody := request{
		Model: model,
		Messages: []message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userContent},
		},
		Temperature: 0.1,
		MaxTokens:   2048,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(
		context.Background(),
		"POST",
		baseURL+"/chat/completions",
		bytes.NewReader(data),
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := aiClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		return "", fmt.Errorf("openai API error %d: %s", resp.StatusCode, errResp.Error.Message)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding openai response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in openai response")
	}

	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}

// callAnthropic sends a prompt to the Anthropic Claude API
func callAnthropic(cfg config.AIConfig, systemPrompt, userContent string) (string, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}

	model := cfg.Model
	if model == "" {
		model = "claude-3-5-sonnet-20241022"
	}

	type contentBlock struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type message struct {
		Role    string         `json:"role"`
		Content []contentBlock `json:"content"`
	}
	type request struct {
		Model     string    `json:"model"`
		System    string    `json:"system"`
		Messages  []message `json:"messages"`
		MaxTokens int       `json:"max_tokens"`
	}

	reqBody := request{
		Model:  model,
		System: systemPrompt,
		Messages: []message{
			{
				Role: "user",
				Content: []contentBlock{
					{Type: "text", Text: userContent},
				},
			},
		},
		MaxTokens: 2048,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(
		context.Background(),
		"POST",
		baseURL+"/messages",
		bytes.NewReader(data),
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := aiClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("anthropic request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		return "", fmt.Errorf("anthropic API error %d: %s", resp.StatusCode, errResp.Error.Message)
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding anthropic response: %w", err)
	}
	if len(result.Content) == 0 {
		return "", fmt.Errorf("no content in anthropic response")
	}

	return strings.TrimSpace(result.Content[0].Text), nil
}

// ExtractWithAI uses an AI model to extract structured data from page content
func ExtractWithAI(pageHTML string, prompt string, cfg config.AIConfig) (interface{}, error) {
	systemPrompt := `You are a precise web data extractor. Given HTML content and an extraction prompt, 
extract the requested data and return it as valid JSON. 
Only return valid JSON, no markdown formatting, no extra text.
If the data cannot be found, return {"error": "not found"}.`

	userContent := fmt.Sprintf("HTML Content:\n%s\n\nExtraction task: %s", truncateHTML(pageHTML), prompt)

	var (
		response string
		err      error
	)

	switch strings.ToLower(cfg.Provider) {
	case "anthropic":
		response, err = callAnthropic(cfg, systemPrompt, userContent)
	default: // openai
		response, err = callOpenAI(cfg, systemPrompt, userContent)
	}

	if err != nil {
		return nil, fmt.Errorf("AI extraction: %w", err)
	}

	// Try to parse as JSON
	var parsed interface{}
	if jsonErr := json.Unmarshal([]byte(response), &parsed); jsonErr != nil {
		// Return as string if not valid JSON
		return response, nil
	}
	return parsed, nil
}

// truncateHTML truncates very large HTML to avoid token limits
func truncateHTML(html string) string {
	const maxLen = 12000
	if len(html) <= maxLen {
		return html
	}
	return html[:maxLen] + "\n... [truncated]"
}

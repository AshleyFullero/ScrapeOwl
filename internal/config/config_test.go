package config

import (
	"testing"
)

func TestParse_Basic(t *testing.T) {
	yaml := `
name: "test-job"
start_url: "https://example.com"
output:
  format: jsonl
  path: "./output/test.jsonl"
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Name != "test-job" {
		t.Errorf("expected name 'test-job', got '%s'", cfg.Name)
	}
	if cfg.StartURL != "https://example.com" {
		t.Errorf("expected start_url 'https://example.com', got '%s'", cfg.StartURL)
	}
}

func TestParse_EnvVarInterpolation(t *testing.T) {
	t.Setenv("TEST_API_KEY", "abc123")
	yaml := `
name: "env-test"
start_url: "https://example.com"
ai:
  api_key: "${TEST_API_KEY}"
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AI.APIKey != "abc123" {
		t.Errorf("expected ai.api_key 'abc123', got '%s'", cfg.AI.APIKey)
	}
}

func TestParse_EnvVarDefaultValue(t *testing.T) {
	yaml := `
name: "default-test"
start_url: "https://example.com"
ai:
  api_key: "${MISSING_VAR:-fallback_key}"
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AI.APIKey != "fallback_key" {
		t.Errorf("expected fallback value 'fallback_key', got '%s'", cfg.AI.APIKey)
	}
}

func TestParse_MissingName(t *testing.T) {
	yaml := `
start_url: "https://example.com"
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestParse_InvalidOutputFormat(t *testing.T) {
	yaml := `
name: "test"
start_url: "https://example.com"
output:
  format: xml
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
}

func TestParse_InvalidAction(t *testing.T) {
	yaml := `
name: "test"
start_url: "https://example.com"
steps:
  - action: explode
    selector: "button"
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for invalid action")
	}
}

func TestParse_AIExtractorMissingPrompt(t *testing.T) {
	yaml := `
name: "test"
start_url: "https://example.com"
extractors:
  - name: title
    type: ai
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for ai extractor without prompt")
	}
}

func TestParse_Defaults(t *testing.T) {
	yaml := `
name: "defaults-test"
start_url: "https://example.com"
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Output.Format != "jsonl" {
		t.Errorf("expected default format 'jsonl', got '%s'", cfg.Output.Format)
	}
	if cfg.Retry.MaxAttempts != 3 {
		t.Errorf("expected default max_attempts 3, got %d", cfg.Retry.MaxAttempts)
	}
	if cfg.Proxy.Type != "none" {
		t.Errorf("expected default proxy type 'none', got '%s'", cfg.Proxy.Type)
	}
}

func TestParse_FullConfig(t *testing.T) {
	yaml := `
name: "product-scraper"
start_url: "https://example.com/products"
steps:
  - action: click
    selector: "button.load-more"
    wait: 2s
  - action: type
    selector: "input#search"
    text: "laptop"
    wait: 1s
extractors:
  - name: title
    type: css
    selector: "h1.product-title"
    attribute: text
  - name: price
    type: css
    selector: ".price .value"
    attribute: text
output:
  format: jsonl
  path: "./output/products.jsonl"
proxy:
  type: static
  list:
    - "http://user:pass@proxy1:8080"
retry:
  max_attempts: 3
  backoff: "exponential"
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(cfg.Steps))
	}
	if len(cfg.Extract) != 2 {
		t.Errorf("expected 2 extractors, got %d", len(cfg.Extract))
	}
	if cfg.Proxy.Type != "static" {
		t.Errorf("expected proxy type 'static', got '%s'", cfg.Proxy.Type)
	}
}
func TestParse_JSONOutputFormat(t *testing.T) {
	yaml := `
name: "json-test"
start_url: "https://example.com"
output:
  format: json
  path: "./output/test.json"
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error for json format: %v", err)
	}
	if cfg.Output.Format != "json" {
		t.Errorf("expected format 'json', got '%s'", cfg.Output.Format)
	}
}

func TestParse_WebhookConfig(t *testing.T) {
	yaml := `
name: "webhook-test"
start_url: "https://example.com"
webhook:
  url: "https://hooks.example.com/notify"
  secret: "mysecret"
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Webhook.URL != "https://hooks.example.com/notify" {
		t.Errorf("expected webhook url, got '%s'", cfg.Webhook.URL)
	}
	// Defaults: should auto-enable both on_success and on_failure
	if !cfg.Webhook.OnSuccess {
		t.Error("expected on_success to be defaulted to true when URL is set")
	}
	if !cfg.Webhook.OnFailure {
		t.Error("expected on_failure to be defaulted to true when URL is set")
	}
}

func TestParse_WebhookInvalidURL(t *testing.T) {
	yaml := `
name: "bad-webhook"
start_url: "https://example.com"
webhook:
  url: "ftp://invalid"
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for invalid webhook URL")
	}
}

func TestParse_Timeout(t *testing.T) {
	yaml := `
name: "timeout-test"
start_url: "https://example.com"
timeout: "5m"
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Timeout != "5m" {
		t.Errorf("expected timeout '5m', got '%s'", cfg.Timeout)
	}
}

func TestParse_ConcurrencyDefault(t *testing.T) {
	yaml := `
name: "conc-test"
start_url: "https://example.com"
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Concurrency != 1 {
		t.Errorf("expected default concurrency 1, got %d", cfg.Concurrency)
	}
}

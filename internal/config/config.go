package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// envVarPattern matches ${VAR_NAME} style env var references
var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// JobConfig represents the full YAML job definition
type JobConfig struct {
	Name     string      `yaml:"name"`
	StartURL string      `yaml:"start_url"`
	Steps    []Step      `yaml:"steps"`
	Extract  []Extractor `yaml:"extractors"`
	Output   Output      `yaml:"output"`
	Proxy    ProxyConfig `yaml:"proxy"`
	Captcha  Captcha     `yaml:"captcha"`
	AI       AIConfig    `yaml:"ai"`
	Retry    RetryConfig `yaml:"retry"`
	Schedule string      `yaml:"schedule"`
}

// Step represents a single browser automation action
type Step struct {
	Action   string        `yaml:"action"`
	Selector string        `yaml:"selector,omitempty"`
	Text     string        `yaml:"text,omitempty"`
	URL      string        `yaml:"url,omitempty"`
	Wait     time.Duration `yaml:"wait,omitempty"`
	Timeout  time.Duration `yaml:"timeout,omitempty"`
	Optional bool          `yaml:"optional,omitempty"`
	Key      string        `yaml:"key,omitempty"` // for keyboard actions
}

// Extractor defines how to extract a piece of data
type Extractor struct {
	Name      string `yaml:"name"`
	Type      string `yaml:"type"`      // css, xpath, ai, regex
	Selector  string `yaml:"selector,omitempty"`
	Attribute string `yaml:"attribute,omitempty"` // text, href, src, value, html, ...
	Prompt    string `yaml:"prompt,omitempty"`    // for ai type
	Pattern   string `yaml:"pattern,omitempty"`   // for regex type
	Multiple  bool   `yaml:"multiple,omitempty"`  // extract all matching elements
}

// Output defines how to store the extracted data
type Output struct {
	Format string `yaml:"format"` // jsonl, csv
	Path   string `yaml:"path"`
}

// ProxyConfig defines proxy settings
type ProxyConfig struct {
	Type     string   `yaml:"type"` // static, rotating, none
	List     []string `yaml:"list,omitempty"`
	Endpoint string   `yaml:"endpoint,omitempty"` // for rotating proxy service
	Username string   `yaml:"username,omitempty"`
	Password string   `yaml:"password,omitempty"`
}

// Captcha defines CAPTCHA solving settings
type Captcha struct {
	Provider string `yaml:"provider"` // 2captcha, anticaptcha, none
	APIKey   string `yaml:"api_key,omitempty"`
}

// AIConfig defines AI provider settings for extraction
type AIConfig struct {
	Provider string `yaml:"provider"` // openai, anthropic
	APIKey   string `yaml:"api_key,omitempty"`
	Model    string `yaml:"model,omitempty"`
	BaseURL  string `yaml:"base_url,omitempty"` // for custom endpoints
}

// RetryConfig defines retry behavior
type RetryConfig struct {
	MaxAttempts int    `yaml:"max_attempts"`
	Backoff     string `yaml:"backoff"` // linear, exponential
	Delay       string `yaml:"delay,omitempty"`
}

// LoadFile reads and parses a YAML job config from a file path
func LoadFile(path string) (*JobConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}
	return Parse(data)
}

// Parse parses raw YAML bytes into a JobConfig, interpolating env vars
func Parse(data []byte) (*JobConfig, error) {
	// Interpolate env vars before parsing
	expanded := interpolateEnvVars(string(data))

	var cfg JobConfig
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Apply defaults
	cfg.applyDefaults()

	return &cfg, nil
}

// interpolateEnvVars replaces ${VAR} with the actual env var value
func interpolateEnvVars(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		// Extract variable name from ${VAR_NAME}
		inner := match[2 : len(match)-1]
		parts := strings.SplitN(inner, ":-", 2)
		varName := parts[0]
		val := os.Getenv(varName)
		if val == "" && len(parts) == 2 {
			val = parts[1] // default value after :-
		}
		return val
	})
}

// Validate checks that the config is valid
func (c *JobConfig) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("job name is required")
	}
	if c.StartURL == "" {
		return fmt.Errorf("start_url is required")
	}
	if c.Output.Format != "" && c.Output.Format != "jsonl" && c.Output.Format != "csv" {
		return fmt.Errorf("output.format must be 'jsonl' or 'csv', got: %s", c.Output.Format)
	}
	for i, step := range c.Steps {
		if err := validateStep(step, i); err != nil {
			return err
		}
	}
	for i, ext := range c.Extract {
		if err := validateExtractor(ext, i); err != nil {
			return err
		}
	}
	return nil
}

func validateStep(s Step, idx int) error {
	validActions := map[string]bool{
		"click": true, "type": true, "navigate": true, "wait": true,
		"scroll": true, "screenshot": true, "hover": true, "press": true,
		"select": true, "clear": true,
	}
	if !validActions[s.Action] {
		return fmt.Errorf("step[%d]: unknown action '%s'", idx, s.Action)
	}
	return nil
}

func validateExtractor(e Extractor, idx int) error {
	if e.Name == "" {
		return fmt.Errorf("extractor[%d]: name is required", idx)
	}
	validTypes := map[string]bool{"css": true, "xpath": true, "ai": true, "regex": true}
	if !validTypes[e.Type] {
		return fmt.Errorf("extractor[%d] '%s': unknown type '%s'", idx, e.Name, e.Type)
	}
	if (e.Type == "css" || e.Type == "xpath") && e.Selector == "" {
		return fmt.Errorf("extractor[%d] '%s': selector is required for type '%s'", idx, e.Name, e.Type)
	}
	if e.Type == "ai" && e.Prompt == "" {
		return fmt.Errorf("extractor[%d] '%s': prompt is required for type 'ai'", idx, e.Name)
	}
	return nil
}

// applyDefaults sets sensible defaults for missing optional fields
func (c *JobConfig) applyDefaults() {
	if c.Output.Format == "" {
		c.Output.Format = "jsonl"
	}
	if c.Output.Path == "" {
		c.Output.Path = fmt.Sprintf("./output/%s.jsonl", c.Name)
	}
	if c.Retry.MaxAttempts == 0 {
		c.Retry.MaxAttempts = 3
	}
	if c.Retry.Backoff == "" {
		c.Retry.Backoff = "exponential"
	}
	if c.Proxy.Type == "" {
		c.Proxy.Type = "none"
	}
	if c.Captcha.Provider == "" {
		c.Captcha.Provider = "none"
	}
	if c.AI.Provider == "" {
		c.AI.Provider = "openai"
	}
	if c.AI.Model == "" {
		c.AI.Model = "gpt-4o"
	}
}

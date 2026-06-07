package captcha

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ashleyfullero/scrapeowl/internal/config"
)

// Solver is the interface that all CAPTCHA providers implement
type Solver interface {
	SolveImage(imageBase64 string) (string, error)
	SolveRecaptchaV2(siteKey, pageURL string) (string, error)
	SolveRecaptchaV3(siteKey, pageURL, action string) (string, error)
	Name() string
}

// New creates a CAPTCHA solver based on config
func New(cfg config.Captcha) (Solver, error) {
	switch cfg.Provider {
	case "none", "":
		return &NoopSolver{}, nil
	case "2captcha":
		return NewTwoCaptcha(cfg.APIKey), nil
	case "anticaptcha":
		return NewAntiCaptcha(cfg.APIKey), nil
	default:
		return nil, fmt.Errorf("unknown captcha provider: %s", cfg.Provider)
	}
}

// NoopSolver is a no-op solver used when CAPTCHA solving is disabled
type NoopSolver struct{}

func (n *NoopSolver) SolveImage(imageBase64 string) (string, error) {
	return "", fmt.Errorf("captcha solver not configured")
}

func (n *NoopSolver) SolveRecaptchaV2(siteKey, pageURL string) (string, error) {
	return "", fmt.Errorf("captcha solver not configured")
}

func (n *NoopSolver) SolveRecaptchaV3(siteKey, pageURL, action string) (string, error) {
	return "", fmt.Errorf("captcha solver not configured")
}

func (n *NoopSolver) Name() string { return "none" }

// httpClient is a shared HTTP client with timeout
var httpClient = &http.Client{Timeout: 120 * time.Second}

// doPost is a helper for JSON POST requests
func doPost(url string, body interface{}, out interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := httpClient.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(out)
}

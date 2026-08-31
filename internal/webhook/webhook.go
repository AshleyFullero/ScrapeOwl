// Package webhook provides HTTP webhook notifications for ScrapeOwl job events.
package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Payload is the JSON body sent to the webhook endpoint
type Payload struct {
	Event     string                 `json:"event"`      // "run.success" | "run.failure" | "run.complete"
	JobName   string                 `json:"job_name"`
	RunID     string                 `json:"run_id"`
	Status    string                 `json:"status"`
	Records   int                    `json:"records"`
	Error     string                 `json:"error,omitempty"`
	StartedAt *time.Time             `json:"started_at,omitempty"`
	Duration  float64                `json:"duration_seconds,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

// Notifier sends webhook notifications to a configured endpoint.
type Notifier struct {
	URL    string
	Secret string
	logger *slog.Logger
}

// New creates a Notifier. If url is empty, Notify is a no-op.
func New(url, secret string, logger *slog.Logger) *Notifier {
	return &Notifier{URL: url, Secret: secret, logger: logger}
}

// Notify sends a webhook payload to the configured URL.
// It does NOT block the caller if the request fails — failures are logged only.
func (n *Notifier) Notify(p Payload) {
	if n.URL == "" {
		return
	}
	go n.send(p)
}

func (n *Notifier) send(p Payload) {
	p.Timestamp = time.Now()

	body, err := json.Marshal(p)
	if err != nil {
		n.logger.Error("webhook: marshal payload", "err", err)
		return
	}

	req, err := http.NewRequest(http.MethodPost, n.URL, bytes.NewReader(body))
	if err != nil {
		n.logger.Error("webhook: build request", "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ScrapeOwl-Webhook/1.0")
	req.Header.Set("X-ScrapeOwl-Event", p.Event)

	// HMAC-SHA256 signature if secret is set
	if n.Secret != "" {
		mac := hmac.New(sha256.New, []byte(n.Secret))
		mac.Write(body)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-ScrapeOwl-Signature", "sha256="+sig)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		n.logger.Warn("webhook: request failed", "url", n.URL, "err", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		n.logger.Warn("webhook: non-2xx response", "url", n.URL, "status", resp.StatusCode)
		return
	}
	n.logger.Info("webhook: delivered", "url", n.URL, "event", p.Event, "job", p.JobName)
}

// EventSuccess returns a success payload
func EventSuccess(jobName, runID string, records int, startedAt *time.Time, data map[string]interface{}) Payload {
	var dur float64
	if startedAt != nil {
		dur = time.Since(*startedAt).Seconds()
	}
	return Payload{
		Event:     "run.success",
		JobName:   jobName,
		RunID:     runID,
		Status:    "success",
		Records:   records,
		StartedAt: startedAt,
		Duration:  dur,
		Data:      data,
	}
}

// EventFailure returns a failure payload
func EventFailure(jobName, runID, errMsg string, startedAt *time.Time) Payload {
	var dur float64
	if startedAt != nil {
		dur = time.Since(*startedAt).Seconds()
	}
	return Payload{
		Event:     "run.failure",
		JobName:   jobName,
		RunID:     runID,
		Status:    "failed",
		Error:     errMsg,
		StartedAt: startedAt,
		Duration:  dur,
	}
}

// Validate checks if the webhook configuration is valid
func Validate(url string) error {
	if url == "" {
		return nil
	}
	if len(url) < 8 || (url[:7] != "http://" && url[:8] != "https://") {
		return fmt.Errorf("webhook url must start with http:// or https://")
	}
	return nil
}

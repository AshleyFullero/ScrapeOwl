package webhook_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ashleyfullero/scrapeowl/internal/webhook"
)

func logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, nil))
}

func TestNotify_Success(t *testing.T) {
	var received atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)

		// Verify headers
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", ct)
		}
		if ev := r.Header.Get("X-ScrapeOwl-Event"); ev != "run.success" {
			t.Errorf("expected X-ScrapeOwl-Event run.success, got %q", ev)
		}

		// Decode payload
		var p webhook.Payload
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		if p.JobName != "test-job" {
			t.Errorf("expected job_name 'test-job', got %q", p.JobName)
		}
		if p.Records != 42 {
			t.Errorf("expected 42 records, got %d", p.Records)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := webhook.New(srv.URL, "", logger())
	now := time.Now()
	p := webhook.EventSuccess("test-job", "run-1", 42, &now, nil)
	n.Notify(p)

	// Give the async goroutine time to fire
	deadline := time.Now().Add(2 * time.Second)
	for received.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if received.Load() == 0 {
		t.Error("webhook was not called")
	}
}

func TestNotify_NoURL(t *testing.T) {
	// Should not panic or call anything
	n := webhook.New("", "", logger())
	n.Notify(webhook.Payload{Event: "test"})
}

func TestValidate(t *testing.T) {
	tests := []struct {
		url     string
		wantErr bool
	}{
		{"", false},
		{"https://example.com/hook", false},
		{"http://localhost:9000/hook", false},
		{"ftp://invalid", true},
		{"not-a-url", true},
	}

	for _, tt := range tests {
		err := webhook.Validate(tt.url)
		if (err != nil) != tt.wantErr {
			t.Errorf("Validate(%q): wantErr=%v, got %v", tt.url, tt.wantErr, err)
		}
	}
}

func TestHMACSignature(t *testing.T) {
	signed := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sig := r.Header.Get("X-ScrapeOwl-Signature")
		if sig == "" {
			t.Error("expected HMAC signature header")
		} else {
			signed = true
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := webhook.New(srv.URL, "my-secret-key", logger())
	now := time.Now()
	p := webhook.EventFailure("test-job", "run-2", "error msg", &now)
	n.Notify(p)

	deadline := time.Now().Add(2 * time.Second)
	for !signed && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if !signed {
		t.Error("HMAC signature was not sent")
	}
}

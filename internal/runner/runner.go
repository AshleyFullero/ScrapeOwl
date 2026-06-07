package runner

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"time"

	"github.com/ashleyfullero/scrapeowl/internal/browser"
	"github.com/ashleyfullero/scrapeowl/internal/captcha"
	"github.com/ashleyfullero/scrapeowl/internal/config"
	"github.com/ashleyfullero/scrapeowl/internal/extractor"
	"github.com/ashleyfullero/scrapeowl/internal/output"
	"github.com/ashleyfullero/scrapeowl/internal/proxy"
)

// Status represents the current state of a run
type Status string

const (
	StatusPending  Status = "pending"
	StatusRunning  Status = "running"
	StatusSuccess  Status = "success"
	StatusFailed   Status = "failed"
	StatusCanceled Status = "canceled"
)

// Run represents a single execution of a job
type Run struct {
	ID          string
	JobName     string
	Status      Status
	StartedAt   time.Time
	CompletedAt *time.Time
	Records     int
	Error       string
	Bus         *EventBus
	cancel      context.CancelFunc
}

// Runner orchestrates the full execution of a scraping job
type Runner struct {
	cfg        *config.JobConfig
	proxyPool  *proxy.Pool
	captchaSvr captcha.Solver
	logger     *slog.Logger
}

// New creates a new Runner for a job config
func New(cfg *config.JobConfig, logger *slog.Logger) (*Runner, error) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}

	proxyPool, err := proxy.NewPool(cfg.Proxy)
	if err != nil {
		return nil, fmt.Errorf("creating proxy pool: %w", err)
	}

	captchaSolver, err := captcha.New(cfg.Captcha)
	if err != nil {
		return nil, fmt.Errorf("creating captcha solver: %w", err)
	}

	return &Runner{
		cfg:        cfg,
		proxyPool:  proxyPool,
		captchaSvr: captchaSolver,
		logger:     logger,
	}, nil
}

// Execute runs the job, returning the run result. It streams events to the bus.
func (r *Runner) Execute(ctx context.Context, run *Run) {
	run.Status = StatusRunning
	run.StartedAt = time.Now()
	bus := run.Bus

	bus.SetStatus("running", 0, run.JobName, run.ID)
	bus.Log(LogLevelInfo, fmt.Sprintf("Starting job '%s'", r.cfg.Name), run.JobName, run.ID)

	// Attempt with retries
	maxAttempts := r.cfg.Retry.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			delay := r.calcBackoff(attempt, r.cfg.Retry.Backoff)
			bus.Log(LogLevelWarn, fmt.Sprintf("Retry %d/%d in %s", attempt, maxAttempts, delay), run.JobName, run.ID)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				run.Status = StatusCanceled
				bus.SetStatus("canceled", 0, run.JobName, run.ID)
				return
			}
		}

		records, err := r.executeOnce(ctx, run)
		if err == nil {
			run.Records = records
			now := time.Now()
			run.CompletedAt = &now
			run.Status = StatusSuccess
			bus.Log(LogLevelInfo, fmt.Sprintf("Job completed: %d records written", records), run.JobName, run.ID)
			bus.SetStatus("success", 100, run.JobName, run.ID)
			bus.Publish(Event{
				Type:      EventTypeComplete,
				Timestamp: time.Now(),
				JobName:   run.JobName,
				RunID:     run.ID,
				Data:      map[string]interface{}{"records": records},
			})
			return
		}
		lastErr = err
		bus.Log(LogLevelError, fmt.Sprintf("Attempt %d failed: %v", attempt, err), run.JobName, run.ID)
	}

	now := time.Now()
	run.CompletedAt = &now
	run.Status = StatusFailed
	run.Error = lastErr.Error()
	bus.SetStatus("failed", 0, run.JobName, run.ID)
	bus.Publish(Event{
		Type:      EventTypeError,
		Timestamp: time.Now(),
		JobName:   run.JobName,
		RunID:     run.ID,
		Message:   lastErr.Error(),
	})
}

// executeOnce performs a single execution attempt
func (r *Runner) executeOnce(ctx context.Context, run *Run) (int, error) {
	bus := run.Bus
	cfg := r.cfg

	// Select proxy
	proxyURL := r.proxyPool.Next()

	// Start browser
	bus.Log(LogLevelInfo, "Launching browser...", run.JobName, run.ID)
	opts := browser.DefaultOptions()
	if proxyURL != "" {
		opts.ProxyURL = proxyURL
		bus.Log(LogLevelInfo, fmt.Sprintf("Using proxy: %s", maskProxy(proxyURL)), run.JobName, run.ID)
	}

	b, err := browser.New(opts, r.logger)
	if err != nil {
		return 0, fmt.Errorf("launching browser: %w", err)
	}
	defer b.Close()

	// Navigate to start URL
	bus.Log(LogLevelInfo, fmt.Sprintf("Navigating to %s", cfg.StartURL), run.JobName, run.ID)
	if err := b.Navigate(cfg.StartURL); err != nil {
		return 0, fmt.Errorf("navigating to start URL: %w", err)
	}

	// Execute steps
	totalSteps := len(cfg.Steps)
	for i, step := range cfg.Steps {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}

		bus.Log(LogLevelInfo,
			fmt.Sprintf("Step %d/%d: %s %s", i+1, totalSteps, step.Action, step.Selector),
			run.JobName, run.ID,
		)

		progress := int((float64(i+1) / float64(totalSteps+1)) * 80)
		bus.SetStatus("running", progress, run.JobName, run.ID)

		stepErr := b.RunStep(step)
		bus.Publish(Event{
			Type:      EventTypeStep,
			Timestamp: time.Now(),
			JobName:   run.JobName,
			RunID:     run.ID,
			Data: StepEvent{
				Index:    i,
				Action:   step.Action,
				Selector: step.Selector,
				Success:  stepErr == nil,
				Error:    errStr(stepErr),
			},
		})

		if stepErr != nil && !step.Optional {
			if proxyURL != "" {
				r.proxyPool.MarkFailure(proxyURL)
			}
			return 0, fmt.Errorf("step %d (%s): %w", i+1, step.Action, stepErr)
		}
	}

	// Get page HTML for extractors
	bus.Log(LogLevelInfo, "Extracting data...", run.JobName, run.ID)
	pageHTML, err := b.GetPageHTML()
	if err != nil {
		bus.Log(LogLevelWarn, fmt.Sprintf("Could not get page HTML: %v", err), run.JobName, run.ID)
	}

	// Run extractors
	if len(cfg.Extract) == 0 {
		bus.Log(LogLevelWarn, "No extractors defined, job complete with 0 records", run.JobName, run.ID)
		return 0, nil
	}

	results, extractErrs := extractor.ExtractAll(b, cfg.Extract, pageHTML, cfg.AI)
	for _, ee := range extractErrs {
		bus.Log(LogLevelWarn, fmt.Sprintf("Extraction warning: %v", ee), run.JobName, run.ID)
	}

	// Emit extract events
	for name, val := range results {
		bus.Publish(Event{
			Type:      EventTypeExtract,
			Timestamp: time.Now(),
			JobName:   run.JobName,
			RunID:     run.ID,
			Data:      ExtractEvent{Name: name, Value: val},
		})
	}

	// Write output
	bus.Log(LogLevelInfo, fmt.Sprintf("Writing output to %s", cfg.Output.Path), run.JobName, run.ID)
	w, err := output.NewWriter(cfg.Output.Format, cfg.Output.Path)
	if err != nil {
		return 0, fmt.Errorf("creating output writer: %w", err)
	}
	defer w.Close()

	if err := w.Write(results); err != nil {
		return 0, fmt.Errorf("writing output: %w", err)
	}

	bus.Publish(Event{
		Type:      EventTypeOutput,
		Timestamp: time.Now(),
		JobName:   run.JobName,
		RunID:     run.ID,
		Message:   fmt.Sprintf("Wrote 1 record to %s", cfg.Output.Path),
	})

	if proxyURL != "" {
		r.proxyPool.MarkSuccess(proxyURL)
	}

	return 1, nil
}

// calcBackoff returns the wait duration for a given attempt
func (r *Runner) calcBackoff(attempt int, strategy string) time.Duration {
	base := 2 * time.Second
	switch strategy {
	case "exponential":
		return time.Duration(math.Pow(2, float64(attempt-1))) * base
	case "linear":
		return time.Duration(attempt) * base
	default:
		return base
	}
}

func errStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func maskProxy(u string) string {
	if len(u) > 20 {
		return u[:8] + "***"
	}
	return "***"
}

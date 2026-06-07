package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ashleyfullero/scrapeowl/internal/api"
	"github.com/ashleyfullero/scrapeowl/internal/config"
	"github.com/ashleyfullero/scrapeowl/internal/runner"
	"github.com/ashleyfullero/scrapeowl/internal/scheduler"
	"github.com/ashleyfullero/scrapeowl/internal/store"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// --- CLI flags ---
	var (
		serveCmd   = flag.NewFlagSet("serve", flag.ExitOnError)
		serveAddr  = serveCmd.String("addr", ":8080", "HTTP listen address")
		serveDB    = serveCmd.String("db", "./scrapeowl.db", "SQLite database path")
		serveDebug = serveCmd.Bool("debug", false, "Enable debug logging")

		runCmd    = flag.NewFlagSet("run", flag.ExitOnError)
		runFile   = runCmd.String("file", "", "Path to YAML job file")
		runDebug  = runCmd.Bool("debug", false, "Enable debug logging")

		validateCmd  = flag.NewFlagSet("validate", flag.ExitOnError)
		validateFile = validateCmd.String("file", "", "Path to YAML job file to validate")
	)

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		_ = serveCmd.Parse(os.Args[2:])
		runServe(*serveAddr, *serveDB, *serveDebug)

	case "run":
		_ = runCmd.Parse(os.Args[2:])
		if *runFile == "" {
			fmt.Fprintln(os.Stderr, "error: --file is required")
			runCmd.Usage()
			os.Exit(1)
		}
		runJob(*runFile, *runDebug)

	case "validate":
		_ = validateCmd.Parse(os.Args[2:])
		if *validateFile == "" {
			fmt.Fprintln(os.Stderr, "error: --file is required")
			validateCmd.Usage()
			os.Exit(1)
		}
		validateJob(*validateFile)

	case "version":
		fmt.Printf("ScrapeOwl %s (commit: %s, built: %s)\n", version, commit, date)

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

// runServe starts the ScrapeOwl dashboard server
func runServe(addr, dbPath string, debug bool) {
	logger := newLogger(debug)
	logger.Info("Starting ScrapeOwl", "version", version)

	// Open database
	st, err := store.Open(dbPath)
	if err != nil {
		logger.Error("opening database", "path", dbPath, "err", err)
		os.Exit(1)
	}
	defer st.Close()

	// Create scheduler with run callback
	var srv *api.Server

	sched := scheduler.New(func(ctx context.Context, jobName string) {
		job, err := st.GetJobByName(jobName)
		if err != nil || job == nil {
			logger.Error("scheduled job not found", "job", jobName)
			return
		}
		// We need access to the server's handlers to run, so we call through the API
		logger.Info("Scheduled job triggered", "job", jobName)

		cfg, err := config.Parse([]byte(job.YAMLContent))
		if err != nil {
			logger.Error("parsing job config", "job", jobName, "err", err)
			return
		}

		jobRunner, err := runner.New(cfg, logger)
		if err != nil {
			logger.Error("creating runner", "job", jobName, "err", err)
			return
		}

		bus := runner.NewEventBus()
		rn := &runner.Run{
			ID:      fmt.Sprintf("sched-%d", time.Now().UnixNano()),
			JobName: jobName,
			Status:  runner.StatusPending,
			Bus:     bus,
		}

		// Subscribe and broadcast to WebSocket if server is up
		ch := bus.Subscribe()
		go func() {
			for event := range ch {
				if srv != nil {
					srv.Hub().Broadcast(event)
				}
			}
		}()

		jobRunner.Execute(ctx, rn)
		bus.Close()

		storeRun := &store.Run{
			ID:          rn.ID,
			JobID:       job.ID,
			JobName:     job.Name,
			Status:      string(rn.Status),
			StartedAt:   &rn.StartedAt,
			CompletedAt: rn.CompletedAt,
			Records:     rn.Records,
			Error:       rn.Error,
			CreatedAt:   time.Now(),
		}
		if err := st.CreateRun(storeRun); err == nil {
			_ = st.UpdateRun(storeRun)
		}
	}, logger)

	// Re-schedule all existing jobs
	jobs, _ := st.ListJobs()
	for _, job := range jobs {
		if job.Schedule != "" && job.Enabled {
			if err := sched.Add(job.ID, job.Name, job.Schedule); err != nil {
				logger.Warn("restoring schedule", "job", job.Name, "err", err)
			}
		}
	}
	sched.Start()
	defer sched.Stop()

	// Start HTTP server
	srv = api.NewServer(addr, st, sched, logger)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		logger.Info("Shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	if err := srv.Start(); err != nil {
		logger.Info("Server stopped", "reason", err)
	}
}

// runJob executes a single job from a YAML file and exits
func runJob(filePath string, debug bool) {
	logger := newLogger(debug)

	cfg, err := config.LoadFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading job file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Running job: %s\n", cfg.Name)
	fmt.Printf("Start URL: %s\n", cfg.StartURL)

	jobRunner, err := runner.New(cfg, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating runner: %v\n", err)
		os.Exit(1)
	}

	bus := runner.NewEventBus()
	rn := &runner.Run{
		ID:      fmt.Sprintf("cli-%d", time.Now().UnixNano()),
		JobName: cfg.Name,
		Status:  runner.StatusPending,
		Bus:     bus,
	}

	// Print events to stdout
	ch := bus.Subscribe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for event := range ch {
			printEvent(event)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle Ctrl+C
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		fmt.Println("\nCanceling job...")
		cancel()
	}()

	jobRunner.Execute(ctx, rn)
	bus.Close()
	<-done

	switch rn.Status {
	case runner.StatusSuccess:
		fmt.Printf("\n✓ Job completed successfully. Records written: %d\n", rn.Records)
		fmt.Printf("  Output: %s\n", cfg.Output.Path)
	case runner.StatusFailed:
		fmt.Printf("\n✗ Job failed: %s\n", rn.Error)
		os.Exit(1)
	case runner.StatusCanceled:
		fmt.Println("\n⊘ Job canceled")
		os.Exit(130)
	}
}

// validateJob validates a YAML job file and exits
func validateJob(filePath string) {
	cfg, err := config.LoadFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "✗ Invalid: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Valid job: '%s'\n", cfg.Name)
	fmt.Printf("  Start URL:  %s\n", cfg.StartURL)
	fmt.Printf("  Steps:      %d\n", len(cfg.Steps))
	fmt.Printf("  Extractors: %d\n", len(cfg.Extract))
	fmt.Printf("  Output:     %s (%s)\n", cfg.Output.Path, cfg.Output.Format)
	if cfg.Schedule != "" {
		fmt.Printf("  Schedule:   %s\n", cfg.Schedule)
	}
	if cfg.Proxy.Type != "none" {
		fmt.Printf("  Proxies:    %d (%s)\n", len(cfg.Proxy.List), cfg.Proxy.Type)
	}
}

func printEvent(event runner.Event) {
	ts := event.Timestamp.Format("15:04:05")
	switch event.Type {
	case runner.EventTypeLog:
		var icon string
		switch event.Level {
		case runner.LogLevelError:
			icon = "✗"
		case runner.LogLevelWarn:
			icon = "⚠"
		default:
			icon = "›"
		}
		fmt.Printf("[%s] %s %s\n", ts, icon, event.Message)
	case runner.EventTypeStatus:
		// Only print major status changes
	case runner.EventTypeExtract:
		fmt.Printf("[%s] ↳ Extracted field\n", ts)
	case runner.EventTypeComplete:
		// Handled in main
	}
}

func newLogger(debug bool) *slog.Logger {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}

func printUsage() {
	fmt.Println(`ScrapeOwl - Web Scraping Operations Platform

Usage:
  scrapeowl <command> [options]

Commands:
  serve       Start the dashboard server
  run         Execute a job from a YAML file
  validate    Validate a YAML job file
  version     Print version information

Run 'scrapeowl <command> --help' for command options.

Examples:
  scrapeowl serve --addr :8080
  scrapeowl run --file ./examples/product-scraper.yaml
  scrapeowl validate --file ./scrape.yaml
`)
}

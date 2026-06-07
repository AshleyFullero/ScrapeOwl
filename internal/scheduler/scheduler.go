package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/robfig/cron/v3"
)

// JobFunc is the function to call when a scheduled job fires
type JobFunc func(ctx context.Context, jobName string)

// Entry represents a scheduled job entry
type Entry struct {
	JobID      string
	JobName    string
	Expression string
	EntryID    cron.EntryID
}

// Scheduler manages cron-based job scheduling
type Scheduler struct {
	mu      sync.RWMutex
	cron    *cron.Cron
	entries map[string]*Entry // jobID -> Entry
	logger  *slog.Logger
	fn      JobFunc
}

// New creates a new Scheduler with the given job execution function
func New(fn JobFunc, logger *slog.Logger) *Scheduler {
	c := cron.New(cron.WithSeconds())
	return &Scheduler{
		cron:    c,
		entries: make(map[string]*Entry),
		logger:  logger,
		fn:      fn,
	}
}

// Start begins the cron scheduler
func (s *Scheduler) Start() {
	s.cron.Start()
	s.logger.Info("Scheduler started")
}

// Stop gracefully stops the scheduler
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
	s.logger.Info("Scheduler stopped")
}

// Add schedules a job with a cron expression
func (s *Scheduler) Add(jobID, jobName, expression string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove existing entry if present
	if existing, ok := s.entries[jobID]; ok {
		s.cron.Remove(existing.EntryID)
		delete(s.entries, jobID)
	}

	if expression == "" {
		return nil // No schedule, don't add
	}

	entryID, err := s.cron.AddFunc(expression, func() {
		s.logger.Info("Scheduled job triggered", "job", jobName)
		s.fn(context.Background(), jobName)
	})
	if err != nil {
		return fmt.Errorf("adding cron job '%s': %w", jobName, err)
	}

	s.entries[jobID] = &Entry{
		JobID:      jobID,
		JobName:    jobName,
		Expression: expression,
		EntryID:    entryID,
	}

	s.logger.Info("Job scheduled", "job", jobName, "cron", expression)
	return nil
}

// Remove unschedules a job
func (s *Scheduler) Remove(jobID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry, ok := s.entries[jobID]; ok {
		s.cron.Remove(entry.EntryID)
		delete(s.entries, jobID)
		s.logger.Info("Job unscheduled", "job", entry.JobName)
	}
}

// List returns all scheduled entries
func (s *Scheduler) List() []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries := make([]Entry, 0, len(s.entries))
	for _, e := range s.entries {
		entries = append(entries, *e)
	}
	return entries
}

// NextRun returns the next scheduled run time for a job
func (s *Scheduler) NextRun(jobID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.entries[jobID]
	if !ok {
		return ""
	}

	ce := s.cron.Entry(entry.EntryID)
	if ce.ID == 0 {
		return ""
	}

	return ce.Next.Format("2006-01-02T15:04:05Z07:00")
}

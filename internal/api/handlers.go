package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/ashleyfullero/scrapeowl/internal/config"
	"github.com/ashleyfullero/scrapeowl/internal/license"
	"github.com/ashleyfullero/scrapeowl/internal/proxy"
	"github.com/ashleyfullero/scrapeowl/internal/runner"
	"github.com/ashleyfullero/scrapeowl/internal/scheduler"
	"github.com/ashleyfullero/scrapeowl/internal/store"
)

// Handlers holds all API handler dependencies
type Handlers struct {
	store      *store.Store
	hub        *Hub
	scheduler  *scheduler.Scheduler
	logger     *slog.Logger
	mu         sync.RWMutex
	activeRuns map[string]*runner.Run
	runCancel  map[string]context.CancelFunc
}

// NewHandlers creates a handler set
func NewHandlers(st *store.Store, hub *Hub, sched *scheduler.Scheduler, logger *slog.Logger) *Handlers {
	return &Handlers{
		store:      st,
		hub:        hub,
		scheduler:  sched,
		logger:     logger,
		activeRuns: make(map[string]*runner.Run),
		runCancel:  make(map[string]context.CancelFunc),
	}
}

// --- Response helpers ---

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// --- Job Handlers ---

// ListJobs handles GET /api/jobs
func (h *Handlers) ListJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := h.store.ListJobs()
	if err != nil {
		h.logger.Error("listing jobs", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list jobs")
		return
	}

	// Attach next run time from scheduler
	type jobResponse struct {
		*store.Job
		NextRun string `json:"next_run,omitempty"`
	}

	resp := make([]jobResponse, len(jobs))
	for i, j := range jobs {
		resp[i] = jobResponse{
			Job:     j,
			NextRun: h.scheduler.NextRun(j.ID),
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetJob handles GET /api/jobs/{id}
func (h *Handlers) GetJob(w http.ResponseWriter, r *http.Request) {
	id := extractID(r.URL.Path, "/api/jobs/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing job id")
		return
	}

	job, err := h.store.GetJob(id)
	if err != nil || job == nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// CreateJob handles POST /api/jobs
func (h *Handlers) CreateJob(w http.ResponseWriter, r *http.Request) {
	var req struct {
		YAMLContent string `json:"yaml_content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Parse and validate the YAML
	cfg, err := config.Parse([]byte(req.YAMLContent))
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid job YAML: %v", err))
		return
	}

	// Check for duplicate name
	existing, _ := h.store.GetJobByName(cfg.Name)
	if existing != nil {
		writeError(w, http.StatusConflict, fmt.Sprintf("job with name '%s' already exists", cfg.Name))
		return
	}

	job := &store.Job{
		ID:          uuid.New().String(),
		Name:        cfg.Name,
		YAMLContent: req.YAMLContent,
		Schedule:    cfg.Schedule,
		Enabled:     true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := h.store.CreateJob(job); err != nil {
		h.logger.Error("creating job", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to create job")
		return
	}

	// Schedule if a cron expression was provided
	if cfg.Schedule != "" {
		if err := h.scheduler.Add(job.ID, job.Name, cfg.Schedule); err != nil {
			h.logger.Warn("scheduling job", "job", job.Name, "err", err)
		}
	}

	writeJSON(w, http.StatusCreated, job)
}

// UpdateJob handles PUT /api/jobs/{id}
func (h *Handlers) UpdateJob(w http.ResponseWriter, r *http.Request) {
	id := extractID(r.URL.Path, "/api/jobs/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing job id")
		return
	}

	var req struct {
		YAMLContent string `json:"yaml_content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	cfg, err := config.Parse([]byte(req.YAMLContent))
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid job YAML: %v", err))
		return
	}

	if err := h.store.UpdateJob(id, req.YAMLContent, cfg.Schedule); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update job")
		return
	}

	// Re-schedule
	h.scheduler.Remove(id)
	if cfg.Schedule != "" {
		if err := h.scheduler.Add(id, cfg.Name, cfg.Schedule); err != nil {
			h.logger.Warn("rescheduling job", "job", cfg.Name, "err", err)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// DeleteJob handles DELETE /api/jobs/{id}
func (h *Handlers) DeleteJob(w http.ResponseWriter, r *http.Request) {
	id := extractID(r.URL.Path, "/api/jobs/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing job id")
		return
	}

	h.scheduler.Remove(id)

	if err := h.store.DeleteJob(id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete job")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// RunJob handles POST /api/jobs/{id}/run
func (h *Handlers) RunJob(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	id := extractID(path, "/api/jobs/")
	id = strings.TrimSuffix(id, "/run")

	job, err := h.store.GetJob(id)
	if err != nil || job == nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}

	runID, err := h.launchRun(job)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to start run: %v", err))
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"run_id": runID})
}

// launchRun starts a job run asynchronously and returns the run ID
func (h *Handlers) launchRun(job *store.Job) (string, error) {
	cfg, err := config.Parse([]byte(job.YAMLContent))
	if err != nil {
		return "", fmt.Errorf("parsing job config: %w", err)
	}

	runID := uuid.New().String()
	run := &store.Run{
		ID:        runID,
		JobID:     job.ID,
		JobName:   job.Name,
		Status:    string(runner.StatusPending),
		CreatedAt: time.Now(),
	}

	if err := h.store.CreateRun(run); err != nil {
		return "", fmt.Errorf("creating run record: %w", err)
	}

	// Create runner run with event bus
	bus := runner.NewEventBus()
	rn := &runner.Run{
		ID:      runID,
		JobName: job.Name,
		Status:  runner.StatusPending,
		Bus:     bus,
	}

	ctx, cancel := context.WithCancel(context.Background())
	h.mu.Lock()
	h.activeRuns[runID] = rn
	h.runCancel[runID] = cancel
	h.mu.Unlock()

	// Subscribe to events and broadcast to WebSocket
	ch := bus.Subscribe()
	go func() {
		for event := range ch {
			h.hub.Broadcast(event)
		}
	}()

	// Execute the run asynchronously
	go func() {
		defer func() {
			bus.Close()
			h.mu.Lock()
			delete(h.activeRuns, runID)
			delete(h.runCancel, runID)
			h.mu.Unlock()
			cancel()
		}()

		jobRunner, err := runner.NewWithStore(cfg, h.logger, h.store)
		if err != nil {
			h.logger.Error("creating runner", "err", err)
			return
		}

		jobRunner.Execute(ctx, rn)

		// Persist the run result
		now := time.Now()
		storeRun := &store.Run{
			ID:          runID,
			JobID:       job.ID,
			JobName:     job.Name,
			Status:      string(rn.Status),
			StartedAt:   &rn.StartedAt,
			CompletedAt: rn.CompletedAt,
			Records:     rn.Records,
			Error:       rn.Error,
		}
		if err := h.store.UpdateRun(storeRun); err != nil {
			h.logger.Error("updating run", "run_id", runID, "err", err)
		}
		_ = now
	}()

	return runID, nil
}

// StopRun handles POST /api/runs/{id}/stop
func (h *Handlers) StopRun(w http.ResponseWriter, r *http.Request) {
	id := extractID(r.URL.Path, "/api/runs/")
	id = strings.TrimSuffix(id, "/stop")

	h.mu.RLock()
	cancel, ok := h.runCancel[id]
	h.mu.RUnlock()
	if !ok {
		writeError(w, http.StatusNotFound, "run not found or already completed")
		return
	}
	cancel()
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopping"})
}

// --- Run Handlers ---

// ListRuns handles GET /api/runs
func (h *Handlers) ListRuns(w http.ResponseWriter, r *http.Request) {
	jobID := r.URL.Query().Get("job_id")

	// Pagination params
	limit := parseIntQuery(r, "limit", 25)
	offset := parseIntQuery(r, "offset", 0)
	if limit > 100 {
		limit = 100
	}

	runs, err := h.store.ListRunsPaged(jobID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list runs")
		return
	}
	if runs == nil {
		runs = []*store.Run{}
	}

	total, _ := h.store.CountRuns(jobID)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"runs":   runs,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// GetRun handles GET /api/runs/{id}
func (h *Handlers) GetRun(w http.ResponseWriter, r *http.Request) {
	id := extractID(r.URL.Path, "/api/runs/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing run id")
		return
	}

	run, err := h.store.GetRun(id)
	if err != nil || run == nil {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}
	writeJSON(w, http.StatusOK, run)
}

// --- Stats Handler ---

// GetStats handles GET /api/stats
func (h *Handlers) GetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.store.GetStats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get stats")
		return
	}

	// Enrich with proxy pool info
	type statsResponse struct {
		*store.Stats
		ActiveRuns int         `json:"active_runs"`
		License    interface{} `json:"license"`
	}

	writeJSON(w, http.StatusOK, statsResponse{
		Stats:      stats,
		ActiveRuns: func() int { h.mu.RLock(); defer h.mu.RUnlock(); return len(h.activeRuns) }(),
		License:    license.Get(),
	})
}

// GetProxyStats handles GET /api/proxy/stats
func (h *Handlers) GetProxyStats(w http.ResponseWriter, r *http.Request) {
	// Return empty pool stats since proxy pool is per-job
	writeJSON(w, http.StatusOK, proxy.PoolStats{
		ProxyType: "per-job",
	})
}

// ValidateYAML handles POST /api/validate
func (h *Handlers) ValidateYAML(w http.ResponseWriter, r *http.Request) {
	var req struct {
		YAMLContent string `json:"yaml_content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	_, err := config.Parse([]byte(req.YAMLContent))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"valid": false,
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"valid": true,
	})
}

// GetRunData handles GET /api/runs/{id}/data
func (h *Handlers) GetRunData(w http.ResponseWriter, r *http.Request) {
	id := extractID(r.URL.Path, "/api/runs/")
	id = strings.TrimSuffix(id, "/data")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing run id")
		return
	}

	data, err := h.store.GetExtractedData(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get run data")
		return
	}
	if data == nil {
		data = map[string]interface{}{}
	}
	writeJSON(w, http.StatusOK, data)
}

// EnableJob handles POST /api/jobs/{id}/enable
func (h *Handlers) EnableJob(w http.ResponseWriter, r *http.Request) {
	id := extractID(r.URL.Path, "/api/jobs/")
	id = strings.TrimSuffix(id, "/enable")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing job id")
		return
	}

	if err := h.store.SetJobEnabled(id, true); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to enable job")
		return
	}

	// Re-schedule if job has a schedule
	job, _ := h.store.GetJob(id)
	if job != nil && job.Schedule != "" {
		_ = h.scheduler.Add(job.ID, job.Name, job.Schedule)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "enabled"})
}

// DisableJob handles POST /api/jobs/{id}/disable
func (h *Handlers) DisableJob(w http.ResponseWriter, r *http.Request) {
	id := extractID(r.URL.Path, "/api/jobs/")
	id = strings.TrimSuffix(id, "/disable")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing job id")
		return
	}

	if err := h.store.SetJobEnabled(id, false); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to disable job")
		return
	}

	// Remove from scheduler
	h.scheduler.Remove(id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "disabled"})
}

// extractID extracts a path segment after a prefix
func extractID(path, prefix string) string {
	after := strings.TrimPrefix(path, prefix)
	parts := strings.SplitN(after, "/", 2)
	return parts[0]
}

// parseIntQuery parses an integer query parameter with a fallback default
func parseIntQuery(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n := 0
	for _, c := range v {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	return n
}

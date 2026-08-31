package store_test

import (
	"os"
	"testing"
	"time"

	"github.com/ashleyfullero/scrapeowl/internal/store"
)

func tempDB(t *testing.T) (*store.Store, func()) {
	t.Helper()
	f, err := os.CreateTemp("", "scrapeowl-test-*.db")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	f.Close()

	st, err := store.Open(f.Name())
	if err != nil {
		os.Remove(f.Name())
		t.Fatalf("open store: %v", err)
	}

	return st, func() {
		st.Close()
		os.Remove(f.Name())
	}
}

func TestJobCRUD(t *testing.T) {
	st, cleanup := tempDB(t)
	defer cleanup()

	// Create
	job := &store.Job{
		ID:          "job-1",
		Name:        "test-job",
		YAMLContent: "name: test-job\nstart_url: https://example.com\n",
		Schedule:    "",
		Enabled:     true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := st.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	// Get
	got, err := st.GetJob("job-1")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got == nil {
		t.Fatal("GetJob returned nil")
	}
	if got.Name != "test-job" {
		t.Errorf("expected name 'test-job', got %q", got.Name)
	}

	// GetByName
	byName, err := st.GetJobByName("test-job")
	if err != nil || byName == nil {
		t.Fatalf("GetJobByName: %v", err)
	}

	// List
	jobs, err := st.ListJobs()
	if err != nil || len(jobs) != 1 {
		t.Fatalf("ListJobs: expected 1 job, got %d: %v", len(jobs), err)
	}

	// Update
	if err := st.UpdateJob("job-1", "name: test-job\nstart_url: https://updated.com\n", "0 * * * *"); err != nil {
		t.Fatalf("UpdateJob: %v", err)
	}

	// Enable/Disable toggle
	if err := st.SetJobEnabled("job-1", false); err != nil {
		t.Fatalf("SetJobEnabled(false): %v", err)
	}
	got, _ = st.GetJob("job-1")
	if got.Enabled {
		t.Error("expected job to be disabled")
	}

	if err := st.SetJobEnabled("job-1", true); err != nil {
		t.Fatalf("SetJobEnabled(true): %v", err)
	}
	got, _ = st.GetJob("job-1")
	if !got.Enabled {
		t.Error("expected job to be re-enabled")
	}

	// Delete
	if err := st.DeleteJob("job-1"); err != nil {
		t.Fatalf("DeleteJob: %v", err)
	}
	deleted, _ := st.GetJob("job-1")
	if deleted != nil {
		t.Error("expected nil after delete")
	}
}

func TestRunCRUD(t *testing.T) {
	st, cleanup := tempDB(t)
	defer cleanup()

	// Create a job first (runs have FK to jobs)
	job := &store.Job{
		ID: "j1", Name: "myjob", YAMLContent: "x",
		Enabled: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	_ = st.CreateJob(job)

	run := &store.Run{
		ID: "r1", JobID: "j1", JobName: "myjob",
		Status: "pending", CreatedAt: time.Now(),
	}
	if err := st.CreateRun(run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	now := time.Now()
	run.Status = "success"
	run.StartedAt = &now
	run.CompletedAt = &now
	run.Records = 5
	if err := st.UpdateRun(run); err != nil {
		t.Fatalf("UpdateRun: %v", err)
	}

	got, err := st.GetRun("r1")
	if err != nil || got == nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.Records != 5 {
		t.Errorf("expected 5 records, got %d", got.Records)
	}
}

func TestExtractedData(t *testing.T) {
	st, cleanup := tempDB(t)
	defer cleanup()

	// Setup
	job := &store.Job{ID: "j2", Name: "job2", YAMLContent: "x", Enabled: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	_ = st.CreateJob(job)
	run := &store.Run{ID: "r2", JobID: "j2", JobName: "job2", Status: "success", CreatedAt: time.Now()}
	_ = st.CreateRun(run)

	data := map[string]interface{}{
		"title":  "Hello World",
		"price":  29.99,
		"tags":   []interface{}{"go", "web"},
		"active": true,
	}

	if err := st.SaveExtractedData("r2", data); err != nil {
		t.Fatalf("SaveExtractedData: %v", err)
	}

	retrieved, err := st.GetExtractedData("r2")
	if err != nil {
		t.Fatalf("GetExtractedData: %v", err)
	}

	if retrieved["title"] != "Hello World" {
		t.Errorf("expected title 'Hello World', got %v", retrieved["title"])
	}

	// Idempotent: save again should replace
	data2 := map[string]interface{}{"title": "Updated"}
	if err := st.SaveExtractedData("r2", data2); err != nil {
		t.Fatalf("SaveExtractedData (update): %v", err)
	}
	retrieved2, _ := st.GetExtractedData("r2")
	if retrieved2["title"] != "Updated" {
		t.Errorf("expected updated title, got %v", retrieved2["title"])
	}
	if _, hasPrice := retrieved2["price"]; hasPrice {
		t.Error("expected old fields to be cleared after update")
	}
}

func TestListRunsPaged(t *testing.T) {
	st, cleanup := tempDB(t)
	defer cleanup()

	job := &store.Job{ID: "j3", Name: "job3", YAMLContent: "x", Enabled: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	_ = st.CreateJob(job)

	// Insert 10 runs
	for i := 0; i < 10; i++ {
		r := &store.Run{
			ID: "run-" + string(rune('0'+i)), JobID: "j3", JobName: "job3",
			Status: "success", CreatedAt: time.Now(),
		}
		_ = st.CreateRun(r)
	}

	total, err := st.CountRuns("j3")
	if err != nil {
		t.Fatalf("CountRuns: %v", err)
	}
	if total != 10 {
		t.Errorf("expected 10 runs, got %d", total)
	}

	page1, err := st.ListRunsPaged("j3", 5, 0)
	if err != nil || len(page1) != 5 {
		t.Errorf("page1: expected 5, got %d: %v", len(page1), err)
	}

	page2, err := st.ListRunsPaged("j3", 5, 5)
	if err != nil || len(page2) != 5 {
		t.Errorf("page2: expected 5, got %d: %v", len(page2), err)
	}
}

func TestGetStats(t *testing.T) {
	st, cleanup := tempDB(t)
	defer cleanup()

	stats, err := st.GetStats()
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats == nil {
		t.Fatal("GetStats returned nil")
	}
	// Initial state should be zeros
	if stats.TotalRuns != 0 {
		t.Errorf("expected 0 total runs, got %d", stats.TotalRuns)
	}
}

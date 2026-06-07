package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Store manages the SQLite database
type Store struct {
	db *sql.DB
}

// Job represents a stored job definition
type Job struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	YAMLContent string    `json:"yaml_content"`
	Schedule    string    `json:"schedule"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Run represents a stored job execution record
type Run struct {
	ID          string     `json:"id"`
	JobID       string     `json:"job_id"`
	JobName     string     `json:"job_name"`
	Status      string     `json:"status"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
	Records     int        `json:"records"`
	Error       string     `json:"error"`
	CreatedAt   time.Time  `json:"created_at"`
}

// Stats holds aggregated platform statistics
type Stats struct {
	TotalRuns    int `json:"total_runs"`
	TotalRecords int `json:"total_records"`
	TotalErrors  int `json:"total_errors"`
}

// Open opens (or creates) the SQLite database at the given path
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return s, nil
}

// Close closes the database connection
func (s *Store) Close() error {
	return s.db.Close()
}

// migrate runs all pending migrations
func (s *Store) migrate() error {
	for _, migration := range migrations {
		if _, err := s.db.Exec(migration); err != nil {
			return fmt.Errorf("migration failed (%s): %w", migration[:min(50, len(migration))], err)
		}
	}
	return nil
}

// --- Job CRUD ---

// CreateJob inserts a new job into the database
func (s *Store) CreateJob(job *Job) error {
	_, err := s.db.Exec(
		`INSERT INTO jobs (id, name, yaml_content, schedule, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		job.ID, job.Name, job.YAMLContent, job.Schedule, boolToInt(job.Enabled),
		job.CreatedAt, job.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("creating job: %w", err)
	}
	return nil
}

// GetJob retrieves a job by ID
func (s *Store) GetJob(id string) (*Job, error) {
	row := s.db.QueryRow(
		`SELECT id, name, yaml_content, schedule, enabled, created_at, updated_at
		 FROM jobs WHERE id = ?`, id,
	)
	return scanJob(row)
}

// GetJobByName retrieves a job by name
func (s *Store) GetJobByName(name string) (*Job, error) {
	row := s.db.QueryRow(
		`SELECT id, name, yaml_content, schedule, enabled, created_at, updated_at
		 FROM jobs WHERE name = ?`, name,
	)
	return scanJob(row)
}

// ListJobs returns all jobs ordered by creation date
func (s *Store) ListJobs() ([]*Job, error) {
	rows, err := s.db.Query(
		`SELECT id, name, yaml_content, schedule, enabled, created_at, updated_at
		 FROM jobs ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*Job
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

// UpdateJob updates a job's YAML content and schedule
func (s *Store) UpdateJob(id, yamlContent, schedule string) error {
	_, err := s.db.Exec(
		`UPDATE jobs SET yaml_content = ?, schedule = ?, updated_at = ? WHERE id = ?`,
		yamlContent, schedule, time.Now(), id,
	)
	return err
}

// DeleteJob removes a job and all associated runs
func (s *Store) DeleteJob(id string) error {
	_, err := s.db.Exec(`DELETE FROM jobs WHERE id = ?`, id)
	return err
}

// SetJobEnabled enables or disables a job
func (s *Store) SetJobEnabled(id string, enabled bool) error {
	_, err := s.db.Exec(
		`UPDATE jobs SET enabled = ?, updated_at = ? WHERE id = ?`,
		boolToInt(enabled), time.Now(), id,
	)
	return err
}

// --- Run CRUD ---

// CreateRun inserts a new run record
func (s *Store) CreateRun(run *Run) error {
	_, err := s.db.Exec(
		`INSERT INTO runs (id, job_id, job_name, status, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		run.ID, run.JobID, run.JobName, run.Status, run.CreatedAt,
	)
	return err
}

// UpdateRun updates a run's status, timing, and results
func (s *Store) UpdateRun(run *Run) error {
	_, err := s.db.Exec(
		`UPDATE runs SET status = ?, started_at = ?, completed_at = ?, records = ?, error = ?
		 WHERE id = ?`,
		run.Status, run.StartedAt, run.CompletedAt, run.Records, run.Error, run.ID,
	)
	if err != nil {
		return err
	}
	// Update stats
	if run.Status == "success" {
		_, _ = s.db.Exec(
			`UPDATE stats SET total_runs = total_runs + 1, total_records = total_records + ?, updated_at = ? WHERE id = 1`,
			run.Records, time.Now(),
		)
	} else if run.Status == "failed" {
		_, _ = s.db.Exec(
			`UPDATE stats SET total_runs = total_runs + 1, total_errors = total_errors + 1, updated_at = ? WHERE id = 1`,
			time.Now(),
		)
	}
	return nil
}

// GetRun retrieves a run by ID
func (s *Store) GetRun(id string) (*Run, error) {
	row := s.db.QueryRow(
		`SELECT id, job_id, job_name, status, started_at, completed_at, records, error, created_at
		 FROM runs WHERE id = ?`, id,
	)
	return scanRun(row)
}

// ListRuns returns runs, optionally filtered by job ID
func (s *Store) ListRuns(jobID string, limit int) ([]*Run, error) {
	var rows *sql.Rows
	var err error

	if limit <= 0 {
		limit = 50
	}

	if jobID != "" {
		rows, err = s.db.Query(
			`SELECT id, job_id, job_name, status, started_at, completed_at, records, error, created_at
			 FROM runs WHERE job_id = ? ORDER BY created_at DESC LIMIT ?`,
			jobID, limit,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT id, job_id, job_name, status, started_at, completed_at, records, error, created_at
			 FROM runs ORDER BY created_at DESC LIMIT ?`,
			limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []*Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

// GetStats returns platform-wide statistics
func (s *Store) GetStats() (*Stats, error) {
	row := s.db.QueryRow(
		`SELECT total_runs, total_records, total_errors FROM stats WHERE id = 1`,
	)
	var st Stats
	err := row.Scan(&st.TotalRuns, &st.TotalRecords, &st.TotalErrors)
	if err != nil {
		return nil, err
	}
	return &st, nil
}

// --- Helpers ---

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanJob(s scanner) (*Job, error) {
	var j Job
	var enabled int
	err := s.Scan(&j.ID, &j.Name, &j.YAMLContent, &j.Schedule, &enabled, &j.CreatedAt, &j.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scanning job: %w", err)
	}
	j.Enabled = enabled == 1
	return &j, nil
}

func scanRun(s scanner) (*Run, error) {
	var r Run
	err := s.Scan(
		&r.ID, &r.JobID, &r.JobName, &r.Status,
		&r.StartedAt, &r.CompletedAt, &r.Records, &r.Error, &r.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scanning run: %w", err)
	}
	return &r, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

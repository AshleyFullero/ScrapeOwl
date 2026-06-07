package store

// migrations holds all SQL migration statements in order
var migrations = []string{
	`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,

	`CREATE TABLE IF NOT EXISTS jobs (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL UNIQUE,
		yaml_content TEXT NOT NULL,
		schedule TEXT,
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,

	`CREATE TABLE IF NOT EXISTS runs (
		id TEXT PRIMARY KEY,
		job_id TEXT NOT NULL,
		job_name TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending',
		started_at DATETIME,
		completed_at DATETIME,
		records INTEGER NOT NULL DEFAULT 0,
		error TEXT,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE
	)`,

	`CREATE INDEX IF NOT EXISTS idx_runs_job_id ON runs(job_id)`,
	`CREATE INDEX IF NOT EXISTS idx_runs_status ON runs(status)`,
	`CREATE INDEX IF NOT EXISTS idx_runs_created_at ON runs(created_at DESC)`,

	`CREATE TABLE IF NOT EXISTS extracted_data (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		run_id TEXT NOT NULL,
		field_name TEXT NOT NULL,
		field_value TEXT,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE CASCADE
	)`,

	`CREATE INDEX IF NOT EXISTS idx_extracted_run_id ON extracted_data(run_id)`,

	`CREATE TABLE IF NOT EXISTS stats (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		total_runs INTEGER NOT NULL DEFAULT 0,
		total_records INTEGER NOT NULL DEFAULT 0,
		total_errors INTEGER NOT NULL DEFAULT 0,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,

	`INSERT OR IGNORE INTO stats (id) VALUES (1)`,
}

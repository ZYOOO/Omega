package omegalocal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSQLiteMigrationsAreExecutableAndIdempotent(t *testing.T) {
	repo := NewSQLiteRepository(filepath.Join(t.TempDir(), "omega.db"))
	if err := repo.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	first, err := repo.Migrations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != len(sqliteMigrations) {
		t.Fatalf("migration count = %d, want %d: %+v", len(first), len(sqliteMigrations), first)
	}

	if err := repo.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	second, err := repo.Migrations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != len(first) {
		t.Fatalf("migration count changed after second initialize: first=%d second=%d", len(first), len(second))
	}
	if text(second[len(second)-1], "version") != sqliteMigrations[len(sqliteMigrations)-1].Version {
		t.Fatalf("last migration = %+v", second[len(second)-1])
	}
}

func TestSQLiteMigrationFailureIsNotRecorded(t *testing.T) {
	original := sqliteMigrations
	defer func() { sqliteMigrations = original }()
	sqliteMigrations = []sqliteMigration{{
		Version: "99999999_001",
		Name:    "intentional_failure",
		Up: func(context.Context, *SQLiteRepository) error {
			return errors.New("boom")
		},
	}}

	repo := NewSQLiteRepository(filepath.Join(t.TempDir(), "omega.db"))
	err := repo.Initialize(context.Background())
	if err == nil || !strings.Contains(err.Error(), "intentional_failure") {
		t.Fatalf("Initialize error = %v", err)
	}
	rows, queryErr := repo.query(context.Background(), "SELECT version FROM omega_migrations WHERE version = '99999999_001';")
	if queryErr != nil {
		t.Fatal(queryErr)
	}
	if strings.TrimSpace(rows) != "" {
		t.Fatalf("failed migration should not be recorded: %q", rows)
	}
}

func TestSQLiteRuntimeLogQueryExtensionMigratesOldTable(t *testing.T) {
	dir := t.TempDir()
	repo := NewSQLiteRepository(filepath.Join(dir, "omega.db"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := repo.exec(context.Background(), `
CREATE TABLE omega_migrations (version TEXT PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL);
INSERT INTO omega_migrations VALUES ('20260424_001', 'bootstrap_go_local_service_schema', '2026-04-24T00:00:00Z');
INSERT INTO omega_migrations VALUES ('20260427_001', 'agent_profiles_first_class_table', '2026-04-27T00:00:00Z');
INSERT INTO omega_migrations VALUES ('20260428_001', 'page_pilot_runs_first_class_table', '2026-04-28T00:00:00Z');
INSERT INTO omega_migrations VALUES ('20260429_001', 'runtime_logs_append_only_table', '2026-04-29T00:00:00Z');
CREATE TABLE runtime_logs (
  id TEXT PRIMARY KEY,
  level TEXT NOT NULL,
  event_type TEXT NOT NULL,
  message TEXT NOT NULL,
  entity_type TEXT,
  entity_id TEXT,
  project_id TEXT,
  repository_target_id TEXT,
  work_item_id TEXT,
  pipeline_id TEXT,
  attempt_id TEXT,
  stage_id TEXT,
  agent_id TEXT,
  request_id TEXT,
  details_json TEXT NOT NULL,
  created_at TEXT NOT NULL
);
`); err != nil {
		t.Fatal(err)
	}

	if err := repo.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	columns, err := repo.query(context.Background(), `.mode json
PRAGMA table_info(runtime_logs);
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(columns, `"name":"requirement_id"`) {
		t.Fatalf("runtime_logs requirement_id column missing: %s", columns)
	}
	migrations, err := repo.Migrations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, migration := range migrations {
		if text(migration, "version") == "20260501_001" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("runtime log extension migration not recorded: %+v", migrations)
	}
}

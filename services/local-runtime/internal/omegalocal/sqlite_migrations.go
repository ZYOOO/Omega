package omegalocal

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type sqliteMigration struct {
	Version string
	Name    string
	Up      func(context.Context, *SQLiteRepository) error
}

var sqliteMigrations = []sqliteMigration{
	{Version: "20260424_001", Name: "bootstrap_go_local_service_schema", Up: func(ctx context.Context, repo *SQLiteRepository) error {
		return repo.initializeBaselineSchema(ctx)
	}},
	{Version: "20260427_001", Name: "agent_profiles_first_class_table", Up: func(ctx context.Context, repo *SQLiteRepository) error {
		return repo.initializeBaselineSchema(ctx)
	}},
	{Version: "20260428_001", Name: "page_pilot_runs_first_class_table", Up: func(ctx context.Context, repo *SQLiteRepository) error {
		return repo.initializeBaselineSchema(ctx)
	}},
	{Version: "20260429_001", Name: "runtime_logs_append_only_table", Up: func(ctx context.Context, repo *SQLiteRepository) error {
		return repo.initializeBaselineSchema(ctx)
	}},
	{Version: "20260501_001", Name: "runtime_logs_query_extensions", Up: func(ctx context.Context, repo *SQLiteRepository) error {
		return repo.ensureRuntimeLogQueryExtensions(ctx)
	}},
	{Version: "20260502_001", Name: "workflow_templates_first_class_table", Up: func(ctx context.Context, repo *SQLiteRepository) error {
		return repo.initializeBaselineSchema(ctx)
	}},
	{Version: "20260502_002", Name: "repository_audit_tables", Up: func(ctx context.Context, repo *SQLiteRepository) error {
		return repo.initializeBaselineSchema(ctx)
	}},
}

func (repo *SQLiteRepository) applyPendingSQLiteMigrations(ctx context.Context) error {
	applied, err := repo.appliedSQLiteMigrationVersions(ctx)
	if err != nil {
		return err
	}
	for _, migration := range sqliteMigrations {
		if applied[migration.Version] {
			continue
		}
		if migration.Up == nil {
			return fmt.Errorf("sqlite migration %s has no executable Up step", migration.Version)
		}
		if err := migration.Up(ctx, repo); err != nil {
			return fmt.Errorf("apply sqlite migration %s %s: %w", migration.Version, migration.Name, err)
		}
		if err := repo.recordSQLiteMigration(ctx, migration); err != nil {
			return err
		}
		applied[migration.Version] = true
	}
	return nil
}

func (repo *SQLiteRepository) appliedSQLiteMigrationVersions(ctx context.Context) (map[string]bool, error) {
	output, err := repo.query(ctx, `.mode json
SELECT version FROM omega_migrations ORDER BY version;
`)
	if err != nil {
		return nil, err
	}
	applied := map[string]bool{}
	if strings.TrimSpace(output) == "" {
		return applied, nil
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(output), &rows); err != nil {
		return nil, err
	}
	for _, row := range rows {
		if version := text(row, "version"); version != "" {
			applied[version] = true
		}
	}
	return applied, nil
}

func (repo *SQLiteRepository) recordSQLiteMigration(ctx context.Context, migration sqliteMigration) error {
	return repo.exec(ctx, fmt.Sprintf(
		"INSERT OR REPLACE INTO omega_migrations (version, name, applied_at) VALUES (%s, %s, %s);",
		sqlQuote(migration.Version),
		sqlQuote(migration.Name),
		sqlQuote(nowISO()),
	))
}

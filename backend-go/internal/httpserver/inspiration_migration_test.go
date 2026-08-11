package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestMigration108BackfillsTemplateAndVersionDefinitions(t *testing.T) {
	migrationPath := filepath.Join("..", "..", "..", "database", "migrations", "108-inspiration-template-definition-expand.sql")
	migrationSQL, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read migration 108: %v", err)
	}

	databaseURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if databaseURL == "" {
		databaseURL = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL/DATABASE_URL is not configured")
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	schema := fmt.Sprintf("inspiration_migration_108_%d", time.Now().UTC().UnixNano())
	if _, err = conn.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), `DROP SCHEMA `+schema+` CASCADE`) }()
	if _, err = conn.ExecContext(ctx, `SET search_path TO `+schema); err != nil {
		t.Fatal(err)
	}

	legacyDDL := `
		CREATE TABLE inspiration_templates (
			id varchar(64) PRIMARY KEY,
			tenant_id varchar(64) NOT NULL DEFAULT 'default',
			content_type varchar(20) NOT NULL,
			prompt text NOT NULL,
			negative_prompt text NOT NULL DEFAULT '',
			model_id varchar(120) NOT NULL DEFAULT '',
			scenario_code varchar(64) NOT NULL DEFAULT '',
			display_config_json jsonb NOT NULL DEFAULT '{}'::jsonb,
			input_requirements_json jsonb NOT NULL DEFAULT '{}'::jsonb,
			preset_config_json jsonb NOT NULL DEFAULT '{}'::jsonb,
			parameters_json jsonb NOT NULL DEFAULT '{}'::jsonb,
			reference_assets_json jsonb NOT NULL DEFAULT '[]'::jsonb,
			status varchar(20) NOT NULL DEFAULT 'DRAFT',
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now(),
			deleted_at timestamptz,
			CONSTRAINT inspiration_templates_content_type_check CHECK (content_type IN ('image', 'video', 'ppt'))
		);
		CREATE TABLE inspiration_template_versions (
			id varchar(64) PRIMARY KEY,
			snapshot_json jsonb NOT NULL
		);`
	if _, err = conn.ExecContext(ctx, legacyDDL); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.ExecContext(ctx, `
		INSERT INTO inspiration_templates(
			id,content_type,prompt,negative_prompt,model_id,scenario_code,
			display_config_json,input_requirements_json,preset_config_json,
			parameters_json,reference_assets_json
		) VALUES(
			'template-1','image','Create a polished product image','watermark','gpt-image-2','product_image',
			'{"layout":"comparison"}',
			'{"referenceImageRequired":true,"referenceImageMin":1,"referenceImageMax":2}',
			'{"style":"minimal"}',
			'{"ratio":"1:1"}',
			'[{"assetId":"asset-example"}]'
		)`); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.ExecContext(ctx, `
		INSERT INTO inspiration_template_versions(id,snapshot_json) VALUES(
			'version-1',
			'{
				"contentType":"video",
				"prompt":"Animate the product",
				"negativePrompt":"flicker",
				"modelId":"seedance-fast-2.0",
				"scenarioCode":"product_video",
				"displayConfig":{"layout":"single"},
				"inputRequirements":{},
				"presetConfig":{"motion":"smooth"},
				"parameters":{"duration":5},
				"referenceAssets":[]
			}'::jsonb
		)`); err != nil {
		t.Fatal(err)
	}

	for run := 1; run <= 2; run++ {
		if _, err = conn.ExecContext(ctx, string(migrationSQL)); err != nil {
			t.Fatalf("migration run %d: %v", run, err)
		}
	}

	var raw []byte
	if err = conn.QueryRowContext(ctx, `SELECT definition_json FROM inspiration_templates WHERE id='template-1'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var definition InternalTemplateDefinition
	if err = json.Unmarshal(raw, &definition); err != nil {
		t.Fatal(err)
	}
	if definition.SchemaVersion != 1 || definition.Prompt.Template != "Create a polished product image" || definition.Prompt.NegativeTemplate != "watermark" {
		t.Fatalf("template definition prompt backfill = %#v", definition.Prompt)
	}
	if definition.Capability.CapabilityKey != "image_generation" || definition.Capability.ModelHint != "gpt-image-2" {
		t.Fatalf("template capability backfill = %#v", definition.Capability)
	}
	if definition.Handoff.TargetType != "IMAGE_CREATION" || definition.Handoff.IntentKey != "product_image" {
		t.Fatalf("template handoff backfill = %#v", definition.Handoff)
	}
	if len(definition.Inputs) != 1 || definition.Inputs[0].Key != "referenceImages" || definition.Inputs[0].Type != TemplateInputImage {
		t.Fatalf("template inputs backfill = %#v", definition.Inputs)
	}
	if definition.Presets.GenerationDefaults["ratio"] != "1:1" || definition.Presets.InputDefaults["style"] != "minimal" {
		t.Fatalf("template presets backfill = %#v", definition.Presets)
	}

	if err = conn.QueryRowContext(ctx, `SELECT snapshot_json->'definition' FROM inspiration_template_versions WHERE id='version-1'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &definition); err != nil {
		t.Fatal(err)
	}
	if definition.SchemaVersion != 1 || definition.Capability.CapabilityKey != "video_generation" || definition.Prompt.Template != "Animate the product" {
		t.Fatalf("version definition backfill = %#v", definition)
	}

	var invalid int
	if err = conn.QueryRowContext(ctx, `
		SELECT count(*) FROM inspiration_templates
		WHERE definition_json IS NULL
		   OR jsonb_typeof(definition_json) <> 'object'
		   OR definition_json->>'schemaVersion' <> '1'
	`).Scan(&invalid); err != nil {
		t.Fatal(err)
	}
	if invalid != 0 {
		t.Fatalf("invalid backfilled template definitions = %d", invalid)
	}
}

func TestMigration108FailureRollsBackPSQLStyle(t *testing.T) {
	migrationPath := filepath.Join("..", "..", "..", "database", "migrations", "108-inspiration-template-definition-expand.sql")
	migrationSQL, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read migration 108: %v", err)
	}

	databaseURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if databaseURL == "" {
		databaseURL = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL/DATABASE_URL is not configured")
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	schema := fmt.Sprintf("inspiration_migration_108_rollback_%d", time.Now().UTC().UnixNano())
	if _, err = conn.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), `DROP SCHEMA `+schema+` CASCADE`) }()
	if _, err = conn.ExecContext(ctx, `SET search_path TO `+schema); err != nil {
		t.Fatal(err)
	}

	legacyDDL := `
		CREATE TABLE inspiration_templates (
			id varchar(64) PRIMARY KEY,
			tenant_id varchar(64) NOT NULL DEFAULT 'default',
			content_type varchar(20) NOT NULL,
			slug varchar(160),
			prompt text NOT NULL,
			negative_prompt text NOT NULL DEFAULT '',
			model_id varchar(120) NOT NULL DEFAULT '',
			scenario_code varchar(64) NOT NULL DEFAULT '',
			display_config_json jsonb NOT NULL DEFAULT '{}'::jsonb,
			input_requirements_json jsonb NOT NULL DEFAULT '{}'::jsonb,
			preset_config_json jsonb NOT NULL DEFAULT '{}'::jsonb,
			parameters_json jsonb NOT NULL DEFAULT '{}'::jsonb,
			reference_assets_json jsonb NOT NULL DEFAULT '[]'::jsonb,
			status varchar(20) NOT NULL DEFAULT 'DRAFT',
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now(),
			deleted_at timestamptz,
			CONSTRAINT inspiration_templates_content_type_check CHECK (content_type IN ('image', 'video', 'ppt'))
		);
		CREATE TABLE inspiration_template_versions (
			id varchar(64) PRIMARY KEY,
			snapshot_json jsonb NOT NULL
		);
		INSERT INTO inspiration_templates(id,tenant_id,content_type,slug,prompt,status) VALUES
			('duplicate-a','tenant-a','image','duplicate','First','PUBLISHED'),
			('duplicate-b','tenant-a','image','duplicate','Second','PUBLISHED');
		INSERT INTO inspiration_template_versions(id,snapshot_json) VALUES
			('version-rollback','{"contentType":"image","prompt":"Snapshot"}'::jsonb);`
	if _, err = conn.ExecContext(ctx, legacyDDL); err != nil {
		t.Fatal(err)
	}

	before := migration108DatabaseState(t, ctx, conn)
	statements, err := splitMigration108Statements(string(migrationSQL))
	if err != nil {
		t.Fatal(err)
	}
	var migrationErr error
	for _, statement := range statements {
		if _, migrationErr = conn.ExecContext(ctx, statement); migrationErr != nil {
			break
		}
	}
	if migrationErr == nil {
		t.Fatal("migration 108 unexpectedly accepted duplicate tenant slug")
	}
	_, _ = conn.ExecContext(ctx, `ROLLBACK`)

	after := migration108DatabaseState(t, ctx, conn)
	if before != after {
		t.Fatalf("migration 108 left a partial state after failure\nbefore: %+v\nafter:  %+v\nfailure: %v", before, after, migrationErr)
	}
}

type migration108State struct {
	Columns     string
	Constraints string
	Indexes     string
	Templates   string
	Versions    string
}

func migration108DatabaseState(t *testing.T, ctx context.Context, conn *sql.Conn) migration108State {
	t.Helper()
	state := migration108State{}
	queries := []struct {
		target *string
		query  string
	}{
		{&state.Columns, `
			SELECT coalesce(jsonb_agg(to_jsonb(item) ORDER BY table_name, ordinal_position), '[]'::jsonb)::text
			FROM (
				SELECT table_name, ordinal_position, column_name, data_type, is_nullable, column_default
				FROM information_schema.columns
				WHERE table_schema=current_schema()
				  AND table_name IN ('inspiration_templates','inspiration_template_versions')
			) item`},
		{&state.Constraints, `
			SELECT coalesce(jsonb_agg(to_jsonb(item) ORDER BY table_name, constraint_name), '[]'::jsonb)::text
			FROM (
				SELECT relation.relname AS table_name, constraint_record.conname AS constraint_name,
				       constraint_record.contype AS constraint_type, pg_get_constraintdef(constraint_record.oid) AS definition
				FROM pg_constraint constraint_record
				JOIN pg_class relation ON relation.oid=constraint_record.conrelid
				JOIN pg_namespace namespace ON namespace.oid=relation.relnamespace
				WHERE namespace.nspname=current_schema()
				  AND relation.relname IN ('inspiration_templates','inspiration_template_versions')
			) item`},
		{&state.Indexes, `
			SELECT coalesce(jsonb_agg(to_jsonb(item) ORDER BY tablename, indexname), '[]'::jsonb)::text
			FROM (
				SELECT tablename, indexname, indexdef
				FROM pg_indexes
				WHERE schemaname=current_schema()
				  AND tablename IN ('inspiration_templates','inspiration_template_versions')
			) item`},
		{&state.Templates, `SELECT coalesce(jsonb_agg(to_jsonb(item) ORDER BY id), '[]'::jsonb)::text FROM inspiration_templates item`},
		{&state.Versions, `SELECT coalesce(jsonb_agg(to_jsonb(item) ORDER BY id), '[]'::jsonb)::text FROM inspiration_template_versions item`},
	}
	for _, query := range queries {
		if err := conn.QueryRowContext(ctx, query.query).Scan(query.target); err != nil {
			t.Fatal(err)
		}
	}
	return state
}

func splitMigration108Statements(source string) ([]string, error) {
	statements := make([]string, 0)
	start := 0
	inSingleQuote := false
	inDoubleQuote := false
	inLineComment := false
	inDollarQuote := false
	for index := 0; index < len(source); index++ {
		current := source[index]
		if inLineComment {
			if current == '\n' {
				inLineComment = false
			}
			continue
		}
		if inDollarQuote {
			if current == '$' && index+1 < len(source) && source[index+1] == '$' {
				inDollarQuote = false
				index++
			}
			continue
		}
		if inSingleQuote {
			if current == '\'' {
				if index+1 < len(source) && source[index+1] == '\'' {
					index++
				} else {
					inSingleQuote = false
				}
			}
			continue
		}
		if inDoubleQuote {
			if current == '"' {
				if index+1 < len(source) && source[index+1] == '"' {
					index++
				} else {
					inDoubleQuote = false
				}
			}
			continue
		}
		if current == '-' && index+1 < len(source) && source[index+1] == '-' {
			inLineComment = true
			index++
			continue
		}
		if current == '$' && index+1 < len(source) && source[index+1] == '$' {
			inDollarQuote = true
			index++
			continue
		}
		if current == '\'' {
			inSingleQuote = true
			continue
		}
		if current == '"' {
			inDoubleQuote = true
			continue
		}
		if current == ';' {
			if statement := strings.TrimSpace(source[start : index+1]); statement != "" {
				statements = append(statements, statement)
			}
			start = index + 1
		}
	}
	if inSingleQuote || inDoubleQuote || inDollarQuote {
		return nil, fmt.Errorf("migration 108 has unterminated SQL quoting")
	}
	if statement := strings.TrimSpace(source[start:]); statement != "" {
		statements = append(statements, statement)
	}
	return statements, nil
}

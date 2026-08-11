package httpserver

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestInspirationRepositoryDefinitionRoundTripAndHistoricalVersion(t *testing.T) {
	repo := newMemoryInspirationRepository()
	definitionV1 := testInspirationDefinition("Create {{subject}}", "avoid {{subject}}")
	item, err := repo.SaveTemplate(context.Background(), inspirationTemplate{
		ID: "template-definition-round-trip", Slug: "definition-round-trip", TenantID: "default",
		Title: "Definition round trip", Description: "repository contract", ContentType: "image",
		CategoryID: "inspiration-category-product", CoverURL: "https://example.test/cover.webp",
		Definition: definitionV1, Platforms: []string{"miniprogram"}, Status: "PUBLISHED",
		AuditStatus: "APPROVED", SourceAuthorized: true, CreatedBy: "admin", UpdatedBy: "admin",
	}, "create")
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := repo.GetTemplateBySlug(context.Background(), "default", "", item.Slug, false)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Definition.Prompt.Template != definitionV1.Prompt.Template || loaded.Definition.Capability.ModelHint != definitionV1.Capability.ModelHint {
		t.Fatalf("definition round trip = %#v", loaded.Definition)
	}
	duplicate := item
	duplicate.ID = "template-definition-duplicate"
	if _, err = repo.SaveTemplate(context.Background(), duplicate, "duplicate slug"); err == nil {
		t.Fatal("repository accepted a duplicate slug in the same tenant")
	}
	tenantScoped := duplicate
	tenantScoped.ID = "template-definition-tenant-scoped"
	tenantScoped.TenantID = "tenant-a"
	if _, err = repo.SaveTemplate(context.Background(), tenantScoped, "same slug in another tenant"); err != nil {
		t.Fatalf("repository rejected the same slug in another tenant: %v", err)
	}

	item.Definition.Prompt.Template = "Updated {{subject}}"
	item.UpdatedBy = "editor"
	updated, err := repo.SaveTemplate(context.Background(), item, "update definition")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 {
		t.Fatalf("updated version = %d, want 2", updated.Version)
	}

	historical, err := repo.GetTemplateVersionBySlug(context.Background(), "default", "", item.Slug, 1)
	if err != nil {
		t.Fatal(err)
	}
	if historical.Version != 1 || historical.Definition.Prompt.Template != definitionV1.Prompt.Template {
		t.Fatalf("historical definition = version %d %#v", historical.Version, historical.Definition)
	}
}

func TestPostgresInspirationRepositoryPersistsOnlyDefinitionJSON(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	schema := fmt.Sprintf("inspiration_repository_5b_%d", time.Now().UTC().UnixNano())
	if _, err = db.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = db.ExecContext(context.Background(), `DROP SCHEMA `+schema+` CASCADE`) }()
	if _, err = db.ExecContext(ctx, `SET search_path TO `+schema); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, inspirationRepositoryTestDDL); err != nil {
		t.Fatal(err)
	}

	repo := postgresInspirationRepository{db: db}
	definition := testInspirationDefinition("Create {{subject}}", "avoid {{subject}}")
	item, err := repo.SaveTemplate(ctx, inspirationTemplate{
		ID: "postgres-definition", Slug: "postgres-definition", TenantID: "default", Title: "Postgres",
		Description: "round trip", ContentType: "image", CategoryID: "category", CoverURL: "cover",
		Definition: definition, Platforms: []string{"miniprogram"}, Tags: []string{}, ApplicableTenantIDs: []string{},
		Status: "PUBLISHED", AuditStatus: "APPROVED", SourceAuthorized: true, CreatedBy: "admin", UpdatedBy: "admin",
	}, "create")
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := repo.GetTemplateBySlug(ctx, "default", "", item.Slug, false)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Definition.Prompt.Template != definition.Prompt.Template {
		t.Fatalf("postgres definition = %#v", loaded.Definition)
	}
	var legacyPrompt, legacyModel, legacyScenario string
	if err = db.QueryRowContext(ctx, `SELECT prompt,model_id,scenario_code FROM inspiration_templates WHERE id=$1`, item.ID).Scan(&legacyPrompt, &legacyModel, &legacyScenario); err != nil {
		t.Fatal(err)
	}
	if legacyPrompt != "" || legacyModel != "" || legacyScenario != "" {
		t.Fatalf("repository dual-wrote legacy dynamic fields: prompt=%q model=%q scenario=%q", legacyPrompt, legacyModel, legacyScenario)
	}

	item.Definition.Prompt.Template = "Updated {{subject}}"
	item.UpdatedBy = "editor"
	if _, err = repo.SaveTemplate(ctx, item, "update"); err != nil {
		t.Fatal(err)
	}
	historical, err := repo.GetTemplateVersionBySlug(ctx, "default", "", item.Slug, 1)
	if err != nil {
		t.Fatal(err)
	}
	if historical.Definition.Prompt.Template != definition.Prompt.Template {
		t.Fatalf("historical snapshot definition = %#v", historical.Definition)
	}
}

const inspirationRepositoryTestDDL = `
CREATE TABLE inspiration_categories (
  id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, code varchar(64) NOT NULL,
  name varchar(80) NOT NULL, sort_order integer NOT NULL DEFAULT 0, status varchar(20) NOT NULL DEFAULT 'ACTIVE',
  deleted_at timestamptz
);
INSERT INTO inspiration_categories(id,tenant_id,code,name) VALUES('category','default','category','Category');
CREATE TABLE inspiration_templates (
  id varchar(64) PRIMARY KEY, slug varchar(160) NOT NULL, tenant_id varchar(64) NOT NULL DEFAULT 'default',
  title varchar(160) NOT NULL, description text NOT NULL DEFAULT '', content_type varchar(20) NOT NULL,
  category_id varchar(64) NOT NULL REFERENCES inspiration_categories(id), cover_url text NOT NULL,
  thumbnail_url text NOT NULL DEFAULT '', result_url text NOT NULL DEFAULT '', prompt text NOT NULL,
  negative_prompt text NOT NULL DEFAULT '', model_id varchar(120) NOT NULL DEFAULT '', scenario_code varchar(64) NOT NULL DEFAULT '',
  display_config_json jsonb NOT NULL DEFAULT '{}'::jsonb, input_requirements_json jsonb NOT NULL DEFAULT '{}'::jsonb,
  preset_config_json jsonb NOT NULL DEFAULT '{}'::jsonb, parameters_json jsonb NOT NULL DEFAULT '{}'::jsonb,
  reference_assets_json jsonb NOT NULL DEFAULT '[]'::jsonb, definition_json jsonb NOT NULL,
  platforms_json jsonb NOT NULL DEFAULT '["miniprogram"]'::jsonb, tags_json jsonb NOT NULL DEFAULT '[]'::jsonb,
  applicable_tenant_ids_json jsonb NOT NULL DEFAULT '[]'::jsonb, featured boolean NOT NULL DEFAULT false,
  hot boolean NOT NULL DEFAULT false, pinned boolean NOT NULL DEFAULT false, sort_order integer NOT NULL DEFAULT 0,
  status varchar(20) NOT NULL DEFAULT 'DRAFT', audit_status varchar(20) NOT NULL DEFAULT 'PENDING', audit_note text NOT NULL DEFAULT '',
  start_time timestamptz, end_time timestamptz, version integer NOT NULL DEFAULT 1, source_asset_id varchar(64),
  source_authorized boolean NOT NULL DEFAULT false, created_by varchar(64) NOT NULL DEFAULT '', updated_by varchar(64) NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz
);
CREATE UNIQUE INDEX ux_inspiration_templates_tenant_slug ON inspiration_templates(tenant_id,slug) WHERE deleted_at IS NULL;
CREATE TABLE inspiration_favorites (
  id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, template_id varchar(64) NOT NULL REFERENCES inspiration_templates(id),
  user_id varchar(64) NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(template_id,user_id)
);
CREATE TABLE inspiration_events (
  id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, template_id varchar(64) NOT NULL REFERENCES inspiration_templates(id),
  user_id varchar(64), event_type varchar(32) NOT NULL, generation_task_id varchar(64), platform varchar(32) NOT NULL DEFAULT 'miniprogram',
  request_id varchar(120) NOT NULL DEFAULT '', metadata_json jsonb NOT NULL DEFAULT '{}'::jsonb, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE inspiration_template_versions (
  id varchar(64) PRIMARY KEY, template_id varchar(64) NOT NULL REFERENCES inspiration_templates(id), tenant_id varchar(64) NOT NULL,
  version integer NOT NULL, snapshot_json jsonb NOT NULL, change_note text NOT NULL DEFAULT '', created_by varchar(64) NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(template_id,version)
);`

func testInspirationDefinition(basePrompt, negativePrompt string) InternalTemplateDefinition {
	return InternalTemplateDefinition{
		SchemaVersion: 1,
		Inputs:        []TemplateInputDefinition{{Key: "subject", Type: TemplateInputText, Label: "Subject", Required: true}},
		Prompt: TemplatePromptDefinition{
			Template: basePrompt, NegativeTemplate: negativePrompt,
			Composer: TemplateComposerDefinition{Key: "deterministic-template", Version: 1},
		},
		Bindings:     []TemplateBindingDefinition{{Source: "inputs.subject", Target: "prompt.variables.subject", Transform: TemplateTransformTrim}},
		Presets:      TemplatePresetsDefinition{InputDefaults: map[string]any{}, GenerationDefaults: map[string]any{"ratio": "1:1"}},
		Presentation: map[string]any{"layout": "single"},
		Handoff:      TemplateHandoffDefinition{TargetType: "IMAGE_CREATION", TargetKey: "image.create", IntentKey: "optional-hint"},
		Capability:   TemplateCapabilityDefinition{CapabilityKey: "image_generation", ModelHint: "gpt-image-2"},
	}
}

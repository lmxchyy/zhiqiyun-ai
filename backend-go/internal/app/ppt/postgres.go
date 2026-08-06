package ppt

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"xianzhi-ai/backend-go/internal/app/ppt/skills"
)

const postgresOperationTimeout = 5 * time.Second

const postgresIdempotencyScopeCreateSession = "create-session"

var errPPTPostgresReadOnly = errors.New("ppt postgres read-only operation")

const postgresReadinessTableSQL = `
select count(*) > 0
from pg_catalog.pg_class c
join pg_catalog.pg_namespace n on n.oid = c.relnamespace
where n.nspname = 'public' and c.relname = 'xz_ppt_tasks' and c.relkind in ('r', 'p')
`

const postgresReadinessColumnsSQL = `
select a.attname, a.attnotnull, format_type(a.atttypid, a.atttypmod),
       coalesce(pg_get_expr(d.adbin, d.adrelid), '')
from pg_catalog.pg_attribute a
join pg_catalog.pg_class c on c.oid = a.attrelid
join pg_catalog.pg_namespace n on n.oid = c.relnamespace
left join pg_catalog.pg_attrdef d on d.adrelid = a.attrelid and d.adnum = a.attnum
where n.nspname = 'public' and c.relname = 'xz_ppt_tasks'
  and a.attnum > 0 and not a.attisdropped
`

const postgresReadinessIndexesSQL = `
select indexrel.relname, i.indisvalid, i.indisready, i.indisunique,
       coalesce(
         jsonb_agg(
           jsonb_build_object(
             'column', att.attname,
             'descending', (i.indoption[keys.ordinality - 1] & 1) <> 0
           ) order by keys.ordinality
         ) filter (where keys.ordinality <= i.indnkeyatts),
         '[]'::jsonb
       ),
       coalesce(pg_get_expr(i.indpred, i.indrelid), '')
from pg_catalog.pg_index i
join pg_catalog.pg_class tablerel on tablerel.oid = i.indrelid
join pg_catalog.pg_namespace n on n.oid = tablerel.relnamespace
join pg_catalog.pg_class indexrel on indexrel.oid = i.indexrelid
left join lateral unnest(i.indkey::smallint[]) with ordinality as keys(attnum, ordinality) on true
left join pg_catalog.pg_attribute att on att.attrelid = i.indrelid and att.attnum = keys.attnum
where n.nspname = 'public' and tablerel.relname = 'xz_ppt_tasks'
group by indexrel.relname, i.indisvalid, i.indisready, i.indisunique, i.indoption, i.indnkeyatts, i.indpred, i.indrelid
`

type postgresSchemaColumn struct {
	notNull     bool
	typeName    string
	defaultExpr string
}

type postgresExpectedColumn struct {
	typeName        string
	notNull         bool
	defaultExpr     string
	requiresDefault bool
}

type postgresSchemaIndexKey struct {
	Column     string `json:"column"`
	Descending bool   `json:"descending"`
}

type postgresSchemaIndex struct {
	valid     bool
	ready     bool
	unique    bool
	keys      []postgresSchemaIndexKey
	predicate string
}

func (s *Service) ensurePostgresReady(ctx context.Context) error {
	if s.db == nil {
		return fmt.Errorf("%w: database", ErrPostgresUnavailable)
	}
	s.postgresReadyMu.Lock()
	defer s.postgresReadyMu.Unlock()
	if s.postgresReady {
		return nil
	}
	var tablePresent bool
	if err := s.db.QueryRowContext(ctx, postgresReadinessTableSQL).Scan(&tablePresent); err != nil {
		return postgresSchemaUnavailable("table", err)
	}
	if !tablePresent {
		return postgresSchemaUnavailable("table", nil)
	}
	columns, err := readPostgresSchemaColumns(ctx, s.db)
	if err != nil {
		return postgresSchemaUnavailable("columns", err)
	}
	for name, expected := range postgresReadinessColumns() {
		if !postgresColumnMatches(columns[name], expected) {
			return postgresSchemaUnavailable(name, nil)
		}
	}
	indexes, err := readPostgresSchemaIndexes(ctx, s.db)
	if err != nil {
		return postgresSchemaUnavailable("indexes", err)
	}
	if !postgresIndexMatches(indexes["idx_xz_ppt_tasks_tenant_user_client_request"], []postgresSchemaIndexKey{{Column: "tenant_id"}, {Column: "user_id"}, {Column: "client_request_id"}}, "client_request_id <> ''", true) {
		return postgresSchemaUnavailable("idx_xz_ppt_tasks_tenant_user_client_request", nil)
	}
	if !postgresIndexMatches(indexes["idx_xz_ppt_tasks_tenant_user_session"], []postgresSchemaIndexKey{{Column: "tenant_id"}, {Column: "user_id"}, {Column: "session_id"}}, "session_id is not null", false) {
		return postgresSchemaUnavailable("idx_xz_ppt_tasks_tenant_user_session", nil)
	}
	if !postgresIndexMatches(indexes["idx_xz_ppt_tasks_tenant_user_stage_updated"], []postgresSchemaIndexKey{{Column: "tenant_id"}, {Column: "user_id"}, {Column: "stage"}, {Column: "updated_at", Descending: true}}, "", false) {
		return postgresSchemaUnavailable("idx_xz_ppt_tasks_tenant_user_stage_updated", nil)
	}
	s.postgresReady = true
	return nil
}

func readPostgresSchemaColumns(ctx context.Context, db *sql.DB) (map[string]postgresSchemaColumn, error) {
	rows, err := db.QueryContext(ctx, postgresReadinessColumnsSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := make(map[string]postgresSchemaColumn)
	for rows.Next() {
		var name, typeName, defaultExpr string
		var notNull bool
		if err := rows.Scan(&name, &notNull, &typeName, &defaultExpr); err != nil {
			return nil, err
		}
		columns[strings.TrimSpace(name)] = postgresSchemaColumn{notNull: notNull, typeName: typeName, defaultExpr: defaultExpr}
	}
	return columns, rows.Err()
}

func postgresReadinessColumns() map[string]postgresExpectedColumn {
	return map[string]postgresExpectedColumn{
		"task_id":           {typeName: "character varying(128)", notNull: true},
		"user_id":           {typeName: "character varying(128)", notNull: true},
		"tenant_id":         {typeName: "character varying(128)", notNull: true},
		"client_request_id": {typeName: "character varying(256)", notNull: true, requiresDefault: true, defaultExpr: "''"},
		"status":            {typeName: "character varying(32)", notNull: true},
		"created_at":        {typeName: "timestamp with time zone", notNull: true},
		"updated_at":        {typeName: "timestamp with time zone", notNull: true},
		"raw":               {typeName: "jsonb", notNull: true},
		"session_id":        {typeName: "character varying(128)", notNull: false},
		"skill_code":        {typeName: "character varying(64)", notNull: true, requiresDefault: true, defaultExpr: "''"},
		"stage":             {typeName: "character varying(32)", notNull: true, requiresDefault: true, defaultExpr: "'draft'"},
		"source_file_ids":   {typeName: "jsonb", notNull: true, requiresDefault: true, defaultExpr: "'[]'"},
	}
}

func postgresColumnMatches(column postgresSchemaColumn, expected postgresExpectedColumn) bool {
	if normalizePostgresCatalogType(column.typeName) != normalizePostgresCatalogType(expected.typeName) || column.notNull != expected.notNull {
		return false
	}
	return !expected.requiresDefault || postgresDefaultMatches(column.defaultExpr, expected.defaultExpr)
}

func readPostgresSchemaIndexes(ctx context.Context, db *sql.DB) (map[string]postgresSchemaIndex, error) {
	rows, err := db.QueryContext(ctx, postgresReadinessIndexesSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	indexes := make(map[string]postgresSchemaIndex)
	for rows.Next() {
		var name, predicate string
		var valid, ready, unique bool
		var rawKeys []byte
		if err := rows.Scan(&name, &valid, &ready, &unique, &rawKeys, &predicate); err != nil {
			return nil, err
		}
		var keys []postgresSchemaIndexKey
		if err := json.Unmarshal(rawKeys, &keys); err != nil {
			return nil, err
		}
		indexes[strings.TrimSpace(name)] = postgresSchemaIndex{valid: valid, ready: ready, unique: unique, keys: keys, predicate: predicate}
	}
	return indexes, rows.Err()
}

func postgresSchemaUnavailable(component string, cause error) error {
	if cause != nil {
		return fmt.Errorf("%w: ppt schema %s: %w", ErrPostgresUnavailable, component, cause)
	}
	return fmt.Errorf("%w: ppt schema %s", ErrPostgresUnavailable, component)
}

func normalizePostgresCatalogType(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

func postgresDefaultMatches(expression, want string) bool {
	return normalizePostgresCatalogDefault(expression) == normalizePostgresCatalogDefault(want)
}

func normalizePostgresCatalogDefault(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	for strings.HasPrefix(value, "(") && strings.HasSuffix(value, ")") {
		value = strings.TrimSpace(value[1 : len(value)-1])
	}
	if castAt := strings.Index(value, "::"); castAt >= 0 {
		value = strings.TrimSpace(value[:castAt])
	}
	return strings.Join(strings.Fields(value), "")
}

func postgresIndexMatches(index postgresSchemaIndex, expectedKeys []postgresSchemaIndexKey, expectedPredicate string, requireUnique bool) bool {
	if !index.valid || !index.ready || requireUnique && !index.unique || len(index.keys) != len(expectedKeys) || normalizePostgresCatalogPredicate(index.predicate) != normalizePostgresCatalogPredicate(expectedPredicate) {
		return false
	}
	for position, expected := range expectedKeys {
		actual := index.keys[position]
		if strings.TrimSpace(actual.Column) != expected.Column || actual.Descending != expected.Descending {
			return false
		}
	}
	return true
}

func normalizePostgresCatalogPredicate(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	for strings.HasPrefix(value, "(") && strings.HasSuffix(value, ")") {
		value = strings.TrimSpace(value[1 : len(value)-1])
	}
	value = strings.ReplaceAll(value, "\"", "")
	return strings.Join(strings.Fields(value), "")
}

func (s *Service) generatePostgres(req GenerateRequest, externalActive, limit int) (GenerateResponse, error) {
	owner, err := req.Owner.Validated()
	if err != nil {
		return GenerateResponse{}, err
	}
	req.UserID, req.TenantID = owner.UserID, owner.TenantID
	actualSlides := req.SlideCount
	if req.Outline != nil {
		actualSlides = len(req.Outline.Slides)
	}
	skill, ok := skills.Resolve("general")
	if !ok || actualSlides <= 0 || actualSlides > skill.MaxSlides {
		return GenerateResponse{}, ErrInvalidSkill
	}
	ctx, cancel := context.WithTimeout(context.Background(), postgresOperationTimeout)
	defer cancel()
	if err := s.ensurePostgresReady(ctx); err != nil {
		return GenerateResponse{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return GenerateResponse{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `select pg_advisory_xact_lock(hashtext($1))`, "ppt:owner:"+owner.TenantID+":"+owner.UserID); err != nil {
		return GenerateResponse{}, err
	}
	if req.ClientRequestID != "" {
		var existingID, existingStatus string
		err := tx.QueryRowContext(ctx, `select task_id,status from xz_ppt_tasks where tenant_id=$1 and user_id=$2 and client_request_id=$3`, owner.TenantID, owner.UserID, req.ClientRequestID).Scan(&existingID, &existingStatus)
		if err == nil {
			return GenerateResponse{TaskID: existingID, Status: existingStatus}, tx.Commit()
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return GenerateResponse{}, err
		}
	}
	if limit > 0 {
		active, err := countActivePostgresTasks(ctx, tx, owner)
		if err != nil {
			return GenerateResponse{}, err
		}
		active += externalActive
		if active >= limit {
			return GenerateResponse{}, fmt.Errorf("%w: active %d, limit %d", ErrConcurrency, active, limit)
		}
	}
	task := NormalizeTask(taskFromGenerateRequest(req))
	if err := persistPostgresTask(ctx, tx, task); err != nil {
		return GenerateResponse{}, err
	}
	if err := tx.Commit(); err != nil {
		return GenerateResponse{}, err
	}
	return GenerateResponse{TaskID: task.TaskID, Status: task.Status}, nil
}

func countActivePostgresTasks(ctx context.Context, tx *sql.Tx, owner OwnerScope) (int, error) {
	rows, err := tx.QueryContext(ctx, `select stage,status from xz_ppt_tasks where tenant_id=$1 and user_id=$2`, owner.TenantID, owner.UserID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	active := 0
	for rows.Next() {
		var stageValue, status string
		if err := rows.Scan(&stageValue, &status); err != nil {
			return 0, err
		}
		stage := Stage(strings.TrimSpace(stageValue))
		if StageStatus(stage) == "" || status != StageStatus(stage) {
			return 0, ErrInvalidStage
		}
		if stage == StageDraft || stage == StageOutlineReady || stage == StageGenerating {
			active++
		}
	}
	return active, rows.Err()
}

func taskFromGenerateRequest(req GenerateRequest) Task {
	now := time.Now().UTC()
	stage := StageDraft
	if req.Outline != nil && len(req.Outline.Slides) > 0 {
		stage = StageReady
	}
	return Task{
		TaskID: fmt.Sprintf("ppt_%d", now.UnixNano()), UserID: req.UserID, TenantID: req.TenantID,
		OrganizationID: req.OrganizationID, ContextType: req.ContextType, BillingScope: req.BillingScope, BillingAccountID: req.BillingAccountID,
		ClientRequestID: req.ClientRequestID, Type: "ppt", MediaType: "ppt", SkillCode: "general", Stage: stage,
		Status: StageStatus(stage), Title: titleFromPrompt(req.Prompt), Prompt: req.Prompt, SlideCount: req.SlideCount,
		Language: req.Language, Tone: req.Tone, TextContent: req.TextContent, Audience: req.Audience, Scenario: req.Scenario,
		GenerationAspectRatio: req.GenerationAspectRatio, Theme: req.Theme, AutoThemeEnabled: req.AutoThemeEnabled,
		EnableWebSearch: req.EnableWebSearch, ImageSource: req.ImageSource, TextModel: req.TextModel, ImageModel: req.ImageModel,
		ImageStyle: req.ImageStyle, PeopleStyle: req.PeopleStyle, ImageLighting: req.ImageLighting,
		ImageComposition: req.ImageComposition, TextInImage: false, Outline: req.Outline, Slides: slidesFromOutline(req.Outline, req),
		CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano),
	}
}

type postgresSessionRequestIdentity struct {
	UserID           string   `json:"userId"`
	TenantID         string   `json:"tenantId"`
	OrganizationID   string   `json:"organizationId"`
	ContextType      string   `json:"contextType"`
	BillingScope     string   `json:"billingScope"`
	BillingAccountID string   `json:"billingAccountId"`
	ClientRequestID  string   `json:"clientRequestId"`
	Prompt           string   `json:"prompt"`
	SkillCode        string   `json:"skillCode"`
	SourceFileIDs    []string `json:"sourceFileIds"`
	SlideCount       int      `json:"slideCount"`
	Language         string   `json:"language"`
	Audience         string   `json:"audience"`
	DeckSpec         DeckSpec `json:"deckSpec"`
}

type postgresSessionQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func normalizePostgresSessionRequest(req SessionRequest) SessionRequest {
	req.Owner.TenantID = strings.TrimSpace(req.Owner.TenantID)
	req.Owner.UserID = strings.TrimSpace(req.Owner.UserID)
	req.OrganizationID = strings.TrimSpace(req.OrganizationID)
	req.ContextType = strings.ToUpper(strings.TrimSpace(req.ContextType))
	req.BillingScope = strings.ToUpper(strings.TrimSpace(req.BillingScope))
	req.BillingAccountID = strings.TrimSpace(req.BillingAccountID)
	req.ClientRequestID = strings.TrimSpace(req.ClientRequestID)
	req.Prompt = strings.TrimSpace(req.Prompt)
	req.SkillCode = strings.ToLower(strings.TrimSpace(req.SkillCode))
	req.SourceFileIDs = normalizePPTSourceFileIDs(req.SourceFileIDs)
	req.Language = strings.ToLower(strings.TrimSpace(req.Language))
	req.Audience = strings.TrimSpace(req.Audience)
	req.DeckSpec = normalizeDeckSpec(req.DeckSpec)
	return req
}

func postgresSessionIdentityFromRequest(req SessionRequest) postgresSessionRequestIdentity {
	req = normalizePostgresSessionRequest(req)
	return postgresSessionRequestIdentity{
		UserID: req.Owner.UserID, TenantID: req.Owner.TenantID, OrganizationID: req.OrganizationID, ContextType: req.ContextType,
		BillingScope: req.BillingScope, BillingAccountID: req.BillingAccountID, ClientRequestID: req.ClientRequestID,
		Prompt: req.Prompt, SkillCode: req.SkillCode, SourceFileIDs: append([]string(nil), req.SourceFileIDs...),
		SlideCount: req.SlideCount, Language: req.Language, Audience: req.Audience, DeckSpec: req.DeckSpec,
	}
}

func postgresSessionIdentityFromTask(task Task) postgresSessionRequestIdentity {
	return postgresSessionIdentityFromRequest(SessionRequest{
		Owner: OwnerScope{UserID: task.UserID, TenantID: task.TenantID}, OrganizationID: task.OrganizationID, ContextType: task.ContextType,
		BillingScope: task.BillingScope, BillingAccountID: task.BillingAccountID, ClientRequestID: task.ClientRequestID,
		Prompt: task.Prompt, SkillCode: task.SkillCode, SourceFileIDs: task.SourceFileIDs,
		SlideCount: task.SlideCount, Language: task.Language, Audience: task.Audience, DeckSpec: deckSpecFromTask(task),
	})
}

func postgresSessionIdentityHash(identity postgresSessionRequestIdentity) string {
	raw, _ := json.Marshal(identity)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func postgresSessionRequestHash(req SessionRequest) string {
	return postgresSessionIdentityHash(postgresSessionIdentityFromRequest(req))
}

func postgresSessionTaskRequestHash(task Task) string {
	for _, record := range task.IdempotencyRecords {
		if strings.EqualFold(strings.TrimSpace(record.Scope), postgresIdempotencyScopeCreateSession) && record.Key == strings.TrimSpace(task.ClientRequestID) && strings.TrimSpace(record.RequestHash) != "" {
			return strings.TrimSpace(record.RequestHash)
		}
	}
	return postgresSessionIdentityHash(postgresSessionIdentityFromTask(task))
}

func readPostgresSessionByClientRequest(ctx context.Context, queryer postgresSessionQueryer, owner OwnerScope, clientRequestID string) (Task, error) {
	projection, err := scanPostgresTaskProjection(queryer.QueryRowContext(
		ctx, postgresTaskProjectionSQL+` where tenant_id=$1 and user_id=$2 and client_request_id=$3`, owner.TenantID, owner.UserID, strings.TrimSpace(clientRequestID),
	))
	if err != nil {
		return Task{}, err
	}
	return taskFromPostgresProjection(projection)
}

func validatePostgresSessionReplay(req SessionRequest, existing Task) (Task, error) {
	if postgresSessionRequestHash(req) != postgresSessionTaskRequestHash(existing) {
		return Task{}, ErrIdempotencyConflict
	}
	return existing, nil
}

func (s *Service) recoverPostgresSessionCreate(ctx context.Context, req SessionRequest, createErr error) (Task, error) {
	if req.ClientRequestID == "" {
		return Task{}, createErr
	}
	existing, err := readPostgresSessionByClientRequest(ctx, s.db, req.Owner, req.ClientRequestID)
	if err == nil {
		return validatePostgresSessionReplay(req, existing)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Task{}, fmt.Errorf("%w (session replay recovery failed: %v)", createErr, err)
	}
	return Task{}, createErr
}

func (s *Service) CreateSession(ctx context.Context, req SessionRequest) (Task, error) {
	if s.db == nil {
		return Task{}, ErrPostgresUnavailable
	}
	req = normalizePostgresSessionRequest(req)
	owner, err := req.Owner.Validated()
	if err != nil {
		return Task{}, err
	}
	req.Owner = owner
	if req.Prompt == "" {
		return Task{}, ErrInvalidPrompt
	}
	skill, ok := skills.Resolve(req.SkillCode)
	if !ok || req.SlideCount <= 0 || req.SlideCount > skill.MaxSlides {
		return Task{}, ErrInvalidSkill
	}
	ctx, cancel := context.WithTimeout(ctx, postgresOperationTimeout)
	defer cancel()
	if err := s.ensurePostgresReady(ctx); err != nil {
		return Task{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return Task{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if req.ClientRequestID != "" {
		lockHash := sha256.Sum256([]byte(req.Owner.TenantID + "\x00" + req.Owner.UserID + "\x00" + req.ClientRequestID))
		if _, err := tx.ExecContext(ctx, `select pg_advisory_xact_lock(hashtext($1))`, "ppt:session:"+hex.EncodeToString(lockHash[:])); err != nil {
			return Task{}, err
		}
		existing, err := readPostgresSessionByClientRequest(ctx, tx, req.Owner, req.ClientRequestID)
		if err == nil {
			existing, err = validatePostgresSessionReplay(req, existing)
			if err != nil {
				return Task{}, err
			}
			if err := tx.Commit(); err != nil {
				return s.recoverPostgresSessionCreate(ctx, req, err)
			}
			return existing, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return Task{}, err
		}
	}
	taskID, err := securePPTToken("ppt")
	if err != nil {
		return Task{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := NormalizeTask(Task{
		TaskID: taskID, SessionID: taskID, UserID: req.Owner.UserID, ClientRequestID: req.ClientRequestID,
		TenantID: req.Owner.TenantID, OrganizationID: req.OrganizationID, ContextType: req.ContextType,
		BillingScope: req.BillingScope, BillingAccountID: req.BillingAccountID,
		Type: "ppt", MediaType: "ppt", SkillCode: req.SkillCode, Stage: StageDraft, Status: StatusPending,
		Title: titleFromPrompt(req.Prompt), Prompt: req.Prompt, SlideCount: req.SlideCount,
		Language: req.Language, Audience: req.Audience, SourceFileIDs: append([]string(nil), req.SourceFileIDs...),
		Tone: req.DeckSpec.Tone, TextContent: req.DeckSpec.TextContent, Scenario: req.DeckSpec.Scenario,
		GenerationAspectRatio: req.DeckSpec.GenerationAspectRatio, Theme: req.DeckSpec.Theme,
		AutoThemeEnabled: req.DeckSpec.AutoThemeEnabled, EnableWebSearch: req.DeckSpec.EnableWebSearch,
		ImageSource: req.DeckSpec.ImageSource, TextModel: req.DeckSpec.TextModel, ImageModel: req.DeckSpec.ImageModel,
		ImageStyle: req.DeckSpec.ImageStyle, PeopleStyle: req.DeckSpec.PeopleStyle,
		ImageLighting: req.DeckSpec.ImageLighting, ImageComposition: req.DeckSpec.ImageComposition,
		TextInImage: req.DeckSpec.TextInImage,
		CreatedAt:   now, UpdatedAt: now,
	})
	if req.ClientRequestID != "" {
		task.IdempotencyRecords = []IdempotencyRecord{{
			Scope: postgresIdempotencyScopeCreateSession, Key: req.ClientRequestID, RequestHash: postgresSessionRequestHash(req),
			State: idempotencyStateCompleted, CreatedAt: now, UpdatedAt: now,
		}}
	}
	if err := persistPostgresTask(ctx, tx, task); err != nil {
		_ = tx.Rollback()
		return s.recoverPostgresSessionCreate(ctx, req, err)
	}
	if err := tx.Commit(); err != nil {
		return s.recoverPostgresSessionCreate(ctx, req, err)
	}
	return task, nil
}

func (s *Service) BeginOperation(ctx context.Context, owner OwnerScope, taskID, scope, key, requestHash string) (OperationClaim, Task, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return OperationClaim{}, Task{}, ErrIdempotencyKeyRequired
	}
	scope = strings.ToLower(strings.TrimSpace(scope))
	requestHash = strings.TrimSpace(requestHash)
	var claim OperationClaim
	var replayTask Task
	var inFlight bool
	task, err := s.updatePostgresTaskContext(ctx, owner, taskID, func(task *Task, _ time.Time) error {
		if index := findIdempotencyRecord(task.IdempotencyRecords, scope, key); index >= 0 {
			record := &task.IdempotencyRecords[index]
			if record.RequestHash != requestHash {
				return ErrIdempotencyConflict
			}
			if record.State == idempotencyStateCompleted {
				claim = operationClaimFromRecord(*record, true)
				replayTask = idempotencyResponseTask(*record, *task)
				return errPPTPostgresReadOnly
			}
			if record.State == idempotencyStateProcessing && !operationRecordIsStale(*record, time.Now().UTC()) {
				claim = operationClaimFromRecord(*record, false)
				claim.InFlight = true
				inFlight = true
				return errPPTPostgresReadOnly
			}
			if hasLiveCancelClaim(*task) {
				return ErrSessionCancelled
			}
			if err := requireOperationStage(*task, scope); err != nil {
				return err
			}
			token, err := securePPTToken("op")
			if err != nil {
				return err
			}
			now := time.Now().UTC().Format(time.RFC3339Nano)
			record.State = idempotencyStateProcessing
			record.OperationToken = token
			record.ResponseJSON = ""
			record.UpdatedAt = now
			task.ErrorCode = ""
			claim = operationClaimFromRecord(*record, false)
			return nil
		}
		if hasLiveCancelClaim(*task) {
			return ErrSessionCancelled
		}
		if err := requireOperationStage(*task, scope); err != nil {
			return err
		}
		token, err := securePPTToken("op")
		if err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		record := IdempotencyRecord{
			Scope: scope, Key: key, RequestHash: requestHash, State: idempotencyStateProcessing,
			OperationToken: token, CreatedAt: now, UpdatedAt: now,
		}
		task.IdempotencyRecords = append(task.IdempotencyRecords, record)
		pruneIdempotencyRecords(task)
		claim = operationClaimFromRecord(record, false)
		return nil
	})
	if err != nil {
		return OperationClaim{}, Task{}, err
	}
	if replayTask.TaskID != "" {
		return claim, replayTask, nil
	}
	if inFlight {
		return claim, task, ErrOperationInProgress
	}
	return claim, task, nil
}

func (s *Service) CompleteOutlineOperation(ctx context.Context, owner OwnerScope, taskID string, claim OperationClaim, messages []AgentMessage, outline Outline) (Task, error) {
	return s.completeOutlineOperation(ctx, owner, taskID, claim, messages, outline, nil, false)
}

func (s *Service) CompleteImportOutlineOperation(ctx context.Context, owner OwnerScope, taskID string, claim OperationClaim, messages []AgentMessage, outline Outline, sourceFileIDs []string) (Task, error) {
	return s.completeOutlineOperation(ctx, owner, taskID, claim, messages, outline, sourceFileIDs, true)
}

func (s *Service) completeOutlineOperation(ctx context.Context, owner OwnerScope, taskID string, claim OperationClaim, messages []AgentMessage, outline Outline, sourceFileIDs []string, replaceSourceFiles bool) (Task, error) {
	return s.updatePostgresTaskContext(ctx, owner, taskID, func(task *Task, mutationTime time.Time) error {
		index, err := validateOperationClaim(*task, claim)
		if err != nil {
			return err
		}
		if task.IdempotencyRecords[index].State == idempotencyStateCompleted {
			return errPPTPostgresReadOnly
		}
		if err := requireOutlineOperationStage(*task); err != nil {
			return err
		}
		skill, ok := skills.Resolve(strings.TrimSpace(task.SkillCode))
		if !ok || len(outline.Slides) == 0 || len(outline.Slides) > skill.MaxSlides {
			return ErrInvalidSkill
		}
		outlineCopy := outline
		outlineCopy.Slides = append([]OutlineSlide(nil), outline.Slides...)
		for slideIndex := range outlineCopy.Slides {
			outlineCopy.Slides[slideIndex].BulletPoints = append([]string(nil), outlineCopy.Slides[slideIndex].BulletPoints...)
		}
		task.Outline = &outlineCopy
		if len(outlineCopy.Slides) > 0 {
			task.SlideCount = len(outlineCopy.Slides)
		}
		task.AgentMessages = append(task.AgentMessages, messages...)
		if len(task.AgentMessages) > maxAgentMessages {
			task.AgentMessages = append([]AgentMessage(nil), task.AgentMessages[len(task.AgentMessages)-maxAgentMessages:]...)
		}
		if replaceSourceFiles {
			task.SourceFileIDs = normalizePPTSourceFileIDs(sourceFileIDs)
		}
		task.Stage = StageOutlineReady
		task.Status = StageStatus(task.Stage)
		task.ErrorCode = ""
		completeIdempotencyRecord(task, index, mutationTime)
		return nil
	})
}

// CompleteSlideRevision commits a claimed READY-only text revision under the
// same row lock as the idempotency record. Provider-owned coordinates and
// media references are ignored so a text revision cannot reorder pages or
// silently replace an existing visual asset.
func (s *Service) CompleteSlideRevision(ctx context.Context, owner OwnerScope, taskID string, claim OperationClaim, slideID string, revision Slide) (Task, error) {
	return s.updatePostgresTaskContext(ctx, owner, taskID, func(task *Task, mutationTime time.Time) error {
		index, err := validateOperationClaim(*task, claim)
		if err != nil {
			return err
		}
		if task.IdempotencyRecords[index].State == idempotencyStateCompleted {
			return errPPTPostgresReadOnly
		}
		if err := requireOperationStage(*task, "revise-slide"); err != nil {
			return err
		}

		slideID = strings.TrimSpace(slideID)
		target := -1
		for slideIndex := range task.Slides {
			if strings.TrimSpace(task.Slides[slideIndex].ID) == slideID {
				target = slideIndex
				break
			}
		}
		if target < 0 {
			return ErrTaskNotFound
		}
		if len(revision.Blocks) == 0 {
			return ErrInvalidSlideIR
		}

		original := NormalizeSlideIR(task.Slides[target])
		revision = NormalizeSlideIR(revision)
		blocks := make([]SlideBlock, 0, len(revision.Blocks)+2)
		for _, block := range revision.Blocks {
			if block.Type != "image" && block.Type != "note" {
				blocks = append(blocks, block)
			}
		}
		for _, block := range original.Blocks {
			if block.Type == "image" || block.Type == "note" {
				blocks = append(blocks, block)
			}
		}
		if err := applySlideContentUpdate(task, slideID, Slide{Blocks: blocks}); err != nil {
			return err
		}
		task.Slides[target].ID = original.ID
		task.Slides[target].Page = original.Page
		task.Slides[target].Layout = original.Layout
		task.Stage = StageReady
		task.Status = StageStatus(task.Stage)
		task.ErrorCode = ""
		completeIdempotencyRecord(task, index, mutationTime)
		return nil
	})
}

func (s *Service) FailOperation(ctx context.Context, owner OwnerScope, taskID string, claim OperationClaim, errorCode string) (Task, error) {
	return s.updatePostgresTaskContext(ctx, owner, taskID, func(task *Task, _ time.Time) error {
		index, err := validateOperationClaim(*task, claim)
		if err != nil {
			return err
		}
		if task.IdempotencyRecords[index].State == idempotencyStateCompleted {
			return errPPTPostgresReadOnly
		}
		task.ErrorCode = strings.TrimSpace(errorCode)
		failIdempotencyRecord(task, index, time.Now().UTC(), task.ErrorCode)
		return nil
	})
}

// BeginGenerationClaim durably owns the reserve-before-bind interval without
// changing the public task stage. A matching replay may resume reservation and
// binding; a different confirm or cancellation cannot terminally overtake it.
func (s *Service) BeginGenerationClaim(ctx context.Context, owner OwnerScope, taskID, key, requestHash string) (OperationClaim, Task, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return OperationClaim{}, Task{}, ErrIdempotencyKeyRequired
	}
	requestHash = strings.TrimSpace(requestHash)
	var claim OperationClaim
	var replayTask Task
	task, err := s.updatePostgresTaskContext(ctx, owner, taskID, func(task *Task, _ time.Time) error {
		if index := findIdempotencyRecord(task.IdempotencyRecords, idempotencyScopeConfirm, key); index >= 0 {
			record := &task.IdempotencyRecords[index]
			if record.RequestHash != requestHash {
				return ErrIdempotencyConflict
			}
			switch record.State {
			case idempotencyStateCompleted:
				if task.Stage != StageReady {
					return fmt.Errorf("%w: completed confirm in %s", ErrInvalidStage, task.Stage)
				}
				claim = operationClaimFromRecord(*record, true)
				replayTask = idempotencyResponseTask(*record, *task)
				return errPPTPostgresReadOnly
			case idempotencyStateProcessing:
				if task.Stage == StageCancelled || hasLiveCancelClaim(*task) {
					return ErrSessionCancelled
				}
				if task.Stage != StageOutlineReady && task.Stage != StageGenerating {
					return fmt.Errorf("%w: processing confirm in %s", ErrInvalidStage, task.Stage)
				}
				claim = operationClaimFromRecord(*record, false)
				claim.Replay = true
				record.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
				return nil
			}
			if task.Stage != StageOutlineReady {
				return stageTransitionError(*task)
			}
			if hasLiveCancelClaim(*task) {
				return ErrSessionCancelled
			}
			token, err := securePPTToken("confirm")
			if err != nil {
				return err
			}
			now := time.Now().UTC().Format(time.RFC3339Nano)
			record.State = idempotencyStateProcessing
			record.OperationToken = token
			record.ResponseJSON = ""
			record.ErrorCode = ""
			record.UpdatedAt = now
			task.ErrorCode = ""
			claim = operationClaimFromRecord(*record, false)
			return nil
		}
		if task.Stage == StageCancelled || hasLiveCancelClaim(*task) {
			return ErrSessionCancelled
		}
		if task.Stage != StageOutlineReady {
			return fmt.Errorf("%w: cannot confirm from %s", ErrInvalidStage, task.Stage)
		}
		if task.Outline == nil || len(task.Outline.Slides) == 0 {
			return ErrOutlineRequired
		}
		if hasLiveConfirmClaim(*task) {
			return ErrOperationInProgress
		}
		token, err := securePPTToken("confirm")
		if err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		record := IdempotencyRecord{
			Scope: idempotencyScopeConfirm, Key: key, RequestHash: requestHash, State: idempotencyStateProcessing,
			OperationToken: token, CreatedAt: now, UpdatedAt: now,
		}
		task.IdempotencyRecords = append(task.IdempotencyRecords, record)
		pruneIdempotencyRecords(task)
		claim = operationClaimFromRecord(record, false)
		return nil
	})
	if err != nil {
		return OperationClaim{}, Task{}, err
	}
	if replayTask.TaskID != "" {
		return claim, replayTask, nil
	}
	return claim, task, nil
}

// FailGenerationClaim is safe only before a billing task is bound. It makes a
// definite reservation failure retryable and allows a later cancellation.
func (s *Service) FailGenerationClaim(ctx context.Context, owner OwnerScope, taskID string, claim OperationClaim, errorCode string) (Task, error) {
	return s.updatePostgresTaskContext(ctx, owner, taskID, func(task *Task, _ time.Time) error {
		index, err := validateOperationClaim(*task, claim)
		if err != nil {
			return err
		}
		if task.IdempotencyRecords[index].Scope != idempotencyScopeConfirm {
			return ErrOperationTokenMismatch
		}
		if task.IdempotencyRecords[index].State == idempotencyStateCompleted {
			return errPPTPostgresReadOnly
		}
		if task.Stage != StageOutlineReady || strings.TrimSpace(task.BillingTaskID) != "" {
			return fmt.Errorf("%w: cannot fail reservation claim in %s", ErrInvalidStage, task.Stage)
		}
		task.ErrorCode = strings.TrimSpace(errorCode)
		failIdempotencyRecord(task, index, time.Now().UTC(), task.ErrorCode)
		return nil
	})
}

// BeginCancelAfterStaleGenerationClaim transfers a stale bound confirm into a
// retryable cancel claim. Billing must already have been attached by the
// claim-fenced BindGenerationBilling path; recovery never accepts a caller-
// supplied billing identifier.
func (s *Service) BeginCancelAfterStaleGenerationClaim(ctx context.Context, owner OwnerScope, taskID string, generationClaim OperationClaim, key, requestHash string, now time.Time) (CancelClaim, Task, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return CancelClaim{}, Task{}, ErrIdempotencyKeyRequired
	}
	requestHash = strings.TrimSpace(requestHash)
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	var cancelClaim CancelClaim
	task, err := s.updatePostgresTaskContext(ctx, owner, taskID, func(task *Task, _ time.Time) error {
		confirmIndex, err := validateOperationClaim(*task, generationClaim)
		if err != nil {
			return err
		}
		confirmRecord := task.IdempotencyRecords[confirmIndex]
		if confirmRecord.Scope != idempotencyScopeConfirm || confirmRecord.State != idempotencyStateProcessing {
			return ErrOperationTokenMismatch
		}
		if strings.TrimSpace(task.BillingTaskID) == "" {
			return ErrBillingBindingMissing
		}
		if task.Stage != StageGenerating {
			return fmt.Errorf("%w: cannot transfer confirm claim in %s", ErrInvalidStage, task.Stage)
		}
		if !operationRecordIsStale(confirmRecord, now) {
			return ErrOperationInProgress
		}
		if hasLiveCancelClaim(*task) {
			return ErrGenerationAlreadyRunning
		}
		if index := findIdempotencyRecord(task.IdempotencyRecords, idempotencyScopeCancel, key); index >= 0 {
			record := task.IdempotencyRecords[index]
			if record.RequestHash != requestHash {
				return ErrIdempotencyConflict
			}
			cancelClaim = cancelClaimFromRecord(record, true)
			return errPPTPostgresReadOnly
		}
		failIdempotencyRecord(task, confirmIndex, now, ErrSessionCancelled.Error())
		task.ErrorCode = ""
		token, err := securePPTToken("cancel")
		if err != nil {
			return err
		}
		timestamp := now.Format(time.RFC3339Nano)
		record := IdempotencyRecord{
			Scope: idempotencyScopeCancel, Key: key, RequestHash: requestHash, State: idempotencyStateProcessing,
			OperationToken: token, CreatedAt: timestamp, UpdatedAt: timestamp,
		}
		task.IdempotencyRecords = append(task.IdempotencyRecords, record)
		pruneIdempotencyRecords(task)
		cancelClaim = cancelClaimFromRecord(record, false)
		return nil
	})
	if err != nil {
		return CancelClaim{}, Task{}, err
	}
	return cancelClaim, task, nil
}

func (s *Service) BindGenerationBilling(ctx context.Context, owner OwnerScope, taskID string, claim OperationClaim, billingTaskID string) (Task, error) {
	billingTaskID = strings.TrimSpace(billingTaskID)
	if billingTaskID == "" {
		return Task{}, ErrBillingTaskRequired
	}
	var replayTask Task
	task, err := s.updatePostgresTaskContext(ctx, owner, taskID, func(task *Task, _ time.Time) error {
		index, err := validateOperationClaim(*task, claim)
		if err != nil {
			return err
		}
		record := &task.IdempotencyRecords[index]
		if record.State == idempotencyStateCompleted {
			if err := requireBillingBinding(*task, billingTaskID); err != nil {
				return err
			}
			if task.Stage == StageReady {
				replayTask = idempotencyResponseTask(*record, *task)
				return errPPTPostgresReadOnly
			}
			return stageTransitionError(*task)
		}
		if hasLiveCancelClaim(*task) {
			return ErrSessionCancelled
		}
		if task.Stage == StageCancelled {
			return ErrSessionCancelled
		}
		if task.Stage == StageGenerating {
			if err := requireBillingBinding(*task, billingTaskID); err != nil {
				return err
			}
			return errPPTPostgresReadOnly
		}
		if task.Stage != StageOutlineReady {
			return stageTransitionError(*task)
		}
		if task.Outline == nil || len(task.Outline.Slides) == 0 {
			return ErrOutlineRequired
		}
		if stored := strings.TrimSpace(task.BillingTaskID); stored != "" && stored != billingTaskID {
			return ErrBillingBindingMismatch
		}
		now := time.Now().UTC()
		task.Stage = StageGenerating
		task.Status = StageStatus(task.Stage)
		task.BillingTaskID = billingTaskID
		task.GenerationStartedAt = now.Format(time.RFC3339Nano)
		task.CompletedAt = ""
		task.ErrorCode = ""
		record.ErrorCode = ""
		record.UpdatedAt = now.Format(time.RFC3339Nano)
		return nil
	})
	if err != nil {
		return Task{}, err
	}
	if replayTask.TaskID != "" {
		return replayTask, nil
	}
	return task, nil
}

func (s *Service) ClaimGenerationRun(ctx context.Context, owner OwnerScope, taskID string, now time.Time) (GenerationClaim, Task, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	var claim GenerationClaim
	task, err := s.updatePostgresTaskContext(ctx, owner, taskID, func(task *Task, _ time.Time) error {
		if err := requireGenerating(*task); err != nil {
			return err
		}
		if hasLiveCancelClaim(*task) {
			return ErrSessionCancelled
		}
		if task.GenerationLease != nil {
			leaseUntil, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(task.GenerationLease.LeaseUntil))
			if err == nil && leaseUntil.After(now) {
				return ErrGenerationAlreadyRunning
			}
		}
		runToken, err := securePPTToken("run")
		if err != nil {
			return err
		}
		leaseUntil := now.Add(generationLeaseDuration)
		task.GenerationLease = &GenerationLease{RunToken: runToken, LeaseUntil: leaseUntil.Format(time.RFC3339Nano)}
		claim = GenerationClaim{RunToken: runToken, LeaseUntil: leaseUntil.Format(time.RFC3339Nano)}
		return nil
	})
	if err != nil {
		return GenerationClaim{}, Task{}, err
	}
	return claim, task, nil
}

func (s *Service) RenewGenerationRun(ctx context.Context, owner OwnerScope, taskID string, claim GenerationClaim, now time.Time, leaseDuration time.Duration) (GenerationClaim, Task, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	if leaseDuration <= 0 {
		return GenerationClaim{}, Task{}, errors.New("generation lease duration must be positive")
	}
	var renewed GenerationClaim
	task, err := s.updatePostgresTaskContext(ctx, owner, taskID, func(task *Task, _ time.Time) error {
		if err := requireGenerationClaim(*task, claim); err != nil {
			return err
		}
		if hasLiveCancelClaim(*task) {
			return ErrSessionCancelled
		}
		currentUntil, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(task.GenerationLease.LeaseUntil))
		if err != nil || !currentUntil.After(now) {
			return ErrGenerationRunMismatch
		}
		leaseUntil := now.Add(leaseDuration).UTC().Format(time.RFC3339Nano)
		task.GenerationLease = &GenerationLease{RunToken: strings.TrimSpace(claim.RunToken), LeaseUntil: leaseUntil}
		renewed = GenerationClaim{RunToken: strings.TrimSpace(claim.RunToken), LeaseUntil: leaseUntil}
		return nil
	})
	if err != nil {
		return GenerationClaim{}, Task{}, err
	}
	return renewed, task, nil
}

// AcquireGenerationCleanupFence atomically extends the canonical lease only
// when claim still owns its exact run token. Unlike ordinary renewal it allows
// a live cancel claim, because billing release must still settle before cancel.
func (s *Service) AcquireGenerationCleanupFence(ctx context.Context, owner OwnerScope, taskID string, claim GenerationClaim, now time.Time) (GenerationClaim, Task, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	var fenced GenerationClaim
	task, err := s.updatePostgresTaskContext(ctx, owner, taskID, func(task *Task, _ time.Time) error {
		if err := requireGenerationClaim(*task, claim); err != nil {
			return err
		}
		renewGenerationLease(task, claim.RunToken, now)
		fenced = GenerationClaim{
			RunToken:   task.GenerationLease.RunToken,
			LeaseUntil: task.GenerationLease.LeaseUntil,
		}
		return nil
	})
	if err != nil {
		return GenerationClaim{}, Task{}, err
	}
	return fenced, task, nil
}

func (s *Service) PersistGeneratedSlide(ctx context.Context, owner OwnerScope, taskID string, claim GenerationClaim, slide Slide) (Task, error) {
	return s.updatePostgresTaskContext(ctx, owner, taskID, func(task *Task, _ time.Time) error {
		if err := requireGenerationClaim(*task, claim); err != nil {
			return err
		}
		if hasLiveCancelClaim(*task) {
			return ErrSessionCancelled
		}
		total := expectedTextSlides(*task)
		slide.ID = strings.TrimSpace(slide.ID)
		if total <= 0 || slide.Page < 1 || slide.Page > total || slide.ID == "" {
			return fmt.Errorf("%w: id=%q page=%d total=%d", ErrInvalidSlideCoordinate, slide.ID, slide.Page, total)
		}
		if len(slide.Blocks) == 0 {
			return ErrInvalidSlideIR
		}
		for _, existing := range task.Slides {
			existingID := strings.TrimSpace(existing.ID)
			if existingID == slide.ID && existing.Page == slide.Page {
				renewGenerationLease(task, claim.RunToken, time.Now().UTC())
				return nil
			}
			if existingID == slide.ID || existing.Page == slide.Page {
				return fmt.Errorf("%w: id=%q page=%d", ErrSlideCoordinateConflict, slide.ID, slide.Page)
			}
		}
		slide = NormalizeSlideIR(slide)
		task.Slides = append(task.Slides, slide)
		for index := len(task.Slides) - 1; index > 0 && task.Slides[index].Page < task.Slides[index-1].Page; index-- {
			task.Slides[index], task.Slides[index-1] = task.Slides[index-1], task.Slides[index]
		}
		if task.SlideCount <= 0 {
			task.SlideCount = total
		}
		completed, _ := textSlidePageCoverage(task.Slides, total)
		if completed > task.CurrentPage {
			task.CurrentPage = completed
		}
		if total > 0 {
			progress := completed * 100 / total
			if progress > task.Progress {
				task.Progress = progress
			}
		}
		renewGenerationLease(task, claim.RunToken, time.Now().UTC())
		return nil
	})
}

func (s *Service) CompleteGeneration(ctx context.Context, owner OwnerScope, taskID string, claim GenerationClaim) (Task, error) {
	return s.completeGeneration(ctx, owner, taskID, claim, false)
}

// CompleteGenerationAfterCapture makes the billing terminal state authoritative.
// Unlike CompleteGeneration, it may close a concurrent live cancel claim because
// captured funds can no longer be released. The task row lock makes READY and the
// cancel-claim failure one durable state transition.
func (s *Service) CompleteGenerationAfterCapture(ctx context.Context, owner OwnerScope, taskID string, claim GenerationClaim) (Task, error) {
	return s.completeGeneration(ctx, owner, taskID, claim, true)
}

func (s *Service) completeGeneration(ctx context.Context, owner OwnerScope, taskID string, claim GenerationClaim, billingCaptured bool) (Task, error) {
	return s.updatePostgresTaskContext(ctx, owner, taskID, func(task *Task, mutationTime time.Time) error {
		if err := requireGenerationClaim(*task, claim); err != nil {
			return err
		}
		if hasLiveCancelClaim(*task) && !billingCaptured {
			return ErrSessionCancelled
		}
		total := expectedTextSlides(*task)
		completed, exact := textSlidePageCoverage(task.Slides, total)
		if total <= 0 || !exact {
			return fmt.Errorf("%w: completed %d of %d", ErrGenerationIncomplete, completed, total)
		}
		now := mutationTime
		task.Stage = StageReady
		task.Status = StageStatus(task.Stage)
		task.Progress = 100
		task.CurrentPage = total
		task.CompletedAt = now.Format(time.RFC3339Nano)
		task.GenerationLease = nil
		task.ErrorCode = ""
		if billingCaptured {
			failProcessingIdempotencyScope(task, idempotencyScopeCancel, now, ErrBillingAlreadyCaptured.Error())
		}
		completeLatestIdempotencyScope(task, idempotencyScopeConfirm, now)
		return nil
	})
}

func (s *Service) BeginCancel(ctx context.Context, owner OwnerScope, taskID, key, requestHash string) (CancelClaim, Task, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return CancelClaim{}, Task{}, ErrIdempotencyKeyRequired
	}
	requestHash = strings.TrimSpace(requestHash)
	var claim CancelClaim
	var replayTask Task
	task, err := s.updatePostgresTaskContext(ctx, owner, taskID, func(task *Task, _ time.Time) error {
		if index := findIdempotencyRecord(task.IdempotencyRecords, idempotencyScopeCancel, key); index >= 0 {
			record := task.IdempotencyRecords[index]
			if record.RequestHash != requestHash {
				return ErrIdempotencyConflict
			}
			if record.State == idempotencyStateFailed && record.ErrorCode == ErrBillingAlreadyCaptured.Error() {
				return ErrBillingAlreadyCaptured
			}
			claim = cancelClaimFromRecord(record, true)
			if record.State == idempotencyStateCompleted {
				replayTask = idempotencyResponseTask(record, *task)
			}
			return errPPTPostgresReadOnly
		}
		if task.Stage == StageCancelled {
			return ErrSessionCancelled
		}
		if task.Stage == StageOutlineReady && hasLiveConfirmClaim(*task) {
			return ErrOperationInProgress
		}
		if task.Stage != StageDraft && task.Stage != StageOutlineReady && task.Stage != StageGenerating {
			return fmt.Errorf("%w: cannot cancel from %s", ErrInvalidStage, task.Stage)
		}
		if hasLiveCancelClaim(*task) {
			return ErrGenerationAlreadyRunning
		}
		token, err := securePPTToken("cancel")
		if err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		record := IdempotencyRecord{
			Scope: idempotencyScopeCancel, Key: key, RequestHash: requestHash, State: idempotencyStateProcessing,
			OperationToken: token, CreatedAt: now, UpdatedAt: now,
		}
		task.IdempotencyRecords = append(task.IdempotencyRecords, record)
		pruneIdempotencyRecords(task)
		claim = cancelClaimFromRecord(record, false)
		return nil
	})
	if err != nil {
		return CancelClaim{}, Task{}, err
	}
	if replayTask.TaskID != "" {
		return claim, replayTask, nil
	}
	return claim, task, nil
}

func (s *Service) CompleteCancel(ctx context.Context, owner OwnerScope, taskID string, claim CancelClaim) (Task, error) {
	return s.updatePostgresTaskContext(ctx, owner, taskID, func(task *Task, mutationTime time.Time) error {
		index, err := validateCancelClaim(*task, claim)
		if err != nil {
			return err
		}
		if task.IdempotencyRecords[index].State == idempotencyStateCompleted {
			return errPPTPostgresReadOnly
		}
		if task.Stage != StageDraft && task.Stage != StageOutlineReady && task.Stage != StageGenerating {
			return stageTransitionError(*task)
		}
		now := mutationTime
		task.Stage = StageCancelled
		task.Status = StageStatus(task.Stage)
		task.CompletedAt = now.Format(time.RFC3339Nano)
		task.GenerationLease = nil
		failProcessingIdempotencyScope(task, idempotencyScopeConfirm, now, ErrSessionCancelled.Error())
		completeIdempotencyRecord(task, index, now)
		return nil
	})
}

func (s *Service) FailGenerationAfterRelease(ctx context.Context, owner OwnerScope, taskID string, claim GenerationClaim, errorCode string) (Task, error) {
	return s.updatePostgresTaskContext(ctx, owner, taskID, func(task *Task, _ time.Time) error {
		if err := requireGenerationClaim(*task, claim); err != nil {
			return err
		}
		if hasLiveCancelClaim(*task) {
			return ErrSessionCancelled
		}
		now := time.Now().UTC()
		task.Stage = StageFailed
		task.Status = StageStatus(task.Stage)
		task.ErrorCode = strings.TrimSpace(errorCode)
		task.CompletedAt = now.Format(time.RFC3339Nano)
		task.GenerationLease = nil
		failLatestIdempotencyScope(task, idempotencyScopeConfirm, now, task.ErrorCode)
		return nil
	})
}

func (s *Service) getTaskPostgres(owner OwnerScope, taskID string) (Task, error) {
	ctx, cancel := context.WithTimeout(context.Background(), postgresOperationTimeout)
	defer cancel()
	if err := s.ensurePostgresReady(ctx); err != nil {
		return Task{}, err
	}
	projection, err := scanPostgresTaskProjection(s.db.QueryRowContext(ctx, postgresTaskProjectionSQL+` where task_id=$1 and tenant_id=$2 and user_id=$3`, strings.TrimSpace(taskID), owner.TenantID, owner.UserID))
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrTaskNotFound
	}
	if err != nil {
		return Task{}, err
	}
	return taskFromPostgresProjection(projection)
}

func (s *Service) historyPostgres(owner OwnerScope) ([]Task, error) {
	ctx, cancel := context.WithTimeout(context.Background(), postgresOperationTimeout)
	defer cancel()
	if err := s.ensurePostgresReady(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, postgresTaskProjectionSQL+` where tenant_id=$1 and user_id=$2 order by created_at desc`, owner.TenantID, owner.UserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Task{}
	for rows.Next() {
		projection, err := scanPostgresTaskProjection(rows)
		if err != nil {
			return nil, err
		}
		task, err := taskFromPostgresProjection(projection)
		if err != nil {
			return nil, err
		}
		items = append(items, task)
	}
	return items, rows.Err()
}

func (s *Service) deletePostgres(owner OwnerScope, taskID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), postgresOperationTimeout)
	defer cancel()
	if err := s.ensurePostgresReady(ctx); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `delete from xz_ppt_tasks where task_id=$1 and tenant_id=$2 and user_id=$3`, strings.TrimSpace(taskID), owner.TenantID, owner.UserID)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrTaskNotFound
	}
	return nil
}

func (s *Service) updateSlideImagePostgres(owner OwnerScope, taskID, slideID, imageURL string) (Task, error) {
	return s.updatePostgresTask(owner, taskID, func(task *Task) error {
		slideID, imageURL := strings.TrimSpace(slideID), strings.TrimSpace(imageURL)
		for i := range task.Slides {
			if task.Slides[i].ID != slideID {
				continue
			}
			if old := slideImageRef(task.Slides[i]); old != "" && old != imageURL {
				task.Slides[i].VisualHistory = append(task.Slides[i].VisualHistory, VisualAsset{URL: old, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)})
			}
			task.Slides[i].VisualStatus = "success"
			task.Slides[i].VisualError = ""
			task.Slides[i] = setSlideImageRef(task.Slides[i], imageURL)
			return nil
		}
		return ErrTaskNotFound
	})
}

func (s *Service) updateSlideContentPostgres(owner OwnerScope, taskID, slideID string, update Slide) (Task, error) {
	return s.updatePostgresTask(owner, taskID, func(task *Task) error {
		return applySlideContentUpdate(task, slideID, update)
	})
}

func (s *Service) updateSlideVisualPlanPostgres(owner OwnerScope, taskID, slideID string, plan VisualPlan, visualTaskID, status, errorMessage string) (Task, error) {
	return s.updatePostgresTask(owner, taskID, func(task *Task) error {
		for i := range task.Slides {
			if task.Slides[i].ID != strings.TrimSpace(slideID) {
				continue
			}
			task.Slides[i].VisualPlan = &plan
			if visualTaskID = strings.TrimSpace(visualTaskID); visualTaskID != "" {
				task.Slides[i].VisualTaskID = visualTaskID
			}
			task.Slides[i].VisualStatus = strings.TrimSpace(status)
			task.Slides[i].VisualError = strings.TrimSpace(errorMessage)
			return nil
		}
		return ErrTaskNotFound
	})
}

func (s *Service) disableSlideVisualPostgres(owner OwnerScope, taskID, slideID string, plan VisualPlan) (Task, error) {
	return s.updatePostgresTask(owner, taskID, func(task *Task) error {
		return disableSlideVisual(task, slideID, plan)
	})
}

func (s *Service) completeSlideVisualPostgres(owner OwnerScope, taskID, slideID string, plan VisualPlan, asset VisualAsset) (Task, error) {
	return s.updatePostgresTask(owner, taskID, func(task *Task) error {
		return completeSlideVisual(task, slideID, plan, asset)
	})
}

func (s *Service) restoreSlideVisualPostgres(owner OwnerScope, taskID, slideID, createdAt, imageURL string) (Task, error) {
	return s.updatePostgresTask(owner, taskID, func(task *Task) error {
		return restoreSlideVisual(task, slideID, createdAt, imageURL)
	})
}

func (s *Service) updatePostgresTask(owner OwnerScope, taskID string, update func(*Task) error) (Task, error) {
	ctx, cancel := context.WithTimeout(context.Background(), postgresOperationTimeout)
	defer cancel()
	return s.updatePostgresTaskContext(ctx, owner, taskID, func(task *Task, _ time.Time) error {
		return update(task)
	})
}

const postgresTaskProjectionSQL = `select task_id,tenant_id,user_id,client_request_id,status,session_id,skill_code,stage,source_file_ids,raw from xz_ppt_tasks`

type postgresTaskProjection struct {
	TaskID            string
	TenantID          string
	UserID            string
	ClientRequestID   string
	Status            string
	SessionID         string
	SkillCode         string
	Stage             Stage
	SourceFileIDsJSON []byte
	Raw               []byte
}

type postgresTaskScanner interface {
	Scan(dest ...any) error
}

func scanPostgresTaskProjection(scanner postgresTaskScanner) (postgresTaskProjection, error) {
	var projection postgresTaskProjection
	var sessionID sql.NullString
	var stage string
	if err := scanner.Scan(
		&projection.TaskID,
		&projection.TenantID,
		&projection.UserID,
		&projection.ClientRequestID,
		&projection.Status,
		&sessionID,
		&projection.SkillCode,
		&stage,
		&projection.SourceFileIDsJSON,
		&projection.Raw,
	); err != nil {
		return postgresTaskProjection{}, err
	}
	if sessionID.Valid {
		projection.SessionID = sessionID.String
	}
	projection.Stage = Stage(strings.TrimSpace(stage))
	return projection, nil
}

func taskFromPostgresProjection(projection postgresTaskProjection) (Task, error) {
	var task Task
	if err := json.Unmarshal(projection.Raw, &task); err != nil {
		return Task{}, err
	}
	task.TaskID = strings.TrimSpace(projection.TaskID)
	task.TenantID = strings.TrimSpace(projection.TenantID)
	task.UserID = strings.TrimSpace(projection.UserID)
	task.ClientRequestID = strings.TrimSpace(projection.ClientRequestID)
	task.Status = strings.TrimSpace(projection.Status)
	task.SessionID = strings.TrimSpace(projection.SessionID)
	task.SkillCode = strings.TrimSpace(projection.SkillCode)
	task.Stage = projection.Stage
	task.SourceFileIDs = nil
	if len(projection.SourceFileIDsJSON) > 0 {
		if err := json.Unmarshal(projection.SourceFileIDsJSON, &task.SourceFileIDs); err != nil {
			return Task{}, err
		}
	}
	if err := ValidateTaskStage(task); err != nil {
		return Task{}, err
	}
	task = NormalizeTask(task)
	if _, err := (OwnerScope{TenantID: task.TenantID, UserID: task.UserID}).Validated(); err != nil {
		return Task{}, err
	}
	return task, nil
}

func persistPostgresTask(ctx context.Context, tx *sql.Tx, task Task) error {
	if err := ValidateTaskStage(task); err != nil {
		return err
	}
	task = NormalizeTask(task)
	if _, err := (OwnerScope{TenantID: task.TenantID, UserID: task.UserID}).Validated(); err != nil {
		return err
	}
	raw, err := marshalPostgresTask(task)
	if err != nil {
		return err
	}
	sourceFileIDs := task.SourceFileIDs
	if sourceFileIDs == nil {
		sourceFileIDs = []string{}
	}
	sourceFileIDsJSON, err := json.Marshal(sourceFileIDs)
	if err != nil {
		return err
	}
	createdAt := parseTaskTime(task.CreatedAt)
	updatedAt := parseTaskTime(task.UpdatedAt)
	_, err = tx.ExecContext(ctx, `
		insert into xz_ppt_tasks(task_id,tenant_id,user_id,client_request_id,status,session_id,skill_code,stage,source_file_ids,created_at,updated_at,raw)
		values($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11,$12::jsonb)
		on conflict(task_id) do update set
		  client_request_id=excluded.client_request_id,
	  status=excluded.status,
	  session_id=excluded.session_id,
	  skill_code=excluded.skill_code,
	  stage=excluded.stage,
	  source_file_ids=excluded.source_file_ids,
	  updated_at=excluded.updated_at,
		  raw=excluded.raw
		where xz_ppt_tasks.tenant_id=excluded.tenant_id and xz_ppt_tasks.user_id=excluded.user_id
		`, task.TaskID, task.TenantID, task.UserID, task.ClientRequestID, task.Status, task.SessionID, task.SkillCode, task.Stage, string(sourceFileIDsJSON), createdAt, updatedAt, string(raw))
	return err
}

// marshalPostgresTask keeps the durable slide document canonical. Legacy
// title/content/image fields are HTTP boundary projections and must never
// become a second persisted representation of slide content.
func marshalPostgresTask(task Task) ([]byte, error) {
	raw, err := json.Marshal(task)
	if err != nil {
		return nil, err
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, err
	}
	if slides, ok := document["slides"].([]any); ok {
		for _, item := range slides {
			slide, ok := item.(map[string]any)
			if !ok {
				continue
			}
			delete(slide, "title")
			delete(slide, "content")
			delete(slide, "bulletPoints")
			delete(slide, "imageUrl")
			delete(slide, "speakerNotes")
		}
	}
	return json.Marshal(document)
}

func (s *Service) updatePostgresTaskContext(ctx context.Context, owner OwnerScope, taskID string, update func(*Task, time.Time) error) (Task, error) {
	return s.updatePostgresTaskContextAt(ctx, owner, taskID, time.Now().UTC(), update)
}

func (s *Service) updatePostgresTaskContextAt(ctx context.Context, owner OwnerScope, taskID string, mutationTime time.Time, update func(*Task, time.Time) error) (Task, error) {
	owner, ownerErr := owner.Validated()
	if ownerErr != nil {
		return Task{}, ownerErr
	}
	if s.db == nil {
		return Task{}, ErrPostgresUnavailable
	}
	if err := s.ensurePostgresReady(ctx); err != nil {
		return Task{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return Task{}, err
	}
	defer func() { _ = tx.Rollback() }()
	projection, err := scanPostgresTaskProjection(tx.QueryRowContext(
		ctx,
		postgresTaskProjectionSQL+` where task_id=$1 and tenant_id=$2 and user_id=$3 for update`,
		strings.TrimSpace(taskID),
		owner.TenantID,
		owner.UserID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrTaskNotFound
	}
	if err != nil {
		return Task{}, err
	}
	task, err := taskFromPostgresProjection(projection)
	if err != nil {
		return Task{}, err
	}
	task = cloneTask(task)
	mutationTime = mutationTime.UTC()
	if err := update(&task, mutationTime); err != nil {
		if errors.Is(err, errPPTPostgresReadOnly) {
			return task, nil
		}
		return Task{}, err
	}
	task.UpdatedAt = mutationTime.Format(time.RFC3339Nano)
	task = NormalizeTask(task)
	if err := persistPostgresTask(ctx, tx, task); err != nil {
		return Task{}, err
	}
	if err := tx.Commit(); err != nil {
		return Task{}, err
	}
	return task, nil
}

func securePPTToken(prefix string) (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return strings.TrimSpace(prefix) + "_" + hex.EncodeToString(random), nil
}

func requireOperationStage(task Task, scope string) error {
	scope = strings.ToLower(strings.TrimSpace(scope))
	if strings.HasPrefix(scope, "message:") || strings.HasPrefix(scope, "messages:") {
		return requireOutlineOperationStage(task)
	}
	if strings.HasPrefix(scope, "revise-slide:") || strings.HasPrefix(scope, "revise_slide:") {
		if task.Stage == StageReady {
			return nil
		}
		return stageTransitionError(task)
	}
	switch scope {
	case "message", "messages", "import-outline", "import_outline":
		return requireOutlineOperationStage(task)
	case "revise-slide", "revise_slide":
		if task.Stage == StageReady {
			return nil
		}
	default:
		return fmt.Errorf("%w: unsupported operation scope %q", ErrInvalidStage, scope)
	}
	return stageTransitionError(task)
}

func requireOutlineOperationStage(task Task) error {
	if task.Stage == StageDraft || task.Stage == StageOutlineReady {
		return nil
	}
	return stageTransitionError(task)
}

func requireGenerating(task Task) error {
	if task.Stage == StageGenerating {
		return nil
	}
	return stageTransitionError(task)
}

func requireBillingBinding(task Task, billingTaskID string) error {
	stored := strings.TrimSpace(task.BillingTaskID)
	if stored == "" {
		return ErrBillingBindingMissing
	}
	if stored != strings.TrimSpace(billingTaskID) {
		return ErrBillingBindingMismatch
	}
	return nil
}

func stageTransitionError(task Task) error {
	if task.Stage == StageCancelled {
		return ErrSessionCancelled
	}
	return fmt.Errorf("%w: stage %s", ErrInvalidStage, task.Stage)
}

func operationClaimFromRecord(record IdempotencyRecord, replay bool) OperationClaim {
	return OperationClaim{
		Scope: record.Scope, Key: record.Key, RequestHash: record.RequestHash,
		OperationToken: record.OperationToken, Replay: replay, CompletedReplay: replay,
	}
}

func operationRecordIsStale(record IdempotencyRecord, now time.Time) bool {
	updatedAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(record.UpdatedAt))
	if err != nil {
		updatedAt, err = time.Parse(time.RFC3339Nano, strings.TrimSpace(record.CreatedAt))
	}
	if err != nil {
		return true
	}
	return !updatedAt.Add(operationProcessingStaleAfter).After(now)
}

func cancelClaimFromRecord(record IdempotencyRecord, replay bool) CancelClaim {
	return CancelClaim{
		Key: record.Key, RequestHash: record.RequestHash,
		OperationToken: record.OperationToken, Replay: replay,
	}
}

func findIdempotencyRecord(records []IdempotencyRecord, scope, key string) int {
	scope = strings.ToLower(strings.TrimSpace(scope))
	key = strings.TrimSpace(key)
	for index := len(records) - 1; index >= 0; index-- {
		if strings.ToLower(strings.TrimSpace(records[index].Scope)) == scope && strings.TrimSpace(records[index].Key) == key {
			return index
		}
	}
	return -1
}

func validateOperationClaim(task Task, claim OperationClaim) (int, error) {
	index := findIdempotencyRecord(task.IdempotencyRecords, claim.Scope, claim.Key)
	if index < 0 {
		return -1, ErrOperationTokenMismatch
	}
	record := task.IdempotencyRecords[index]
	if record.RequestHash != strings.TrimSpace(claim.RequestHash) {
		return -1, ErrIdempotencyConflict
	}
	if strings.TrimSpace(claim.OperationToken) == "" || record.OperationToken != strings.TrimSpace(claim.OperationToken) {
		return -1, ErrOperationTokenMismatch
	}
	if record.State != idempotencyStateProcessing && record.State != idempotencyStateCompleted {
		return -1, ErrOperationTokenMismatch
	}
	return index, nil
}

func validateCancelClaim(task Task, claim CancelClaim) (int, error) {
	index := findIdempotencyRecord(task.IdempotencyRecords, idempotencyScopeCancel, claim.Key)
	if index < 0 {
		return -1, ErrOperationTokenMismatch
	}
	record := task.IdempotencyRecords[index]
	if record.RequestHash != strings.TrimSpace(claim.RequestHash) {
		return -1, ErrIdempotencyConflict
	}
	if strings.TrimSpace(claim.OperationToken) == "" || record.OperationToken != strings.TrimSpace(claim.OperationToken) {
		return -1, ErrOperationTokenMismatch
	}
	if record.State != idempotencyStateProcessing && record.State != idempotencyStateCompleted {
		return -1, ErrOperationTokenMismatch
	}
	return index, nil
}

func pruneIdempotencyRecords(task *Task) {
	for len(task.IdempotencyRecords) > maxIdempotencyRecords {
		removeAt := -1
		for index, record := range task.IdempotencyRecords {
			if isProtectedIdempotencyRecord(record, task.Stage) {
				continue
			}
			removeAt = index
			break
		}
		if removeAt < 0 {
			return
		}
		task.IdempotencyRecords = append(task.IdempotencyRecords[:removeAt], task.IdempotencyRecords[removeAt+1:]...)
	}
}

func isProtectedIdempotencyRecord(record IdempotencyRecord, stage Stage) bool {
	if strings.EqualFold(strings.TrimSpace(record.Scope), postgresIdempotencyScopeCreateSession) {
		return true
	}
	if stage == StageReady || stage == StageFailed || stage == StageCancelled || record.State != idempotencyStateProcessing {
		return false
	}
	scope := strings.ToLower(strings.TrimSpace(record.Scope))
	return scope == idempotencyScopeConfirm || scope == idempotencyScopeCancel
}

func completeIdempotencyRecord(task *Task, index int, now time.Time) {
	record := &task.IdempotencyRecords[index]
	record.State = idempotencyStateCompleted
	record.ErrorCode = ""
	record.UpdatedAt = now.UTC().Format(time.RFC3339Nano)
	record.ResponseJSON = ""
	task.UpdatedAt = record.UpdatedAt
	snapshot := NormalizeTask(*task)
	snapshot.IdempotencyRecords = nil
	if raw, err := json.Marshal(snapshot); err == nil {
		record.ResponseJSON = string(raw)
	}
}

func failIdempotencyRecord(task *Task, index int, now time.Time, errorCode string) {
	record := &task.IdempotencyRecords[index]
	record.State = idempotencyStateFailed
	record.ResponseJSON = ""
	record.ErrorCode = strings.TrimSpace(errorCode)
	record.UpdatedAt = now.UTC().Format(time.RFC3339Nano)
}

func completeLatestIdempotencyScope(task *Task, scope string, now time.Time) {
	for index := len(task.IdempotencyRecords) - 1; index >= 0; index-- {
		if task.IdempotencyRecords[index].Scope == scope && task.IdempotencyRecords[index].State == idempotencyStateProcessing {
			completeIdempotencyRecord(task, index, now)
			return
		}
	}
}

func failLatestIdempotencyScope(task *Task, scope string, now time.Time, errorCode string) {
	for index := len(task.IdempotencyRecords) - 1; index >= 0; index-- {
		if task.IdempotencyRecords[index].Scope == scope && task.IdempotencyRecords[index].State == idempotencyStateProcessing {
			failIdempotencyRecord(task, index, now, errorCode)
			return
		}
	}
}

func failProcessingIdempotencyScope(task *Task, scope string, now time.Time, errorCode string) {
	for index := range task.IdempotencyRecords {
		if task.IdempotencyRecords[index].Scope == scope && task.IdempotencyRecords[index].State == idempotencyStateProcessing {
			failIdempotencyRecord(task, index, now, errorCode)
		}
	}
}

func idempotencyResponseTask(record IdempotencyRecord, fallback Task) Task {
	if strings.TrimSpace(record.ResponseJSON) == "" {
		return fallback
	}
	var snapshot Task
	if err := json.Unmarshal([]byte(record.ResponseJSON), &snapshot); err != nil || strings.TrimSpace(snapshot.TaskID) == "" {
		return fallback
	}
	snapshot.UserID = fallback.UserID
	return NormalizeTask(snapshot)
}

func hasLiveCancelClaim(task Task) bool {
	if task.Stage == StageCancelled {
		return true
	}
	for _, record := range task.IdempotencyRecords {
		if record.Scope == idempotencyScopeCancel && record.State == idempotencyStateProcessing {
			return true
		}
	}
	return false
}

func hasLiveConfirmClaim(task Task) bool {
	for _, record := range task.IdempotencyRecords {
		if record.Scope == idempotencyScopeConfirm && record.State == idempotencyStateProcessing {
			return true
		}
	}
	return false
}

func requireGenerationClaim(task Task, claim GenerationClaim) error {
	if err := requireGenerating(task); err != nil {
		return err
	}
	if task.GenerationLease == nil || strings.TrimSpace(claim.RunToken) == "" || task.GenerationLease.RunToken != strings.TrimSpace(claim.RunToken) {
		return ErrGenerationRunMismatch
	}
	return nil
}

func renewGenerationLease(task *Task, runToken string, now time.Time) {
	runToken = strings.TrimSpace(runToken)
	leaseUntil := now.UTC().Add(generationLeaseDuration)
	if task.GenerationLease != nil && task.GenerationLease.RunToken == runToken {
		if currentUntil, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(task.GenerationLease.LeaseUntil)); err == nil && currentUntil.After(leaseUntil) {
			leaseUntil = currentUntil
		}
	}
	task.GenerationLease = &GenerationLease{RunToken: runToken, LeaseUntil: leaseUntil.Format(time.RFC3339Nano)}
}

func expectedTextSlides(task Task) int {
	if task.SlideCount > 0 {
		return task.SlideCount
	}
	if task.Outline != nil && len(task.Outline.Slides) > 0 {
		return len(task.Outline.Slides)
	}
	return len(task.Slides)
}

func parseTaskTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
	if err != nil {
		return time.Now().UTC()
	}
	return parsed
}

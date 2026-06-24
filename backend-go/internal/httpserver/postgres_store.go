package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type postgresStore struct {
	db           *sql.DB
	fallbackPath string
}

func newPostgresPrimaryStore(db *sql.DB, fallbackPath string) *postgresStore {
	return &postgresStore{db: db, fallbackPath: fallbackPath}
}

func (s *postgresStore) withTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

func (s *postgresStore) ensureReady(ctx context.Context) error {
	backend := postgresStateBackend{db: s.db, fallbackPath: s.fallbackPath}
	if err := backend.ensureSchema(ctx); err != nil {
		return err
	}
	if err := backend.ensureProjectionSchema(ctx); err != nil {
		return err
	}
	if err := ensureGovernanceSchema(ctx, s.db); err != nil {
		return err
	}
	if err := s.seedPrimaryTables(ctx); err != nil {
		return err
	}
	return s.seedAPITables(ctx)
}

func (s *postgresStore) seedPrimaryTables(ctx context.Context) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `select count(*) from xz_users`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	data := seedAdminData()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := upsertUsers(ctx, tx, data.Users); err != nil {
		return err
	}
	if err := upsertPlans(ctx, tx, data.Plans); err != nil {
		return err
	}
	if err := upsertPointAccounts(ctx, tx, data.PointAccounts); err != nil {
		return err
	}
	if err := upsertChannelAgents(ctx, tx, data.ChannelAgents); err != nil {
		return err
	}
	if err := upsertCommissions(ctx, tx, data.Commissions); err != nil {
		return err
	}
	if err := upsertWithdrawals(ctx, tx, data.Withdrawals); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *postgresStore) seedAPITables(ctx context.Context) error {
	var channelCount int
	if err := s.db.QueryRowContext(ctx, `select count(*) from xz_api_channels`).Scan(&channelCount); err != nil {
		return err
	}
	var keyCount int
	if err := s.db.QueryRowContext(ctx, `select count(*) from xz_api_keys`).Scan(&keyCount); err != nil {
		return err
	}
	if channelCount > 0 && keyCount > 0 {
		return nil
	}
	data := seedAdminData()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if channelCount == 0 {
		if err := upsertAPIChannels(ctx, tx, data.APIChannels); err != nil {
			return err
		}
	}
	if keyCount == 0 {
		if err := upsertAPIKeys(ctx, tx, data.APIKeys); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *postgresStore) AdminData() (adminPlatformData, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminPlatformData{}, err
	}
	data := seedAdminData()
	var err error
	if data.Users, err = s.listUsers(ctx); err != nil {
		return data, err
	}
	if data.Plans, err = s.listPlans(ctx); err != nil {
		return data, err
	}
	if data.PointAccounts, err = s.listPointAccounts(ctx); err != nil {
		return data, err
	}
	if data.Orders, err = s.listOrders(ctx); err != nil {
		return data, err
	}
	if data.ChannelAgents, err = s.listChannelAgents(ctx); err != nil {
		return data, err
	}
	if data.Commissions, err = s.listCommissions(ctx); err != nil {
		return data, err
	}
	if data.Withdrawals, err = s.listWithdrawals(ctx); err != nil {
		return data, err
	}
	if data.APIChannels, err = s.listAPIChannels(ctx); err != nil {
		return data, err
	}
	if data.APIKeys, err = s.listAPIKeys(ctx); err != nil {
		return data, err
	}
	if data.GenerationTasks, err = s.ListGenerationTasks(); err != nil {
		return data, err
	}
	if data.Assets, err = s.ListAssets(); err != nil {
		return data, err
	}
	data = withAdminDefaults(data)
	return data, nil
}

func (s *postgresStore) ListGenerationTasks() ([]generationTask, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `select raw from xz_generation_tasks order by created_at desc, id desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []generationTask{}
	for rows.Next() {
		var item generationTask
		if err := scanRawJSON(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) ListAssets() ([]asset, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `select raw from xz_assets order by created_at desc, id desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []asset{}
	for rows.Next() {
		var item asset
		if err := scanRawJSON(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) UserAIState(userID string) (userAIState, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return userAIState{}, err
	}
	var item userAIState
	err := s.db.QueryRowContext(ctx, `select raw from xz_ai_state where user_id = $1`, userID).Scan(rawScanner(&item))
	if errors.Is(err, sql.ErrNoRows) {
		return normalizeUserAIState(userAIState{}, userID), nil
	}
	if err != nil {
		return userAIState{}, err
	}
	return normalizeUserAIState(item, userID), nil
}

func (s *postgresStore) UpdateUserAIState(userID string, req userAIState) (userAIState, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return userAIState{}, err
	}
	item := normalizeUserAIState(req, userID)
	item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
		insert into xz_ai_state (user_id, favorite_task_ids, hidden_task_ids, favorite_collections, agent_conversations, active_conversation_id, active_collection_id, updated_at, raw)
		values ($1,$2::jsonb,$3::jsonb,$4::jsonb,$5::jsonb,$6,$7,$8,$9::jsonb)
		on conflict (user_id) do update set
			favorite_task_ids = excluded.favorite_task_ids,
			hidden_task_ids = excluded.hidden_task_ids,
			favorite_collections = excluded.favorite_collections,
			agent_conversations = excluded.agent_conversations,
			active_conversation_id = excluded.active_conversation_id,
			active_collection_id = excluded.active_collection_id,
			updated_at = excluded.updated_at,
			raw = excluded.raw
	`, item.UserID, jsonProjection(item.FavoriteTaskIDs), jsonProjection(item.HiddenTaskIDs), jsonProjection(item.FavoriteCollections), jsonProjection(item.AgentConversations), item.ActiveConversationID, item.ActiveCollectionID, item.UpdatedAt, jsonProjection(item))
	return item, err
}

func (s *postgresStore) PointAccount() (pointAccount, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return pointAccount{}, err
	}
	var item pointAccount
	err := s.db.QueryRowContext(ctx, `
		select id, user_id, available, frozen
		from xz_point_accounts
		where user_id = $1
	`, "user_000002").Scan(&item.ID, &item.UserID, &item.Available, &item.Frozen)
	if errors.Is(err, sql.ErrNoRows) {
		return pointAccount{ID: "points_000002", UserID: "user_000002", Available: defaultPointsAvailable}, nil
	}
	return item, err
}

func (s *postgresStore) CreateGenerationTask(req createGenerationTaskRequest) (generationTask, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return generationTask{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return generationTask{}, err
	}
	defer func() { _ = tx.Rollback() }()

	userID := "user_000002"
	pointCost := imageCount(req.Params) * modelPointCost(req.Model)
	account, err := pointAccountForUpdate(ctx, tx, userID)
	if err != nil {
		return generationTask{}, err
	}
	if account.Available < pointCost {
		return generationTask{}, fmt.Errorf("insufficient remaining points: available %d, required %d", account.Available, pointCost)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	taskID, err := nextTableID(ctx, tx, "xz_generation_tasks", "task")
	if err != nil {
		return generationTask{}, err
	}
	task := generationTask{
		ID:               taskID,
		UserID:           userID,
		Type:             req.Type,
		Prompt:           req.Prompt,
		Params:           req.Params,
		Model:            req.Model,
		Status:           "SUCCEEDED",
		Progress:         100,
		PointCost:        pointCost,
		ResultIDs:        []string{},
		CreatedAt:        now,
		UpdatedAt:        now,
		WorkerFinishedAt: now,
	}
	count := imageCount(req.Params)
	for i := 0; i < count; i++ {
		assetID, err := nextTableID(ctx, tx, "xz_assets", "asset")
		if err != nil {
			return generationTask{}, err
		}
		task.ResultIDs = append(task.ResultIDs, assetID)
		item := generatedAssetForRequest(req, taskID, assetID, i, now)
		if err := insertAsset(ctx, tx, item); err != nil {
			return generationTask{}, err
		}
	}
	if err := insertGenerationTask(ctx, tx, task); err != nil {
		return generationTask{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		update xz_point_accounts
		set available = available - $1,
			raw = jsonb_set(raw, '{available}', to_jsonb((available - $1)::int), true)
		where id = $2
	`, pointCost, account.ID); err != nil {
		return generationTask{}, err
	}
	if err := insertAuditLog(ctx, tx, "user_000002", "MEMBER", "generation.create", "generation_task", task.ID, "", "", 200, map[string]any{"pointCost": pointCost}); err != nil {
		return generationTask{}, err
	}
	if err := tx.Commit(); err != nil {
		return generationTask{}, err
	}
	return task, nil
}

func generatedAssetForRequest(req createGenerationTaskRequest, taskID string, assetID string, index int, now string) asset {
	referenceCount := 0
	referenceImages := req.Params["referenceImages"]
	if items, ok := referenceImages.([]any); ok {
		referenceCount = len(items)
	}
	imageURL := promptPreviewImage(req.Prompt)
	contentType := "image/svg+xml"
	source := "local-prompt-preview"
	width := previewImageWidth
	height := previewImageHeight
	thumbnailURL := imageURL
	if index < len(req.GeneratedImages) && req.GeneratedImages[index].URL != "" {
		imageURL = req.GeneratedImages[index].URL
		thumbnailURL = req.GeneratedImages[index].ThumbnailURL
		contentType = req.GeneratedImages[index].ContentType
		source = req.GeneratedImages[index].Source
		width = req.GeneratedImages[index].Width
		height = req.GeneratedImages[index].Height
		if contentType == "" {
			contentType = "image/png"
		}
		if source == "" {
			source = "model-provider"
		}
		if width <= 0 || height <= 0 {
			width = previewImageWidth
			height = previewImageHeight
		}
	}
	if thumbnailURL == "" {
		thumbnailURL = imageURL
	}
	return asset{
		ID:           assetID,
		UserID:       "user_000002",
		TaskID:       taskID,
		Name:         fmt.Sprintf("TEXT_TO_IMAGE-%s-%02d", taskID, index+1),
		MediaType:    "image",
		URL:          imageURL,
		ThumbnailURL: thumbnailURL,
		Favorite:     false,
		Metadata: map[string]any{
			"prompt":          req.Prompt,
			"model":           req.Model,
			"type":            req.Type,
			"sourceType":      req.Type,
			"contentType":     contentType,
			"source":          source,
			"thumbnailUrl":    thumbnailURL,
			"width":           width,
			"height":          height,
			"resolution":      fmt.Sprintf("%dx%d", width, height),
			"index":           index + 1,
			"referenceCount":  referenceCount,
			"referenceImages": referenceImages,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (s *postgresStore) CreateAdminOrder(req adminOrderMutation) (adminOrder, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminOrder{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminOrder{}, err
	}
	defer func() { _ = tx.Rollback() }()
	id, err := nextTableID(ctx, tx, "xz_orders", "order")
	if err != nil {
		return adminOrder{}, err
	}
	item := adminOrder{ID: id, UserID: req.UserID, PlanID: req.PlanID, Amount: req.AmountCents, AmountCents: req.AmountCents, Status: fallback(req.Status, "PENDING"), CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if err := insertOrder(ctx, tx, item); err != nil {
		return adminOrder{}, err
	}
	if err := insertAuditLog(ctx, tx, "", "", "orders.create", "order", item.ID, "", "", 200, map[string]any{"userId": item.UserID, "planId": item.PlanID}); err != nil {
		return adminOrder{}, err
	}
	return item, tx.Commit()
}

func (s *postgresStore) MarkAdminOrderPaid(id string) (adminOrder, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminOrder{}, err
	}
	defer func() { _ = tx.Rollback() }()
	item, err := getOrderForUpdate(ctx, tx, id)
	if err != nil {
		return adminOrder{}, err
	}
	item.Status = "PAID"
	item.PaidAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := insertOrder(ctx, tx, item); err != nil {
		return adminOrder{}, err
	}
	if err := insertAuditLog(ctx, tx, "", "", "orders.mark_paid", "order", item.ID, "", "", 200, nil); err != nil {
		return adminOrder{}, err
	}
	return item, tx.Commit()
}

func (s *postgresStore) RenewAdminOrder(id string) (adminOrder, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminOrder{}, err
	}
	defer func() { _ = tx.Rollback() }()
	source, err := getOrderForUpdate(ctx, tx, id)
	if err != nil {
		return adminOrder{}, err
	}
	nextID, err := nextTableID(ctx, tx, "xz_orders", "order")
	if err != nil {
		return adminOrder{}, err
	}
	item := adminOrder{ID: nextID, UserID: source.UserID, PlanID: source.PlanID, Amount: orderAmount(source), AmountCents: orderAmount(source), Status: "PENDING", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), PriceSnapshot: map[string]any{"renewOf": source.ID}}
	if err := insertOrder(ctx, tx, item); err != nil {
		return adminOrder{}, err
	}
	if err := insertAuditLog(ctx, tx, "", "", "orders.renew", "order", item.ID, "", "", 200, map[string]any{"renewOf": source.ID}); err != nil {
		return adminOrder{}, err
	}
	return item, tx.Commit()
}

func (s *postgresStore) CreateAdminCommission(req adminCommissionMutation) (adminCommission, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminCommission{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if req.OrderID == "" || req.AgentID == "" || req.AmountCents <= 0 {
		return adminCommission{}, errors.New("orderId, agentId and positive amountCents are required")
	}
	id, err := nextTableID(ctx, tx, "xz_commissions", "commission")
	if err != nil {
		return adminCommission{}, err
	}
	item := adminCommission{ID: id, OrderID: req.OrderID, AgentID: req.AgentID, AmountCents: req.AmountCents, Rate: req.Rate, Status: fallback(req.Status, "PENDING"), RuleSnapshot: map[string]any{"source": "manual", "rate": req.Rate}, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if err := insertCommission(ctx, tx, item); err != nil {
		return adminCommission{}, err
	}
	if err := insertAuditLog(ctx, tx, "", "", "commissions.create", "commission", item.ID, "", "", 200, map[string]any{"orderId": item.OrderID, "agentId": item.AgentID}); err != nil {
		return adminCommission{}, err
	}
	return item, tx.Commit()
}

func (s *postgresStore) CreateAdminWithdrawal(req adminWithdrawalMutation) (adminWithdrawal, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminWithdrawal{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if req.AgentID == "" || req.AmountCents <= 0 {
		return adminWithdrawal{}, errors.New("agentId and positive amountCents are required")
	}
	id, err := nextTableID(ctx, tx, "xz_withdrawals", "withdrawal")
	if err != nil {
		return adminWithdrawal{}, err
	}
	item := adminWithdrawal{ID: id, AgentID: req.AgentID, AmountCents: req.AmountCents, Status: "PENDING", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if err := insertWithdrawal(ctx, tx, item); err != nil {
		return adminWithdrawal{}, err
	}
	if err := insertAuditLog(ctx, tx, "", "", "withdrawals.create", "withdrawal", item.ID, "", "", 200, map[string]any{"agentId": item.AgentID}); err != nil {
		return adminWithdrawal{}, err
	}
	return item, tx.Commit()
}

func (s *postgresStore) ReviewAdminWithdrawal(id string, status string) (adminWithdrawal, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminWithdrawal{}, err
	}
	defer func() { _ = tx.Rollback() }()
	status = strings.ToUpper(strings.TrimSpace(status))
	if status != "APPROVED" && status != "REJECTED" {
		return adminWithdrawal{}, fmt.Errorf("invalid withdrawal status: %s", status)
	}
	item, err := getWithdrawalForUpdate(ctx, tx, id)
	if err != nil {
		return adminWithdrawal{}, err
	}
	item.Status = status
	item.ReviewedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := insertWithdrawal(ctx, tx, item); err != nil {
		return adminWithdrawal{}, err
	}
	if err := insertAuditLog(ctx, tx, "", "", "withdrawals.review", "withdrawal", item.ID, "", "", 200, map[string]any{"status": status}); err != nil {
		return adminWithdrawal{}, err
	}
	return item, tx.Commit()
}

func (s *postgresStore) DeleteAsset(id string) error {
	ctx, cancel := s.withTimeout()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var taskID string
	var resultIDsRaw string
	err = tx.QueryRowContext(ctx, `select task_id from xz_assets where id = $1 for update`, id).Scan(&taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %s", errAssetNotFound, id)
	}
	if err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `select result_ids from xz_generation_tasks where id = $1 for update`, taskID).Scan(&resultIDsRaw); err == nil {
		var resultIDs []string
		_ = json.Unmarshal([]byte(resultIDsRaw), &resultIDs)
		filtered := []string{}
		for _, resultID := range resultIDs {
			if resultID != id {
				filtered = append(filtered, resultID)
			}
		}
		if _, err := tx.ExecContext(ctx, `update xz_generation_tasks set result_ids = $1::jsonb, raw = jsonb_set(raw, '{resultIds}', $1::jsonb, true) where id = $2`, jsonProjection(filtered), taskID); err != nil {
			return err
		}
	}
	res, err := tx.ExecContext(ctx, `delete from xz_assets where id = $1`, id)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("%w: %s", errAssetNotFound, id)
	}
	if err := insertAuditLog(ctx, tx, "", "", "assets.delete", "asset", id, "", "", 200, map[string]any{"taskId": taskID}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *postgresStore) UpdateAssetThumbnails(updates map[string]string) (int, error) {
	if len(updates) == 0 {
		return 0, nil
	}
	ctx, cancel := s.withTimeout()
	defer cancel()
	updated := 0
	for id, thumbnailURL := range updates {
		res, err := s.db.ExecContext(ctx, `
			update xz_assets
			set thumbnail_url = $2,
				updated_at = $3,
				raw = jsonb_set(jsonb_set(raw, '{thumbnailUrl}', to_jsonb($2::text), true), '{metadata,thumbnailUrl}', to_jsonb($2::text), true)
			where id = $1 and coalesce(thumbnail_url, '') = ''
		`, id, thumbnailURL, time.Now().UTC().Format(time.RFC3339Nano))
		if err != nil {
			return updated, err
		}
		rows, _ := res.RowsAffected()
		updated += int(rows)
	}
	return updated, nil
}

func (s *postgresStore) UpdateAssetImageInfo(updates map[string]assetImageInfo) (int, error) {
	if len(updates) == 0 {
		return 0, nil
	}
	ctx, cancel := s.withTimeout()
	defer cancel()
	updated := 0
	for id, info := range updates {
		var item asset
		err := s.db.QueryRowContext(ctx, `select raw from xz_assets where id = $1`, id).Scan(rawScanner(&item))
		if err != nil {
			continue
		}
		changed := false
		if item.Metadata == nil {
			item.Metadata = map[string]any{}
		}
		if info.ThumbnailURL != "" && item.ThumbnailURL == "" {
			item.ThumbnailURL = info.ThumbnailURL
			item.Metadata["thumbnailUrl"] = info.ThumbnailURL
			changed = true
		}
		if info.Width > 0 && info.Height > 0 {
			item.Metadata["width"] = info.Width
			item.Metadata["height"] = info.Height
			item.Metadata["resolution"] = fmt.Sprintf("%dx%d", info.Width, info.Height)
			changed = true
		}
		if !changed {
			continue
		}
		item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := s.db.ExecContext(ctx, `
			update xz_assets
			set thumbnail_url = $2,
				metadata = $3::jsonb,
				updated_at = $4,
				raw = $5::jsonb
			where id = $1
		`, item.ID, item.ThumbnailURL, jsonProjection(item.Metadata), item.UpdatedAt, jsonProjection(item)); err != nil {
			return updated, err
		}
		updated++
	}
	return updated, nil
}

func (s *postgresStore) UpdateUserPassword(userID string, passwordHash string) (adminUser, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminUser{}, err
	}
	var item adminUser
	err := s.db.QueryRowContext(ctx, `select raw from xz_users where id = $1`, userID).Scan(rawScanner(&item))
	if err != nil {
		return adminUser{}, err
	}
	item.PasswordHash = passwordHash
	item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(ctx, `update xz_users set password_hash = $2, updated_at = $3, raw = $4::jsonb where id = $1`, item.ID, item.PasswordHash, item.UpdatedAt, jsonProjection(item))
	return item, err
}

func (s *postgresStore) CreateAdminCustomer(req adminCustomerMutation) (adminUser, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminUser{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminUser{}, err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	userID, err := nextTableID(ctx, tx, "xz_users", "user")
	if err != nil {
		return adminUser{}, err
	}
	item := adminUser{
		ID:         userID,
		Email:      req.Email,
		Name:       req.Name,
		Role:       fallback(req.Role, "MEMBER"),
		Status:     fallback(req.Status, "ACTIVE"),
		PlanID:     fallback(req.PlanID, "plan_free"),
		ReferredBy: strings.TrimSpace(req.ReferredBy),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := insertUser(ctx, tx, item); err != nil {
		return adminUser{}, err
	}
	pointID, err := nextTableID(ctx, tx, "xz_point_accounts", "points")
	if err != nil {
		return adminUser{}, err
	}
	if err := insertPointAccount(ctx, tx, adminPointAccount{ID: pointID, UserID: item.ID, Available: req.Available}); err != nil {
		return adminUser{}, err
	}
	if err := insertAuditLog(ctx, tx, "", "", "customers.create", "user", item.ID, "", "", 200, map[string]any{"email": item.Email}); err != nil {
		return adminUser{}, err
	}
	return item, tx.Commit()
}

func (s *postgresStore) UpdateAdminCustomer(id string, req adminCustomerMutation) (adminUser, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminUser{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var item adminUser
	if err := tx.QueryRowContext(ctx, `select raw from xz_users where id = $1 for update`, id).Scan(rawScanner(&item)); err != nil {
		return adminUser{}, err
	}
	if req.Name != "" {
		item.Name = req.Name
	}
	if req.Email != "" {
		item.Email = req.Email
	}
	if req.Role != "" {
		item.Role = req.Role
	}
	if req.Status != "" {
		item.Status = req.Status
	}
	if req.PlanID != "" {
		item.PlanID = req.PlanID
	}
	item.ReferredBy = strings.TrimSpace(req.ReferredBy)
	item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := insertUser(ctx, tx, item); err != nil {
		return adminUser{}, err
	}
	if req.Available >= 0 {
		if err := upsertPointAccountByUser(ctx, tx, item.ID, req.Available); err != nil {
			return adminUser{}, err
		}
	}
	if err := insertAuditLog(ctx, tx, "", "", "customers.update", "user", item.ID, "", "", 200, nil); err != nil {
		return adminUser{}, err
	}
	return item, tx.Commit()
}

func (s *postgresStore) CreateAdminChannelAgent(req adminChannelCreateMutation) (adminChannelAgent, adminUser, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminChannelAgent{}, adminUser{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminChannelAgent{}, adminUser{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if req.Level == 2 {
		var exists bool
		if err := tx.QueryRowContext(ctx, `select exists(select 1 from xz_channel_agents where id = $1)`, req.ParentID).Scan(&exists); err != nil {
			return adminChannelAgent{}, adminUser{}, err
		}
		if !exists {
			return adminChannelAgent{}, adminUser{}, fmt.Errorf("parent channel agent not found: %s", req.ParentID)
		}
	}
	role := "AGENT_L1"
	if req.Level == 2 {
		role = "AGENT_L2"
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	userID, err := nextTableID(ctx, tx, "xz_users", "user")
	if err != nil {
		return adminChannelAgent{}, adminUser{}, err
	}
	user := adminUser{ID: userID, Email: req.Email, Name: req.Name, Role: role, Status: fallback(req.Status, "ACTIVE"), PlanID: "plan_free", CreatedAt: now, UpdatedAt: now}
	if err := insertUser(ctx, tx, user); err != nil {
		return adminChannelAgent{}, adminUser{}, err
	}
	agentID, err := nextTableID(ctx, tx, "xz_channel_agents", "channel")
	if err != nil {
		return adminChannelAgent{}, adminUser{}, err
	}
	agent := adminChannelAgent{ID: agentID, UserID: user.ID, ParentID: req.ParentID, Level: req.Level, Status: fallback(req.Status, "ACTIVE"), InviteCode: req.InviteCode, CreatedAt: now, UpdatedAt: now}
	if agent.InviteCode == "" {
		agent.InviteCode = strings.ToUpper("AG" + shortID(agent.ID))
	}
	if err := insertChannelAgent(ctx, tx, agent); err != nil {
		return adminChannelAgent{}, adminUser{}, err
	}
	if err := upsertPointAccountByUser(ctx, tx, user.ID, req.Available); err != nil {
		return adminChannelAgent{}, adminUser{}, err
	}
	if err := insertAuditLog(ctx, tx, "", "", "channel_agents.create", "channel_agent", agent.ID, "", "", 200, map[string]any{"userId": user.ID}); err != nil {
		return adminChannelAgent{}, adminUser{}, err
	}
	return agent, user, tx.Commit()
}

func (s *postgresStore) UpdateAdminChannelAgent(id string, req adminChannelMutation) (adminChannelAgent, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminChannelAgent{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var item adminChannelAgent
	if err := tx.QueryRowContext(ctx, `select raw from xz_channel_agents where id = $1 for update`, id).Scan(rawScanner(&item)); err != nil {
		return adminChannelAgent{}, err
	}
	item.Status = fallback(req.Status, item.Status)
	item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := insertChannelAgent(ctx, tx, item); err != nil {
		return adminChannelAgent{}, err
	}
	if err := insertAuditLog(ctx, tx, "", "", "channel_agents.update", "channel_agent", item.ID, "", "", 200, nil); err != nil {
		return adminChannelAgent{}, err
	}
	return item, tx.Commit()
}

func (s *postgresStore) UpdateAdminPlan(id string, req adminPlanMutation) (adminPlan, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminPlan{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var item adminPlan
	if err := tx.QueryRowContext(ctx, `select raw from xz_plans where id = $1 for update`, id).Scan(rawScanner(&item)); err != nil {
		return adminPlan{}, err
	}
	if req.Name != "" {
		item.Name = req.Name
	}
	if req.PriceCents >= 0 {
		item.Price = req.PriceCents
		item.PriceCents = req.PriceCents
	}
	if req.GrantPoints >= 0 {
		item.Points = req.GrantPoints
		item.GrantPoints = req.GrantPoints
	}
	if req.DurationDays > 0 {
		item.DurationDays = req.DurationDays
	}
	if req.Concurrency > 0 {
		item.Concurrency = req.Concurrency
	}
	item.Active = req.Active
	if req.Entitlements != nil {
		item.Entitlements = req.Entitlements
	}
	if err := insertPlan(ctx, tx, item); err != nil {
		return adminPlan{}, err
	}
	return item, tx.Commit()
}

func (s *postgresStore) UpdateAdminProduct(id string, req adminProductMutation) (adminProduct, error) {
	data, err := s.AdminData()
	if err != nil {
		return adminProduct{}, err
	}
	for _, item := range productsWithUsage(data) {
		if item.ID == id {
			if req.Name != "" {
				item.Name = req.Name
			}
			if req.Type != "" {
				item.Type = req.Type
			}
			if req.Status != "" {
				item.Status = req.Status
			}
			if len(req.Entitlements) > 0 {
				item.Entitlements = req.Entitlements
			}
			return item, nil
		}
	}
	return adminProduct{}, fmt.Errorf("product not found: %s", id)
}

func (s *postgresStore) UpdateAdminDeliveryProject(id string, req adminDeliveryMutation) (map[string]any, error) {
	return map[string]any{"id": id, "status": req.Status, "progress": req.Progress}, nil
}

func (s *postgresStore) UpdateAdminSystemSettings(req adminSystemMutation) (adminSystemSettings, error) {
	settings := defaultSystemSettings()
	if req.Brand.Name != "" {
		settings.Brand = req.Brand
	}
	if len(req.Payments) > 0 {
		settings.Payments = req.Payments
	}
	if len(req.Permissions) > 0 {
		settings.Permissions = req.Permissions
	}
	return settings, nil
}

func (s *postgresStore) CreateAdminAPIChannel(req adminAPIChannelMutation) (adminAPIChannel, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminAPIChannel{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminAPIChannel{}, err
	}
	defer func() { _ = tx.Rollback() }()
	item := adminAPIChannel{
		ID:                      "channel_api_" + strconv.FormatInt(time.Now().UnixNano(), 10),
		Name:                    fallback(req.Name, "API"),
		BaseURL:                 fallback(req.BaseURL, "https://example.com/v1"),
		Protocol:                fallback(req.Protocol, "openai"),
		ImageRequestMode:        fallback(req.ImageRequestMode, "openai"),
		ImageGenerationEndpoint: fallback(req.ImageGenerationEndpoint, "/v1/images/generations"),
		ImageEditEndpoint:       fallback(req.ImageEditEndpoint, "/v1/images/edits"),
		FetchModelsPath:         fallback(req.FetchModelsPath, "/models"),
		APIKeyEnv:               req.APIKeyEnv,
		ComfyInstances:          req.ComfyInstances,
		Notes:                   req.Notes,
		Primary:                 req.Primary,
		Status:                  fallback(req.Status, "CONFIGURABLE"),
		Priority:                req.Priority,
		Models:                  req.Models,
	}
	if item.Priority == 0 {
		item.Priority = 100
	}
	if len(item.Models) == 0 {
		item.Models = []string{"gpt-image-2", "mock-standard"}
	}
	if err := insertAPIChannel(ctx, tx, item); err != nil {
		return adminAPIChannel{}, err
	}
	return item, tx.Commit()
}

func (s *postgresStore) UpdateAdminAPIChannel(id string, req adminAPIChannelMutation) (adminAPIChannel, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminAPIChannel{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminAPIChannel{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var item adminAPIChannel
	if err := tx.QueryRowContext(ctx, `select raw from xz_api_channels where id = $1 for update`, id).Scan(rawScanner(&item)); err != nil {
		return adminAPIChannel{}, err
	}
	if req.Name != "" {
		item.Name = req.Name
	}
	if req.BaseURL != "" {
		item.BaseURL = req.BaseURL
	}
	if req.Protocol != "" {
		item.Protocol = req.Protocol
	}
	if req.ImageRequestMode != "" {
		item.ImageRequestMode = req.ImageRequestMode
	}
	if req.ImageGenerationEndpoint != "" {
		item.ImageGenerationEndpoint = req.ImageGenerationEndpoint
	}
	if req.ImageEditEndpoint != "" {
		item.ImageEditEndpoint = req.ImageEditEndpoint
	}
	if req.FetchModelsPath != "" {
		item.FetchModelsPath = req.FetchModelsPath
	}
	if req.APIKeyEnv != "" {
		item.APIKeyEnv = req.APIKeyEnv
	}
	if req.ComfyInstances != nil {
		item.ComfyInstances = req.ComfyInstances
	}
	if req.Notes != "" {
		item.Notes = req.Notes
	}
	item.Primary = req.Primary
	if req.Status != "" {
		item.Status = req.Status
	}
	if req.Priority > 0 {
		item.Priority = req.Priority
	}
	if req.Models != nil {
		item.Models = req.Models
	}
	if err := insertAPIChannel(ctx, tx, item); err != nil {
		return adminAPIChannel{}, err
	}
	return item, tx.Commit()
}

func (s *postgresStore) TestAdminAPIChannel(id string, req adminAPIChannelTestRequest) (map[string]any, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return nil, err
	}
	channels, err := s.listAPIChannels(ctx)
	if err != nil {
		return nil, err
	}
	for _, item := range channels {
		if item.ID != id {
			continue
		}
		if strings.TrimSpace(req.APIKey) == "" {
			keys, _ := s.listAPIKeys(ctx)
			req.APIKey = savedAPIKeyForChannel(keys, item)
		}
		return testAPIChannelConnection(item, req), nil
	}
	return nil, fmt.Errorf("api channel not found: %s", id)
}

func (s *postgresStore) UpdateAdminAPIModel(id string, req adminAPIModelMutation) (adminAPIModel, error) {
	return adminAPIModel{ID: id, Name: req.Name, Capability: req.Capability, BillingMode: req.BillingMode, FixedQuota: req.FixedQuota, ModelRatio: req.ModelRatio, CompletionRatio: req.CompletionRatio, Status: req.Status}, nil
}

func (s *postgresStore) CreateAdminAPIKey(req adminAPIKeyMutation) (adminAPIKey, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminAPIKey{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminAPIKey{}, err
	}
	defer func() { _ = tx.Rollback() }()
	secret := strings.TrimSpace(firstNonEmptyString(req.Secret, req.APIKey))
	item := adminAPIKey{ID: "key_" + strconv.FormatInt(time.Now().UnixNano(), 10), Customer: fallback(req.Customer, "API"), Prefix: apiKeyPrefix(secret, time.Now().Nanosecond()), Secret: secret, Status: fallback(req.Status, "ACTIVE"), Models: req.Models, QuotaLimit: req.QuotaLimit}
	if err := insertAPIKey(ctx, tx, item); err != nil {
		return adminAPIKey{}, err
	}
	return item, tx.Commit()
}

func (s *postgresStore) UpdateAdminAPIKey(id string, req adminAPIKeyMutation) (adminAPIKey, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminAPIKey{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminAPIKey{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var item adminAPIKey
	if err := tx.QueryRowContext(ctx, `select raw from xz_api_keys where id = $1 for update`, id).Scan(rawScanner(&item)); err != nil {
		return adminAPIKey{}, err
	}
	if req.Customer != "" {
		item.Customer = req.Customer
	}
	if req.Status != "" {
		item.Status = req.Status
	}
	if secret := strings.TrimSpace(firstNonEmptyString(req.Secret, req.APIKey)); secret != "" {
		item.Secret = secret
		item.Prefix = apiKeyPrefix(secret, time.Now().Nanosecond())
	}
	if req.Models != nil {
		item.Models = req.Models
	}
	if req.QuotaLimit > 0 {
		item.QuotaLimit = req.QuotaLimit
	}
	if err := insertAPIKey(ctx, tx, item); err != nil {
		return adminAPIKey{}, err
	}
	return item, tx.Commit()
}

func (s *postgresStore) UpdateAdminCustomerGroup(id string, req adminCustomerGroupMutation) (adminCustomerGroup, error) {
	return adminCustomerGroup{ID: id, Name: req.Name, Ratio: req.Ratio, Models: req.Models, Description: req.Description}, nil
}

type jsonRawScanner struct {
	target any
}

func rawScanner(target any) sql.Scanner {
	return jsonRawScanner{target: target}
}

func (s jsonRawScanner) Scan(value any) error {
	var raw []byte
	switch typed := value.(type) {
	case []byte:
		raw = typed
	case string:
		raw = []byte(typed)
	default:
		return fmt.Errorf("unsupported json raw type %T", value)
	}
	return json.Unmarshal(raw, s.target)
}

func scanRawJSON(rows *sql.Rows, target any) error {
	return rows.Scan(rawScanner(target))
}

func (s *postgresStore) listUsers(ctx context.Context) ([]adminUser, error) {
	rows, err := s.db.QueryContext(ctx, `select raw from xz_users order by created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminUser{}
	for rows.Next() {
		var item adminUser
		if err := scanRawJSON(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) listPlans(ctx context.Context) ([]adminPlan, error) {
	rows, err := s.db.QueryContext(ctx, `select raw from xz_plans order by price_cents, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminPlan{}
	for rows.Next() {
		var item adminPlan
		if err := scanRawJSON(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) listPointAccounts(ctx context.Context) ([]adminPointAccount, error) {
	rows, err := s.db.QueryContext(ctx, `select raw from xz_point_accounts order by id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminPointAccount{}
	for rows.Next() {
		var item adminPointAccount
		if err := scanRawJSON(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) listOrders(ctx context.Context) ([]adminOrder, error) {
	rows, err := s.db.QueryContext(ctx, `select raw from xz_orders order by created_at desc, id desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminOrder{}
	for rows.Next() {
		var item adminOrder
		if err := scanRawJSON(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) listChannelAgents(ctx context.Context) ([]adminChannelAgent, error) {
	rows, err := s.db.QueryContext(ctx, `select raw from xz_channel_agents order by level, created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminChannelAgent{}
	for rows.Next() {
		var item adminChannelAgent
		if err := scanRawJSON(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) listCommissions(ctx context.Context) ([]adminCommission, error) {
	rows, err := s.db.QueryContext(ctx, `select raw from xz_commissions order by created_at desc, id desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminCommission{}
	for rows.Next() {
		var item adminCommission
		if err := scanRawJSON(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) listWithdrawals(ctx context.Context) ([]adminWithdrawal, error) {
	rows, err := s.db.QueryContext(ctx, `select raw from xz_withdrawals order by created_at desc, id desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminWithdrawal{}
	for rows.Next() {
		var item adminWithdrawal
		if err := scanRawJSON(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) listAPIChannels(ctx context.Context) ([]adminAPIChannel, error) {
	rows, err := s.db.QueryContext(ctx, `select raw from xz_api_channels order by priority, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminAPIChannel{}
	for rows.Next() {
		var item adminAPIChannel
		if err := scanRawJSON(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) listAPIKeys(ctx context.Context) ([]adminAPIKey, error) {
	rows, err := s.db.QueryContext(ctx, `select raw from xz_api_keys order by id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminAPIKey{}
	for rows.Next() {
		var item adminAPIKey
		if err := scanRawJSON(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func insertUser(ctx context.Context, tx *sql.Tx, item adminUser) error {
	_, err := tx.ExecContext(ctx, `
		insert into xz_users (id, email, name, role, status, password_hash, plan_id, referred_by, subscription_expires_at, created_at, updated_at, raw)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb)
		on conflict (id) do update set email=excluded.email, name=excluded.name, role=excluded.role, status=excluded.status, password_hash=excluded.password_hash, plan_id=excluded.plan_id, referred_by=excluded.referred_by, subscription_expires_at=excluded.subscription_expires_at, created_at=excluded.created_at, updated_at=excluded.updated_at, raw=excluded.raw
	`, item.ID, item.Email, item.Name, item.Role, item.Status, item.PasswordHash, item.PlanID, item.ReferredBy, item.SubscriptionExpiresAt, item.CreatedAt, item.UpdatedAt, jsonProjection(item))
	return err
}

func insertPlan(ctx context.Context, tx *sql.Tx, item adminPlan) error {
	_, err := tx.ExecContext(ctx, `
		insert into xz_plans (id, code, name, price_cents, grant_points, duration_days, concurrency, active, entitlements, raw)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10::jsonb)
		on conflict (id) do update set code=excluded.code, name=excluded.name, price_cents=excluded.price_cents, grant_points=excluded.grant_points, duration_days=excluded.duration_days, concurrency=excluded.concurrency, active=excluded.active, entitlements=excluded.entitlements, raw=excluded.raw
	`, item.ID, item.Code, item.Name, planPriceCents(item), planGrantPoints(item), item.DurationDays, item.Concurrency, item.Active, jsonProjection(item.Entitlements), jsonProjection(item))
	return err
}

func insertPointAccount(ctx context.Context, tx *sql.Tx, item adminPointAccount) error {
	_, err := tx.ExecContext(ctx, `
		insert into xz_point_accounts (id, user_id, available, frozen, raw)
		values ($1,$2,$3,$4,$5::jsonb)
		on conflict (id) do update set user_id=excluded.user_id, available=excluded.available, frozen=excluded.frozen, raw=excluded.raw
	`, item.ID, item.UserID, item.Available, item.Frozen, jsonProjection(item))
	return err
}

func upsertPointAccountByUser(ctx context.Context, tx *sql.Tx, userID string, available int) error {
	var item adminPointAccount
	err := tx.QueryRowContext(ctx, `select raw from xz_point_accounts where user_id = $1 for update`, userID).Scan(rawScanner(&item))
	if errors.Is(err, sql.ErrNoRows) {
		id, idErr := nextTableID(ctx, tx, "xz_point_accounts", "points")
		if idErr != nil {
			return idErr
		}
		return insertPointAccount(ctx, tx, adminPointAccount{ID: id, UserID: userID, Available: available})
	}
	if err != nil {
		return err
	}
	item.Available = available
	return insertPointAccount(ctx, tx, item)
}

func insertChannelAgent(ctx context.Context, tx *sql.Tx, item adminChannelAgent) error {
	_, err := tx.ExecContext(ctx, `
		insert into xz_channel_agents (id, user_id, parent_id, level, status, invite_code, created_at, updated_at, raw)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb)
		on conflict (id) do update set user_id=excluded.user_id, parent_id=excluded.parent_id, level=excluded.level, status=excluded.status, invite_code=excluded.invite_code, created_at=excluded.created_at, updated_at=excluded.updated_at, raw=excluded.raw
	`, item.ID, item.UserID, item.ParentID, item.Level, item.Status, item.InviteCode, item.CreatedAt, item.UpdatedAt, jsonProjection(item))
	return err
}

func insertOrder(ctx context.Context, tx *sql.Tx, item adminOrder) error {
	_, err := tx.ExecContext(ctx, `
		insert into xz_orders (id, user_id, plan_id, amount_cents, status, paid_at, created_at, price_snapshot, raw)
		values ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9::jsonb)
		on conflict (id) do update set user_id=excluded.user_id, plan_id=excluded.plan_id, amount_cents=excluded.amount_cents, status=excluded.status, paid_at=excluded.paid_at, created_at=excluded.created_at, price_snapshot=excluded.price_snapshot, raw=excluded.raw
	`, item.ID, item.UserID, item.PlanID, orderAmount(item), item.Status, item.PaidAt, item.CreatedAt, jsonProjection(item.PriceSnapshot), jsonProjection(item))
	return err
}

func insertCommission(ctx context.Context, tx *sql.Tx, item adminCommission) error {
	_, err := tx.ExecContext(ctx, `
		insert into xz_commissions (id, order_id, agent_id, amount_cents, rate, status, rule_snapshot, created_at, raw)
		values ($1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9::jsonb)
		on conflict (id) do update set order_id=excluded.order_id, agent_id=excluded.agent_id, amount_cents=excluded.amount_cents, rate=excluded.rate, status=excluded.status, rule_snapshot=excluded.rule_snapshot, created_at=excluded.created_at, raw=excluded.raw
	`, item.ID, item.OrderID, item.AgentID, item.AmountCents, item.Rate, item.Status, jsonProjection(item.RuleSnapshot), item.CreatedAt, jsonProjection(item))
	return err
}

func insertWithdrawal(ctx context.Context, tx *sql.Tx, item adminWithdrawal) error {
	_, err := tx.ExecContext(ctx, `
		insert into xz_withdrawals (id, agent_id, amount_cents, status, created_at, reviewed_at, raw)
		values ($1,$2,$3,$4,$5,$6,$7::jsonb)
		on conflict (id) do update set agent_id=excluded.agent_id, amount_cents=excluded.amount_cents, status=excluded.status, created_at=excluded.created_at, reviewed_at=excluded.reviewed_at, raw=excluded.raw
	`, item.ID, item.AgentID, item.AmountCents, item.Status, item.CreatedAt, item.ReviewedAt, jsonProjection(item))
	return err
}

func insertAPIChannel(ctx context.Context, tx *sql.Tx, item adminAPIChannel) error {
	_, err := tx.ExecContext(ctx, `
		insert into xz_api_channels (id, name, base_url, protocol, status, priority, raw)
		values ($1,$2,$3,$4,$5,$6,$7::jsonb)
		on conflict (id) do update set name=excluded.name, base_url=excluded.base_url, protocol=excluded.protocol, status=excluded.status, priority=excluded.priority, raw=excluded.raw
	`, item.ID, item.Name, item.BaseURL, item.Protocol, item.Status, item.Priority, jsonProjection(item))
	return err
}

func insertAPIKey(ctx context.Context, tx *sql.Tx, item adminAPIKey) error {
	_, err := tx.ExecContext(ctx, `
		insert into xz_api_keys (id, customer, prefix, status, quota_limit, raw)
		values ($1,$2,$3,$4,$5,$6::jsonb)
		on conflict (id) do update set customer=excluded.customer, prefix=excluded.prefix, status=excluded.status, quota_limit=excluded.quota_limit, raw=excluded.raw
	`, item.ID, item.Customer, item.Prefix, item.Status, item.QuotaLimit, jsonProjection(item))
	return err
}

func insertGenerationTask(ctx context.Context, tx *sql.Tx, item generationTask) error {
	_, err := tx.ExecContext(ctx, `
		insert into xz_generation_tasks (id, user_id, type, model, status, progress, point_cost, prompt, params, result_ids, error, created_at, updated_at, worker_finished_at, raw)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10::jsonb,$11::jsonb,$12,$13,$14,$15::jsonb)
		on conflict (id) do update set user_id=excluded.user_id, type=excluded.type, model=excluded.model, status=excluded.status, progress=excluded.progress, point_cost=excluded.point_cost, prompt=excluded.prompt, params=excluded.params, result_ids=excluded.result_ids, error=excluded.error, created_at=excluded.created_at, updated_at=excluded.updated_at, worker_finished_at=excluded.worker_finished_at, raw=excluded.raw
	`, item.ID, item.UserID, item.Type, item.Model, item.Status, item.Progress, item.PointCost, item.Prompt, jsonProjection(item.Params), jsonProjection(item.ResultIDs), jsonProjection(item.Error), item.CreatedAt, item.UpdatedAt, item.WorkerFinishedAt, jsonProjection(item))
	return err
}

func insertAsset(ctx context.Context, tx *sql.Tx, item asset) error {
	_, err := tx.ExecContext(ctx, `
		insert into xz_assets (id, user_id, task_id, name, media_type, url, thumbnail_url, favorite, metadata, created_at, updated_at, raw)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11,$12::jsonb)
		on conflict (id) do update set user_id=excluded.user_id, task_id=excluded.task_id, name=excluded.name, media_type=excluded.media_type, url=excluded.url, thumbnail_url=excluded.thumbnail_url, favorite=excluded.favorite, metadata=excluded.metadata, created_at=excluded.created_at, updated_at=excluded.updated_at, raw=excluded.raw
	`, item.ID, item.UserID, item.TaskID, item.Name, item.MediaType, item.URL, item.ThumbnailURL, item.Favorite, jsonProjection(item.Metadata), item.CreatedAt, item.UpdatedAt, jsonProjection(item))
	return err
}

func pointAccountForUpdate(ctx context.Context, tx *sql.Tx, userID string) (adminPointAccount, error) {
	var item adminPointAccount
	err := tx.QueryRowContext(ctx, `select raw from xz_point_accounts where user_id = $1 for update`, userID).Scan(rawScanner(&item))
	if errors.Is(err, sql.ErrNoRows) {
		id, idErr := nextTableID(ctx, tx, "xz_point_accounts", "points")
		if idErr != nil {
			return adminPointAccount{}, idErr
		}
		item = adminPointAccount{ID: id, UserID: userID, Available: defaultPointsAvailable}
		return item, insertPointAccount(ctx, tx, item)
	}
	return item, err
}

func getOrderForUpdate(ctx context.Context, tx *sql.Tx, id string) (adminOrder, error) {
	var item adminOrder
	err := tx.QueryRowContext(ctx, `select raw from xz_orders where id = $1 for update`, id).Scan(rawScanner(&item))
	return item, err
}

func getWithdrawalForUpdate(ctx context.Context, tx *sql.Tx, id string) (adminWithdrawal, error) {
	var item adminWithdrawal
	err := tx.QueryRowContext(ctx, `select raw from xz_withdrawals where id = $1 for update`, id).Scan(rawScanner(&item))
	return item, err
}

func nextTableID(ctx context.Context, tx *sql.Tx, table string, prefix string) (string, error) {
	allowed := map[string]bool{"xz_users": true, "xz_point_accounts": true, "xz_orders": true, "xz_channel_agents": true, "xz_commissions": true, "xz_withdrawals": true, "xz_generation_tasks": true, "xz_assets": true, "xz_audit_logs": true, "xz_operation_logs": true, "xz_backup_runs": true}
	if !allowed[table] {
		return "", fmt.Errorf("unsupported id table: %s", table)
	}
	query := fmt.Sprintf(`select id from %s where id like $1`, table)
	rows, err := tx.QueryContext(ctx, query, prefix+"_%")
	if err != nil {
		return "", err
	}
	defer rows.Close()
	maxValue := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", err
		}
		parts := strings.Split(id, "_")
		value, _ := strconv.Atoi(parts[len(parts)-1])
		if value > maxValue {
			maxValue = value
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s_%06d", prefix, maxValue+1), nil
}

func insertAuditLog(ctx context.Context, tx *sql.Tx, actorID string, actorRole string, action string, resource string, resourceID string, method string, path string, status int, metadata map[string]any) error {
	id := newAuditID()
	_, err := tx.ExecContext(ctx, `
		insert into xz_audit_logs (id, actor_id, actor_role, action, resource, resource_id, method, path, status, metadata)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb)
		on conflict (id) do nothing
	`, id, actorID, actorRole, action, resource, resourceID, method, path, status, jsonProjection(metadata))
	return err
}

func ensureGovernanceSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		create table if not exists xz_audit_logs (id text primary key, actor_id text, actor_role text, action text not null, resource text not null, resource_id text, method text, path text, status int, metadata jsonb not null default '{}'::jsonb, created_at timestamptz not null default now());
		create table if not exists xz_operation_logs (id text primary key, actor_id text, operation text not null, target text not null, target_id text, before_state jsonb, after_state jsonb, created_at timestamptz not null default now());
		create table if not exists xz_role_permissions (role text not null, permission text not null, created_at timestamptz not null default now(), primary key(role, permission));
		create table if not exists xz_backup_runs (id text primary key, status text not null, target text not null, metadata jsonb not null default '{}'::jsonb, started_at timestamptz not null default now(), finished_at timestamptz);
	`)
	return err
}

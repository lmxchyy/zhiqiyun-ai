package httpserver

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

type operationCenterProfileUpdate struct {
	Name              string         `json:"name"`
	Region            string         `json:"region"`
	InviteCode        string         `json:"inviteCode"`
	ResponsiblePerson string         `json:"responsiblePerson"`
	ContactInfo       string         `json:"contactInfo"`
	SettlementProfile map[string]any `json:"settlementProfile"`
	AgreementStatus   string         `json:"agreementStatus"`
}

type operationCenterProfileStore interface {
	UpdateOperationCenterProfile(actorID, actorRole, centerID string, request operationCenterProfileUpdate) (adminOperationCenter, error)
}
type operationCenterProfileAPI struct{ store platformStore }

func (a operationCenterProfileAPI) update(w http.ResponseWriter, r *http.Request) {
	store, ok := a.store.(operationCenterProfileStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, errors.New("operation center profile store is unavailable"))
		return
	}
	var request operationCenterProfileUpdate
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	actorID, actorRole := actorFromRequest(r)
	item, err := store.UpdateOperationCenterProfile(actorID, actorRole, strings.TrimSpace(r.PathValue("id")), request)
	if err != nil {
		writeIdentityChangeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (s *postgresStore) UpdateOperationCenterProfile(actorID, actorRole, centerID string, request operationCenterProfileUpdate) (adminOperationCenter, error) {
	if actorID == "" || !identityChangeAdminRoleAllowed(actorRole) {
		return adminOperationCenter{}, errIdentityPermission
	}
	request.Name = strings.TrimSpace(request.Name)
	request.Region = strings.TrimSpace(request.Region)
	request.InviteCode = strings.ToUpper(strings.TrimSpace(request.InviteCode))
	request.ResponsiblePerson = strings.TrimSpace(request.ResponsiblePerson)
	request.ContactInfo = strings.TrimSpace(request.ContactInfo)
	request.AgreementStatus = strings.ToUpper(strings.TrimSpace(request.AgreementStatus))
	if request.Name == "" {
		return adminOperationCenter{}, errIdentityChangeInvalid
	}
	ctx, cancel := s.withTimeout()
	defer cancel()
	allowed, err := s.roleHasPermission(ctx, strings.ToUpper(strings.TrimSpace(actorRole)), "identity:operation-profile:update")
	if err != nil {
		return adminOperationCenter{}, err
	}
	if !allowed {
		return adminOperationCenter{}, errIdentityPermission
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminOperationCenter{}, err
	}
	defer tx.Rollback()
	var item adminOperationCenter
	var before []byte
	err = tx.QueryRowContext(ctx, `SELECT raw FROM xz_operation_centers WHERE id=$1 FOR UPDATE`, centerID).Scan(&before)
	if errors.Is(err, sql.ErrNoRows) {
		return adminOperationCenter{}, errIdentityUserNotFound
	}
	if err != nil {
		return adminOperationCenter{}, err
	}
	_ = json.Unmarshal(before, &item)
	beforeSafe := map[string]any{
		"name":              item.Name,
		"region":            item.Region,
		"responsiblePerson": item.ResponsiblePerson,
		"agreementStatus":   item.AgreementStatus,
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item.Name = request.Name
	item.Region = request.Region
	item.InviteCode = request.InviteCode
	item.ResponsiblePerson = request.ResponsiblePerson
	item.ContactInfo = request.ContactInfo
	item.SettlementProfile = request.SettlementProfile
	item.AgreementStatus = request.AgreementStatus
	item.UpdatedAt = now
	_, err = tx.ExecContext(ctx, `UPDATE xz_operation_centers SET name=$2,region=$3,invite_code=nullif($4,''),responsible_person=$5,contact_info=$6,settlement_profile=$7::jsonb,agreement_status=$8,updated_at=$9,raw=$10::jsonb WHERE id=$1`, centerID, item.Name, item.Region, item.InviteCode, item.ResponsiblePerson, item.ContactInfo, jsonProjection(item.SettlementProfile), item.AgreementStatus, now, jsonProjection(item))
	if err != nil {
		return adminOperationCenter{}, err
	}
	if err = insertAuditLog(ctx, tx, actorID, actorRole, "admin.operation_center.profile.update", "operation_center", centerID, http.MethodPatch, "/api/v1/admin/operation-centers/"+centerID+"/profile", 200, map[string]any{"before": beforeSafe, "after": map[string]any{"name": item.Name, "region": item.Region, "responsiblePerson": item.ResponsiblePerson, "agreementStatus": item.AgreementStatus}}); err != nil {
		return adminOperationCenter{}, err
	}
	return item, tx.Commit()
}

package httpserver

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type adminEnterpriseAPI struct {
	store adminEnterpriseStore
}

func newAdminEnterpriseAPI(store platformStore) adminEnterpriseAPI {
	repository, _ := store.(adminEnterpriseStore)
	return adminEnterpriseAPI{store: repository}
}

func (a adminEnterpriseAPI) available(w http.ResponseWriter) bool {
	if a.store != nil {
		return true
	}
	writeError(w, http.StatusServiceUnavailable, errors.New("admin enterprise store is unavailable"))
	return false
}

func (a adminEnterpriseAPI) list(w http.ResponseWriter, r *http.Request) {
	if !a.available(w) {
		return
	}
	result, err := a.store.ListAdminEnterprises(parseAdminEnterpriseListQuery(r))
	if err != nil {
		writeAdminEnterpriseError(w, err)
		return
	}
	writeJSON(w, result)
}

func (a adminEnterpriseAPI) detail(w http.ResponseWriter, r *http.Request) {
	if !a.available(w) {
		return
	}
	item, err := a.store.GetAdminEnterprise(strings.TrimSpace(r.PathValue("enterpriseId")))
	if err != nil {
		writeAdminEnterpriseError(w, err)
		return
	}
	writeJSON(w, item)
}

func (a adminEnterpriseAPI) certifications(w http.ResponseWriter, _ *http.Request) {
	if !a.available(w) {
		return
	}
	result, err := a.store.ListAdminEnterpriseCertifications()
	if err != nil {
		writeAdminEnterpriseError(w, err)
		return
	}
	writeJSON(w, result)
}

func (a adminEnterpriseAPI) section(section string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.available(w) {
			return
		}
		result, err := a.store.GetAdminEnterpriseSection(strings.TrimSpace(r.PathValue("enterpriseId")), section)
		if err != nil {
			writeAdminEnterpriseError(w, err)
			return
		}
		writeJSON(w, result)
	}
}

func (a adminEnterpriseAPI) mutate(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.available(w) {
			return
		}
		var request adminEnterpriseMutationRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		request.Action = action
		request.RequestID = strings.TrimSpace(request.RequestID)
		request.Reason = strings.TrimSpace(request.Reason)
		if request.RequestID == "" || request.Reason == "" {
			writeAdminEnterpriseError(w, fmt.Errorf("%w: requestId and reason are required", errEnterpriseInvalid))
			return
		}
		actorID, actorRole := actorFromRequest(r)
		result, err := a.store.MutateAdminEnterprise(actorID, actorRole, strings.TrimSpace(r.PathValue("enterpriseId")), request)
		if err != nil {
			writeAdminEnterpriseError(w, err)
			return
		}
		writeJSON(w, result)
	}
}

func (a adminEnterpriseAPI) create(w http.ResponseWriter, r *http.Request) {
	if !a.available(w) {
		return
	}
	var request adminEnterpriseCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	actorID, actorRole := actorFromRequest(r)
	item, err := a.store.CreateAdminEnterprise(actorID, actorRole, request)
	if err != nil {
		writeAdminEnterpriseError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(item)
}

func (a adminEnterpriseAPI) export(w http.ResponseWriter, r *http.Request) {
	if !a.available(w) {
		return
	}
	query := parseAdminEnterpriseListQuery(r)
	query.Page = 1
	query.PageSize = 5000
	result, err := a.store.ListAdminEnterprises(query)
	if err != nil {
		writeAdminEnterpriseError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=enterprises-"+time.Now().UTC().Format("20060102-150405")+".csv")
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{"企业名称", "企业ID", "认证状态", "套餐", "套餐到期时间", "成员数", "成员席位", "算力余额(点)", "来源代理商", "所属运营中心", "企业状态", "创建时间"})
	for _, item := range result.Items {
		_ = writer.Write([]string{
			item.Name,
			item.EnterpriseCode,
			item.CertificationStatus,
			item.Plan.Name,
			item.Plan.ExpiresAt,
			strconv.Itoa(item.MemberCount),
			strconv.Itoa(item.SeatLimit),
			strconv.FormatInt(item.Compute.Balance, 10),
			item.SourceAgent.Name,
			item.OperationCenter.Name,
			item.Status,
			item.CreatedAt,
		})
	}
	writer.Flush()
}

func parseAdminEnterpriseListQuery(r *http.Request) adminEnterpriseListQuery {
	values := r.URL.Query()
	page, _ := strconv.Atoi(values.Get("page"))
	pageSize, _ := strconv.Atoi(values.Get("pageSize"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 5000 {
		pageSize = 5000
	}
	query := adminEnterpriseListQuery{
		Page:              page,
		PageSize:          pageSize,
		Keyword:           strings.TrimSpace(values.Get("keyword")),
		Certification:     strings.ToUpper(strings.TrimSpace(values.Get("certificationStatus"))),
		PlanCode:          strings.TrimSpace(values.Get("planCode")),
		Status:            strings.ToUpper(strings.TrimSpace(values.Get("status"))),
		SourceAgentID:     strings.TrimSpace(values.Get("sourceAgentId")),
		OperationCenterID: strings.TrimSpace(values.Get("operationCenterId")),
	}
	query.CreatedFrom = parseAdminEnterpriseTime(values.Get("createdFrom"), false)
	query.CreatedTo = parseAdminEnterpriseTime(values.Get("createdTo"), true)
	return query
}

func parseAdminEnterpriseTime(value string, endOfDay bool) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		parsed, err := time.Parse(layout, value)
		if err != nil {
			continue
		}
		if layout == "2006-01-02" && endOfDay {
			parsed = parsed.Add(24*time.Hour - time.Nanosecond)
		}
		return &parsed
	}
	return nil
}

func writeAdminEnterpriseError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errEnterpriseNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, errEnterpriseConflict):
		writeError(w, http.StatusConflict, err)
	case errors.Is(err, errEnterpriseInvalid):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, errForbidden):
		writeError(w, http.StatusForbidden, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}

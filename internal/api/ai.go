package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/cz8jmh4n7f-bit/opord-ai-demo/internal/auth"
	"github.com/cz8jmh4n7f-bit/opord-ai-demo/internal/db"
	"github.com/cz8jmh4n7f-bit/opord-ai-demo/internal/orchestrator"
)

type aiProviderDTO struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Type      string         `json:"type"`
	Config    map[string]any `json:"config,omitempty"`
	Status    string         `json:"status"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
}

type aiServiceDTO struct {
	ID                    string         `json:"id"`
	ProviderID            string         `json:"providerId"`
	ProviderName          string         `json:"providerName"`
	ProviderType          string         `json:"providerType"`
	Name                  string         `json:"name"`
	Slug                  string         `json:"slug"`
	Category              string         `json:"category"`
	Description           string         `json:"description"`
	RequestSchema         map[string]any `json:"requestSchema"`
	DefaultExpirationDays int32          `json:"defaultExpirationDays"`
	RequiresApproval      bool           `json:"requiresApproval"`
	Status                string         `json:"status"`
	CreatedAt             time.Time      `json:"createdAt"`
	UpdatedAt             time.Time      `json:"updatedAt"`
}

type aiInstanceDTO struct {
	ID               string         `json:"id"`
	ServiceID        string         `json:"serviceId"`
	ServiceName      string         `json:"serviceName"`
	ServiceSlug      string         `json:"serviceSlug"`
	ProviderName     string         `json:"providerName"`
	ProviderType     string         `json:"providerType"`
	RequestID        string         `json:"requestId,omitempty"`
	ProviderAccessID string         `json:"providerAccessId"`
	Owner            string         `json:"owner"`
	Workspace        string         `json:"workspace"`
	Status           string         `json:"status"`
	Spec             map[string]any `json:"spec,omitempty"`
	Observed         map[string]any `json:"observed,omitempty"`
	ProvisionedAt    *time.Time     `json:"provisionedAt,omitempty"`
	ExpiresAt        *time.Time     `json:"expiresAt,omitempty"`
	RevokedAt        *time.Time     `json:"revokedAt,omitempty"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
}

type aiUsageDTO struct {
	ID           string         `json:"id"`
	InstanceID   string         `json:"instanceId,omitempty"`
	ProviderID   string         `json:"providerId"`
	ProviderName string         `json:"providerName"`
	Owner        string         `json:"owner,omitempty"`
	Workspace    string         `json:"workspace,omitempty"`
	PeriodStart  time.Time      `json:"periodStart"`
	PeriodEnd    time.Time      `json:"periodEnd"`
	Metric       string         `json:"metric"`
	Quantity     float64        `json:"quantity"`
	Unit         string         `json:"unit"`
	CostUSD      float64        `json:"costUsd"`
	Raw          map[string]any `json:"raw,omitempty"`
	CreatedAt    time.Time      `json:"createdAt"`
}

type aiAuditDTO struct {
	ID          string         `json:"id"`
	Actor       string         `json:"actor"`
	SubjectType string         `json:"subjectType"`
	SubjectID   string         `json:"subjectId,omitempty"`
	Action      string         `json:"action"`
	Message     string         `json:"message"`
	Fields      map[string]any `json:"fields,omitempty"`
	CreatedAt   time.Time      `json:"createdAt"`
}

type aiBudgetDTO struct {
	ID               string    `json:"id"`
	Scope            string    `json:"scope"`
	ScopeRef         string    `json:"scopeRef"`
	LimitUSD         float64   `json:"limitUsd"`
	Period           string    `json:"period"`
	SoftThresholdPct int32     `json:"softThresholdPct"`
	HardThresholdPct int32     `json:"hardThresholdPct"`
	ActualUSD        float64   `json:"actualUsd"`
	RemainingUSD     float64   `json:"remainingUsd"`
	UsagePct         float64   `json:"usagePct"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type aiQuotaDTO struct {
	ID            string    `json:"id"`
	ServiceID     string    `json:"serviceId,omitempty"`
	Metric        string    `json:"metric"`
	LimitQuantity float64   `json:"limitQuantity"`
	Period        string    `json:"period"`
	Enforcement   string    `json:"enforcement"`
	CreatedAt     time.Time `json:"createdAt"`
}

type aiPolicyDTO struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Rules     map[string]any `json:"rules"`
	Status    string         `json:"status"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
}

type aiModelDTO struct {
	ID           string         `json:"id"`
	ProviderID   string         `json:"providerId"`
	ProviderName string         `json:"providerName"`
	ProviderType string         `json:"providerType"`
	Model        string         `json:"model"`
	DisplayName  string         `json:"displayName"`
	Modality     string         `json:"modality"`
	Status       string         `json:"status"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	CreatedAt    time.Time      `json:"createdAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
}

func aiProviderToDTO(p db.AiProvider) aiProviderDTO {
	var cfg map[string]any
	_ = json.Unmarshal(p.Config, &cfg)
	return aiProviderDTO{
		ID:        p.ID.String(),
		Name:      p.Name,
		Type:      p.Type,
		Config:    redactAIProviderConfig(cfg),
		Status:    p.Status,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}
}

func aiBudgetSummaryToDTO(s orchestrator.AIBudgetSummary) aiBudgetDTO {
	b := s.Budget
	return aiBudgetDTO{
		ID: b.ID.String(), Scope: b.Scope, ScopeRef: b.ScopeRef, LimitUSD: b.LimitUsd, Period: b.Period,
		SoftThresholdPct: b.SoftThresholdPct, HardThresholdPct: b.HardThresholdPct,
		ActualUSD: s.ActualUSD, RemainingUSD: s.RemainingUSD, UsagePct: s.UsagePct, Status: s.Status,
		CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt,
	}
}

func aiQuotaToDTO(q db.AiQuota) aiQuotaDTO {
	serviceID := ""
	if q.ServiceID.Valid {
		serviceID = uuid.UUID(q.ServiceID.Bytes).String()
	}
	return aiQuotaDTO{
		ID: q.ID.String(), ServiceID: serviceID, Metric: q.Metric, LimitQuantity: q.LimitQuantity,
		Period: q.Period, Enforcement: q.Enforcement, CreatedAt: q.CreatedAt,
	}
}

func aiPolicyToDTO(p db.AiAccessPolicy) aiPolicyDTO {
	var rules map[string]any
	_ = json.Unmarshal(p.Rules, &rules)
	return aiPolicyDTO{
		ID: p.ID.String(), Name: p.Name, Rules: rules, Status: p.Status, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

func aiModelToDTO(m db.ListAIModelCatalogRow) aiModelDTO {
	var meta map[string]any
	_ = json.Unmarshal(m.Metadata, &meta)
	return aiModelDTO{
		ID: m.ID.String(), ProviderID: m.ProviderID.String(), ProviderName: m.ProviderName, ProviderType: m.ProviderType,
		Model: m.Model, DisplayName: m.DisplayName, Modality: m.Modality, Status: m.Status, Metadata: meta,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

func redactAIProviderConfig(cfg map[string]any) map[string]any {
	if cfg == nil {
		return nil
	}
	out := make(map[string]any, len(cfg))
	for k, v := range cfg {
		if isSensitiveAIProviderConfigKey(k) {
			out[k] = "[redacted]"
			continue
		}
		out[k] = v
	}
	return out
}

func isSensitiveAIProviderConfigKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "api_key", "openai_api_key", "anthropic_api_key", "admin_api_key", "token", "access_token",
		"bearer_token", "secret", "client_secret", "password", "master_key", "litellm_master_key":
		return true
	default:
		return false
	}
}

func aiServiceToDTO(s db.ListAIServicesRow) aiServiceDTO {
	var schema map[string]any
	_ = json.Unmarshal(s.RequestSchema, &schema)
	return aiServiceDTO{
		ID:                    s.ID.String(),
		ProviderID:            s.ProviderID.String(),
		ProviderName:          s.ProviderName,
		ProviderType:          s.ProviderType,
		Name:                  s.Name,
		Slug:                  s.Slug,
		Category:              s.Category,
		Description:           s.Description,
		RequestSchema:         schema,
		DefaultExpirationDays: s.DefaultExpirationDays,
		RequiresApproval:      s.RequiresApproval,
		Status:                s.Status,
		CreatedAt:             s.CreatedAt,
		UpdatedAt:             s.UpdatedAt,
	}
}

func aiInstanceToDTO(i db.ListAIServiceInstancesRow) aiInstanceDTO {
	var spec, observed map[string]any
	_ = json.Unmarshal(i.Spec, &spec)
	_ = json.Unmarshal(i.Observed, &observed)
	requestID := ""
	if i.RequestID.Valid {
		requestID = uuid.UUID(i.RequestID.Bytes).String()
	}
	return aiInstanceDTO{
		ID:               i.ID.String(),
		ServiceID:        i.ServiceID.String(),
		ServiceName:      i.ServiceName,
		ServiceSlug:      i.ServiceSlug,
		ProviderName:     i.ProviderName,
		ProviderType:     i.ProviderType,
		RequestID:        requestID,
		ProviderAccessID: i.ProviderAccessID,
		Owner:            i.Owner,
		Workspace:        i.Workspace,
		Status:           i.Status,
		Spec:             spec,
		Observed:         observed,
		ProvisionedAt:    tsPtr(i.ProvisionedAt),
		ExpiresAt:        tsPtr(i.ExpiresAt),
		RevokedAt:        tsPtr(i.RevokedAt),
		CreatedAt:        i.CreatedAt,
		UpdatedAt:        i.UpdatedAt,
	}
}

func aiUsageToDTO(u db.ListAIUsageRecordsRow) aiUsageDTO {
	var raw map[string]any
	_ = json.Unmarshal(u.Raw, &raw)
	instanceID := ""
	if u.InstanceID.Valid {
		instanceID = uuid.UUID(u.InstanceID.Bytes).String()
	}
	owner := ""
	if u.Owner != nil {
		owner = *u.Owner
	}
	workspace := ""
	if u.Workspace != nil {
		workspace = *u.Workspace
	}
	return aiUsageDTO{
		ID:           u.ID.String(),
		InstanceID:   instanceID,
		ProviderID:   u.ProviderID.String(),
		ProviderName: u.ProviderName,
		Owner:        owner,
		Workspace:    workspace,
		PeriodStart:  u.PeriodStart,
		PeriodEnd:    u.PeriodEnd,
		Metric:       u.Metric,
		Quantity:     u.Quantity,
		Unit:         u.Unit,
		CostUSD:      u.CostUsd,
		Raw:          raw,
		CreatedAt:    u.CreatedAt,
	}
}

func aiAuditToDTO(e db.AiAuditEvent) aiAuditDTO {
	var fields map[string]any
	_ = json.Unmarshal(e.Fields, &fields)
	subjectID := ""
	if e.SubjectID.Valid {
		subjectID = uuid.UUID(e.SubjectID.Bytes).String()
	}
	return aiAuditDTO{
		ID:          e.ID.String(),
		Actor:       e.Actor,
		SubjectType: e.SubjectType,
		SubjectID:   subjectID,
		Action:      e.Action,
		Message:     e.Message,
		Fields:      fields,
		CreatedAt:   e.CreatedAt,
	}
}

func (s *Server) listAIProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := s.svc.ListAIProviders(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]aiProviderDTO, 0, len(providers))
	for _, p := range providers {
		out = append(out, aiProviderToDTO(p))
	}
	writeJSON(w, http.StatusOK, out)
}

type createAIProviderReq struct {
	Name      string         `json:"name"`
	Type      string         `json:"type"`
	Config    map[string]any `json:"config"`
	SecretRef string         `json:"secretRef"`
	Scopes    []string       `json:"scopes"`
}

func (s *Server) createAIProvider(w http.ResponseWriter, r *http.Request) {
	var req createAIProviderReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	p, err := s.svc.CreateAIProvider(r.Context(), orchestrator.AIProviderInput{
		Name:      req.Name,
		Type:      req.Type,
		Config:    req.Config,
		SecretRef: req.SecretRef,
		Scopes:    req.Scopes,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, aiProviderToDTO(p))
}

type updateAIProviderReq struct {
	Config    map[string]any `json:"config"`
	SecretRef *string        `json:"secretRef"`
	Status    string         `json:"status"`
}

func (s *Server) updateAIProvider(w http.ResponseWriter, r *http.Request) {
	var req updateAIProviderReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	p, err := s.svc.UpdateAIProvider(r.Context(), chi.URLParam(r, "name"), orchestrator.UpdateAIProviderInput{
		Config:    req.Config,
		SecretRef: req.SecretRef,
		Status:    req.Status,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, aiProviderToDTO(p))
}

func (s *Server) deleteAIProvider(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := s.svc.DeleteAIProvider(r.Context(), name); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, orchestrator.ErrAIProviderHasActiveInstances) {
			status = http.StatusConflict
		}
		writeErr(w, status, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": name, "status": "deleted"})
}

func (s *Server) checkAIProvider(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := s.svc.CheckAIProvider(r.Context(), name); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"provider": name, "status": "ok"})
}

func (s *Server) syncAIProvider(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := s.svc.SyncAIProviderServicesByName(r.Context(), name); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"provider": name, "status": "synced"})
}

func (s *Server) syncAIProviderModels(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := s.svc.SyncAIProviderModelsByName(r.Context(), name); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"provider": name, "status": "models_synced"})
}

func (s *Server) listAIServices(w http.ResponseWriter, r *http.Request) {
	services, err := s.svc.ListAIServices(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]aiServiceDTO, 0, len(services))
	for _, svc := range services {
		out = append(out, aiServiceToDTO(svc))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) listAIRequests(w http.ResponseWriter, r *http.Request) {
	requests, err := s.svc.ListAIRequests(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]requestDTO, 0, len(requests))
	for _, req := range requests {
		out = append(out, requestToDTO(req))
	}
	writeJSON(w, http.StatusOK, out)
}

type createAIRequestReq struct {
	Name          string         `json:"name"`
	Requester     string         `json:"requester"`
	ServiceID     string         `json:"serviceId"`
	ServiceSlug   string         `json:"serviceSlug"`
	Owner         string         `json:"owner"`
	Workspace     string         `json:"workspace"`
	Justification string         `json:"justification"`
	ExpiresAt     string         `json:"expiresAt"`
	Metadata      map[string]any `json:"metadata"`
}

func (s *Server) createAIRequest(w http.ResponseWriter, r *http.Request) {
	var req createAIRequestReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	out, err := s.svc.CreateAIRequest(r.Context(), orchestrator.CreateAIRequestInput{
		Name:          req.Name,
		Requester:     req.Requester,
		ServiceID:     req.ServiceID,
		ServiceSlug:   req.ServiceSlug,
		Owner:         req.Owner,
		Workspace:     req.Workspace,
		Justification: req.Justification,
		ExpiresAt:     req.ExpiresAt,
		Metadata:      req.Metadata,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, requestToDTO(*out))
}

func (s *Server) approveAIRequest(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var body decisionReq
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.By == "" {
		body.By = "api"
	}
	if err := s.svc.ApproveRequest(r.Context(), name, "dev", body.By); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "status": "approved"})
}

func (s *Server) rejectAIRequest(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var body decisionReq
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.By == "" {
		body.By = "api"
	}
	if err := s.svc.RejectRequest(r.Context(), name, "dev", body.By, body.Reason); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "status": "rejected"})
}

func (s *Server) listAIInstances(w http.ResponseWriter, r *http.Request) {
	instances, err := s.svc.ListAIInstances(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]aiInstanceDTO, 0, len(instances))
	for _, inst := range instances {
		out = append(out, aiInstanceToDTO(inst))
	}
	writeJSON(w, http.StatusOK, out)
}

// revealAIInstanceSecret returns the raw provider-minted credential (e.g. a
// LiteLLM virtual key) for an instance, read from the secret store on demand -
// the key is never stored in the DB. Operator+; every reveal is audited.
func (s *Server) revealAIInstanceSecret(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	actor := "api"
	if ident, ok := auth.IdentityFrom(r.Context()); ok && ident.Email != "" {
		actor = ident.Email
	}
	key, err := s.svc.RevealAIInstanceSecret(r.Context(), id, actor)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]string{"key": key})
}

// reapExpiredAIInstances runs the access-expiry sweep on demand (it also runs on
// a timer in the API). Operator+; returns how many grants were revoked.
func (s *Server) reapExpiredAIInstances(w http.ResponseWriter, r *http.Request) {
	n, err := s.svc.ReapExpiredAIInstances(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"revoked": n})
}

func (s *Server) revokeAIInstance(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	var body decisionReq
	_ = json.NewDecoder(r.Body).Decode(&body)
	inst, err := s.svc.RevokeAIInstance(r.Context(), id, body.By)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": inst.ID.String(), "status": inst.Status})
}

func (s *Server) listAIUsage(w http.ResponseWriter, r *http.Request) {
	records, err := s.svc.ListAIUsageRecords(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]aiUsageDTO, 0, len(records))
	for _, rec := range records {
		out = append(out, aiUsageToDTO(rec))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) listAIAudit(w http.ResponseWriter, r *http.Request) {
	events, err := s.svc.ListAIAuditEvents(r.Context(), 100)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]aiAuditDTO, 0, len(events))
	for _, ev := range events {
		out = append(out, aiAuditToDTO(ev))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) listAIBudgets(w http.ResponseWriter, r *http.Request) {
	rows, err := s.svc.ListAIBudgetSummaries(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]aiBudgetDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, aiBudgetSummaryToDTO(row))
	}
	writeJSON(w, http.StatusOK, out)
}

type createAIBudgetReq struct {
	Scope            string  `json:"scope"`
	ScopeRef         string  `json:"scopeRef"`
	LimitUSD         float64 `json:"limitUsd"`
	Period           string  `json:"period"`
	SoftThresholdPct int32   `json:"softThresholdPct"`
	HardThresholdPct int32   `json:"hardThresholdPct"`
}

func (s *Server) createAIBudget(w http.ResponseWriter, r *http.Request) {
	var req createAIBudgetReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	b, err := s.svc.CreateAIBudget(r.Context(), orchestrator.AIBudgetInput{
		Scope: req.Scope, ScopeRef: req.ScopeRef, LimitUSD: req.LimitUSD, Period: req.Period,
		SoftThresholdPct: req.SoftThresholdPct, HardThresholdPct: req.HardThresholdPct,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, aiBudgetSummaryToDTO(orchestrator.AIBudgetSummary{Budget: b, RemainingUSD: b.LimitUsd, Status: "ok"}))
}

func (s *Server) listAIQuotas(w http.ResponseWriter, r *http.Request) {
	rows, err := s.svc.ListAIQuotas(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]aiQuotaDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, aiQuotaToDTO(row))
	}
	writeJSON(w, http.StatusOK, out)
}

type createAIQuotaReq struct {
	ServiceSlug   string  `json:"serviceSlug"`
	Metric        string  `json:"metric"`
	LimitQuantity float64 `json:"limitQuantity"`
	Period        string  `json:"period"`
	Enforcement   string  `json:"enforcement"`
}

func (s *Server) createAIQuota(w http.ResponseWriter, r *http.Request) {
	var req createAIQuotaReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	q, err := s.svc.CreateAIQuota(r.Context(), orchestrator.AIQuotaInput{
		ServiceSlug: req.ServiceSlug, Metric: req.Metric, LimitQuantity: req.LimitQuantity, Period: req.Period, Enforcement: req.Enforcement,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, aiQuotaToDTO(q))
}

func (s *Server) listAIPolicies(w http.ResponseWriter, r *http.Request) {
	rows, err := s.svc.ListAIAccessPolicies(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]aiPolicyDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, aiPolicyToDTO(row))
	}
	writeJSON(w, http.StatusOK, out)
}

type createAIPolicyReq struct {
	Name   string         `json:"name"`
	Rules  map[string]any `json:"rules"`
	Status string         `json:"status"`
}

func (s *Server) createAIPolicy(w http.ResponseWriter, r *http.Request) {
	var req createAIPolicyReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	p, err := s.svc.CreateAIAccessPolicy(r.Context(), orchestrator.AIAccessPolicyInput{Name: req.Name, Rules: req.Rules, Status: req.Status})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, aiPolicyToDTO(p))
}

func (s *Server) listAIModels(w http.ResponseWriter, r *http.Request) {
	rows, err := s.svc.ListAIModelCatalog(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]aiModelDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, aiModelToDTO(row))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) listAIRenewals(w http.ResponseWriter, r *http.Request) {
	days := int32(30)
	if raw := r.URL.Query().Get("days"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			days = int32(n)
		}
	}
	rows, err := s.svc.ListAIExpiringInstances(r.Context(), days)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]aiInstanceDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, aiInstanceToDTO(db.ListAIServiceInstancesRow(row)))
	}
	writeJSON(w, http.StatusOK, out)
}

type importOpenAIUsageReq struct {
	ProviderName string `json:"providerName"`
	Start        string `json:"start"`
	End          string `json:"end"`
}

type importAnthropicUsageReq struct {
	ProviderName string `json:"providerName"`
	Start        string `json:"start"`
	End          string `json:"end"`
}

func (s *Server) importOpenAIUsage(w http.ResponseWriter, r *http.Request) {
	var req importOpenAIUsageReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	start, err := parseOptionalTime(req.Start)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	end, err := parseOptionalTime(req.End)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	result, err := s.svc.ImportOpenAICosts(r.Context(), orchestrator.OpenAICostImportInput{ProviderName: req.ProviderName, Start: start, End: end})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) importAnthropicUsage(w http.ResponseWriter, r *http.Request) {
	var req importAnthropicUsageReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	start, err := parseOptionalTime(req.Start)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	end, err := parseOptionalTime(req.End)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	result, err := s.svc.ImportAnthropicCosts(r.Context(), orchestrator.AnthropicCostImportInput{ProviderName: req.ProviderName, Start: start, End: end})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func parseOptionalTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		t, err := time.Parse(layout, raw)
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, &time.ParseError{Layout: "RFC3339 or YYYY-MM-DD", Value: raw}
}

func (s *Server) gatewayOpenAIResponses(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	res, err := s.svc.GatewayOpenAIResponses(r.Context(), r.URL.Query().Get("provider"), body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	w.Header().Set("Content-Type", res.ContentType)
	w.WriteHeader(res.StatusCode)
	_, _ = w.Write(res.Body)
}

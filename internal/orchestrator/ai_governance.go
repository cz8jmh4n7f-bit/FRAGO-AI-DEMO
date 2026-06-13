package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cz8jmh4n7f-bit/opord-ai-demo/internal/aiproviders"
	"github.com/cz8jmh4n7f-bit/opord-ai-demo/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type AIBudgetInput struct {
	Scope            string
	ScopeRef         string
	LimitUSD         float64
	Period           string
	SoftThresholdPct int32
	HardThresholdPct int32
}

type AIBudgetSummary struct {
	Budget       db.AiBudget
	ActualUSD    float64
	RemainingUSD float64
	UsagePct     float64
	Status       string
}

type AIQuotaInput struct {
	ServiceSlug   string
	Metric        string
	LimitQuantity float64
	Period        string
	Enforcement   string
}

type AIAccessPolicyInput struct {
	Name   string
	Rules  map[string]any
	Status string
}

type OpenAICostImportInput struct {
	ProviderName string
	Start        time.Time
	End          time.Time
}

type OpenAICostImportResult struct {
	ProviderName string
	Imported     int
	Skipped      int
	PeriodStart  time.Time
	PeriodEnd    time.Time
}

type AIGatewayResponse struct {
	StatusCode  int
	ContentType string
	Body        []byte
}

func (s *Service) CreateAIBudget(ctx context.Context, in AIBudgetInput) (db.AiBudget, error) {
	scope := strings.TrimSpace(in.Scope)
	if scope == "" {
		scope = "global"
	}
	period := strings.TrimSpace(in.Period)
	if period == "" {
		period = "monthly"
	}
	soft := in.SoftThresholdPct
	if soft == 0 {
		soft = 80
	}
	hard := in.HardThresholdPct
	if hard == 0 {
		hard = 100
	}
	if in.LimitUSD <= 0 {
		return db.AiBudget{}, fmt.Errorf("limit_usd must be greater than zero")
	}
	if err := validateAIBudgetThresholds(soft, hard); err != nil {
		return db.AiBudget{}, err
	}
	b, err := s.q.CreateAIBudget(ctx, db.CreateAIBudgetParams{
		TenantID:         tenantForCreate(ctx),
		Scope:            scope,
		ScopeRef:         strings.TrimSpace(in.ScopeRef),
		LimitUsd:         in.LimitUSD,
		Period:           period,
		SoftThresholdPct: soft,
		HardThresholdPct: hard,
	})
	if err != nil {
		return db.AiBudget{}, fmt.Errorf("creating ai budget: %w", err)
	}
	s.emitAIAudit(ctx, "ai_budget", b.ID, "created", "AI budget created", map[string]any{"scope": b.Scope, "scope_ref": b.ScopeRef, "limit_usd": b.LimitUsd}, "")
	return b, nil
}

func (s *Service) ListAIBudgetSummaries(ctx context.Context) ([]AIBudgetSummary, error) {
	budgets, err := s.q.ListAIBudgets(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing ai budgets: %w", err)
	}
	usage, err := s.ListAIUsageRecords(ctx)
	if err != nil {
		return nil, err
	}
	tid, scoped := scopeTenant(ctx)
	out := make([]AIBudgetSummary, 0, len(budgets))
	for _, b := range budgets {
		if scoped && !tenantVisible(b.TenantID, tid) {
			continue
		}
		actual := aiBudgetActualUSD(b, usage)
		remaining := b.LimitUsd - actual
		pct := 0.0
		if b.LimitUsd > 0 {
			pct = (actual / b.LimitUsd) * 100
		}
		status := "ok"
		if pct >= float64(b.HardThresholdPct) {
			status = "hard_limit"
		} else if pct >= float64(b.SoftThresholdPct) {
			status = "warning"
		}
		out = append(out, AIBudgetSummary{Budget: b, ActualUSD: actual, RemainingUSD: remaining, UsagePct: pct, Status: status})
	}
	return out, nil
}

func aiBudgetActualUSD(b db.AiBudget, usage []db.ListAIUsageRecordsRow) float64 {
	start := aiBudgetPeriodStart(b.Period)
	return aiCostUSD(usage, start, func(u db.ListAIUsageRecordsRow) bool {
		return aiBudgetScopeMatches(b, u)
	})
}

// aiAuthoritativeCostSources are spend-report imports whose cost SUPERSEDES the
// gateway's own per-call estimate (opord_gateway_lite) for the same provider in
// the same window. Without this, a provider that has BOTH a gateway estimate and
// an authoritative import for a period would be double-counted in budgets/quotas.
var aiAuthoritativeCostSources = map[string]bool{
	"openai_costs_api":      true,
	"anthropic_cost_report": true,
	"litellm_spend":         true,
}

// usageSource returns the lowercased "source" tag from a usage row's raw JSON.
func usageSource(raw []byte) string {
	return strings.ToLower(usageRawField(raw, "source"))
}

// aiCostUSD sums CostUsd over usage rows that match inScope and whose period_start
// is at or after start, applying SUPERSESSION: when an authoritative spend-report
// row exists for a provider in the window, that provider's gateway estimate
// (opord_gateway_lite) is dropped so the estimate and the authoritative figure are
// not double-counted. Providers without an import keep their gateway estimate.
func aiCostUSD(usage []db.ListAIUsageRecordsRow, start time.Time, inScope func(db.ListAIUsageRecordsRow) bool) float64 {
	authoritative := map[string]bool{}
	for _, u := range usage {
		if u.PeriodStart.Before(start) || !inScope(u) {
			continue
		}
		if aiAuthoritativeCostSources[usageSource(u.Raw)] {
			authoritative[strings.ToLower(u.ProviderName)] = true
		}
	}
	var total float64
	for _, u := range usage {
		if u.PeriodStart.Before(start) || !inScope(u) {
			continue
		}
		if usageSource(u.Raw) == "opord_gateway_lite" && authoritative[strings.ToLower(u.ProviderName)] {
			continue // superseded by an authoritative import for this provider
		}
		total += u.CostUsd
	}
	return total
}

func aiBudgetPeriodStart(period string) time.Time {
	now := time.Now()
	switch strings.ToLower(strings.TrimSpace(period)) {
	case "daily":
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	case "yearly", "annual":
		return time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location())
	default:
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	}
}

func aiBudgetScopeMatches(b db.AiBudget, u db.ListAIUsageRecordsRow) bool {
	ref := strings.TrimSpace(b.ScopeRef)
	switch strings.ToLower(strings.TrimSpace(b.Scope)) {
	case "", "global":
		return true
	case "provider":
		return ref == "" || strings.EqualFold(u.ProviderName, ref) || u.ProviderID.String() == ref
	case "owner":
		if u.Owner != nil && strings.EqualFold(*u.Owner, ref) {
			return true
		}
		return strings.EqualFold(usageRawField(u.Raw, "owner"), ref)
	case "workspace":
		// Prefer the joined instance workspace; for IMPORTED usage (no instance)
		// fall back to the provider workspace recorded in raw (name or id), so
		// workspace-scoped budgets count real imported spend.
		if u.Workspace != nil && strings.EqualFold(*u.Workspace, ref) {
			return true
		}
		return strings.EqualFold(usageRawField(u.Raw, "workspace"), ref) ||
			strings.EqualFold(usageRawField(u.Raw, "workspace_id"), ref)
	case "tenant":
		if ref == "" {
			return !u.TenantID.Valid
		}
		return u.TenantID.Valid && uuid.UUID(u.TenantID.Bytes).String() == ref
	default:
		return false
	}
}

// usageRawField pulls a string field out of a usage record's raw JSON (used to
// attribute imported, instance-less usage to a budget scope).
func usageRawField(raw []byte, key string) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func (s *Service) CreateAIQuota(ctx context.Context, in AIQuotaInput) (db.AiQuota, error) {
	metric := strings.TrimSpace(in.Metric)
	if metric == "" {
		metric = "tokens"
	}
	period := strings.TrimSpace(in.Period)
	if period == "" {
		period = "monthly"
	}
	enforcement := strings.TrimSpace(in.Enforcement)
	if enforcement == "" {
		enforcement = "warn"
	}
	if in.LimitQuantity <= 0 {
		return db.AiQuota{}, fmt.Errorf("limit_quantity must be greater than zero")
	}
	var serviceID pgtype.UUID
	if strings.TrimSpace(in.ServiceSlug) != "" {
		svc, err := s.q.GetAIServiceBySlug(ctx, strings.TrimSpace(in.ServiceSlug))
		if err != nil {
			return db.AiQuota{}, fmt.Errorf("ai service %q not found: %w", in.ServiceSlug, err)
		}
		serviceID = pgtype.UUID{Bytes: svc.ID, Valid: true}
	}
	q, err := s.q.CreateAIQuota(ctx, db.CreateAIQuotaParams{
		ServiceID:     serviceID,
		TenantID:      tenantForCreate(ctx),
		Metric:        metric,
		LimitQuantity: in.LimitQuantity,
		Period:        period,
		Enforcement:   enforcement,
	})
	if err != nil {
		return db.AiQuota{}, fmt.Errorf("creating ai quota: %w", err)
	}
	s.emitAIAudit(ctx, "ai_quota", q.ID, "created", "AI quota created", map[string]any{"metric": q.Metric, "limit_quantity": q.LimitQuantity, "period": q.Period}, "")
	return q, nil
}

func (s *Service) ListAIQuotas(ctx context.Context) ([]db.AiQuota, error) {
	rows, err := s.q.ListAIQuotas(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing ai quotas: %w", err)
	}
	tid, scoped := scopeTenant(ctx)
	if !scoped {
		return rows, nil
	}
	out := make([]db.AiQuota, 0, len(rows))
	for _, r := range rows {
		if tenantVisible(r.TenantID, tid) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *Service) CreateAIAccessPolicy(ctx context.Context, in AIAccessPolicyInput) (db.AiAccessPolicy, error) {
	if strings.TrimSpace(in.Name) == "" {
		return db.AiAccessPolicy{}, fmt.Errorf("policy name is required")
	}
	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = "active"
	}
	rules := in.Rules
	if rules == nil {
		rules = map[string]any{}
	}
	raw, err := json.Marshal(rules)
	if err != nil {
		return db.AiAccessPolicy{}, fmt.Errorf("marshaling policy rules: %w", err)
	}
	p, err := s.q.CreateAIAccessPolicy(ctx, db.CreateAIAccessPolicyParams{
		Name:     strings.TrimSpace(in.Name),
		TenantID: tenantForCreate(ctx),
		Rules:    raw,
		Status:   status,
	})
	if err != nil {
		return db.AiAccessPolicy{}, fmt.Errorf("creating ai access policy: %w", err)
	}
	s.emitAIAudit(ctx, "ai_policy", p.ID, "created", "AI access policy created", map[string]any{"name": p.Name, "status": p.Status}, "")
	return p, nil
}

func (s *Service) ListAIAccessPolicies(ctx context.Context) ([]db.AiAccessPolicy, error) {
	rows, err := s.q.ListAIAccessPolicies(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing ai access policies: %w", err)
	}
	tid, scoped := scopeTenant(ctx)
	if !scoped {
		return rows, nil
	}
	out := make([]db.AiAccessPolicy, 0, len(rows))
	for _, r := range rows {
		if tenantVisible(r.TenantID, tid) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *Service) SyncAIProviderModelsByName(ctx context.Context, name string) error {
	p, err := s.q.GetAIProviderByName(ctx, name)
	if err != nil {
		return fmt.Errorf("ai provider %q not found: %w", name, err)
	}
	prov, err := s.aiProvider(p.Type)
	if err != nil {
		return err
	}
	modelProvider, ok := prov.(aiproviders.ModelCatalogProvider)
	if !ok {
		return fmt.Errorf("ai provider %q does not expose a model catalog", p.Type)
	}
	models, err := modelProvider.ListModels(ctx, aiproviders.ModelListRequest{Credentials: s.aiCredentials(ctx, p), Config: aiProviderConfig(p)})
	if err != nil {
		s.emitAIAudit(ctx, "ai_provider", p.ID, "model_sync_failed", err.Error(), map[string]any{"name": p.Name, "type": p.Type}, "")
		return err
	}
	for _, m := range models {
		if strings.TrimSpace(m.Model) == "" {
			continue
		}
		meta := m.Metadata
		if meta == nil {
			meta = map[string]any{}
		}
		raw, _ := json.Marshal(meta)
		_, err := s.q.UpsertAIModelCatalog(ctx, db.UpsertAIModelCatalogParams{
			ProviderID:  p.ID,
			Model:       m.Model,
			DisplayName: firstNonEmpty(m.DisplayName, m.Model),
			Modality:    firstNonEmpty(m.Modality, "text"),
			Status:      firstNonEmpty(m.Status, "active"),
			Metadata:    raw,
		})
		if err != nil {
			return fmt.Errorf("upserting ai model %q: %w", m.Model, err)
		}
	}
	s.emitAIAudit(ctx, "ai_provider", p.ID, "models_synced", "AI provider models synced", map[string]any{"name": p.Name, "count": len(models)}, "")
	return nil
}

func (s *Service) ListAIModelCatalog(ctx context.Context) ([]db.ListAIModelCatalogRow, error) {
	rows, err := s.q.ListAIModelCatalog(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing ai model catalog: %w", err)
	}
	return rows, nil
}

func (s *Service) ListAIExpiringInstances(ctx context.Context, days int32) ([]db.ListAIExpiringInstancesRow, error) {
	if days <= 0 {
		days = 30
	}
	rows, err := s.q.ListAIExpiringInstances(ctx, days)
	if err != nil {
		return nil, fmt.Errorf("listing ai renewals: %w", err)
	}
	tid, scoped := scopeTenant(ctx)
	if !scoped {
		return rows, nil
	}
	out := make([]db.ListAIExpiringInstancesRow, 0, len(rows))
	for _, r := range rows {
		if tenantVisible(r.TenantID, tid) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *Service) ImportOpenAICosts(ctx context.Context, in OpenAICostImportInput) (OpenAICostImportResult, error) {
	name := strings.TrimSpace(in.ProviderName)
	if name == "" {
		return OpenAICostImportResult{}, fmt.Errorf("provider name is required")
	}
	p, err := s.q.GetAIProviderByName(ctx, name)
	if err != nil {
		return OpenAICostImportResult{}, fmt.Errorf("ai provider %q not found: %w", name, err)
	}
	if p.Type != string(aiproviders.ProviderOpenAI) {
		return OpenAICostImportResult{}, fmt.Errorf("ai provider %q is %q, not openai", p.Name, p.Type)
	}
	creds := s.aiCredentials(ctx, p)
	// Same two-credential model as Anthropic (ADR-0022): prefer a dedicated
	// admin_api_key for the org-costs API; fall back to the single-key layout.
	key := firstNonEmpty(creds["admin_api_key"], creds["api_key"], creds["openai_api_key"], creds["token"])
	if key == "" {
		return OpenAICostImportResult{}, fmt.Errorf("openai admin api key missing in secret_ref (store it as admin_api_key)")
	}
	start, end := in.Start, in.End
	if start.IsZero() {
		start = time.Now().AddDate(0, 0, -7)
	}
	if end.IsZero() {
		end = time.Now()
	}
	if !end.After(start) {
		return OpenAICostImportResult{}, fmt.Errorf("end must be after start")
	}
	cfg := aiProviderConfig(p)
	baseURL := "https://api.openai.com"
	if v, ok := cfg["base_url"].(string); ok && strings.TrimSpace(v) != "" {
		baseURL = strings.TrimSpace(v)
	}
	u, _ := url.Parse(strings.TrimRight(baseURL, "/") + "/v1/organization/costs")
	q := u.Query()
	q.Set("start_time", fmt.Sprintf("%d", start.Unix()))
	q.Set("end_time", fmt.Sprintf("%d", end.Unix()))
	q.Set("bucket_width", "1d")
	q.Set("limit", "180")
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return OpenAICostImportResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return OpenAICostImportResult{}, fmt.Errorf("openai costs import failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return OpenAICostImportResult{}, fmt.Errorf("openai costs import returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var payload openAICostsPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return OpenAICostImportResult{}, fmt.Errorf("decoding openai costs: %w", err)
	}
	result := OpenAICostImportResult{ProviderName: p.Name, PeriodStart: start, PeriodEnd: end}
	for _, bucket := range payload.Data {
		bs := time.Unix(bucket.StartTime, 0)
		be := time.Unix(bucket.EndTime, 0)
		for idx, item := range bucket.Results {
			importKey := fmt.Sprintf("openai-costs:%d:%d:%s:%s:%d", bucket.StartTime, bucket.EndTime, item.ProjectID, item.LineItem, idx)
			if _, err := s.q.FindAIUsageRecordByImportKey(ctx, db.FindAIUsageRecordByImportKeyParams{
				ProviderID: p.ID, PeriodStart: bs, PeriodEnd: be, Metric: "cost_usd", ImportKey: importKey,
			}); err == nil {
				result.Skipped++
				continue
			} else if err != pgx.ErrNoRows {
				return result, fmt.Errorf("checking ai usage import key: %w", err)
			}
			raw, _ := json.Marshal(map[string]any{
				"source":     "openai_costs_api",
				"import_key": importKey,
				"currency":   item.Amount.Currency,
				"project_id": item.ProjectID,
				"line_item":  item.LineItem,
			})
			if _, err := s.q.CreateAIUsageRecord(ctx, db.CreateAIUsageRecordParams{
				ProviderID: p.ID, PeriodStart: bs, PeriodEnd: be, Metric: "cost_usd", Quantity: item.Amount.Value, Unit: "usd", CostUsd: item.Amount.Value, Raw: raw,
			}); err != nil {
				return result, fmt.Errorf("creating openai cost usage record: %w", err)
			}
			result.Imported++
		}
	}
	s.emitAIAudit(ctx, "ai_provider", p.ID, "usage_imported", "OpenAI costs imported", map[string]any{"provider": p.Name, "imported": result.Imported, "skipped": result.Skipped}, "")
	return result, nil
}

type openAICostsPayload struct {
	Data []struct {
		StartTime int64 `json:"start_time"`
		EndTime   int64 `json:"end_time"`
		Results   []struct {
			Amount struct {
				Value    float64 `json:"value"`
				Currency string  `json:"currency"`
			} `json:"amount"`
			LineItem  string `json:"line_item"`
			ProjectID string `json:"project_id"`
		} `json:"results"`
	} `json:"data"`
}

// AnthropicCostImportInput / Result mirror the OpenAI cost import above.
type AnthropicCostImportInput struct {
	ProviderName string
	Start        time.Time
	End          time.Time
}
type AnthropicCostImportResult struct {
	ProviderName string
	Imported     int
	Skipped      int
	PeriodStart  time.Time
	PeriodEnd    time.Time
}

// UpdateAIBudget edits a budget's limit/period/thresholds.
// aiTenantGuard verifies the caller may mutate a tenant-owned governance row.
// Scoped (non-admin, tenant-bound) callers may only touch their OWN tenant's
// rows; on a mismatch it returns a not-found error so an IDOR probe can neither
// edit nor enumerate another tenant's budgets/quotas/policies. Admins, CLI, and
// dev mode are unscoped and pass through.
func (s *Service) aiTenantGuard(ctx context.Context, rowTenant pgtype.UUID) error {
	if tid, scoped := scopeTenant(ctx); scoped && !tenantVisible(rowTenant, tid) {
		return fmt.Errorf("not found")
	}
	return nil
}

// validateAIBudgetThresholds rejects nonsensical alert/stop bands: both must be
// positive and the soft (warning) threshold must not exceed the hard (stop) one.
func validateAIBudgetThresholds(soft, hard int32) error {
	if soft <= 0 || hard <= 0 {
		return fmt.Errorf("threshold percentages must be greater than zero")
	}
	if soft > hard {
		return fmt.Errorf("soft threshold (%d%%) must not exceed hard threshold (%d%%)", soft, hard)
	}
	return nil
}

func (s *Service) UpdateAIBudget(ctx context.Context, id uuid.UUID, in AIBudgetInput) (db.AiBudget, error) {
	if in.LimitUSD <= 0 {
		return db.AiBudget{}, fmt.Errorf("limit_usd must be greater than zero")
	}
	existing, err := s.q.GetAIBudget(ctx, id)
	if err != nil {
		return db.AiBudget{}, fmt.Errorf("loading ai budget: %w", err)
	}
	if err := s.aiTenantGuard(ctx, existing.TenantID); err != nil {
		return db.AiBudget{}, err
	}
	period := strings.TrimSpace(in.Period)
	if period == "" {
		period = "monthly"
	}
	soft := in.SoftThresholdPct
	if soft == 0 {
		soft = 80
	}
	hard := in.HardThresholdPct
	if hard == 0 {
		hard = 100
	}
	if err := validateAIBudgetThresholds(soft, hard); err != nil {
		return db.AiBudget{}, err
	}
	// Preserve the scope when the caller omits it, so a limit-only edit can't
	// silently reset a provider/owner/workspace budget back to global.
	scope := strings.TrimSpace(in.Scope)
	scopeRef := strings.TrimSpace(in.ScopeRef)
	if scope == "" {
		scope, scopeRef = existing.Scope, existing.ScopeRef
	}
	b, err := s.q.UpdateAIBudget(ctx, db.UpdateAIBudgetParams{ID: id, Scope: scope, ScopeRef: scopeRef, LimitUsd: in.LimitUSD, Period: period, SoftThresholdPct: soft, HardThresholdPct: hard})
	if err != nil {
		return db.AiBudget{}, fmt.Errorf("updating ai budget: %w", err)
	}
	s.emitAIAudit(ctx, "ai_budget", b.ID, "updated", "AI budget updated", map[string]any{"limit_usd": b.LimitUsd, "period": b.Period, "scope": b.Scope}, "")
	return b, nil
}

func (s *Service) DeleteAIBudget(ctx context.Context, id uuid.UUID) error {
	existing, err := s.q.GetAIBudget(ctx, id)
	if err != nil {
		return fmt.Errorf("loading ai budget: %w", err)
	}
	if err := s.aiTenantGuard(ctx, existing.TenantID); err != nil {
		return err
	}
	if err := s.q.DeleteAIBudget(ctx, id); err != nil {
		return fmt.Errorf("deleting ai budget: %w", err)
	}
	s.emitAIAudit(ctx, "ai_budget", id, "deleted", "AI budget deleted", nil, "")
	return nil
}

// UpdateAIQuota edits a quota's limit/period/enforcement.
func (s *Service) UpdateAIQuota(ctx context.Context, id uuid.UUID, limit float64, period, enforcement string) (db.AiQuota, error) {
	if limit <= 0 {
		return db.AiQuota{}, fmt.Errorf("limit_quantity must be greater than zero")
	}
	existing, err := s.q.GetAIQuota(ctx, id)
	if err != nil {
		return db.AiQuota{}, fmt.Errorf("loading ai quota: %w", err)
	}
	if err := s.aiTenantGuard(ctx, existing.TenantID); err != nil {
		return db.AiQuota{}, err
	}
	if period = strings.TrimSpace(period); period == "" {
		period = "monthly"
	}
	if enforcement = strings.ToLower(strings.TrimSpace(enforcement)); enforcement != "block" {
		enforcement = "warn"
	}
	q, err := s.q.UpdateAIQuota(ctx, db.UpdateAIQuotaParams{ID: id, LimitQuantity: limit, Period: period, Enforcement: enforcement})
	if err != nil {
		return db.AiQuota{}, fmt.Errorf("updating ai quota: %w", err)
	}
	s.emitAIAudit(ctx, "ai_quota", q.ID, "updated", "AI quota updated", map[string]any{"metric": q.Metric, "limit": q.LimitQuantity, "enforcement": q.Enforcement}, "")
	return q, nil
}

func (s *Service) DeleteAIQuota(ctx context.Context, id uuid.UUID) error {
	existing, err := s.q.GetAIQuota(ctx, id)
	if err != nil {
		return fmt.Errorf("loading ai quota: %w", err)
	}
	if err := s.aiTenantGuard(ctx, existing.TenantID); err != nil {
		return err
	}
	if err := s.q.DeleteAIQuota(ctx, id); err != nil {
		return fmt.Errorf("deleting ai quota: %w", err)
	}
	s.emitAIAudit(ctx, "ai_quota", id, "deleted", "AI quota deleted", nil, "")
	return nil
}

// UpdateAIAccessPolicy edits a policy's rules/status.
func (s *Service) UpdateAIAccessPolicy(ctx context.Context, id uuid.UUID, rules map[string]any, status string) (db.AiAccessPolicy, error) {
	existing, err := s.q.GetAIAccessPolicy(ctx, id)
	if err != nil {
		return db.AiAccessPolicy{}, fmt.Errorf("loading ai policy: %w", err)
	}
	if err := s.aiTenantGuard(ctx, existing.TenantID); err != nil {
		return db.AiAccessPolicy{}, err
	}
	if status = strings.ToLower(strings.TrimSpace(status)); status != "disabled" {
		status = "active"
	}
	if rules == nil {
		rules = map[string]any{} // store {} not null so the rule unmarshal is well-defined
	}
	raw, err := json.Marshal(rules)
	if err != nil {
		return db.AiAccessPolicy{}, fmt.Errorf("encoding policy rules: %w", err)
	}
	p, err := s.q.UpdateAIAccessPolicy(ctx, db.UpdateAIAccessPolicyParams{ID: id, Rules: raw, Status: status})
	if err != nil {
		return db.AiAccessPolicy{}, fmt.Errorf("updating ai policy: %w", err)
	}
	s.emitAIAudit(ctx, "ai_policy", p.ID, "updated", "AI policy updated", map[string]any{"name": p.Name, "status": p.Status}, "")
	return p, nil
}

func (s *Service) DeleteAIAccessPolicy(ctx context.Context, id uuid.UUID) error {
	existing, err := s.q.GetAIAccessPolicy(ctx, id)
	if err != nil {
		return fmt.Errorf("loading ai policy: %w", err)
	}
	if err := s.aiTenantGuard(ctx, existing.TenantID); err != nil {
		return err
	}
	if err := s.q.DeleteAIAccessPolicy(ctx, id); err != nil {
		return fmt.Errorf("deleting ai policy: %w", err)
	}
	s.emitAIAudit(ctx, "ai_policy", id, "deleted", "AI policy deleted", nil, "")
	return nil
}

// LiteLLMSpendImportResult summarizes a spend-back sync.
type LiteLLMSpendImportResult struct {
	ProviderName string
	Keys         int
	TotalUSD     float64
}

// ImportLiteLLMSpend pulls the authoritative cumulative spend for each active
// LiteLLM virtual key OPORD minted and records it as one updated-in-place usage
// row per key (attributed to the key's owner/workspace via the instance), so
// budgets reflect REAL LiteLLM spend - closing the cost-governance loop.
func (s *Service) ImportLiteLLMSpend(ctx context.Context, providerName string) (LiteLLMSpendImportResult, error) {
	p, err := s.q.GetAIProviderByName(ctx, providerName)
	if err != nil {
		return LiteLLMSpendImportResult{}, fmt.Errorf("ai provider %q not found: %w", providerName, err)
	}
	if p.Type != string(aiproviders.ProviderLiteLLM) {
		return LiteLLMSpendImportResult{}, fmt.Errorf("ai provider %q is %q, not litellm", p.Name, p.Type)
	}
	prov, err := s.aiProvider(p.Type)
	if err != nil {
		return LiteLLMSpendImportResult{}, err
	}
	instances, err := s.q.ListAIServiceInstances(ctx)
	if err != nil {
		return LiteLLMSpendImportResult{}, err
	}
	baseCreds := s.aiCredentials(ctx, p)
	cfg := aiProviderConfig(p)
	result := LiteLLMSpendImportResult{ProviderName: p.Name}
	for _, inst := range instances {
		if inst.ProviderName != p.Name || inst.Status != "active" {
			continue
		}
		creds := map[string]string{}
		for k, v := range baseCreds {
			creds[k] = v
		}
		if k := s.readMintedKey(ctx, mintedKeyPath(inst.Observed)); k != "" {
			creds["minted_key"] = k
		}
		recs, uerr := prov.GetUsage(ctx, aiproviders.UsageRequest{
			ProviderAccessID: inst.ProviderAccessID, Credentials: creds, Config: cfg,
		})
		if uerr != nil || len(recs) == 0 {
			continue
		}
		spend := recs[0].CostUSD
		result.Keys++
		result.TotalUSD += spend
		raw, _ := json.Marshal(map[string]any{"source": "litellm_spend", "key_alias": inst.ProviderAccessID})
		instID := pgtype.UUID{Bytes: inst.ID, Valid: true}
		if existing, ferr := s.q.FindAIKeySpendRecord(ctx, instID); ferr == nil {
			_ = s.q.UpdateAIUsageRecordCost(ctx, db.UpdateAIUsageRecordCostParams{ID: existing.ID, CostUsd: spend})
		} else {
			now := time.Now()
			_, _ = s.q.CreateAIUsageRecord(ctx, db.CreateAIUsageRecordParams{
				InstanceID: instID, ProviderID: p.ID, PeriodStart: now, PeriodEnd: now,
				Metric: "cost_usd", Quantity: spend, Unit: "usd", CostUsd: spend, Raw: raw,
			})
		}
	}
	s.emitAIAudit(ctx, "ai_provider", p.ID, "spend_imported", "LiteLLM spend imported",
		map[string]any{"provider": p.Name, "keys": result.Keys, "total_usd": result.TotalUSD}, "")
	return result, nil
}

// ImportAnthropicCosts pulls org-level spend from the Anthropic Admin Cost Report
// API (GET /v1/organizations/cost_report) and records it as ai_usage_records - the
// Anthropic twin of ImportOpenAICosts. Differences from the OpenAI path, per the
// Anthropic docs: auth is the `x-api-key` header (needs an ADMIN key,
// sk-ant-admin..., in the provider's secret_ref) + `anthropic-version`; the time
// params are RFC 3339 strings (not Unix); `amount` comes back as a decimal string
// in the lowest currency unit (cents), so it is divided by 100 to USD; daily
// buckets only; grouped by workspace_id + description for per-model/line-item
// detail; paginates on has_more/next_page. Idempotent via a per-row import_key.
func (s *Service) ImportAnthropicCosts(ctx context.Context, in AnthropicCostImportInput) (AnthropicCostImportResult, error) {
	name := strings.TrimSpace(in.ProviderName)
	if name == "" {
		return AnthropicCostImportResult{}, fmt.Errorf("provider name is required")
	}
	p, err := s.q.GetAIProviderByName(ctx, name)
	if err != nil {
		return AnthropicCostImportResult{}, fmt.Errorf("ai provider %q not found: %w", name, err)
	}
	if p.Type != string(aiproviders.ProviderAnthropic) {
		return AnthropicCostImportResult{}, fmt.Errorf("ai provider %q is %q, not anthropic", p.Name, p.Type)
	}
	creds := s.aiCredentials(ctx, p)
	// Anthropic's two-credential model (ADR-0022): the ADMIN key (sk-ant-admin...)
	// serves /v1/organizations/* but NOT /v1/models, and vice versa for the
	// inference key. One secret_ref carries both: `admin_api_key` (preferred here)
	// for billing/provisioning, `api_key` for check/sync. The fallback keeps a
	// single-key secret working when it really is an admin key.
	key := firstNonEmpty(creds["admin_api_key"], creds["api_key"], creds["anthropic_api_key"], creds["token"])
	if key == "" {
		return AnthropicCostImportResult{}, fmt.Errorf("anthropic admin api key missing in secret_ref (store it as admin_api_key, sk-ant-admin...)")
	}
	start, end := in.Start, in.End
	if start.IsZero() {
		start = time.Now().AddDate(0, 0, -7)
	}
	if end.IsZero() {
		end = time.Now()
	}
	if !end.After(start) {
		return AnthropicCostImportResult{}, fmt.Errorf("end must be after start")
	}
	cfg := aiProviderConfig(p)
	baseURL := "https://api.anthropic.com"
	if v, ok := cfg["base_url"].(string); ok && strings.TrimSpace(v) != "" {
		baseURL = strings.TrimSpace(v)
	}

	// Resolve workspace ids -> names so imported spend can be attributed to a
	// workspace-scoped budget (which keys on the human workspace name, not the
	// Anthropic id). Best-effort: if the admin lookup fails we fall back to the id.
	wsNames := map[string]string{}
	if prov, perr := s.aiProvider(p.Type); perr == nil {
		if admin, ok := prov.(aiproviders.AdminProvisioner); ok {
			ac := aiproviders.AdminContext{Credentials: creds, Config: cfg}
			if wss, werr := admin.ListWorkspaces(ctx, ac); werr == nil {
				for _, w := range wss {
					wsNames[w.ID] = w.Name
				}
			}
		}
	}

	result := AnthropicCostImportResult{ProviderName: p.Name, PeriodStart: start, PeriodEnd: end}
	page := ""
	// Daily buckets cap at 31; bound the pagination loop defensively regardless.
	for iter := 0; iter < 40; iter++ {
		u, _ := url.Parse(strings.TrimRight(baseURL, "/") + "/v1/organizations/cost_report")
		q := u.Query()
		q.Set("starting_at", start.UTC().Format(time.RFC3339))
		q.Set("ending_at", end.UTC().Format(time.RFC3339))
		q.Set("bucket_width", "1d")
		q.Add("group_by[]", "workspace_id")
		q.Add("group_by[]", "description")
		q.Set("limit", "31")
		if page != "" {
			q.Set("page", page)
		}
		u.RawQuery = q.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return result, err
		}
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return result, fmt.Errorf("anthropic cost import failed: %w", err)
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			msg := string(body)
			if len(msg) > 512 {
				msg = msg[:512]
			}
			return result, fmt.Errorf("anthropic cost import returned %s: %s", resp.Status, strings.TrimSpace(msg))
		}
		var payload anthropicCostReport
		if err := json.Unmarshal(body, &payload); err != nil {
			return result, fmt.Errorf("decoding anthropic cost report: %w", err)
		}
		for _, bucket := range payload.Data {
			bs, _ := time.Parse(time.RFC3339, bucket.StartingAt)
			be, _ := time.Parse(time.RFC3339, bucket.EndingAt)
			for idx, item := range bucket.Results {
				cents, perr := strconv.ParseFloat(strings.TrimSpace(item.Amount), 64)
				if perr != nil {
					continue
				}
				usd := cents / 100.0
				importKey := fmt.Sprintf("anthropic-cost:%s:%s:%s:%s:%s:%d",
					bucket.StartingAt, item.WorkspaceID, item.Description, item.CostType, item.TokenType, idx)
				if _, err := s.q.FindAIUsageRecordByImportKey(ctx, db.FindAIUsageRecordByImportKeyParams{
					ProviderID: p.ID, PeriodStart: bs, PeriodEnd: be, Metric: "cost_usd", ImportKey: importKey,
				}); err == nil {
					result.Skipped++
					continue
				} else if err != pgx.ErrNoRows {
					return result, fmt.Errorf("checking ai usage import key: %w", err)
				}
				raw, _ := json.Marshal(map[string]any{
					"source":       "anthropic_cost_report",
					"import_key":   importKey,
					"currency":     item.Currency,
					"workspace_id": item.WorkspaceID,
					"workspace":    wsNames[item.WorkspaceID], // human name for budget attribution
					"description":  item.Description,
					"model":        item.Model,
					"cost_type":    item.CostType,
				})
				if _, err := s.q.CreateAIUsageRecord(ctx, db.CreateAIUsageRecordParams{
					ProviderID: p.ID, PeriodStart: bs, PeriodEnd: be, Metric: "cost_usd", Quantity: usd, Unit: "usd", CostUsd: usd, Raw: raw,
				}); err != nil {
					return result, fmt.Errorf("creating anthropic cost usage record: %w", err)
				}
				result.Imported++
			}
		}
		if !payload.HasMore || strings.TrimSpace(payload.NextPage) == "" {
			break
		}
		page = payload.NextPage
	}
	s.emitAIAudit(ctx, "ai_provider", p.ID, "usage_imported", "Anthropic costs imported", map[string]any{"provider": p.Name, "imported": result.Imported, "skipped": result.Skipped}, "")
	return result, nil
}

// anthropicCostReport models the Admin Cost Report response. `amount` is a decimal
// string in cents (e.g. "123.45" = $1.2345); `starting_at`/`ending_at` are RFC 3339.
type anthropicCostReport struct {
	Data []struct {
		StartingAt string `json:"starting_at"`
		EndingAt   string `json:"ending_at"`
		Results    []struct {
			Amount      string `json:"amount"`
			Currency    string `json:"currency"`
			CostType    string `json:"cost_type"`
			Description string `json:"description"`
			Model       string `json:"model"`
			WorkspaceID string `json:"workspace_id"`
			TokenType   string `json:"token_type"`
		} `json:"results"`
	} `json:"data"`
	HasMore  bool   `json:"has_more"`
	NextPage string `json:"next_page"`
}

func (s *Service) GatewayOpenAIResponses(ctx context.Context, providerName string, payload []byte) (AIGatewayResponse, error) {
	name := strings.TrimSpace(providerName)
	if name == "" {
		name = "openai-main"
	}
	p, err := s.q.GetAIProviderByName(ctx, name)
	if err != nil {
		return AIGatewayResponse{}, fmt.Errorf("ai provider %q not found: %w", name, err)
	}
	if p.Type != string(aiproviders.ProviderOpenAI) {
		return AIGatewayResponse{}, fmt.Errorf("ai provider %q is %q, not openai", p.Name, p.Type)
	}
	creds := s.aiCredentials(ctx, p)
	key := firstNonEmpty(creds["api_key"], creds["openai_api_key"], creds["token"])
	if key == "" {
		return AIGatewayResponse{}, fmt.Errorf("openai api key missing in secret_ref")
	}
	// Spend gate: block the proxy when the provider/global budget is exhausted.
	if err := s.evaluateGatewayBudget(ctx, p.Name); err != nil {
		return AIGatewayResponse{}, err
	}
	cfg := aiProviderConfig(p)
	payload, derr := s.applyGatewayDLP(ctx, p, cfg, payload)
	if derr != nil {
		return AIGatewayResponse{}, derr
	}
	baseURL := "https://api.openai.com"
	if v, ok := cfg["base_url"].(string); ok && strings.TrimSpace(v) != "" {
		baseURL = strings.TrimSpace(v)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/v1/responses", bytes.NewReader(payload))
	if err != nil {
		return AIGatewayResponse{}, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.emitAIAudit(ctx, "ai_gateway", uuid.Nil, "request_failed", err.Error(), map[string]any{"provider": p.Name}, "")
		return AIGatewayResponse{}, fmt.Errorf("openai gateway request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return AIGatewayResponse{}, fmt.Errorf("reading openai gateway response: %w", err)
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	fields := map[string]any{"provider": p.Name, "status_code": resp.StatusCode}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		s.recordGatewayUsage(ctx, p.ID, body)
		s.emitAIAudit(ctx, "ai_gateway", uuid.Nil, "request_completed", "OpenAI gateway request completed", fields, "")
	} else {
		s.emitAIAudit(ctx, "ai_gateway", uuid.Nil, "request_rejected", "OpenAI gateway request returned an error", fields, "")
	}
	return AIGatewayResponse{StatusCode: resp.StatusCode, ContentType: contentType, Body: body}, nil
}

// GatewayAnthropicMessages proxies an Anthropic /v1/messages call with the SAME
// governance as the OpenAI gateway: budget/quota spend gate, then forward with the
// inference key (never exposed to the caller), then record real-cost usage. Lets
// an app route Claude traffic through OPORD's enforcement + audit, not just OpenAI.
func (s *Service) GatewayAnthropicMessages(ctx context.Context, providerName string, payload []byte) (AIGatewayResponse, error) {
	name := strings.TrimSpace(providerName)
	if name == "" {
		name = "anthropic-main"
	}
	p, err := s.q.GetAIProviderByName(ctx, name)
	if err != nil {
		return AIGatewayResponse{}, fmt.Errorf("ai provider %q not found: %w", name, err)
	}
	if p.Type != string(aiproviders.ProviderAnthropic) {
		return AIGatewayResponse{}, fmt.Errorf("ai provider %q is %q, not anthropic", p.Name, p.Type)
	}
	creds := s.aiCredentials(ctx, p)
	// The /messages API needs an INFERENCE key (api_key), NOT the admin key.
	key := firstNonEmpty(creds["api_key"], creds["anthropic_api_key"], creds["token"])
	if key == "" {
		return AIGatewayResponse{}, fmt.Errorf("anthropic inference api key (api_key) missing in secret_ref - the admin key cannot serve /messages")
	}
	if err := s.evaluateGatewayBudget(ctx, p.Name); err != nil {
		return AIGatewayResponse{}, err
	}
	cfg := aiProviderConfig(p)
	baseURL := "https://api.anthropic.com"
	if v, ok := cfg["base_url"].(string); ok && strings.TrimSpace(v) != "" {
		baseURL = strings.TrimSpace(v)
	}
	version := "2023-06-01"
	if v, ok := cfg["anthropic_version"].(string); ok && strings.TrimSpace(v) != "" {
		version = strings.TrimSpace(v)
	}
	payload, derr := s.applyGatewayDLP(ctx, p, cfg, payload)
	if derr != nil {
		return AIGatewayResponse{}, derr
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return AIGatewayResponse{}, err
	}
	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", version)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.emitAIAudit(ctx, "ai_gateway", uuid.Nil, "request_failed", err.Error(), map[string]any{"provider": p.Name}, "")
		return AIGatewayResponse{}, fmt.Errorf("anthropic gateway request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return AIGatewayResponse{}, fmt.Errorf("reading anthropic gateway response: %w", err)
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	fields := map[string]any{"provider": p.Name, "status_code": resp.StatusCode}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		s.recordGatewayUsage(ctx, p.ID, body)
		s.emitAIAudit(ctx, "ai_gateway", uuid.Nil, "request_completed", "Anthropic gateway request completed", fields, "")
	} else {
		s.emitAIAudit(ctx, "ai_gateway", uuid.Nil, "request_rejected", "Anthropic gateway request returned an error", fields, "")
	}
	return AIGatewayResponse{StatusCode: resp.StatusCode, ContentType: contentType, Body: body}, nil
}

// applyGatewayDLP redacts PII/secrets from the request payload when DLP is
// enabled for the provider, auditing the redaction counts by type (never values).
// Returns the payload unchanged when DLP is off or nothing matched.
func (s *Service) applyGatewayDLP(ctx context.Context, p db.AiProvider, cfg map[string]any, payload []byte) ([]byte, error) {
	if !dlpEnabled(cfg) {
		return payload, nil
	}
	redacted, hits, ok := redactDLP(payload)
	if !ok {
		// PII found but couldn't be redacted - fail CLOSED, never forward the raw.
		total := 0
		for _, v := range hits {
			total += v
		}
		s.emitAIAudit(ctx, "ai_gateway", uuid.Nil, "dlp_redaction_failed",
			fmt.Sprintf("DLP found %d sensitive value(s) but could not redact - request blocked", total),
			map[string]any{"provider": p.Name, "total": total}, "")
		return nil, &aiEnforcementError{reasons: []string{"DLP redaction failed - request blocked to avoid forwarding unredacted data"}}
	}
	if len(hits) == 0 {
		return payload, nil
	}
	total := 0
	fields := map[string]any{"provider": p.Name}
	for k, v := range hits {
		fields[k] = v
		total += v
	}
	fields["total"] = total
	s.emitAIAudit(ctx, "ai_gateway", uuid.Nil, "dlp_redacted",
		fmt.Sprintf("DLP redacted %d sensitive value(s) before forwarding", total), fields, "")
	return redacted, nil
}

func (s *Service) recordGatewayUsage(ctx context.Context, providerID uuid.UUID, body []byte) {
	var payload struct {
		Model string `json:"model"`
		Usage struct {
			InputTokens      float64 `json:"input_tokens"`
			OutputTokens     float64 `json:"output_tokens"`
			TotalTokens      float64 `json:"total_tokens"`
			PromptTokens     float64 `json:"prompt_tokens"`     // OpenAI chat/completions alias
			CompletionTokens float64 `json:"completion_tokens"` // OpenAI chat/completions alias
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return
	}
	in := payload.Usage.InputTokens
	if in == 0 {
		in = payload.Usage.PromptTokens
	}
	out := payload.Usage.OutputTokens
	if out == 0 {
		out = payload.Usage.CompletionTokens
	}
	total := payload.Usage.TotalTokens
	if total == 0 {
		total = in + out
	}
	if total == 0 {
		return
	}
	// Real-time cost: price the call from the model table so budgets/cost-quotas
	// bite on live $ (not just tokens). Authoritative spend for LiteLLM keys comes
	// from the spend-back importer. When only a combined total is reported (no
	// in/out split) price the total at the blended rate so cost isn't recorded as $0.
	var cost float64
	if in == 0 && out == 0 {
		cost = estimateAICostTotal(payload.Model, total)
	} else {
		cost = estimateAICost(payload.Model, in, out)
	}
	now := time.Now()
	raw, _ := json.Marshal(map[string]any{
		"source":        "opord_gateway_lite",
		"model":         payload.Model,
		"input_tokens":  in,
		"output_tokens": out,
		"cost_basis":    "estimated",
	})
	_, _ = s.q.CreateAIUsageRecord(ctx, db.CreateAIUsageRecordParams{
		ProviderID: providerID, PeriodStart: now, PeriodEnd: now, Metric: "tokens", Quantity: total, Unit: "tokens", CostUsd: cost, Raw: raw,
	})
}

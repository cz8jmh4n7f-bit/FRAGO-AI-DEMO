package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cz8jmh4n7f-bit/opord-ai-demo/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// aiReqContext carries the facts an AI request is evaluated against. It is built
// at request creation and again at approval so governance is checked on both.
type aiReqContext struct {
	ServiceID       uuid.UUID
	ServiceSlug     string
	ServiceCategory string
	ProviderName    string
	ProviderType    string
	Owner           string
	Workspace       string
	Tenant          pgtype.UUID
}

// aiEnforcementError is returned when an active governance rule BLOCKS a request
// (as opposed to a system error). The API maps it to HTTP 403 so the requester
// sees the reason instead of a generic failure.
type aiEnforcementError struct{ reasons []string }

func (e *aiEnforcementError) Error() string {
	return "blocked by AI governance: " + strings.Join(e.reasons, "; ")
}

// IsAIEnforcementError reports whether err is a governance block.
func IsAIEnforcementError(err error) bool {
	var e *aiEnforcementError
	return errors.As(err, &e)
}

// aiPolicyRule is the (minimal, documented) shape stored in
// ai_access_policies.rules. A rule SELECTS requests: every non-empty selector
// must match (AND); an empty selector is a wildcard. An active rule whose effect
// is "deny" (the default) BLOCKS any request it selects. effect "allow" is
// reserved and is a no-op in v1.
//
// Example - deny external contractors any OpenAI access:
//
//	{"effect":"deny","providers":["openai"],"owner_domains":["contractor.com"]}
type aiPolicyRule struct {
	Effect       string   `json:"effect"`
	Providers    []string `json:"providers"`     // provider name or type
	Categories   []string `json:"categories"`    // service category
	Services     []string `json:"services"`      // service slug
	OwnerDomains []string `json:"owner_domains"` // owner email domain
}

func (r aiPolicyRule) isDeny() bool {
	return !strings.EqualFold(strings.TrimSpace(r.Effect), "allow")
}

// hasSelector reports whether the rule constrains anything (at least one non-empty
// selector). A rule with NO selectors matches EVERY request (each empty selector is
// a wildcard), so a selector-less deny rule would be an accidental BLOCK-ALL - e.g.
// a policy saved with empty/null rules. We treat such a rule as inert: a real AI
// kill-switch must be explicit (disable the provider / set a $0 budget), never the
// silent side effect of an empty rules object.
func (r aiPolicyRule) hasSelector() bool {
	for _, sel := range [][]string{r.Providers, r.Categories, r.Services, r.OwnerDomains} {
		for _, v := range sel {
			if strings.TrimSpace(v) != "" {
				return true
			}
		}
	}
	return false
}

func (r aiPolicyRule) matches(rc aiReqContext) bool {
	return matchAny(r.Providers, rc.ProviderName, rc.ProviderType) &&
		matchAny(r.Categories, rc.ServiceCategory) &&
		matchAny(r.Services, rc.ServiceSlug) &&
		matchAny(r.OwnerDomains, emailDomain(rc.Owner))
}

// matchAny reports whether patterns is empty (wildcard) or any pattern equals
// (case-insensitively) any candidate.
func matchAny(patterns []string, candidates ...string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			return true
		}
		for _, c := range candidates {
			if strings.EqualFold(p, strings.TrimSpace(c)) {
				return true
			}
		}
	}
	return false
}

// emailDomain returns the lowercased domain of an owner email ("" if none).
func emailDomain(owner string) string {
	at := strings.LastIndex(owner, "@")
	if at < 0 || at == len(owner)-1 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(owner[at+1:]))
}

// budgetAppliesToRequest reports whether a budget's scope covers this request.
func budgetAppliesToRequest(b db.AiBudget, rc aiReqContext) bool {
	ref := strings.TrimSpace(b.ScopeRef)
	switch strings.ToLower(strings.TrimSpace(b.Scope)) {
	case "", "global":
		return true
	case "provider":
		return ref == "" || strings.EqualFold(rc.ProviderName, ref)
	case "owner":
		return strings.EqualFold(rc.Owner, ref)
	case "workspace":
		return ref == "" || strings.EqualFold(rc.Workspace, ref)
	case "tenant":
		if ref == "" {
			return !rc.Tenant.Valid
		}
		return rc.Tenant.Valid && uuid.UUID(rc.Tenant.Bytes).String() == ref
	default:
		return false
	}
}

func scopeLabel(b db.AiBudget) string {
	scope := strings.TrimSpace(b.Scope)
	if scope == "" {
		scope = "global"
	}
	if ref := strings.TrimSpace(b.ScopeRef); ref != "" {
		return scope + ":" + ref
	}
	return scope
}

// evaluateAIGovernance enforces active policies, quotas, and budgets against a
// pending AI request. It BLOCKS (returns *aiEnforcementError) on a hard
// violation: an active deny policy, a metric "instances"/"seats" quota with
// enforcement "block" at/over its limit, or an applicable budget at its hard
// threshold. Soft signals (warn-quotas, budgets in the warning band) are audited
// but allowed. All inputs come from the existing tenant-scoped List* methods, so
// no new queries/migrations are needed.
func (s *Service) evaluateAIGovernance(ctx context.Context, rc aiReqContext) error {
	var hard, soft []string

	// 1) Policy guardrails (deny-list).
	if policies, err := s.ListAIAccessPolicies(ctx); err == nil {
		for _, p := range policies {
			if p.Status != "active" {
				continue
			}
			var rule aiPolicyRule
			if json.Unmarshal(p.Rules, &rule) != nil {
				continue
			}
			// A selector-less rule matches everything; never let an empty/null rules
			// object become a silent deny-all (it would block every AI request).
			if rule.isDeny() && rule.hasSelector() && rule.matches(rc) {
				hard = append(hard, fmt.Sprintf("policy %q denies this request", p.Name))
			}
		}
	}

	// 2) Seat/instance quotas. Token & cost quotas are enforced on the gateway path.
	if quotas, err := s.ListAIQuotas(ctx); err == nil {
		var instances []db.ListAIServiceInstancesRow
		loaded := false
		for _, q := range quotas {
			metric := strings.ToLower(strings.TrimSpace(q.Metric))
			if metric != "instances" && metric != "seats" {
				continue
			}
			scoped := q.ServiceID.Valid
			if scoped && uuid.UUID(q.ServiceID.Bytes) != rc.ServiceID {
				continue
			}
			if !loaded {
				instances, _ = s.ListAIInstances(ctx)
				loaded = true
			}
			active := 0
			for _, in := range instances {
				if in.Status != "active" {
					continue
				}
				if scoped && in.ServiceID != rc.ServiceID {
					continue
				}
				active++
			}
			if float64(active) >= q.LimitQuantity {
				msg := fmt.Sprintf("quota reached (%d/%.0f active %s)", active, q.LimitQuantity, metric)
				if q.Enforcement == "block" {
					hard = append(hard, msg)
				} else {
					soft = append(soft, msg)
				}
			}
		}
	}

	// 3) Budget spend gate (reuses the computed ok/warning/hard_limit status).
	if budgets, err := s.ListAIBudgetSummaries(ctx); err == nil {
		for _, b := range budgets {
			if !budgetAppliesToRequest(b.Budget, rc) {
				continue
			}
			switch b.Status {
			case "hard_limit":
				hard = append(hard, fmt.Sprintf("budget exhausted ($%.2f/$%.2f %s)", b.ActualUSD, b.Budget.LimitUsd, scopeLabel(b.Budget)))
			case "warning":
				soft = append(soft, fmt.Sprintf("budget at %.0f%% (%s)", b.UsagePct, scopeLabel(b.Budget)))
			}
		}
	}

	for _, w := range soft {
		s.emitAIAudit(ctx, "ai_request", uuid.Nil, "governance_warning", w, map[string]any{"service": rc.ServiceSlug, "owner": rc.Owner}, rc.Owner)
	}
	if len(hard) > 0 {
		s.emitAIAudit(ctx, "ai_request", uuid.Nil, "governance_blocked", strings.Join(hard, "; "), map[string]any{"service": rc.ServiceSlug, "owner": rc.Owner}, rc.Owner)
		return &aiEnforcementError{reasons: hard}
	}
	return nil
}

// evaluateGatewayBudget blocks an AI gateway proxy call when a global or
// provider budget is at its hard limit (the spend gate for the proxy path) OR a
// token/cost quota with enforcement=block is exhausted (the throughput gate). It
// fails OPEN on a read error so an infra blip never wedges the proxy.
func (s *Service) evaluateGatewayBudget(ctx context.Context, providerName string) error {
	budgets, err := s.ListAIBudgetSummaries(ctx)
	if err != nil {
		return nil
	}
	rc := aiReqContext{ProviderName: providerName}
	for _, b := range budgets {
		scope := strings.ToLower(strings.TrimSpace(b.Budget.Scope))
		if scope != "" && scope != "global" && scope != "provider" {
			continue
		}
		if b.Status == "hard_limit" && budgetAppliesToRequest(b.Budget, rc) {
			s.emitAIAudit(ctx, "ai_gateway", uuid.Nil, "governance_blocked", "budget exhausted", map[string]any{"provider": providerName}, "")
			return &aiEnforcementError{reasons: []string{fmt.Sprintf("budget exhausted for provider %q", providerName)}}
		}
	}
	// Token / cost quotas (the gap the request-time path left to the gateway):
	// sum the current period's usage for the metric and block when a block-quota
	// is exhausted.
	if quotas, err := s.ListAIQuotas(ctx); err == nil && len(quotas) > 0 {
		usage, uerr := s.ListAIUsageRecords(ctx)
		if uerr == nil {
			for _, q := range quotas {
				metric := strings.ToLower(strings.TrimSpace(q.Metric))
				if metric != "tokens" && metric != "cost_usd" && metric != "cost" {
					continue // seats/instances are enforced at request time
				}
				if !strings.EqualFold(q.Enforcement, "block") {
					continue
				}
				used := aiQuotaUsage(metric, q.Period, usage)
				if used >= q.LimitQuantity {
					s.emitAIAudit(ctx, "ai_gateway", uuid.Nil, "governance_blocked",
						fmt.Sprintf("%s quota exhausted (%.0f/%.0f)", metric, used, q.LimitQuantity),
						map[string]any{"provider": providerName, "metric": metric}, "")
					return &aiEnforcementError{reasons: []string{
						fmt.Sprintf("%s quota exhausted (%.0f/%.0f %s)", metric, used, q.LimitQuantity, q.Period),
					}}
				}
			}
		}
	}
	return nil
}

// aiQuotaUsage sums usage for a metric over the quota's current period. A cost
// quota sums the cost_usd column of every row (gateway calls record cost on the
// "tokens" row) via aiCostUSD, which de-duplicates a provider's gateway estimate
// against an authoritative spend import so the two are not double-counted.
func aiQuotaUsage(metric, period string, usage []db.ListAIUsageRecordsRow) float64 {
	start := aiBudgetPeriodStart(period)
	if metric == "cost_usd" || metric == "cost" {
		return aiCostUSD(usage, start, func(db.ListAIUsageRecordsRow) bool { return true })
	}
	var total float64
	for _, u := range usage {
		if u.PeriodStart.Before(start) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(u.Metric), metric) { // tokens
			total += u.Quantity
		}
	}
	return total
}

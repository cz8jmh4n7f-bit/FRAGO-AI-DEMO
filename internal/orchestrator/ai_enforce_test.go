package orchestrator

import (
	"testing"
	"time"

	"github.com/cz8jmh4n7f-bit/opord-ai-demo/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestEmailDomain(t *testing.T) {
	cases := map[string]string{
		"alice@contractor.com": "contractor.com",
		"BOB@Corp.COM":         "corp.com",
		"noatsign":             "",
		"trailing@":            "",
		"":                     "",
	}
	for in, want := range cases {
		if got := emailDomain(in); got != want {
			t.Errorf("emailDomain(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMatchAny(t *testing.T) {
	if !matchAny(nil, "openai") {
		t.Error("empty patterns should be a wildcard (true)")
	}
	if !matchAny([]string{"openai"}, "OpenAI") {
		t.Error("should match case-insensitively")
	}
	if matchAny([]string{"openai"}, "anthropic") {
		t.Error("should not match a different value")
	}
	if !matchAny([]string{"name", "openai"}, "x", "openai") {
		t.Error("should match any candidate against any pattern")
	}
}

func TestPolicyRuleMatchesAndDeny(t *testing.T) {
	rc := aiReqContext{
		ServiceSlug: "claude-api-access", ServiceCategory: "api_access",
		ProviderName: "openai-main", ProviderType: "openai", Owner: "ext@contractor.com",
	}
	deny := aiPolicyRule{Effect: "deny", Providers: []string{"openai"}, OwnerDomains: []string{"contractor.com"}}
	if !deny.isDeny() || !deny.matches(rc) {
		t.Error("deny rule should match the contractor+openai request")
	}
	if (aiPolicyRule{Effect: "deny", Providers: []string{"anthropic"}}).matches(rc) {
		t.Error("rule for a different provider should not match")
	}
	if (aiPolicyRule{Effect: "allow"}).isDeny() {
		t.Error("effect=allow should not be a deny")
	}
	if !(aiPolicyRule{}).isDeny() {
		t.Error("empty effect should default to deny")
	}
}

// TestEmptyPolicyRuleIsNotDenyAll locks in the F14 fix: a policy whose rules object
// is empty/null unmarshals to a zero rule that isDeny()=true and matches()=true
// (empty selectors are wildcards). The enforcement loop additionally requires
// hasSelector(), so such a rule is INERT and does not block-all.
func TestEmptyPolicyRuleIsNotDenyAll(t *testing.T) {
	rc := aiReqContext{ServiceSlug: "claude-api-access", ProviderName: "openai-main", Owner: "a@corp.com"}
	empty := aiPolicyRule{} // what json.Unmarshal of {} or null yields
	if !empty.isDeny() || !empty.matches(rc) {
		t.Fatal("an empty rule is (deny + matches-everything) - that's exactly the footgun")
	}
	if empty.hasSelector() {
		t.Fatal("an empty rule must report NO selectors, so the enforcement guard skips it")
	}
	// A rule with only blank selector values is still selector-less.
	blank := aiPolicyRule{Providers: []string{"", "  "}, OwnerDomains: []string{""}}
	if blank.hasSelector() {
		t.Fatal("blank-only selectors must not count as a real selector")
	}
	// A real selector must register.
	if !(aiPolicyRule{Providers: []string{"openai"}}).hasSelector() {
		t.Fatal("a non-empty selector must be detected")
	}
}

func TestBudgetAppliesToRequest(t *testing.T) {
	tid := uuid.New()
	rc := aiReqContext{
		ProviderName: "openai-main", Owner: "a@corp.com", Workspace: "team-a",
		Tenant: pgtype.UUID{Bytes: tid, Valid: true},
	}
	checks := []struct {
		name string
		b    db.AiBudget
		want bool
	}{
		{"global always applies", db.AiBudget{Scope: "global"}, true},
		{"empty scope == global", db.AiBudget{Scope: ""}, true},
		{"provider match", db.AiBudget{Scope: "provider", ScopeRef: "openai-main"}, true},
		{"provider mismatch", db.AiBudget{Scope: "provider", ScopeRef: "anthropic-main"}, false},
		{"owner match", db.AiBudget{Scope: "owner", ScopeRef: "a@corp.com"}, true},
		{"workspace match", db.AiBudget{Scope: "workspace", ScopeRef: "team-a"}, true},
		{"tenant match", db.AiBudget{Scope: "tenant", ScopeRef: tid.String()}, true},
		{"tenant mismatch", db.AiBudget{Scope: "tenant", ScopeRef: uuid.New().String()}, false},
	}
	for _, c := range checks {
		if got := budgetAppliesToRequest(c.b, rc); got != c.want {
			t.Errorf("%s: budgetAppliesToRequest = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestBudgetScopeMatchesImportedUsage(t *testing.T) {
	// A workspace-scoped budget must count IMPORTED usage (instance_id NULL, so
	// the joined Workspace is nil) by the workspace name recorded in raw.
	b := db.AiBudget{Scope: "workspace", ScopeRef: "team-platform"}
	imported := db.ListAIUsageRecordsRow{
		Metric:  "cost_usd",
		CostUsd: 12.50,
		Raw:     []byte(`{"source":"anthropic_cost_report","workspace":"team-platform","workspace_id":"wrkspc_1"}`),
	}
	if !aiBudgetScopeMatches(b, imported) {
		t.Fatal("imported usage with matching raw workspace name must count toward the budget")
	}
	// Also match by the provider workspace id when the name isn't recorded.
	byID := db.AiBudget{Scope: "workspace", ScopeRef: "wrkspc_1"}
	if !aiBudgetScopeMatches(byID, imported) {
		t.Fatal("imported usage must also match a budget keyed by the workspace id")
	}
	// A different workspace must NOT match.
	other := db.AiBudget{Scope: "workspace", ScopeRef: "team-other"}
	if aiBudgetScopeMatches(other, imported) {
		t.Fatal("imported usage must not leak into an unrelated workspace budget")
	}
}

// TestAICostSupersession locks in the F1 fix: when a provider has BOTH a gateway
// estimate (opord_gateway_lite) and an authoritative import for the same period,
// only the authoritative figure counts - the estimate is dropped, not double-added.
// A provider with only an estimate keeps it.
func TestAICostSupersession(t *testing.T) {
	now := time.Now()
	usage := []db.ListAIUsageRecordsRow{
		// openai: estimate $4 + authoritative import $10 -> only $10 should count.
		{ProviderName: "openai-main", Metric: "tokens", CostUsd: 4, PeriodStart: now, Raw: []byte(`{"source":"opord_gateway_lite"}`)},
		{ProviderName: "openai-main", Metric: "cost_usd", CostUsd: 10, PeriodStart: now, Raw: []byte(`{"source":"openai_costs_api"}`)},
		// anthropic: estimate only -> the $3 estimate is kept (no import to supersede it).
		{ProviderName: "anthropic-main", Metric: "tokens", CostUsd: 3, PeriodStart: now, Raw: []byte(`{"source":"opord_gateway_lite"}`)},
	}
	got := aiCostUSD(usage, aiBudgetPeriodStart("monthly"), func(db.ListAIUsageRecordsRow) bool { return true })
	if got != 13 { // 10 (authoritative openai) + 3 (anthropic estimate), NOT 17
		t.Fatalf("aiCostUSD with supersession = %v, want 13 (10 authoritative + 3 estimate, no double-count)", got)
	}
	// The cost-quota path must agree (it routes through aiCostUSD).
	if q := aiQuotaUsage("cost_usd", "monthly", usage); q != 13 {
		t.Fatalf("cost quota usage = %v, want 13", q)
	}
}

func TestAIQuotaUsageSumsByMetric(t *testing.T) {
	usage := []db.ListAIUsageRecordsRow{
		{Metric: "tokens", Quantity: 1000, PeriodStart: time.Now()},
		{Metric: "tokens", Quantity: 500, PeriodStart: time.Now()},
		{Metric: "cost_usd", CostUsd: 7.25, PeriodStart: time.Now()},
		{Metric: "tokens", Quantity: 999, PeriodStart: time.Now().AddDate(0, -2, 0)}, // last period, excluded
	}
	if got := aiQuotaUsage("tokens", "monthly", usage); got != 1500 {
		t.Fatalf("token quota usage = %v, want 1500 (current period only)", got)
	}
	if got := aiQuotaUsage("cost_usd", "monthly", usage); got != 7.25 {
		t.Fatalf("cost quota usage = %v, want 7.25", got)
	}
}

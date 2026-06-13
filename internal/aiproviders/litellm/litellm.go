// Package litellm is an OPTIONAL, ADDITIONAL data-plane integration - ONE way to
// wire AI access, not the primary path (OPORD governs OpenAI/Anthropic directly
// too). When a `litellm` provider is configured, an approved access request mints
// a scoped LiteLLM VIRTUAL KEY (model allow-list + budget + expiry) via the
// proxy's admin API instead of handing out the org's real provider key. OPORD
// stays the governance control plane (request, approve, audit, budget, expiry);
// LiteLLM enforces the key at runtime. It's inert unless such a provider exists.
package litellm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cz8jmh4n7f-bit/opord-ai-demo/internal/aiproviders"
)

type Provider struct {
	client *http.Client
}

func Register(r *aiproviders.Registry) {
	r.Register(aiproviders.ProviderLiteLLM, func() aiproviders.Provider {
		return Provider{client: &http.Client{Timeout: 15 * time.Second}}
	})
}

func (Provider) Type() aiproviders.ProviderType { return aiproviders.ProviderLiteLLM }

func (p Provider) http() *http.Client {
	if p.client != nil {
		return p.client
	}
	return http.DefaultClient
}

// masterKey reads the LiteLLM proxy master key (sk-...), used as a Bearer token
// for the admin endpoints.
func masterKey(creds map[string]string) string {
	for _, k := range []string{"master_key", "api_key", "litellm_master_key", "token"} {
		if v := strings.TrimSpace(creds[k]); v != "" {
			return v
		}
	}
	return ""
}

func baseURL(cfg map[string]any) string {
	if v, ok := cfg["base_url"].(string); ok && strings.TrimSpace(v) != "" {
		return strings.TrimRight(strings.TrimSpace(v), "/")
	}
	return "http://litellm:4000"
}

// do performs an admin call against the LiteLLM proxy with the master key.
func (p Provider) do(ctx context.Context, creds map[string]string, cfg map[string]any, method, path string, in, out any) error {
	key := masterKey(creds)
	if key == "" {
		return fmt.Errorf("litellm master key missing (store it as master_key in the secret_ref)")
	}
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL(cfg)+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := p.http().Do(req)
	if err != nil {
		return fmt.Errorf("litellm request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(raw))
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return fmt.Errorf("litellm returned %s: %s", resp.Status, msg)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decoding litellm response: %w", err)
		}
	}
	return nil
}

func (p Provider) ValidateCredentials(ctx context.Context, req aiproviders.CredentialRequest) error {
	if masterKey(req.Credentials) == "" {
		return fmt.Errorf("litellm master key missing (store it as master_key in the secret_ref)")
	}
	// /v1/models requires the master key and proves the proxy is reachable + the
	// key is valid (401 on a bad key).
	if err := p.do(ctx, req.Credentials, req.Config, http.MethodGet, "/v1/models", nil, nil); err != nil {
		return fmt.Errorf("litellm credential check: %w", err)
	}
	return nil
}

func (Provider) ListAvailableServices(context.Context, aiproviders.ServiceListRequest) ([]aiproviders.Service, error) {
	return []aiproviders.Service{
		{
			Name:                  "LiteLLM Virtual Key",
			Slug:                  "litellm-virtual-key",
			Category:              "api_access",
			Description:           "A scoped, metered LiteLLM virtual key (model allow-list + budget + expiry). Real, usable, revocable - the org's provider key is never handed out.",
			RequestSchema:         requestSchema("owner", "workspace", "justification", "models", "max_budget", "expires_at"),
			DefaultExpirationDays: 30,
			RequiresApproval:      true,
		},
	}, nil
}

// ListModels returns the proxy's configured model list (OpenAI /v1/models format).
func (p Provider) ListModels(ctx context.Context, req aiproviders.ModelListRequest) ([]aiproviders.Model, error) {
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := p.do(ctx, req.Credentials, req.Config, http.MethodGet, "/v1/models", nil, &payload); err != nil {
		return nil, err
	}
	models := make([]aiproviders.Model, 0, len(payload.Data))
	for _, m := range payload.Data {
		if strings.TrimSpace(m.ID) == "" {
			continue
		}
		models = append(models, aiproviders.Model{
			Model: m.ID, DisplayName: m.ID, Modality: "text", Status: "active",
			Metadata: map[string]any{"source": "litellm_proxy"},
		})
	}
	return models, nil
}

// ProvisionAccess mints a real LiteLLM virtual key scoped to the request's models
// + budget + expiry. The raw key rides in Observed["virtual_key"]; the
// orchestrator moves it into OpenBao and strips it before persisting (never in
// the DB). ProviderAccessID is the non-secret key_alias so RevokeAccess can
// delete it without the raw key.
func (p Provider) ProvisionAccess(ctx context.Context, req aiproviders.ProvisionRequest) (*aiproviders.ProvisionResult, error) {
	if masterKey(req.Credentials) == "" {
		// No proxy configured - record a governance entitlement (manual key issue).
		accessID := deterministicID("litellm", req.RequestID.String(), req.Owner)
		return &aiproviders.ProvisionResult{
			ProviderAccessID: accessID,
			Observed: map[string]any{
				"provider": "litellm", "owner": req.Owner,
				"external_provisioning": "manual",
				"message":               "LiteLLM access approved; issue the virtual key in the proxy (no master_key configured for auto-minting).",
			},
		}, nil
	}

	alias := fmt.Sprintf("opord-%s-%s", safeSlug(req.Owner), shortID(req.RequestID.String()))
	spec := decodeSpec(req.Spec)
	gen := map[string]any{
		"key_alias": alias,
		"metadata": map[string]any{
			"opord_owner":     req.Owner,
			"opord_workspace": req.Workspace,
			"opord_request":   req.RequestID.String(),
		},
	}
	if models := stringList(spec["models"]); len(models) > 0 {
		gen["models"] = models
	}
	if b := numField(spec["max_budget"]); b > 0 {
		gen["max_budget"] = b
	}
	if req.ExpiresAt != nil {
		// LiteLLM accepts a duration string; derive whole days (min 1).
		days := int(time.Until(*req.ExpiresAt).Hours()/24) + 1
		if days < 1 {
			days = 1
		}
		gen["duration"] = fmt.Sprintf("%dd", days)
	}

	var out struct {
		Key     string `json:"key"`
		Expires string `json:"expires"`
	}
	if err := p.do(ctx, req.Credentials, req.Config, http.MethodPost, "/key/generate", gen, &out); err != nil {
		return nil, err
	}
	if strings.TrimSpace(out.Key) == "" {
		return nil, fmt.Errorf("litellm returned an empty key")
	}
	observed := map[string]any{
		"provider": "litellm", "owner": req.Owner, "workspace": req.Workspace,
		"key_alias":             alias,
		"virtual_key":           out.Key, // stripped to OpenBao by the orchestrator
		"external_provisioning": "minted",
		"message":               fmt.Sprintf("Minted LiteLLM virtual key %q (scoped, metered, revocable).", alias),
	}
	if models := stringList(spec["models"]); len(models) > 0 {
		observed["models"] = models
	}
	if b := numField(spec["max_budget"]); b > 0 {
		observed["max_budget"] = b
	}
	return &aiproviders.ProvisionResult{ProviderAccessID: alias, Observed: observed}, nil
}

// RevokeAccess deletes the virtual key by its alias. If alias-delete fails (older
// LiteLLM without key_aliases support) it falls back to delete-by-key when the raw
// key is available (the orchestrator reads it from the secret store and passes it
// as minted_key) - so a revoke can never silently leave a live, spending key.
func (p Provider) RevokeAccess(ctx context.Context, req aiproviders.RevokeRequest) error {
	if req.ProviderAccessID == "" {
		return fmt.Errorf("provider access id is required")
	}
	if masterKey(req.Credentials) == "" {
		return nil // governance-only record; nothing to delete upstream
	}
	err := p.do(ctx, req.Credentials, req.Config, http.MethodPost, "/key/delete",
		map[string]any{"key_aliases": []string{req.ProviderAccessID}}, nil)
	if err != nil {
		if mk := strings.TrimSpace(req.Credentials["minted_key"]); mk != "" {
			return p.do(ctx, req.Credentials, req.Config, http.MethodPost, "/key/delete",
				map[string]any{"keys": []string{mk}}, nil)
		}
		return err
	}
	return nil
}

// GetUsage pulls the key's REAL cumulative spend from LiteLLM. key/info by the
// raw key is authoritative (the minted_key the orchestrator reads from the secret
// store); it falls back to a by-alias lookup. Returns the current TOTAL spend
// (the importer stores it as one updated-in-place row, not additively).
func (p Provider) GetUsage(ctx context.Context, req aiproviders.UsageRequest) ([]aiproviders.UsageRecord, error) {
	if masterKey(req.Credentials) == "" {
		return nil, nil
	}
	var out struct {
		Info struct {
			Spend  float64  `json:"spend"`
			Models []string `json:"models"`
		} `json:"info"`
	}
	q := ""
	if k := strings.TrimSpace(req.Credentials["minted_key"]); k != "" {
		q = "/key/info?key=" + url.QueryEscape(k)
	} else if req.ProviderAccessID != "" {
		q = "/key/info?key_alias=" + url.QueryEscape(req.ProviderAccessID)
	} else {
		return nil, nil
	}
	if err := p.do(ctx, req.Credentials, req.Config, http.MethodGet, q, nil, &out); err != nil {
		return nil, err
	}
	return []aiproviders.UsageRecord{{
		Metric: "cost_usd", Quantity: out.Info.Spend, Unit: "usd", CostUSD: out.Info.Spend,
		Raw: map[string]any{"source": "litellm_spend", "provider_access_id": req.ProviderAccessID},
	}}, nil
}

func (Provider) GetStatus(_ context.Context, req aiproviders.StatusRequest) (*aiproviders.StatusResult, error) {
	return &aiproviders.StatusResult{Status: "active", Observed: map[string]any{"provider_access_id": req.ProviderAccessID}}, nil
}

// --- helpers ---

// decodeSpec returns the request spec FLATTENED: the AI request stores the
// request fields (models, max_budget, …) under a nested "metadata" object, so we
// merge that up to the top level (metadata wins) for easy lookup.
func decodeSpec(spec []byte) map[string]any {
	m := map[string]any{}
	if len(spec) > 0 {
		_ = json.Unmarshal(spec, &m)
	}
	if meta, ok := m["metadata"].(map[string]any); ok {
		for k, v := range meta {
			m[k] = v
		}
	}
	return m
}

func stringList(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

func numField(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	}
	return 0
}

func safeSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if r == '@' || r == '.' || r == ' ' || r == '_' {
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "user"
	}
	if len(out) > 24 {
		out = out[:24]
	}
	return out
}

func shortID(s string) string {
	s = strings.ReplaceAll(s, "-", "")
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

func deterministicID(parts ...string) string {
	return parts[0] + "-" + shortID(strings.Join(parts[1:], ""))
}

func requestSchema(fields ...string) map[string]any {
	return map[string]any{"fields": fields}
}

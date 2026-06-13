package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/cz8jmh4n7f-bit/opord-ai-demo/internal/aiproviders"
	"github.com/cz8jmh4n7f-bit/opord-ai-demo/internal/auth"
	"github.com/cz8jmh4n7f-bit/opord-ai-demo/internal/db"
)

// AIProviderInput registers an AI backend without storing raw provider secrets.
type AIProviderInput struct {
	Name      string
	Type      string
	Config    map[string]any
	SecretRef string
	Scopes    []string
}

// AIRequestSpec is persisted in requests.spec for kind=ai_service.
type AIRequestSpec struct {
	ServiceID     string         `json:"service_id,omitempty"`
	ServiceSlug   string         `json:"service_slug,omitempty"`
	Owner         string         `json:"owner,omitempty"`
	Workspace     string         `json:"workspace,omitempty"`
	Justification string         `json:"justification,omitempty"`
	ExpiresAt     string         `json:"expires_at,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// CreateAIRequestInput creates an AI service request through the existing
// requests workflow.
type CreateAIRequestInput struct {
	Name      string
	Requester string
	// ServiceID pins the exact catalog service (the UI sends it). ServiceSlug
	// alone is ambiguous when two providers of the same type advertise the same
	// slug - it resolves to an arbitrary one of them.
	ServiceID     string
	ServiceSlug   string
	Owner         string
	Workspace     string
	Justification string
	ExpiresAt     string
	Metadata      map[string]any
}

func validAIProviderType(t string) bool {
	switch aiproviders.ProviderType(t) {
	case aiproviders.ProviderMockAI, aiproviders.ProviderOpenAI, aiproviders.ProviderAnthropic,
		aiproviders.ProviderGemini, aiproviders.ProviderGitHubCopilot, aiproviders.ProviderCursor,
		aiproviders.ProviderLiteLLM:
		return true
	default:
		return false
	}
}

// CreateAIProvider records an AI provider instance. It does not touch external
// services; credential validation is a separate check endpoint.
func (s *Service) CreateAIProvider(ctx context.Context, in AIProviderInput) (db.AiProvider, error) {
	if strings.TrimSpace(in.Name) == "" {
		return db.AiProvider{}, fmt.Errorf("ai provider name is required")
	}
	if !validAIProviderType(in.Type) {
		return db.AiProvider{}, fmt.Errorf("unsupported ai provider type %q", in.Type)
	}
	cfg := in.Config
	if cfg == nil {
		cfg = map[string]any{}
	}
	if err := rejectAIProviderSecretConfig(cfg); err != nil {
		return db.AiProvider{}, err
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return db.AiProvider{}, fmt.Errorf("marshaling ai provider config: %w", err)
	}
	p, err := s.q.CreateAIProvider(ctx, db.CreateAIProviderParams{
		Name:     in.Name,
		Type:     in.Type,
		Config:   raw,
		TenantID: tenantForCreate(ctx),
	})
	if err != nil {
		return db.AiProvider{}, fmt.Errorf("creating ai provider: %w", err)
	}
	if strings.TrimSpace(in.SecretRef) != "" {
		// scopes is NOT NULL in the schema; a nil slice violates it. The web form
		// always sends scopes, but an API/curl caller may omit them - default to empty.
		scopes := in.Scopes
		if scopes == nil {
			scopes = []string{}
		}
		if _, err := s.q.CreateAIProviderCredential(ctx, db.CreateAIProviderCredentialParams{
			ProviderID: p.ID,
			SecretRef:  strings.TrimSpace(in.SecretRef),
			Scopes:     scopes,
		}); err != nil {
			return db.AiProvider{}, fmt.Errorf("creating ai provider credential ref: %w", err)
		}
	}
	if err := s.SyncAIProviderServices(ctx, p); err != nil {
		return db.AiProvider{}, err
	}
	s.emit("ai_provider", "registered", p.Name, "", p.Type, "AI provider registered")
	s.emitAIAudit(ctx, "ai_provider", p.ID, "registered", "AI provider registered", map[string]any{"name": p.Name, "type": p.Type, "secret_ref": in.SecretRef != ""}, "")
	return p, nil
}

// ErrAIProviderHasActiveInstances blocks deleting a provider whose services
// still have live access grants; the API maps it to 409.
var ErrAIProviderHasActiveInstances = errors.New("ai provider still has active access instances - revoke them first")

// UpdateAIProviderInput carries a partial provider update. Nil/empty fields are
// left unchanged; a non-nil SecretRef records a NEW credential row (rotation -
// the resolver always reads the latest), and an empty *SecretRef switches the
// provider back to the env-var key.
type UpdateAIProviderInput struct {
	Config    map[string]any
	SecretRef *string
	Status    string
}

// UpdateAIProvider merges config keys (a null value deletes the key), optionally
// rotates the credential reference, and/or flips status (active|disabled).
func (s *Service) UpdateAIProvider(ctx context.Context, name string, in UpdateAIProviderInput) (db.AiProvider, error) {
	p, err := s.q.GetAIProviderByName(ctx, name)
	if err != nil {
		return db.AiProvider{}, fmt.Errorf("ai provider %q not found: %w", name, err)
	}
	if tid, scoped := scopeTenant(ctx); scoped && p.TenantID.Valid && !tenantVisible(p.TenantID, tid) {
		return db.AiProvider{}, fmt.Errorf("ai provider %q not found", name)
	}
	cfg := aiProviderConfig(p)
	changed := []string{}
	if in.Config != nil {
		if err := rejectAIProviderSecretConfig(in.Config); err != nil {
			return db.AiProvider{}, err
		}
		for k, v := range in.Config {
			if v == nil {
				delete(cfg, k)
			} else {
				cfg[k] = v
			}
			changed = append(changed, "config."+k)
		}
	}
	status := p.Status
	if st := strings.TrimSpace(in.Status); st != "" {
		if st != "active" && st != "disabled" {
			return db.AiProvider{}, fmt.Errorf("status must be active or disabled")
		}
		status = st
		changed = append(changed, "status")
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return db.AiProvider{}, fmt.Errorf("marshaling ai provider config: %w", err)
	}
	updated, err := s.q.UpdateAIProvider(ctx, db.UpdateAIProviderParams{
		ID: p.ID, Name: p.Name, Type: p.Type, Config: raw, Status: status,
	})
	if err != nil {
		return db.AiProvider{}, fmt.Errorf("updating ai provider: %w", err)
	}
	if in.SecretRef != nil {
		if _, err := s.q.CreateAIProviderCredential(ctx, db.CreateAIProviderCredentialParams{
			ProviderID: p.ID,
			SecretRef:  strings.TrimSpace(*in.SecretRef),
			Scopes:     []string{},
		}); err != nil {
			return db.AiProvider{}, fmt.Errorf("rotating ai provider credential ref: %w", err)
		}
		changed = append(changed, "secret_ref")
	}
	s.emitAIAudit(ctx, "ai_provider", p.ID, "updated", "AI provider updated", map[string]any{"name": p.Name, "changed": changed}, "")
	return updated, nil
}

// DeleteAIProvider removes a provider, its catalog services, credential refs,
// usage, and TERMINAL (revoked/expired/failed) instance history. It refuses
// while any instance is still active/suspended, so live access is never
// silently orphaned.
func (s *Service) DeleteAIProvider(ctx context.Context, name string) error {
	p, err := s.q.GetAIProviderByName(ctx, name)
	if err != nil {
		return fmt.Errorf("ai provider %q not found: %w", name, err)
	}
	if tid, scoped := scopeTenant(ctx); scoped && p.TenantID.Valid && !tenantVisible(p.TenantID, tid) {
		return fmt.Errorf("ai provider %q not found", name)
	}
	active, err := s.q.CountActiveAIInstancesByProvider(ctx, p.ID)
	if err != nil {
		return fmt.Errorf("checking ai provider instances: %w", err)
	}
	if active > 0 {
		return fmt.Errorf("ai provider %q has %d active access instance(s): %w", name, active, ErrAIProviderHasActiveInstances)
	}
	if err := s.q.DeleteAIServiceInstancesByProvider(ctx, p.ID); err != nil {
		return fmt.Errorf("removing ai instance history: %w", err)
	}
	if err := s.q.DeleteAIProvider(ctx, p.ID); err != nil {
		return fmt.Errorf("deleting ai provider: %w", err)
	}
	s.emit("ai_provider", "deleted", p.Name, "", p.Type, "AI provider deleted")
	s.emitAIAudit(ctx, "ai_provider", p.ID, "deleted", "AI provider deleted", map[string]any{"name": p.Name, "type": p.Type}, "")
	return nil
}

// SyncAIProviderServicesByName imports catalog services for an existing AI
// provider. It is useful after adding a new provider implementation to a
// database that already has provider rows.
func (s *Service) SyncAIProviderServicesByName(ctx context.Context, name string) error {
	p, err := s.q.GetAIProviderByName(ctx, name)
	if err != nil {
		return fmt.Errorf("ai provider %q not found: %w", name, err)
	}
	if err := s.SyncAIProviderServices(ctx, p); err != nil {
		return err
	}
	s.emitAIAudit(ctx, "ai_provider", p.ID, "synced", "AI provider catalog services synced", map[string]any{"name": p.Name, "type": p.Type}, "")
	return nil
}

// SyncAIProviderServices imports the provider-advertised catalog entries into
// ai_services. Existing duplicate slugs are left untouched by the DB unique key.
func (s *Service) SyncAIProviderServices(ctx context.Context, p db.AiProvider) error {
	prov, err := s.aiProvider(p.Type)
	if err != nil {
		return err
	}
	services, err := prov.ListAvailableServices(ctx, aiproviders.ServiceListRequest{Credentials: s.aiCredentials(ctx, p), Config: aiProviderConfig(p)})
	if err != nil {
		return fmt.Errorf("listing ai provider services: %w", err)
	}
	for _, svc := range services {
		schema, _ := json.Marshal(svc.RequestSchema)
		if len(schema) == 0 || string(schema) == "null" {
			schema = []byte(`{}`)
		}
		if _, err := s.q.CreateAIService(ctx, db.CreateAIServiceParams{
			ProviderID:            p.ID,
			Name:                  svc.Name,
			Slug:                  svc.Slug,
			Category:              firstNonEmpty(svc.Category, "access"),
			Description:           svc.Description,
			RequestSchema:         schema,
			DefaultExpirationDays: int32(svc.DefaultExpirationDays),
			RequiresApproval:      svc.RequiresApproval,
		}); err != nil {
			if isUniqueViolation(err) {
				continue
			}
			return fmt.Errorf("creating ai service %q: %w", svc.Slug, err)
		}
	}
	return nil
}

// ListAIProviders returns AI providers visible to the caller.
func (s *Service) ListAIProviders(ctx context.Context) ([]db.AiProvider, error) {
	ps, err := s.q.ListAIProviders(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing ai providers: %w", err)
	}
	tid, scoped := scopeTenant(ctx)
	if !scoped {
		return ps, nil
	}
	out := make([]db.AiProvider, 0, len(ps))
	for _, p := range ps {
		if !p.TenantID.Valid || tenantVisible(p.TenantID, tid) {
			out = append(out, p)
		}
	}
	return out, nil
}

// CheckAIProvider validates credentials through the AI provider interface.
func (s *Service) CheckAIProvider(ctx context.Context, name string) error {
	p, err := s.q.GetAIProviderByName(ctx, name)
	if err != nil {
		return fmt.Errorf("ai provider %q not found: %w", name, err)
	}
	prov, err := s.aiProvider(p.Type)
	if err != nil {
		return err
	}
	if err := prov.ValidateCredentials(ctx, aiproviders.CredentialRequest{Credentials: s.aiCredentials(ctx, p), Config: aiProviderConfig(p)}); err != nil {
		s.emitAIAudit(ctx, "ai_provider", p.ID, "check_failed", err.Error(), map[string]any{"name": p.Name}, "")
		return err
	}
	s.emitAIAudit(ctx, "ai_provider", p.ID, "checked", "AI provider credentials validated", map[string]any{"name": p.Name}, "")
	return nil
}

// ListAIServices returns catalog services visible in the AI governance catalog.
func (s *Service) ListAIServices(ctx context.Context) ([]db.ListAIServicesRow, error) {
	rows, err := s.q.ListAIServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing ai services: %w", err)
	}
	return rows, nil
}

// ListAIRequests returns only AI service requests, reusing the generic request table.
func (s *Service) ListAIRequests(ctx context.Context) ([]db.Request, error) {
	rs, err := s.ListRequests(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]db.Request, 0, len(rs))
	for _, r := range rs {
		if r.Kind == "ai_service" {
			out = append(out, r)
		}
	}
	return out, nil
}

// CreateAIRequest records an AI access request as requests.kind=ai_service.
func (s *Service) CreateAIRequest(ctx context.Context, in CreateAIRequestInput) (*db.Request, error) {
	if strings.TrimSpace(in.Name) == "" || (strings.TrimSpace(in.ServiceSlug) == "" && strings.TrimSpace(in.ServiceID) == "") {
		return nil, fmt.Errorf("name and service_slug (or service_id) are required")
	}
	// Validate the expires_at format up front so the requester gets immediate
	// feedback instead of the approver hitting it at provision time.
	if _, err := aiExpiresAt(in.ExpiresAt, time.Time{}, 0); err != nil {
		return nil, err
	}
	service, err := s.lookupAIService(ctx, AIRequestSpec{ServiceID: strings.TrimSpace(in.ServiceID), ServiceSlug: strings.TrimSpace(in.ServiceSlug)})
	if err != nil {
		return nil, fmt.Errorf("ai service %q not found: %w", firstNonEmpty(in.ServiceSlug, in.ServiceID), err)
	}
	// Governance gate: block the request up front if an active policy/quota/budget
	// forbids it, so the requester gets immediate feedback (not at approval time).
	if err := s.evaluateAIGovernance(ctx, aiReqContext{
		ServiceID: service.ID, ServiceSlug: service.Slug, ServiceCategory: service.Category,
		ProviderName: service.ProviderName, ProviderType: service.ProviderType,
		Owner: firstNonEmpty(in.Owner, in.Requester), Workspace: in.Workspace, Tenant: tenantForCreate(ctx),
	}); err != nil {
		return nil, err
	}
	spec := AIRequestSpec{
		ServiceID:     service.ID.String(),
		ServiceSlug:   service.Slug,
		Owner:         firstNonEmpty(in.Owner, in.Requester),
		Workspace:     in.Workspace,
		Justification: in.Justification,
		ExpiresAt:     in.ExpiresAt,
		Metadata:      in.Metadata,
	}
	raw, _ := json.Marshal(spec)
	req, err := s.CreateRequest(ctx, CreateRequestInput{
		Name:      in.Name,
		Requester: in.Requester,
		Kind:      "ai_service",
		Provider:  service.ProviderName,
		Spec:      raw,
	})
	if err != nil {
		return nil, err
	}
	s.emitAIAudit(ctx, "ai_request", req.ID, "created", "AI service request created", map[string]any{
		"request": req.Name, "service": service.Slug, "provider": service.ProviderName,
	}, in.Requester)
	return req, nil
}

// ProvisionAIRequest grants access for an approved ai_service request and
// creates an AI service instance. It is called from the existing approval flow.
func (s *Service) ProvisionAIRequest(ctx context.Context, req db.Request) (*db.AiServiceInstance, error) {
	spec, err := aiRequestSpecOf(req.Spec)
	if err != nil {
		return nil, err
	}
	if spec.ServiceSlug == "" && spec.ServiceID == "" {
		return nil, fmt.Errorf("ai request missing service_slug or service_id")
	}
	service, err := s.lookupAIService(ctx, spec)
	if err != nil {
		return nil, err
	}
	provider, err := s.q.GetAIProvider(ctx, service.ProviderID)
	if err != nil {
		return nil, fmt.Errorf("loading ai provider: %w", err)
	}
	prov, err := s.aiProvider(provider.Type)
	if err != nil {
		return nil, err
	}

	owner := firstNonEmpty(spec.Owner, req.Requester, "unknown")
	workspace := firstNonEmpty(spec.Workspace, req.Environment, "default")
	expiresAt, err := aiExpiresAt(spec.ExpiresAt, req.CreatedAt, int(service.DefaultExpirationDays))
	if err != nil {
		return nil, err
	}
	// Governance gate again at approval - quota usage / budget spend may have
	// changed since the request was filed.
	if err := s.evaluateAIGovernance(ctx, aiReqContext{
		ServiceID: service.ID, ServiceSlug: service.Slug, ServiceCategory: service.Category,
		ProviderName: provider.Name, ProviderType: provider.Type,
		Owner: owner, Workspace: workspace, Tenant: req.TenantID,
	}); err != nil {
		s.emitAIAudit(ctx, "ai_request", req.ID, "provision_blocked", err.Error(), map[string]any{"request": req.Name}, req.DecidedBy)
		return nil, err
	}
	provService := aiProviderServiceFromRow(service)
	res, err := prov.ProvisionAccess(ctx, aiproviders.ProvisionRequest{
		RequestID: req.ID, ServiceID: service.ID, Service: provService,
		Owner: owner, Workspace: workspace, Spec: req.Spec, ExpiresAt: expiresAt,
		Credentials: s.aiCredentials(ctx, provider),
		Config:      aiProviderConfig(provider),
	})
	if err != nil {
		s.emitAIAudit(ctx, "ai_request", req.ID, "provision_failed", err.Error(), map[string]any{"request": req.Name}, req.DecidedBy)
		return nil, err
	}
	// A provider may mint a real credential (e.g. a LiteLLM virtual key). Never
	// persist it in the DB - move it to OpenBao and record only a pointer + masked
	// preview. An operator retrieves the raw key on demand via
	// GET /ai/instances/{id}/secret (RevealAIInstanceSecret), audited.
	s.stripMintedSecret(ctx, req.ID, res.Observed)
	observed, _ := json.Marshal(res.Observed)
	inst, err := s.q.CreateAIServiceInstance(ctx, db.CreateAIServiceInstanceParams{
		ServiceID:        service.ID,
		RequestID:        pgtype.UUID{Bytes: req.ID, Valid: true},
		ProviderAccessID: res.ProviderAccessID,
		Owner:            owner,
		TenantID:         req.TenantID,
		Workspace:        workspace,
		Status:           "active",
		Spec:             req.Spec,
		Observed:         observed,
		ProvisionedAt:    pgtype.Timestamptz{Time: time.Now(), Valid: true},
		ExpiresAt:        pgTime(expiresAt),
	})
	if err != nil {
		return nil, fmt.Errorf("creating ai service instance: %w", err)
	}
	s.recordAIUsage(ctx, provider.ID, inst.ID, prov, res.ProviderAccessID)
	s.emit("ai_service", "ready", inst.ID.String(), req.Environment, provider.Name, fmt.Sprintf("%s granted to %s", service.Slug, owner))
	s.emitAIAudit(ctx, "ai_instance", inst.ID, "created", "AI service instance created", map[string]any{
		"request": req.Name, "service": service.Slug, "provider": provider.Name, "owner": owner,
	}, req.DecidedBy)
	return &inst, nil
}

// ListAIInstances returns tenant-scoped AI service instances.
func (s *Service) ListAIInstances(ctx context.Context) ([]db.ListAIServiceInstancesRow, error) {
	rows, err := s.q.ListAIServiceInstances(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing ai instances: %w", err)
	}
	tid, scoped := scopeTenant(ctx)
	if !scoped {
		return rows, nil
	}
	out := make([]db.ListAIServiceInstancesRow, 0, len(rows))
	for _, r := range rows {
		if tenantVisible(r.TenantID, tid) {
			out = append(out, r)
		}
	}
	return out, nil
}

// stripMintedSecret moves a provider-minted credential (Observed["virtual_key"])
// out of the map into OpenBao and leaves a non-secret pointer + masked preview, so
// the raw key is never persisted in the DB (OPORD's never-store-raw-keys rule).
// Returns the raw value (the caller may surface it once). No-op when absent.
func (s *Service) stripMintedSecret(ctx context.Context, requestID uuid.UUID, observed map[string]any) string {
	raw, ok := observed["virtual_key"].(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return ""
	}
	delete(observed, "virtual_key")
	alias, _ := observed["key_alias"].(string)
	if alias == "" {
		alias = requestID.String()
	}
	path := "opord/ai/keys/" + alias
	if w, ok := s.creds.(interface {
		WriteSecret(ctx context.Context, path string, data map[string]string) error
	}); ok {
		if err := w.WriteSecret(ctx, path, map[string]string{"key": raw}); err != nil {
			s.log.Warn("could not store minted ai key in openbao; pointer only", "err", err)
		} else {
			observed["virtual_key_secret"] = path
		}
	}
	observed["virtual_key_preview"] = maskKey(raw)
	return raw
}

func maskKey(k string) string {
	if len(k) <= 8 {
		return "****"
	}
	return k[:4] + "…" + k[len(k)-4:]
}

// mintedKeyPath extracts the OpenBao path where a provider-minted key was stored
// (set by stripMintedSecret), or "" if this instance has no stored secret.
func mintedKeyPath(observed []byte) string {
	if len(observed) == 0 {
		return ""
	}
	var m map[string]any
	if json.Unmarshal(observed, &m) != nil {
		return ""
	}
	p, _ := m["virtual_key_secret"].(string)
	return strings.TrimSpace(p)
}

// readMintedKey reads a minted key back from the secret store (nil-safe).
func (s *Service) readMintedKey(ctx context.Context, path string) string {
	if path == "" || s.creds == nil {
		return ""
	}
	reader, ok := s.creds.(aiSecretReader)
	if !ok {
		return ""
	}
	sec, err := reader.ReadSecret(ctx, path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(sec["key"])
}

// RevealAIInstanceSecret returns the raw provider-minted credential (e.g. a
// LiteLLM virtual key) for an instance, read from the secret store on demand -
// the key is never in the DB. Operator-gated at the API; every reveal is audited.
func (s *Service) RevealAIInstanceSecret(ctx context.Context, id uuid.UUID, actor string) (string, error) {
	inst, err := s.q.GetAIServiceInstance(ctx, id)
	if err != nil {
		return "", fmt.Errorf("ai instance %s not found: %w", id, err)
	}
	if tid, scoped := scopeTenant(ctx); scoped && !tenantVisible(inst.TenantID, tid) {
		return "", fmt.Errorf("ai instance %s not found", id)
	}
	path := mintedKeyPath(inst.Observed)
	if path == "" {
		return "", fmt.Errorf("ai instance %s has no stored minted secret", id)
	}
	key := s.readMintedKey(ctx, path)
	if key == "" {
		return "", fmt.Errorf("minted secret not retrievable from the secret store")
	}
	s.emitAIAudit(ctx, "ai_instance", inst.ID, "secret_revealed", "minted AI credential revealed", map[string]any{"owner": inst.Owner}, actor)
	return key, nil
}

// ReapExpiredAIInstances revokes AI access instances whose expiry has passed -
// the access-governance safety net (SOC2/ISO: a grant must not outlive its
// approved window). It revokes through the provider (real workspace removal /
// invite deletion for Anthropic) and records the action as expiry, not a manual
// revoke. Runs as the system actor over ALL tenants (no scope in ctx).
func (s *Service) ReapExpiredAIInstances(ctx context.Context) (int, error) {
	rows, err := s.q.ListAIServiceInstances(ctx)
	if err != nil {
		return 0, fmt.Errorf("listing ai instances: %w", err)
	}
	now := time.Now()
	reaped := 0
	for _, r := range rows {
		if r.Status != "active" && r.Status != "suspended" {
			continue
		}
		if !r.ExpiresAt.Valid || r.ExpiresAt.Time.After(now) {
			continue
		}
		s.log.Info("ai access expired - auto-revoking", "instance", r.ID, "owner", r.Owner, "expired_at", r.ExpiresAt.Time)
		if _, err := s.RevokeAIInstance(ctx, r.ID, "system-expiry"); err != nil {
			s.log.Error("ai expiry auto-revoke failed", "instance", r.ID, "err", err)
			continue
		}
		s.emitAIAudit(ctx, "ai_instance", r.ID, "expired", "AI access auto-revoked on expiry", map[string]any{"owner": r.Owner}, "system-expiry")
		reaped++
	}
	return reaped, nil
}

// RevokeAIInstance revokes an AI access instance through the AI provider.
func (s *Service) RevokeAIInstance(ctx context.Context, id uuid.UUID, actor string) (*db.AiServiceInstance, error) {
	inst, err := s.q.GetAIServiceInstance(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("ai instance %s not found: %w", id, err)
	}
	if tid, scoped := scopeTenant(ctx); scoped && !tenantVisible(inst.TenantID, tid) {
		return nil, fmt.Errorf("ai instance %s not found", id)
	}
	// Only active/suspended instances are revocable; reject re-revoking an
	// already revoked/expired/failed instance instead of silently bumping revoked_at.
	if inst.Status != "active" && inst.Status != "suspended" {
		return nil, fmt.Errorf("ai instance %s is %q, not revocable (must be active or suspended)", id, inst.Status)
	}
	service, err := s.q.GetAIService(ctx, inst.ServiceID)
	if err != nil {
		return nil, fmt.Errorf("loading ai service: %w", err)
	}
	provider, err := s.q.GetAIProvider(ctx, service.ProviderID)
	if err != nil {
		return nil, fmt.Errorf("loading ai provider: %w", err)
	}
	prov, err := s.aiProvider(provider.Type)
	if err != nil {
		return nil, err
	}
	creds := s.aiCredentials(ctx, provider)
	// Hand the raw minted key (read from the secret store) to the revoke so a
	// provider can fall back to delete-by-key if delete-by-alias is unsupported -
	// closing the orphaned-key window.
	if k := s.readMintedKey(ctx, mintedKeyPath(inst.Observed)); k != "" {
		creds["minted_key"] = k
	}
	if err := prov.RevokeAccess(ctx, aiproviders.RevokeRequest{
		InstanceID: inst.ID, ProviderAccessID: inst.ProviderAccessID, Credentials: creds, Config: aiProviderConfig(provider),
	}); err != nil {
		s.emitAIAudit(ctx, "ai_instance", inst.ID, "revoke_failed", err.Error(), map[string]any{"provider": provider.Name}, actor)
		return nil, err
	}
	revoked, err := s.q.RevokeAIServiceInstance(ctx, inst.ID)
	if err != nil {
		return nil, fmt.Errorf("recording ai revoke: %w", err)
	}
	s.emit("ai_service", "revoked", inst.ID.String(), "", provider.Name, fmt.Sprintf("%s revoked", service.Slug))
	s.emitAIAudit(ctx, "ai_instance", inst.ID, "revoked", "AI service instance revoked", map[string]any{
		"service": service.Slug, "provider": provider.Name,
	}, actor)
	return &revoked, nil
}

func (s *Service) ListAIUsageRecords(ctx context.Context) ([]db.ListAIUsageRecordsRow, error) {
	rows, err := s.q.ListAIUsageRecords(ctx)
	if err != nil {
		return nil, err
	}
	tid, scoped := scopeTenant(ctx)
	if !scoped {
		return rows, nil
	}
	out := make([]db.ListAIUsageRecordsRow, 0, len(rows))
	for _, r := range rows {
		if tenantVisible(r.TenantID, tid) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *Service) ListAIAuditEvents(ctx context.Context, limit int32) ([]db.AiAuditEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.q.ListAIAuditEvents(ctx, limit)
	if err != nil {
		return nil, err
	}
	tid, scoped := scopeTenant(ctx)
	if !scoped {
		return rows, nil
	}
	out := make([]db.AiAuditEvent, 0, len(rows))
	for _, r := range rows {
		if !r.TenantID.Valid || tenantVisible(r.TenantID, tid) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *Service) aiProvider(t string) (aiproviders.Provider, error) {
	if s.ai == nil {
		return nil, fmt.Errorf("ai provider registry is not configured")
	}
	return s.ai.Get(aiproviders.ProviderType(t))
}

func aiProviderConfig(p db.AiProvider) map[string]any {
	cfg := map[string]any{}
	_ = json.Unmarshal(p.Config, &cfg)
	if cfg == nil {
		cfg = map[string]any{}
	}
	return cfg
}

func rejectAIProviderSecretConfig(cfg map[string]any) error {
	for key := range cfg {
		if isSensitiveAIConfigKey(key) {
			return fmt.Errorf("ai provider config must not include secret key %q; use secret_ref/OpenBao instead", key)
		}
	}
	return nil
}

func isSensitiveAIConfigKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "api_key", "openai_api_key", "anthropic_api_key", "admin_api_key", "token", "access_token",
		"bearer_token", "secret", "client_secret", "password", "master_key", "litellm_master_key":
		return true
	default:
		return false
	}
}

type aiSecretReader interface {
	ReadSecret(ctx context.Context, path string) (map[string]string, error)
}

func (s *Service) aiCredentials(ctx context.Context, p db.AiProvider) map[string]string {
	out := map[string]string{}
	if s.creds != nil {
		if cred, err := s.q.GetAIProviderCredentialByProvider(ctx, p.ID); err == nil && strings.TrimSpace(cred.SecretRef) != "" {
			if reader, ok := s.creds.(aiSecretReader); ok {
				if sec, serr := reader.ReadSecret(ctx, cred.SecretRef); serr == nil {
					for k, v := range sec {
						out[k] = v
					}
				}
			}
		}
	}
	return out
}

func aiRequestSpecOf(raw json.RawMessage) (AIRequestSpec, error) {
	var spec AIRequestSpec
	if len(raw) == 0 {
		return spec, nil
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		return spec, fmt.Errorf("decoding ai request spec: %w", err)
	}
	return spec, nil
}

func (s *Service) lookupAIService(ctx context.Context, spec AIRequestSpec) (db.GetAIServiceRow, error) {
	if spec.ServiceID != "" {
		id, err := uuid.Parse(spec.ServiceID)
		if err != nil {
			return db.GetAIServiceRow{}, fmt.Errorf("invalid service_id: %w", err)
		}
		return s.q.GetAIService(ctx, id)
	}
	row, err := s.q.GetAIServiceBySlug(ctx, spec.ServiceSlug)
	if err != nil {
		return db.GetAIServiceRow{}, err
	}
	return db.GetAIServiceRow(row), nil
}

func aiProviderServiceFromRow(svc db.GetAIServiceRow) aiproviders.Service {
	var schema map[string]any
	_ = json.Unmarshal(svc.RequestSchema, &schema)
	return aiproviders.Service{
		Name:                  svc.Name,
		Slug:                  svc.Slug,
		Category:              svc.Category,
		Description:           svc.Description,
		RequestSchema:         schema,
		DefaultExpirationDays: int(svc.DefaultExpirationDays),
		RequiresApproval:      svc.RequiresApproval,
	}
}

func aiExpiresAt(raw string, created time.Time, days int) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw != "" {
		for _, layout := range []string{time.RFC3339, "2006-01-02"} {
			t, err := time.Parse(layout, raw)
			if err == nil {
				return &t, nil
			}
		}
		return nil, fmt.Errorf("expires_at must be RFC3339 or YYYY-MM-DD")
	}
	if days <= 0 {
		return nil, nil
	}
	t := created.Add(time.Duration(days) * 24 * time.Hour)
	return &t, nil
}

func pgTime(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func (s *Service) recordAIUsage(ctx context.Context, providerID, instanceID uuid.UUID, prov aiproviders.Provider, accessID string) {
	now := time.Now()
	records, err := prov.GetUsage(ctx, aiproviders.UsageRequest{
		ProviderAccessID: accessID,
		PeriodStart:      now.Add(-24 * time.Hour),
		PeriodEnd:        now,
	})
	if err != nil {
		return
	}
	for _, rec := range records {
		raw, _ := json.Marshal(rec.Raw)
		_, _ = s.q.CreateAIUsageRecord(ctx, db.CreateAIUsageRecordParams{
			InstanceID:  pgtype.UUID{Bytes: instanceID, Valid: true},
			ProviderID:  providerID,
			PeriodStart: now.Add(-24 * time.Hour),
			PeriodEnd:   now,
			Metric:      rec.Metric,
			Quantity:    rec.Quantity,
			Unit:        rec.Unit,
			CostUsd:     rec.CostUSD,
			Raw:         raw,
		})
	}
}

func (s *Service) emitAIAudit(ctx context.Context, subjectType string, subjectID uuid.UUID, action, message string, fields map[string]any, fallbackActor string) {
	actor := fallbackActor
	tenant := tenantForCreate(ctx)
	if id, ok := auth.IdentityFrom(ctx); ok {
		actor = id.Email
		tenant = pgtype.UUID{Bytes: id.TenantID, Valid: id.TenantID != uuid.Nil}
	}
	if actor == "" {
		actor = "system"
	}
	raw, _ := json.Marshal(fields)
	_, _ = s.q.CreateAIAuditEvent(ctx, db.CreateAIAuditEventParams{
		Actor:       actor,
		TenantID:    tenant,
		SubjectType: subjectType,
		SubjectID:   pgtype.UUID{Bytes: subjectID, Valid: subjectID != uuid.Nil},
		Action:      action,
		Message:     message,
		Fields:      raw,
	})
}

package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cz8jmh4n7f-bit/opord-ai-demo/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Agent & MCP governance: a registry of approved MCP (Model Context Protocol)
// servers/tool endpoints + per-team grants, with an authorize check an agent
// runtime calls before it connects. Every decision is recorded in the AI audit
// trail (subject_type mcp_server / mcp_grant). This is the "which tools may an
// agent use" control layer most orgs lack.

// MCPServerInput registers an MCP server in the governed catalog.
type MCPServerInput struct {
	Name         string
	Transport    string // stdio | http | sse
	Endpoint     string
	Description  string
	RiskTier     string // low | medium | high | critical
	AllowedTools []string
}

// MCPAuthorizeResult is the verdict an agent runtime gets before connecting.
type MCPAuthorizeResult struct {
	Allowed  bool   `json:"allowed"`
	Reason   string `json:"reason"`
	Server   string `json:"server"`
	Owner    string `json:"owner"`
	Tool     string `json:"tool,omitempty"`
	RiskTier string `json:"riskTier,omitempty"`
}

func (s *Service) RegisterMCPServer(ctx context.Context, in MCPServerInput) (db.McpServer, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return db.McpServer{}, fmt.Errorf("mcp server name is required")
	}
	transport := strings.ToLower(strings.TrimSpace(in.Transport))
	if transport == "" {
		transport = "stdio"
	}
	risk := strings.ToLower(strings.TrimSpace(in.RiskTier))
	if risk == "" {
		risk = "medium"
	}
	tools := in.AllowedTools
	if tools == nil {
		tools = []string{}
	}
	rawTools, _ := json.Marshal(tools)
	srv, err := s.q.CreateMCPServer(ctx, db.CreateMCPServerParams{
		Name:         name,
		Transport:    transport,
		Endpoint:     strings.TrimSpace(in.Endpoint),
		Description:  strings.TrimSpace(in.Description),
		RiskTier:     risk,
		AllowedTools: rawTools,
		TenantID:     tenantForCreate(ctx),
	})
	if err != nil {
		return db.McpServer{}, fmt.Errorf("registering mcp server: %w", err)
	}
	s.emitAIAudit(ctx, "mcp_server", srv.ID, "registered", "MCP server registered",
		map[string]any{"name": name, "transport": transport, "risk_tier": risk}, "")
	return srv, nil
}

func (s *Service) ListMCPServers(ctx context.Context) ([]db.McpServer, error) {
	rows, err := s.q.ListMCPServers(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing mcp servers: %w", err)
	}
	tid, scoped := scopeTenant(ctx)
	if !scoped {
		return rows, nil
	}
	out := make([]db.McpServer, 0, len(rows))
	for _, r := range rows {
		if tenantVisible(r.TenantID, tid) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *Service) SetMCPServerStatus(ctx context.Context, name, status string) (db.McpServer, error) {
	srv, err := s.q.GetMCPServerByName(ctx, name)
	if err != nil {
		return db.McpServer{}, fmt.Errorf("mcp server %q not found: %w", name, err)
	}
	updated, err := s.q.UpdateMCPServerStatus(ctx, db.UpdateMCPServerStatusParams{ID: srv.ID, Status: status})
	if err != nil {
		return db.McpServer{}, err
	}
	s.emitAIAudit(ctx, "mcp_server", srv.ID, "status_changed", "MCP server status changed",
		map[string]any{"name": name, "status": status}, "")
	return updated, nil
}

func (s *Service) DeleteMCPServer(ctx context.Context, name string) error {
	srv, err := s.q.GetMCPServerByName(ctx, name)
	if err != nil {
		return fmt.Errorf("mcp server %q not found: %w", name, err)
	}
	if err := s.q.DeleteMCPServer(ctx, srv.ID); err != nil {
		return err
	}
	s.emitAIAudit(ctx, "mcp_server", srv.ID, "deleted", "MCP server deleted", map[string]any{"name": name}, "")
	return nil
}

// GrantMCPAccess allows a team/owner to use an MCP server (optionally with an
// expiry the AI reaper will enforce). Idempotent-ish: re-granting an
// already-active owner returns the existing grant.
func (s *Service) GrantMCPAccess(ctx context.Context, serverName, owner string, expiresAt *time.Time, grantedBy string) (db.McpGrant, error) {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return db.McpGrant{}, fmt.Errorf("owner is required")
	}
	srv, err := s.q.GetMCPServerByName(ctx, serverName)
	if err != nil {
		return db.McpGrant{}, fmt.Errorf("mcp server %q not found: %w", serverName, err)
	}
	if srv.Status != "active" {
		return db.McpGrant{}, fmt.Errorf("mcp server %q is %s - enable it before granting access", serverName, srv.Status)
	}
	if existing, err := s.q.FindActiveMCPGrant(ctx, db.FindActiveMCPGrantParams{ServerID: srv.ID, Lower: owner}); err == nil {
		return existing, nil
	}
	exp := pgtype.Timestamptz{}
	if expiresAt != nil {
		exp = pgtype.Timestamptz{Time: *expiresAt, Valid: true}
	}
	grant, err := s.q.CreateMCPGrant(ctx, db.CreateMCPGrantParams{
		ServerID: srv.ID, Owner: owner, ExpiresAt: exp, GrantedBy: grantedBy, TenantID: tenantForCreate(ctx), Status: "active",
	})
	if err != nil {
		return db.McpGrant{}, fmt.Errorf("granting mcp access: %w", err)
	}
	s.emitAIAudit(ctx, "mcp_grant", grant.ID, "granted", "MCP access granted",
		map[string]any{"server": serverName, "owner": owner, "risk_tier": srv.RiskTier}, grantedBy)
	return grant, nil
}

// RequestMCPAccess is the self-service path: a user requests access to a server,
// creating a 'requested' grant that does NOT authorize until an operator approves
// it. Reuses the existing latest grant when one is already active/requested.
func (s *Service) RequestMCPAccess(ctx context.Context, serverName, owner, requester string) (db.McpGrant, error) {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return db.McpGrant{}, fmt.Errorf("owner is required")
	}
	srv, err := s.q.GetMCPServerByName(ctx, serverName)
	if err != nil {
		return db.McpGrant{}, fmt.Errorf("mcp server %q not found: %w", serverName, err)
	}
	if existing, err := s.q.FindLatestMCPGrant(ctx, db.FindLatestMCPGrantParams{ServerID: srv.ID, Lower: owner}); err == nil &&
		(existing.Status == "active" || existing.Status == "requested") {
		return existing, nil
	}
	grant, err := s.q.CreateMCPGrant(ctx, db.CreateMCPGrantParams{
		ServerID: srv.ID, Owner: owner, GrantedBy: requester, TenantID: tenantForCreate(ctx), Status: "requested",
	})
	if err != nil {
		return db.McpGrant{}, fmt.Errorf("requesting mcp access: %w", err)
	}
	s.emitAIAudit(ctx, "mcp_grant", grant.ID, "requested", "MCP access requested",
		map[string]any{"server": serverName, "owner": owner, "risk_tier": srv.RiskTier}, requester)
	return grant, nil
}

// ApproveMCPGrant flips a 'requested' grant to 'active' (operator decision).
func (s *Service) ApproveMCPGrant(ctx context.Context, id uuid.UUID, actor string) (db.McpGrant, error) {
	g, err := s.q.SetMCPGrantStatus(ctx, db.SetMCPGrantStatusParams{ID: id, Status: "active"})
	if err != nil {
		return db.McpGrant{}, fmt.Errorf("approving mcp grant: %w", err)
	}
	s.emitAIAudit(ctx, "mcp_grant", g.ID, "approved", "MCP access approved", map[string]any{"owner": g.Owner}, actor)
	return g, nil
}

// RejectMCPGrant marks a 'requested' grant 'rejected'.
func (s *Service) RejectMCPGrant(ctx context.Context, id uuid.UUID, actor string) (db.McpGrant, error) {
	g, err := s.q.SetMCPGrantStatus(ctx, db.SetMCPGrantStatusParams{ID: id, Status: "rejected"})
	if err != nil {
		return db.McpGrant{}, fmt.Errorf("rejecting mcp grant: %w", err)
	}
	s.emitAIAudit(ctx, "mcp_grant", g.ID, "rejected", "MCP access rejected", map[string]any{"owner": g.Owner}, actor)
	return g, nil
}

func (s *Service) ListMCPGrants(ctx context.Context) ([]db.ListMCPGrantsRow, error) {
	rows, err := s.q.ListMCPGrants(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing mcp grants: %w", err)
	}
	tid, scoped := scopeTenant(ctx)
	if !scoped {
		return rows, nil
	}
	out := make([]db.ListMCPGrantsRow, 0, len(rows))
	for _, r := range rows {
		if tenantVisible(r.TenantID, tid) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *Service) RevokeMCPGrant(ctx context.Context, id uuid.UUID, actor string) (db.McpGrant, error) {
	g, err := s.q.RevokeMCPGrant(ctx, id)
	if err != nil {
		return db.McpGrant{}, fmt.Errorf("revoking mcp grant: %w", err)
	}
	s.emitAIAudit(ctx, "mcp_grant", g.ID, "revoked", "MCP access revoked", map[string]any{"owner": g.Owner}, actor)
	return g, nil
}

// AuthorizeMCP is the enforcement point: an agent runtime calls this before
// connecting to an MCP server. It checks the server is registered + active, the
// owner has an active non-expired grant, and (if a tool is named) the tool is in
// the server's allow-list. Every decision is audited - the agent-tool audit trail.
func (s *Service) AuthorizeMCP(ctx context.Context, serverName, owner, tool string) (MCPAuthorizeResult, error) {
	res := MCPAuthorizeResult{Server: serverName, Owner: owner, Tool: tool}
	srv, err := s.q.GetMCPServerByName(ctx, serverName)
	if err != nil {
		res.Reason = "mcp server not registered in the governed catalog"
		s.auditMCPAuthz(ctx, uuid.Nil, res)
		return res, nil
	}
	res.RiskTier = srv.RiskTier
	if srv.Status != "active" {
		res.Reason = fmt.Sprintf("mcp server is %s", srv.Status)
		s.auditMCPAuthz(ctx, srv.ID, res)
		return res, nil
	}
	grant, err := s.q.FindActiveMCPGrant(ctx, db.FindActiveMCPGrantParams{ServerID: srv.ID, Lower: strings.TrimSpace(owner)})
	if err != nil {
		if err == pgx.ErrNoRows {
			res.Reason = "no active grant for this owner - request access first"
		} else {
			res.Reason = "authorization lookup failed"
		}
		s.auditMCPAuthz(ctx, srv.ID, res)
		return res, nil
	}
	if grant.ExpiresAt.Valid && grant.ExpiresAt.Time.Before(time.Now()) {
		res.Reason = "grant has expired"
		s.auditMCPAuthz(ctx, srv.ID, res)
		return res, nil
	}
	if strings.TrimSpace(tool) != "" {
		var allowed []string
		_ = json.Unmarshal(srv.AllowedTools, &allowed)
		if len(allowed) > 0 && !containsFold(allowed, tool) {
			res.Reason = fmt.Sprintf("tool %q is not in the server's allow-list", tool)
			s.auditMCPAuthz(ctx, srv.ID, res)
			return res, nil
		}
	}
	res.Allowed = true
	res.Reason = "authorized"
	s.auditMCPAuthz(ctx, srv.ID, res)
	return res, nil
}

func (s *Service) auditMCPAuthz(ctx context.Context, serverID uuid.UUID, res MCPAuthorizeResult) {
	action := "authz_denied"
	if res.Allowed {
		action = "authz_allowed"
	}
	s.emitAIAudit(ctx, "mcp_server", serverID, action, res.Reason,
		map[string]any{"server": res.Server, "owner": res.Owner, "tool": res.Tool}, res.Owner)
}

func containsFold(list []string, v string) bool {
	for _, item := range list {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(v)) {
			return true
		}
	}
	return false
}

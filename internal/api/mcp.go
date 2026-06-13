package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/cz8jmh4n7f-bit/opord-ai-demo/internal/db"
	"github.com/cz8jmh4n7f-bit/opord-ai-demo/internal/orchestrator"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Agent & MCP governance HTTP surface: a registry of approved MCP servers,
// per-team grants, and the authorize check an agent runtime calls before
// connecting. Reads viewer+, writes operator+; authorize is viewer+ (an agent
// presents a viewer-scoped key to ask "may I use this server?").

type mcpServerDTO struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Transport    string   `json:"transport"`
	Endpoint     string   `json:"endpoint"`
	Description  string   `json:"description"`
	RiskTier     string   `json:"riskTier"`
	AllowedTools []string `json:"allowedTools"`
	Status       string   `json:"status"`
	CreatedAt    string   `json:"createdAt"`
}

func mcpServerToDTO(s db.McpServer) mcpServerDTO {
	tools := []string{}
	_ = json.Unmarshal(s.AllowedTools, &tools)
	return mcpServerDTO{
		ID: s.ID.String(), Name: s.Name, Transport: s.Transport, Endpoint: s.Endpoint,
		Description: s.Description, RiskTier: s.RiskTier, AllowedTools: tools,
		Status: s.Status, CreatedAt: s.CreatedAt.Format(time.RFC3339),
	}
}

type mcpGrantDTO struct {
	ID        string `json:"id"`
	Server    string `json:"server"`
	RiskTier  string `json:"riskTier"`
	Owner     string `json:"owner"`
	Status    string `json:"status"`
	ExpiresAt string `json:"expiresAt,omitempty"`
	GrantedBy string `json:"grantedBy,omitempty"`
	CreatedAt string `json:"createdAt"`
}

func (s *Server) listMCPServers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.svc.ListMCPServers(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	out := make([]mcpServerDTO, 0, len(rows))
	for _, m := range rows {
		out = append(out, mcpServerToDTO(m))
	}
	writeJSON(w, http.StatusOK, out)
}

type registerMCPServerReq struct {
	Name         string   `json:"name"`
	Transport    string   `json:"transport"`
	Endpoint     string   `json:"endpoint"`
	Description  string   `json:"description"`
	RiskTier     string   `json:"riskTier"`
	AllowedTools []string `json:"allowedTools"`
}

func (s *Server) registerMCPServer(w http.ResponseWriter, r *http.Request) {
	var req registerMCPServerReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	srv, err := s.svc.RegisterMCPServer(r.Context(), orchestrator.MCPServerInput{
		Name: req.Name, Transport: req.Transport, Endpoint: req.Endpoint,
		Description: req.Description, RiskTier: req.RiskTier, AllowedTools: req.AllowedTools,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, mcpServerToDTO(srv))
}

func (s *Server) deleteMCPServer(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.DeleteMCPServer(r.Context(), chi.URLParam(r, "name")); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) listMCPGrants(w http.ResponseWriter, r *http.Request) {
	rows, err := s.svc.ListMCPGrants(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	out := make([]mcpGrantDTO, 0, len(rows))
	for _, g := range rows {
		d := mcpGrantDTO{
			ID: g.ID.String(), Server: g.ServerName, RiskTier: g.ServerRiskTier,
			Owner: g.Owner, Status: g.Status, GrantedBy: g.GrantedBy,
			CreatedAt: g.CreatedAt.Format(time.RFC3339),
		}
		if g.ExpiresAt.Valid {
			d.ExpiresAt = g.ExpiresAt.Time.Format(time.RFC3339)
		}
		out = append(out, d)
	}
	writeJSON(w, http.StatusOK, out)
}

type grantMCPReq struct {
	Server    string `json:"server"`
	Owner     string `json:"owner"`
	ExpiresAt string `json:"expiresAt"`
	GrantedBy string `json:"grantedBy"`
}

func (s *Server) grantMCPAccess(w http.ResponseWriter, r *http.Request) {
	var req grantMCPReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	var exp *time.Time
	if req.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, req.ExpiresAt)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		exp = &t
	}
	g, err := s.svc.GrantMCPAccess(r.Context(), req.Server, req.Owner, exp, req.GrantedBy)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": g.ID.String(), "owner": g.Owner, "status": g.Status})
}

type requestMCPReq struct {
	Server    string `json:"server"`
	Owner     string `json:"owner"`
	Requester string `json:"requester"`
}

// requestMCPAccess is the self-service request (viewer+); creates a pending grant.
func (s *Server) requestMCPAccess(w http.ResponseWriter, r *http.Request) {
	var req requestMCPReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	g, err := s.svc.RequestMCPAccess(r.Context(), req.Server, req.Owner, req.Requester)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": g.ID.String(), "owner": g.Owner, "status": g.Status})
}

func (s *Server) approveMCPGrant(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	var body decisionReq
	_ = json.NewDecoder(r.Body).Decode(&body)
	g, err := s.svc.ApproveMCPGrant(r.Context(), id, body.By)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": g.ID.String(), "status": g.Status})
}

func (s *Server) rejectMCPGrant(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	var body decisionReq
	_ = json.NewDecoder(r.Body).Decode(&body)
	g, err := s.svc.RejectMCPGrant(r.Context(), id, body.By)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": g.ID.String(), "status": g.Status})
}

func (s *Server) revokeMCPGrant(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	var body decisionReq
	_ = json.NewDecoder(r.Body).Decode(&body)
	g, err := s.svc.RevokeMCPGrant(r.Context(), id, body.By)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": g.ID.String(), "status": g.Status})
}

// authorizeMCP is the enforcement endpoint an agent runtime calls before
// connecting: GET /ai/mcp/authorize?server=&owner=&tool= -> {allowed, reason}.
func (s *Server) authorizeMCP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	res, err := s.svc.AuthorizeMCP(r.Context(), q.Get("server"), q.Get("owner"), q.Get("tool"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	status := http.StatusOK
	if !res.Allowed {
		status = http.StatusForbidden
	}
	writeJSON(w, status, res)
}

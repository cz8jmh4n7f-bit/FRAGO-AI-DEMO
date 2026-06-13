// Package api exposes the OPORD orchestrator over HTTP. Handlers are thin: they
// decode requests, call the same orchestrator.Service the CLI uses, and encode
// DTOs. No business logic lives here.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/cz8jmh4n7f-bit/opord-ai-demo/internal/auth"
	"github.com/cz8jmh4n7f-bit/opord-ai-demo/internal/jobs"
	"github.com/cz8jmh4n7f-bit/opord-ai-demo/internal/models"
	"github.com/cz8jmh4n7f-bit/opord-ai-demo/internal/orchestrator"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// JobLister reads the durable job queue (River). Optional: when nil, the queue
// endpoint returns an empty list (e.g. River not configured).
type JobLister interface {
	ListJobs(ctx context.Context, limit int) ([]jobs.JobInfo, error)
}

// Server holds the dependencies for the HTTP API.
type Server struct {
	svc         *orchestrator.Service
	queue       JobLister
	authResolve auth.Resolver
	authEnabled bool
	log         *slog.Logger
}

// NewServer builds the API server.
func NewServer(svc *orchestrator.Service, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{svc: svc, log: log}
}

// SetJobLister wires the durable queue so /api/v1/queue can report River jobs.
func (s *Server) SetJobLister(q JobLister) { s.queue = q }

// SetAuth wires API-key authentication. When enabled is false, the API runs
// open (dev mode) with a default admin identity.
func (s *Server) SetAuth(resolve auth.Resolver, enabled bool) {
	s.authResolve = resolve
	s.authEnabled = enabled
}

// Routes returns the configured HTTP handler.
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(cors)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(auth.Middleware(s.authResolve, s.authEnabled))

		// Reads: viewer and up.
		r.Group(func(r chi.Router) {
			r.Use(auth.RequireRole(auth.RoleViewer))
			r.Get("/me", s.getMe)
			r.Get("/providers", s.listProviders)
			r.Get("/providers/{name}/readiness", s.getProviderReadiness)
			r.Get("/providers/{name}/images", s.listProviderImages)
			r.Get("/providers/{name}/cluster-versions", s.listProviderClusterVersions)
			r.Get("/providers/{name}/billing-scopes", s.listProviderBillingScopes)
			r.Get("/clusters", s.listClusters)
			r.Get("/clusters/{name}", s.getCluster)
			r.Get("/vms", s.listVMs)
			r.Get("/vms/{name}", s.getVM)
			r.Get("/stacks", s.listStacks)
			r.Get("/stacks/{name}", s.getStack)
			r.Get("/databases", s.listDatabases)
			r.Get("/databases/{name}", s.getDatabase)
			r.Get("/tables", s.listTables)
			r.Get("/tables/{name}", s.getTable)
			r.Get("/functions", s.listFunctions)
			r.Get("/functions/{name}", s.getFunction)
			r.Get("/s3", s.listS3)
			r.Get("/s3/{name}", s.getS3)
			r.Get("/secrets", s.listSecrets)
			r.Get("/secrets/{name}", s.getSecret)
			r.Get("/queues", s.listQueues)
			r.Get("/queues/{name}", s.getQueue)
			r.Get("/caches", s.listCaches)
			r.Get("/caches/{name}", s.getCache)
			r.Get("/projects", s.listProjects)
			r.Get("/projects/{name}", s.getProject)
			r.Get("/accounts", s.listAccounts)
			r.Get("/accounts/{name}", s.getAccount)
			r.Get("/dns", s.listDNS)
			r.Get("/dns/{name}", s.getDNS)
			r.Get("/certs", s.listCert)
			r.Get("/certs/{name}", s.getCert)
			r.Get("/loadbalancers", s.listLoadBalancers)
			r.Get("/loadbalancers/{name}", s.getLoadBalancer)
			r.Get("/apigateways", s.listAPIGateways)
			r.Get("/apigateways/{name}", s.getAPIGateway)
			r.Get("/cdns", s.listCDN)
			r.Get("/cdns/{name}", s.getCDN)
			r.Get("/requests", s.listRequests)
			r.Get("/requests/{name}", s.getRequest)
			r.Get("/blueprints", s.listBlueprints)
			r.Get("/environments", s.listEnvironments)
			r.Get("/environments/{name}", s.getEnvironment)
			r.Get("/queue", s.listQueue)
			r.Get("/cost", s.getCost)
			r.Get("/finops", s.getFinOps)
			r.Get("/compliance", s.getCompliance)
			r.Get("/ai/providers", s.listAIProviders)
			r.Get("/ai/services", s.listAIServices)
			r.Get("/ai/requests", s.listAIRequests)
			r.Get("/ai/instances", s.listAIInstances)
			r.Get("/ai/usage", s.listAIUsage)
			r.Get("/ai/budgets", s.listAIBudgets)
			r.Get("/ai/quotas", s.listAIQuotas)
			r.Get("/ai/policies", s.listAIPolicies)
			r.Get("/ai/models", s.listAIModels)
			r.Get("/ai/renewals", s.listAIRenewals)
			r.Get("/ai/access-review", s.listAIAccessReview)
			r.Get("/ai/audit", s.listAIAudit)
			// AI org administration reads (ADR-0022).
			r.Get("/ai/admin/{name}/users", s.listAIOrgUsers)
			r.Get("/ai/admin/{name}/workspaces", s.listAIWorkspaces)
			r.Get("/ai/admin/{name}/invites", s.listAIInvites)
			r.Get("/ai/admin/{name}/workspaces/{wsID}/access", s.listAIWorkspaceAccess)
			// Agent & MCP governance reads + the authorize enforcement check.
			r.Get("/ai/mcp/servers", s.listMCPServers)
			r.Get("/ai/mcp/grants", s.listMCPGrants)
			r.Get("/ai/mcp/authorize", s.authorizeMCP)
			r.Post("/ai/mcp/requests", s.requestMCPAccess) // self-service request (viewer+)
		})

		// Writes: operator and up.
		r.Group(func(r chi.Router) {
			r.Use(auth.RequireRole(auth.RoleOperator))
			r.Post("/providers", s.createProvider)
			r.Patch("/providers/{name}", s.updateProvider)
			r.Delete("/providers/{name}", s.deleteProvider)
			r.Post("/providers/{name}/check", s.checkProvider)
			r.Post("/clusters", s.createCluster)
			r.Post("/clusters/{name}/scale", s.scaleCluster)
			r.Delete("/clusters/{name}", s.destroyCluster)
			r.Post("/vms", s.createVM)
			r.Post("/vms/{name}/scale", s.scaleVM)
			r.Delete("/vms/{name}", s.destroyVM)
			r.Post("/stacks", s.createStack)
			r.Delete("/stacks/{name}", s.destroyStack)
			r.Post("/databases", s.createDatabase)
			r.Delete("/databases/{name}", s.destroyDatabase)
			r.Post("/tables", s.createTable)
			r.Delete("/tables/{name}", s.destroyTable)
			r.Post("/functions", s.createFunction)
			r.Delete("/functions/{name}", s.destroyFunction)
			r.Post("/s3", s.createS3)
			r.Delete("/s3/{name}", s.destroyS3)
			r.Post("/secrets", s.createSecret)
			r.Delete("/secrets/{name}", s.destroySecret)
			r.Post("/queues", s.createQueue)
			r.Delete("/queues/{name}", s.destroyQueue)
			r.Post("/caches", s.createCache)
			r.Delete("/caches/{name}", s.destroyCache)
			r.Post("/projects", s.createProject)
			r.Post("/projects/{name}/members", s.setProjectMembers)
			r.Delete("/projects/{name}", s.destroyProject)
			r.Post("/accounts", s.createAccount)
			r.Delete("/accounts/{name}", s.destroyAccount)
			r.Post("/dns", s.createDNS)
			r.Delete("/dns/{name}", s.destroyDNS)
			r.Post("/certs", s.createCert)
			r.Delete("/certs/{name}", s.destroyCert)
			r.Post("/loadbalancers", s.createLoadBalancer)
			r.Delete("/loadbalancers/{name}", s.destroyLoadBalancer)
			r.Post("/apigateways", s.createAPIGateway)
			r.Delete("/apigateways/{name}", s.destroyAPIGateway)
			r.Post("/cdns", s.createCDN)
			r.Delete("/cdns/{name}", s.destroyCDN)
			r.Post("/entra/grant", s.grantEntra)
			r.Post("/requests", s.createRequest)
			r.Post("/requests/{name}/approve", s.approveRequest)
			r.Post("/requests/{name}/reject", s.rejectRequest)
			r.Post("/ai/providers", s.createAIProvider)
			r.Patch("/ai/providers/{name}", s.updateAIProvider)
			r.Delete("/ai/providers/{name}", s.deleteAIProvider)
			r.Post("/ai/providers/{name}/check", s.checkAIProvider)
			r.Post("/ai/providers/{name}/sync", s.syncAIProvider)
			r.Post("/ai/providers/{name}/sync-models", s.syncAIProviderModels)
			r.Post("/ai/requests", s.createAIRequest)
			r.Post("/ai/requests/{name}/approve", s.approveAIRequest)
			r.Post("/ai/requests/{name}/reject", s.rejectAIRequest)
			r.Post("/ai/instances/{id}/revoke", s.revokeAIInstance)
			r.Get("/ai/instances/{id}/secret", s.revealAIInstanceSecret)
			r.Post("/ai/instances/{id}/recertify", s.recertifyAIInstance)
			r.Post("/ai/instances/reap-expired", s.reapExpiredAIInstances)
			// Agent & MCP governance writes.
			r.Post("/ai/mcp/servers", s.registerMCPServer)
			r.Delete("/ai/mcp/servers/{name}", s.deleteMCPServer)
			r.Post("/ai/mcp/grants", s.grantMCPAccess)
			r.Post("/ai/mcp/grants/{id}/approve", s.approveMCPGrant)
			r.Post("/ai/mcp/grants/{id}/reject", s.rejectMCPGrant)
			r.Post("/ai/mcp/grants/{id}/revoke", s.revokeMCPGrant)
			r.Post("/ai/budgets", s.createAIBudget)
			r.Patch("/ai/budgets/{id}", s.updateAIBudget)
			r.Delete("/ai/budgets/{id}", s.deleteAIBudget)
			r.Post("/ai/quotas", s.createAIQuota)
			r.Patch("/ai/quotas/{id}", s.updateAIQuota)
			r.Delete("/ai/quotas/{id}", s.deleteAIQuota)
			r.Post("/ai/policies", s.createAIPolicy)
			r.Patch("/ai/policies/{id}", s.updateAIPolicy)
			r.Delete("/ai/policies/{id}", s.deleteAIPolicy)
			r.Post("/ai/usage/import/openai", s.importOpenAIUsage)
			r.Post("/ai/usage/import/anthropic", s.importAnthropicUsage)
			r.Post("/ai/usage/import/litellm", s.importLiteLLMSpend)
			// AI org administration writes (ADR-0022): invite/role/workspace/access.
			r.Post("/ai/admin/{name}/invites", s.inviteAIOrgUser)
			r.Post("/ai/admin/{name}/users/{userID}/role", s.setAIOrgRole)
			r.Delete("/ai/admin/{name}/users/{userID}", s.removeAIOrgUser)
			r.Post("/ai/admin/{name}/workspaces", s.createAIWorkspace)
			r.Post("/ai/admin/{name}/workspaces/{wsID}/archive", s.archiveAIWorkspace)
			r.Post("/ai/admin/{name}/workspaces/{wsID}/members", s.grantAIWorkspaceAccess)
			r.Delete("/ai/admin/{name}/workspaces/{wsID}/members/{userID}", s.removeAIWorkspaceMember)
			r.Post("/ai/gateway/openai/responses", s.gatewayOpenAIResponses)
			r.Post("/ai/gateway/anthropic/messages", s.gatewayAnthropicMessages)
			r.Post("/environments", s.createEnvironment)
			r.Delete("/environments/{name}", s.destroyEnvironment)
		})
	})

	return r
}

func (s *Server) listProviders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	provs, err := s.svc.ListProviders(ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	// Count clusters per provider name.
	counts := map[string]int{}
	if summaries, err := s.svc.ListClusters(ctx); err == nil {
		for _, c := range summaries {
			counts[c.Provider]++
		}
	}
	out := make([]providerDTO, 0, len(provs))
	for _, p := range provs {
		out = append(out, providerToDTO(p, counts[p.Name]))
	}
	writeJSON(w, http.StatusOK, out)
}

type createProviderReq struct {
	Name      string         `json:"name"`
	Type      string         `json:"type"`
	Config    map[string]any `json:"config"`
	SecretRef string         `json:"secretRef"`
}

func (s *Server) createProvider(w http.ResponseWriter, r *http.Request) {
	var req createProviderReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	p, err := s.svc.AddProvider(r.Context(), orchestrator.ProviderInput{
		Name:      req.Name,
		Type:      req.Type,
		Config:    req.Config,
		SecretRef: req.SecretRef,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, providerToDTO(p, 0))
}

func (s *Server) listClusters(w http.ResponseWriter, r *http.Request) {
	summaries, err := s.svc.ListClusters(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]clusterDTO, 0, len(summaries))
	for _, c := range summaries {
		out = append(out, clusterSummaryToDTO(c))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getCluster(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	env := r.URL.Query().Get("env")
	if env == "" {
		env = "dev"
	}
	live := r.URL.Query().Get("live") == "true"
	d, err := s.svc.ClusterStatus(r.Context(), name, env, live)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, clusterDetailToDTO(d))
}

// destroyCluster tears down a cluster (tofu destroy). The lookup runs
// synchronously (so a missing cluster returns 404), but the apply itself - which
// can take many minutes - runs in the background; status flows destroying ->
// destroyed/failed.
func (s *Server) destroyCluster(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	env := r.URL.Query().Get("env")
	if env == "" {
		env = "dev"
	}
	if _, err := s.svc.ClusterStatus(r.Context(), name, env, false); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	if r.URL.Query().Get("purge") == "true" {
		if err := s.svc.DeleteClusterRecord(r.Context(), name, env); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"name": name, "status": "removed"})
		return
	}
	s.svc.DestroyClusterAsync(name, env)
	writeJSON(w, http.StatusAccepted, map[string]any{"name": name, "status": "destroying"})
}

type scaleClusterReq struct {
	Workers int `json:"workers"`
}

// scaleCluster changes a cluster's worker count and re-provisions (day-2).
func (s *Server) scaleCluster(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	env := r.URL.Query().Get("env")
	if env == "" {
		env = "dev"
	}
	var req scaleClusterReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.svc.ScaleCluster(r.Context(), name, env, req.Workers); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"name": name, "workers": req.Workers, "status": "provisioning"})
}

type createClusterReq struct {
	Name        string             `json:"name"`
	Environment string             `json:"environment"`
	Provider    string             `json:"provider"`
	Spec        models.ClusterSpec `json:"spec"`
	DryRun      bool               `json:"dryRun"`
}

func (s *Server) createCluster(w http.ResponseWriter, r *http.Request) {
	var req createClusterReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	res, err := s.svc.CreateCluster(r.Context(), orchestrator.CreateClusterInput{
		Name:        req.Name,
		Environment: req.Environment,
		Provider:    req.Provider,
		Spec:        req.Spec,
		DryRun:      req.DryRun,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if res.DryRun {
		summary := ""
		if res.Preflight != nil {
			summary = res.Preflight.Summary
		}
		writeJSON(w, http.StatusOK, map[string]any{"dryRun": true, "summary": summary})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":     res.Cluster.ID.String(),
		"name":   res.Cluster.Name,
		"status": res.Cluster.Status,
		"jobId":  res.JobID.String(),
	})
}

// listQueue reports recent River jobs. Returns [] when no queue is wired.
func (s *Server) listQueue(w http.ResponseWriter, r *http.Request) {
	if s.queue == nil {
		writeJSON(w, http.StatusOK, []jobs.JobInfo{})
		return
	}
	list, err := s.queue.ListJobs(r.Context(), 100)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	if err == nil {
		err = errors.New("unknown error")
	}
	// A governance block means the caller isn't allowed - 403, not 400/500.
	if orchestrator.IsAIEnforcementError(err) {
		status = http.StatusForbidden
	}
	// Friendly-ify a Postgres unique-violation (a duplicate name+environment, the
	// most common create error) instead of leaking the raw SQLSTATE 23505 to the UI.
	// A destroyed resource keeps its row (a tombstone), so reusing its name needs a
	// Remove first - say so.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		status = http.StatusConflict
		err = errors.New("a resource with that name already exists in this environment - choose another name, or remove the existing one first (destroyed resources keep their name until removed)")
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/cz8jmh4n7f-bit/opord-ai-demo/internal/aiproviders"
)

// adminAPIError carries the upstream HTTP status so callers can branch on it
// (e.g. 409 Conflict = the user is already a member) instead of matching the
// error text, which can change between API versions.
type adminAPIError struct {
	StatusCode int
	Body       string
}

func (e *adminAPIError) Error() string {
	return fmt.Sprintf("openai admin api returned %d: %s", e.StatusCode, e.Body)
}

// isAlreadyExists reports whether an admin call failed because the target already
// exists / the user is already a member: a 409 Conflict (preferred signal) or,
// as a fallback for backends that return 400 with an explanatory body, an
// "already" substring.
func isAlreadyExists(err error) bool {
	var apiErr *adminAPIError
	if errors.As(err, &apiErr) {
		if apiErr.StatusCode == http.StatusConflict {
			return true
		}
		return strings.Contains(strings.ToLower(apiErr.Body), "already")
	}
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "already")
}

// AdminProvisioner over the OpenAI organization API (/v1/organization/*), the
// OpenAI twin of the Anthropic admin client. OpenAI "projects" are the workspace
// concept; org roles are owner/reader, project roles owner/member (no
// inherited-billing quirk - effective access is just the explicit members).
// Driven by an OpenAI ADMIN key (sk-admin-..., stored as admin_api_key) with
// Bearer auth.

var _ aiproviders.AdminProvisioner = Provider{}

// adminKey reads the OpenAI org admin key (sk-admin-...).
func adminKey(creds map[string]string) string {
	if v := strings.TrimSpace(creds["admin_api_key"]); v != "" {
		return v
	}
	return strings.TrimSpace(creds["api_key"])
}

func (p Provider) adminDo(ctx context.Context, ac aiproviders.AdminContext, method, path string, in, out any) error {
	key := adminKey(ac.Credentials)
	if key == "" {
		return fmt.Errorf("openai admin key missing (store it as admin_api_key, sk-admin-...)")
	}
	base := strings.TrimRight(baseURL(ac.Config, "https://api.openai.com"), "/")
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := p.http().Do(req)
	if err != nil {
		return fmt.Errorf("openai admin api call failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(raw))
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return &adminAPIError{StatusCode: resp.StatusCode, Body: msg}
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decoding openai admin response: %w", err)
		}
	}
	return nil
}

// adminList pages an org list endpoint (?limit=100&after=...).
func adminList[T any](ctx context.Context, p Provider, ac aiproviders.AdminContext, path string) ([]T, error) {
	var all []T
	after := ""
	for page := 0; page < 50; page++ {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		u := path + sep + "limit=100"
		if after != "" {
			u += "&after=" + url.QueryEscape(after)
		}
		var payload struct {
			Data    []T    `json:"data"`
			HasMore bool   `json:"has_more"`
			LastID  string `json:"last_id"`
		}
		if err := p.adminDo(ctx, ac, http.MethodGet, u, nil, &payload); err != nil {
			return nil, err
		}
		all = append(all, payload.Data...)
		if !payload.HasMore || strings.TrimSpace(payload.LastID) == "" {
			break
		}
		after = payload.LastID
	}
	return all, nil
}

func tsToString(v int64) string {
	if v == 0 {
		return ""
	}
	return strconv.FormatInt(v, 10)
}

type oaiUser struct {
	ID      string `json:"id"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Role    string `json:"role"`
	AddedAt int64  `json:"added_at"`
}

func (p Provider) ListOrgUsers(ctx context.Context, ac aiproviders.AdminContext) ([]aiproviders.OrgUser, error) {
	rows, err := adminList[oaiUser](ctx, p, ac, "/v1/organization/users")
	if err != nil {
		return nil, err
	}
	out := make([]aiproviders.OrgUser, 0, len(rows))
	for _, u := range rows {
		out = append(out, aiproviders.OrgUser{ID: u.ID, Email: u.Email, Name: u.Name, Role: aiproviders.OrgRole(u.Role), AddedAt: tsToString(u.AddedAt)})
	}
	return out, nil
}

func (p Provider) ListWorkspaces(ctx context.Context, ac aiproviders.AdminContext) ([]aiproviders.OrgWorkspace, error) {
	type oaiProject struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Status     string `json:"status"`
		CreatedAt  int64  `json:"created_at"`
		ArchivedAt int64  `json:"archived_at"`
	}
	rows, err := adminList[oaiProject](ctx, p, ac, "/v1/organization/projects?include_archived=true")
	if err != nil {
		return nil, err
	}
	out := make([]aiproviders.OrgWorkspace, 0, len(rows))
	for _, w := range rows {
		out = append(out, aiproviders.OrgWorkspace{ID: w.ID, Name: w.Name, CreatedAt: tsToString(w.CreatedAt), ArchivedAt: tsToString(w.ArchivedAt)})
	}
	return out, nil
}

type oaiInvite struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	InvitedAt int64  `json:"invited_at"`
	ExpiresAt int64  `json:"expires_at"`
}

func (i oaiInvite) result() aiproviders.InviteResult {
	return aiproviders.InviteResult{InviteID: i.ID, Email: i.Email, Role: aiproviders.OrgRole(i.Role), Status: i.Status, InvitedAt: tsToString(i.InvitedAt), ExpiresAt: tsToString(i.ExpiresAt)}
}

func (p Provider) ListInvites(ctx context.Context, ac aiproviders.AdminContext) ([]aiproviders.InviteResult, error) {
	rows, err := adminList[oaiInvite](ctx, p, ac, "/v1/organization/invites")
	if err != nil {
		return nil, err
	}
	out := make([]aiproviders.InviteResult, 0, len(rows))
	for _, i := range rows {
		out = append(out, i.result())
	}
	return out, nil
}

// oaiOrgRole normalizes an abstract org role to OpenAI's allowed set (owner|reader).
// OpenAI org roles are only owner|reader, so an elevated role (admin/owner) maps to
// owner and every other role floors to reader (least privilege). The abstract enum
// is Anthropic-shaped, so "admin" - not a literal "owner" - is the elevated value.
func oaiOrgRole(r aiproviders.OrgRole) string {
	if strings.EqualFold(string(r), string(aiproviders.OrgRoleAdmin)) || strings.EqualFold(string(r), "owner") {
		return "owner"
	}
	return "reader"
}

// oaiProjectRole normalizes a workspace role to OpenAI's project set (owner|member).
func oaiProjectRole(r aiproviders.WorkspaceRole) string {
	if strings.Contains(strings.ToLower(string(r)), "admin") || strings.EqualFold(string(r), "owner") {
		return "owner"
	}
	return "member"
}

func (p Provider) InviteUser(ctx context.Context, ac aiproviders.AdminContext, req aiproviders.InviteRequest) (*aiproviders.InviteResult, error) {
	email := strings.TrimSpace(req.Email)
	if email == "" {
		return nil, fmt.Errorf("email is required")
	}
	var out oaiInvite
	if err := p.adminDo(ctx, ac, http.MethodPost, "/v1/organization/invites",
		map[string]string{"email": email, "role": oaiOrgRole(req.Role)}, &out); err != nil {
		return nil, err
	}
	res := out.result()
	return &res, nil
}

func (p Provider) CreateWorkspace(ctx context.Context, ac aiproviders.AdminContext, name string) (*aiproviders.OrgWorkspace, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("project name is required")
	}
	var out struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		CreatedAt int64  `json:"created_at"`
	}
	if err := p.adminDo(ctx, ac, http.MethodPost, "/v1/organization/projects", map[string]string{"name": name}, &out); err != nil {
		return nil, err
	}
	return &aiproviders.OrgWorkspace{ID: out.ID, Name: out.Name, CreatedAt: tsToString(out.CreatedAt)}, nil
}

func (p Provider) ArchiveWorkspace(ctx context.Context, ac aiproviders.AdminContext, workspaceID string) error {
	if strings.TrimSpace(workspaceID) == "" {
		return fmt.Errorf("project id is required")
	}
	return p.adminDo(ctx, ac, http.MethodPost, "/v1/organization/projects/"+url.PathEscape(workspaceID)+"/archive", nil, nil)
}

func (p Provider) GrantWorkspaceAccess(ctx context.Context, ac aiproviders.AdminContext, req aiproviders.WorkspaceGrantRequest) error {
	if req.WorkspaceID == "" || req.UserID == "" {
		return fmt.Errorf("project id and user id are required")
	}
	memberPath := "/v1/organization/projects/" + url.PathEscape(req.WorkspaceID) + "/users"
	role := oaiProjectRole(req.WorkspaceRole)
	err := p.adminDo(ctx, ac, http.MethodPost, memberPath, map[string]string{"user_id": req.UserID, "role": role}, nil)
	if isAlreadyExists(err) {
		// Already a member -> update the role (upsert).
		return p.adminDo(ctx, ac, http.MethodPost, memberPath+"/"+url.PathEscape(req.UserID), map[string]string{"role": role}, nil)
	}
	return err
}

func (p Provider) RemoveWorkspaceMember(ctx context.Context, ac aiproviders.AdminContext, workspaceID, userID string) error {
	if workspaceID == "" || userID == "" {
		return fmt.Errorf("project id and user id are required")
	}
	return p.adminDo(ctx, ac, http.MethodDelete, "/v1/organization/projects/"+url.PathEscape(workspaceID)+"/users/"+url.PathEscape(userID), nil, nil)
}

func (p Provider) SetOrgRole(ctx context.Context, ac aiproviders.AdminContext, userID string, role aiproviders.OrgRole) (*aiproviders.OrgUser, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("user id is required")
	}
	var out oaiUser
	if err := p.adminDo(ctx, ac, http.MethodPost, "/v1/organization/users/"+url.PathEscape(userID), map[string]string{"role": oaiOrgRole(role)}, &out); err != nil {
		return nil, err
	}
	return &aiproviders.OrgUser{ID: out.ID, Email: out.Email, Name: out.Name, Role: aiproviders.OrgRole(out.Role), AddedAt: tsToString(out.AddedAt)}, nil
}

func (p Provider) RemoveOrgUser(ctx context.Context, ac aiproviders.AdminContext, userID string) error {
	if strings.TrimSpace(userID) == "" {
		return fmt.Errorf("user id is required")
	}
	return p.adminDo(ctx, ac, http.MethodDelete, "/v1/organization/users/"+url.PathEscape(userID), nil, nil)
}

// EffectiveWorkspaceAccess is the explicit project members (OpenAI projects have
// no inherited-billing concept, so there is nothing to union).
func (p Provider) EffectiveWorkspaceAccess(ctx context.Context, ac aiproviders.AdminContext, workspaceID string) ([]aiproviders.WorkspaceAccess, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return nil, fmt.Errorf("project id is required")
	}
	type oaiMember struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	members, err := adminList[oaiMember](ctx, p, ac, "/v1/organization/projects/"+url.PathEscape(workspaceID)+"/users")
	if err != nil {
		return nil, err
	}
	out := make([]aiproviders.WorkspaceAccess, 0, len(members))
	for _, m := range members {
		out = append(out, aiproviders.WorkspaceAccess{
			UserID: m.ID, Email: m.Email, WorkspaceRole: aiproviders.WorkspaceRole(m.Role), Inherited: false,
		})
	}
	return out, nil
}

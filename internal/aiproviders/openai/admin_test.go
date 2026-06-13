package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cz8jmh4n7f-bit/opord-ai-demo/internal/aiproviders"
)

func fakeOpenAIOrg(t *testing.T) (*httptest.Server, aiproviders.AdminContext, *map[string]any) {
	t.Helper()
	lastBody := map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-admin-good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&lastBody)
		}
		switch {
		case r.URL.Path == "/v1/organization/users" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"id": "user-1", "email": "a@x.io", "role": "reader", "added_at": 1700000000}}, "has_more": false})
		case r.URL.Path == "/v1/organization/invites" && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "invite-1", "email": lastBody["email"], "role": lastBody["role"], "status": "pending"})
		case r.URL.Path == "/v1/organization/projects" && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "proj-1", "name": lastBody["name"], "created_at": 1700000001})
		case strings.HasSuffix(r.URL.Path, "/users") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"user_id": lastBody["user_id"], "role": lastBody["role"]})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, aiproviders.AdminContext{Credentials: map[string]string{"admin_api_key": "sk-admin-good"}, Config: map[string]any{"base_url": srv.URL}}, &lastBody
}

func TestOpenAIListUsers(t *testing.T) {
	srv, ac, _ := fakeOpenAIOrg(t)
	p := Provider{client: srv.Client()}
	users, err := p.ListOrgUsers(context.Background(), ac)
	if err != nil || len(users) != 1 || users[0].Email != "a@x.io" || users[0].Role != "reader" {
		t.Fatalf("list users: %v / %+v", err, users)
	}
}

func TestOpenAIInviteRoleNormalized(t *testing.T) {
	srv, ac, body := fakeOpenAIOrg(t)
	p := Provider{client: srv.Client()}
	// A non-owner org role must normalize to "reader" for OpenAI.
	if _, err := p.InviteUser(context.Background(), ac, aiproviders.InviteRequest{Email: "b@x.io", Role: "developer"}); err != nil {
		t.Fatalf("invite: %v", err)
	}
	if (*body)["role"] != "reader" {
		t.Fatalf("role not normalized to reader: %v", (*body)["role"])
	}
	// "owner" stays owner.
	_, _ = p.InviteUser(context.Background(), ac, aiproviders.InviteRequest{Email: "c@x.io", Role: "owner"})
	if (*body)["role"] != "owner" {
		t.Fatalf("owner role lost: %v", (*body)["role"])
	}
}

func TestOpenAIGrantProjectRoleNormalized(t *testing.T) {
	srv, ac, body := fakeOpenAIOrg(t)
	p := Provider{client: srv.Client()}
	// workspace_admin -> openai project "owner"; anything else -> "member".
	_ = p.GrantWorkspaceAccess(context.Background(), ac, aiproviders.WorkspaceGrantRequest{WorkspaceID: "proj-1", UserID: "user-1", WorkspaceRole: aiproviders.WSRoleAdmin})
	if (*body)["role"] != "owner" {
		t.Fatalf("workspace_admin should map to owner: %v", (*body)["role"])
	}
	_ = p.GrantWorkspaceAccess(context.Background(), ac, aiproviders.WorkspaceGrantRequest{WorkspaceID: "proj-1", UserID: "user-1", WorkspaceRole: aiproviders.WSRoleUser})
	if (*body)["role"] != "member" {
		t.Fatalf("workspace_user should map to member: %v", (*body)["role"])
	}
}

func TestOpenAIImplementsAdminProvisioner(t *testing.T) {
	var _ aiproviders.AdminProvisioner = Provider{}
}

func TestIsAlreadyExists(t *testing.T) {
	if !isAlreadyExists(&adminAPIError{StatusCode: http.StatusConflict, Body: "anything"}) {
		t.Error("409 Conflict must be treated as already-exists")
	}
	if !isAlreadyExists(&adminAPIError{StatusCode: http.StatusBadRequest, Body: "user is already a member"}) {
		t.Error("400 with an 'already' body must fall back to already-exists")
	}
	if isAlreadyExists(&adminAPIError{StatusCode: http.StatusBadRequest, Body: "invalid role"}) {
		t.Error("an unrelated 400 must NOT be already-exists")
	}
	if isAlreadyExists(nil) {
		t.Error("nil error must not be already-exists")
	}
}

// TestOpenAIOrgAdminMapsToOwner covers the role-floor fix: the abstract org role
// "admin" (Anthropic-shaped enum) must map to OpenAI's elevated "owner", not floor
// silently to "reader".
func TestOpenAIOrgAdminMapsToOwner(t *testing.T) {
	srv, ac, body := fakeOpenAIOrg(t)
	p := Provider{client: srv.Client()}
	if _, err := p.InviteUser(context.Background(), ac, aiproviders.InviteRequest{Email: "d@x.io", Role: aiproviders.OrgRoleAdmin}); err != nil {
		t.Fatalf("invite: %v", err)
	}
	if (*body)["role"] != "owner" {
		t.Fatalf("admin org role must map to owner, got %v", (*body)["role"])
	}
}

// TestOpenAIGrantUpsertsOnConflict checks that a 409 on member-add triggers the
// role-update (upsert) call instead of returning an error.
func TestOpenAIGrantUpsertsOnConflict(t *testing.T) {
	var roleUpdated bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/users") && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusConflict) // already a member
			_, _ = w.Write([]byte(`{"error":"conflict"}`))
		case strings.Contains(r.URL.Path, "/users/") && r.Method == http.MethodPost:
			roleUpdated = true
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	ac := aiproviders.AdminContext{Credentials: map[string]string{"admin_api_key": "sk-admin-good"}, Config: map[string]any{"base_url": srv.URL}}
	p := Provider{client: srv.Client()}
	if err := p.GrantWorkspaceAccess(context.Background(), ac, aiproviders.WorkspaceGrantRequest{WorkspaceID: "proj-1", UserID: "user-1", WorkspaceRole: aiproviders.WSRoleUser}); err != nil {
		t.Fatalf("grant should upsert on conflict, got %v", err)
	}
	if !roleUpdated {
		t.Fatal("a 409 on member-add must trigger the role-update upsert call")
	}
}

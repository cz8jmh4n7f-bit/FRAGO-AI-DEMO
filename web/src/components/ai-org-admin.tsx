"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { createPortal } from "react-dom";
import { Archive, Loader2, Plus, Trash2, UserCog, UserPlus, Users } from "lucide-react";
import type { AIInvite, AIOrgUser, AIWorkspace, AIWorkspaceAccess } from "@/lib/types";
import { authHeaders } from "@/lib/client-auth";
import { button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { useConfirm } from "@/components/ui/confirm";
import { useToast } from "@/components/ui/toast";
import { cn } from "@/lib/utils";

const API = "/bff";
const inputCls =
  "h-9 w-full rounded-lg border border-input bg-card px-3 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

// Org roles assignable via the API (admin is Console-only - mirrors the backend
// guard in the Anthropic provider). Workspace roles per the role matrix:
// workspace_billing is inherited-only, so it is not an assignable option.
// Role sets are provider-aware: Anthropic (org user/developer/billing… +
// workspace_*) vs OpenAI (org owner/reader + project owner/member).
const ROLE_SETS: Record<string, { org: string[]; ws: string[] }> = {
  anthropic: {
    org: ["user", "claude_code_user", "developer", "billing"],
    ws: ["workspace_user", "workspace_developer", "workspace_restricted_developer", "workspace_admin"],
  },
  openai: { org: ["reader", "owner"], ws: ["member", "owner"] },
};

function orgRolesFor(pt: string): string[] {
  return ROLE_SETS[pt]?.org ?? ROLE_SETS.anthropic.org;
}
function wsRolesFor(pt: string): string[] {
  return ROLE_SETS[pt]?.ws ?? ROLE_SETS.anthropic.ws;
}
// assignableWorkspaceRoles mirrors the backend matrix: an Anthropic billing user
// can ONLY become a workspace_admin; everyone else gets the provider's full set.
function assignableWorkspaceRoles(pt: string, orgRole: string): string[] {
  if (pt === "anthropic" && orgRole === "billing") return ["workspace_admin"];
  return wsRolesFor(pt);
}

function api(provider: string, path: string) {
  return `${API}/api/v1/ai/admin/${encodeURIComponent(provider)}${path}`;
}

// useAction wraps a mutation with busy state + toast + refresh, the pattern used
// across the AI action components.
function useAction() {
  const router = useRouter();
  const { toast } = useToast();
  const [busy, setBusy] = useState(false);
  async function run(label: string, fn: () => Promise<Response>): Promise<boolean> {
    setBusy(true);
    try {
      const res = await fn();
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        toast({ variant: "error", title: `${label} failed`, description: data.error ?? `Request failed (${res.status})` });
        return false;
      }
      toast({ variant: "success", title: `${label} done` });
      router.refresh();
      return true;
    } catch (err) {
      toast({ variant: "error", title: `${label} failed`, description: String(err) });
      return false;
    } finally {
      setBusy(false);
    }
  }
  return { busy, run };
}

function roleLabel(role: string) {
  return role.replace(/_/g, " ");
}

// --- Org users panel ---

export function OrgUsersPanel({ provider, providerType, users }: { provider: string; providerType: string; users: AIOrgUser[] }) {
  return (
    <Card className="overflow-hidden p-0">
      <div className="flex items-center justify-between border-b border-border px-5 py-3">
        <div className="flex items-center gap-2">
          <Users className="size-4 text-muted-foreground" />
          <h2 className="text-sm font-semibold">Organization users ({users.length})</h2>
        </div>
        <InviteUserButton provider={provider} providerType={providerType} />
      </div>
      {users.length === 0 ? (
        <p className="px-5 py-8 text-center text-sm text-muted-foreground">No users yet. Invite one to get started.</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border text-left text-xs uppercase tracking-wide text-muted-foreground">
                <th scope="col" className="px-5 py-3 font-medium">User</th>
                <th scope="col" className="px-5 py-3 font-medium">Org role</th>
                <th scope="col" className="px-5 py-3 font-medium">Added</th>
                <th scope="col" className="px-5 py-3 text-right font-medium">Actions</th>
              </tr>
            </thead>
            <tbody>
              {users.map((u) => (
                <tr key={u.id} className="border-b border-border last:border-0 hover:bg-muted/60">
                  <td className="px-5 py-3">
                    <div className="font-medium text-foreground">{u.email}</div>
                    {u.name && <div className="text-xs text-muted-foreground">{u.name}</div>}
                  </td>
                  <td className="px-5 py-3">
                    <Badge>{roleLabel(u.role)}</Badge>
                  </td>
                  <td className="px-5 py-3 text-muted-foreground">{u.addedAt ? u.addedAt.slice(0, 10) : "-"}</td>
                  <td className="px-5 py-3">
                    <OrgUserActions provider={provider} providerType={providerType} user={u} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Card>
  );
}

function InviteUserButton({ provider, providerType }: { provider: string; providerType: string }) {
  const { busy, run } = useAction();
  const [open, setOpen] = useState(false);
  const [email, setEmail] = useState("");
  const [role, setRole] = useState(orgRolesFor(providerType)[0]);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    const ok = await run("Invite", () =>
      fetch(api(provider, "/invites"), {
        method: "POST",
        headers: { "Content-Type": "application/json", ...authHeaders() },
        body: JSON.stringify({ email, role }),
      }),
    );
    if (ok) {
      setOpen(false);
      setEmail("");
    }
  }

  return (
    <>
      <button type="button" onClick={() => setOpen(true)} className={button({ size: "sm" })}>
        <UserPlus className="size-4" />
        Invite user
      </button>
      {open &&
        typeof document !== "undefined" &&
        createPortal(
          <Modal title="Invite organization user" onClose={() => !busy && setOpen(false)}>
            <form onSubmit={submit} className="space-y-4">
              <p className="text-xs text-muted-foreground">
                Sends a pending invite. The user is added to the organization only after they accept it (the invite can
                expire unaccepted).
              </p>
              <label className="flex flex-col gap-1.5">
                <span className="text-xs font-medium text-muted-foreground">Email</span>
                <input className={inputCls} type="email" required value={email} onChange={(e) => setEmail(e.target.value)} placeholder="person@company.com" />
              </label>
              <label className="flex flex-col gap-1.5">
                <span className="text-xs font-medium text-muted-foreground">Org role</span>
                <select className={inputCls} value={role} onChange={(e) => setRole(e.target.value)}>
                  {orgRolesFor(providerType).map((r) => (
                    <option key={r} value={r}>{roleLabel(r)}</option>
                  ))}
                </select>
                <span className="text-xs text-muted-foreground">The admin role can only be granted in the provider Console.</span>
              </label>
              <ModalActions busy={busy} onCancel={() => setOpen(false)} submitLabel="Send invite" />
            </form>
          </Modal>,
          document.body,
        )}
    </>
  );
}

function OrgUserActions({ provider, providerType, user }: { provider: string; providerType: string; user: AIOrgUser }) {
  const { busy, run } = useAction();
  const { prompt } = useConfirm();
  const [open, setOpen] = useState(false);
  const [role, setRole] = useState(user.role);

  async function saveRole(e: React.FormEvent) {
    e.preventDefault();
    const ok = await run("Role change", () =>
      fetch(api(provider, `/users/${encodeURIComponent(user.id)}/role`), {
        method: "POST",
        headers: { "Content-Type": "application/json", ...authHeaders() },
        body: JSON.stringify({ role }),
      }),
    );
    if (ok) setOpen(false);
  }

  async function remove() {
    const typed = await prompt({
      title: `Remove ${user.email}?`,
      message: "Removes the user from the organization (this does not delete their personal account). Type the email to confirm.",
      requireValue: user.email,
      confirmLabel: "Remove",
      danger: true,
    });
    if (typed == null) return;
    await run("Remove user", () =>
      fetch(api(provider, `/users/${encodeURIComponent(user.id)}`), { method: "DELETE", headers: authHeaders() }),
    );
  }

  return (
    <div className="flex justify-end gap-2">
      <button type="button" onClick={() => setOpen(true)} disabled={busy} className={button({ variant: "outline", size: "sm" })}>
        <UserCog className="size-4" />
        Role
      </button>
      <button type="button" onClick={remove} disabled={busy} className={cn(button({ variant: "outline", size: "sm" }), "text-danger")}>
        {busy ? <Loader2 className="size-4 animate-spin" /> : <Trash2 className="size-4" />}
        Remove
      </button>
      {open &&
        typeof document !== "undefined" &&
        createPortal(
          <Modal title={`Change role · ${user.email}`} onClose={() => !busy && setOpen(false)}>
            <form onSubmit={saveRole} className="space-y-4">
              <label className="flex flex-col gap-1.5">
                <span className="text-xs font-medium text-muted-foreground">Org role</span>
                <select className={inputCls} value={role} onChange={(e) => setRole(e.target.value)}>
                  {orgRolesFor(providerType).map((r) => (
                    <option key={r} value={r}>{roleLabel(r)}</option>
                  ))}
                </select>
              </label>
              <ModalActions busy={busy} onCancel={() => setOpen(false)} submitLabel="Save role" />
            </form>
          </Modal>,
          document.body,
        )}
    </div>
  );
}

// --- Invites panel ---

export function InvitesPanel({ invites }: { invites: AIInvite[] }) {
  const pending = invites.filter((i) => i.status === "pending");
  if (pending.length === 0) return null;
  return (
    <Card className="overflow-hidden p-0">
      <div className="border-b border-border px-5 py-3">
        <h2 className="text-sm font-semibold">Pending invites ({pending.length})</h2>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border text-left text-xs uppercase tracking-wide text-muted-foreground">
              <th scope="col" className="px-5 py-3 font-medium">Email</th>
              <th scope="col" className="px-5 py-3 font-medium">Role</th>
              <th scope="col" className="px-5 py-3 font-medium">Expires</th>
            </tr>
          </thead>
          <tbody>
            {pending.map((i) => (
              <tr key={i.inviteId} className="border-b border-border last:border-0">
                <td className="px-5 py-3 font-medium">{i.email}</td>
                <td className="px-5 py-3"><Badge>{roleLabel(i.role)}</Badge></td>
                <td className="px-5 py-3 text-muted-foreground">{i.expiresAt ? i.expiresAt.slice(0, 10) : "-"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </Card>
  );
}

// --- Workspaces panel ---

export function WorkspacesPanel({ provider, providerType, workspaces, users }: { provider: string; providerType: string; workspaces: AIWorkspace[]; users: AIOrgUser[] }) {
  const active = workspaces.filter((w) => !w.archived);
  return (
    <Card className="overflow-hidden p-0">
      <div className="flex items-center justify-between border-b border-border px-5 py-3">
        <h2 className="text-sm font-semibold">Workspaces ({active.length})</h2>
        <CreateWorkspaceButton provider={provider} />
      </div>
      {active.length === 0 ? (
        <p className="px-5 py-8 text-center text-sm text-muted-foreground">No active workspaces. Create one to scope access.</p>
      ) : (
        <div className="divide-y divide-border">
          {active.map((w) => (
            <WorkspaceRow key={w.id} provider={provider} providerType={providerType} workspace={w} users={users} />
          ))}
        </div>
      )}
    </Card>
  );
}

function CreateWorkspaceButton({ provider }: { provider: string }) {
  const { busy, run } = useAction();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    const ok = await run("Create workspace", () =>
      fetch(api(provider, "/workspaces"), {
        method: "POST",
        headers: { "Content-Type": "application/json", ...authHeaders() },
        body: JSON.stringify({ name }),
      }),
    );
    if (ok) {
      setOpen(false);
      setName("");
    }
  }

  return (
    <>
      <button type="button" onClick={() => setOpen(true)} className={button({ size: "sm" })}>
        <Plus className="size-4" />
        New workspace
      </button>
      {open &&
        typeof document !== "undefined" &&
        createPortal(
          <Modal title="Create workspace" onClose={() => !busy && setOpen(false)}>
            <form onSubmit={submit} className="space-y-4">
              <label className="flex flex-col gap-1.5">
                <span className="text-xs font-medium text-muted-foreground">Name</span>
                <input className={inputCls} required value={name} onChange={(e) => setName(e.target.value)} placeholder="team-platform" />
              </label>
              <ModalActions busy={busy} onCancel={() => setOpen(false)} submitLabel="Create" />
            </form>
          </Modal>,
          document.body,
        )}
    </>
  );
}

function WorkspaceRow({ provider, providerType, workspace, users }: { provider: string; providerType: string; workspace: AIWorkspace; users: AIOrgUser[] }) {
  const { busy, run } = useAction();
  const { prompt } = useConfirm();
  const [open, setOpen] = useState(false);

  async function archive() {
    const typed = await prompt({
      title: `Archive ${workspace.name}?`,
      message: "Archiving retires the workspace (the provider has no hard delete). Type the workspace name to confirm.",
      requireValue: workspace.name,
      confirmLabel: "Archive",
      danger: true,
    });
    if (typed == null) return;
    await run("Archive workspace", () =>
      fetch(api(provider, `/workspaces/${encodeURIComponent(workspace.id)}/archive`), { method: "POST", headers: authHeaders() }),
    );
  }

  return (
    <div className="px-5 py-3">
      <div className="flex items-center justify-between gap-3">
        <div className="min-w-0">
          <div className="truncate font-medium text-foreground">{workspace.name}</div>
          <div className="font-mono text-xs text-muted-foreground">{workspace.id}</div>
        </div>
        <div className="flex shrink-0 gap-2">
          <button type="button" onClick={() => setOpen((v) => !v)} disabled={busy} className={button({ variant: "outline", size: "sm" })}>
            <Users className="size-4" />
            Members
          </button>
          <button type="button" onClick={archive} disabled={busy} className={cn(button({ variant: "outline", size: "sm" }), "text-danger")}>
            {busy ? <Loader2 className="size-4 animate-spin" /> : <Archive className="size-4" />}
            Archive
          </button>
        </div>
      </div>
      {open && <WorkspaceMembers provider={provider} providerType={providerType} workspace={workspace} users={users} />}
    </div>
  );
}

// WorkspaceMembers lazily loads the EFFECTIVE access set (explicit + inherited)
// when expanded, and offers grant/remove. Inherited rows (org admin/billing) are
// shown read-only since they come from the org role, not a workspace membership.
function WorkspaceMembers({ provider, providerType, workspace, users }: { provider: string; providerType: string; workspace: AIWorkspace; users: AIOrgUser[] }) {
  const { busy, run } = useAction();
  const [access, setAccess] = useState<AIWorkspaceAccess[] | null>(null);
  const [userId, setUserId] = useState("");
  const [role, setRole] = useState("workspace_user");
  // The selected user's org role decides which workspace roles are valid (matrix).
  const selectedUser = users.find((u) => u.id === userId);
  const roleOptions = assignableWorkspaceRoles(providerType, selectedUser?.role ?? "");
  // Org admins inherit workspace_admin everywhere - they can't (and needn't) be
  // granted explicitly, so they're not offered in the picker.
  const grantable = users.filter((u) => u.role !== "admin");

  // Fetch the effective access set on mount + after mutations. The setState lives
  // inside the async body (deferred past the first await), mirroring the codebase's
  // fetch-on-mount pattern so the strict set-state-in-effect rule stays satisfied.
  const load = useCallback(() => {
    void (async () => {
      try {
        const res = await fetch(api(provider, `/workspaces/${encodeURIComponent(workspace.id)}/access`), { headers: authHeaders() });
        const data = await res.json().catch(() => []);
        setAccess(res.ok && Array.isArray(data) ? data : []);
      } catch {
        setAccess([]);
      }
    })();
  }, [provider, workspace.id]);

  useEffect(() => {
    load();
  }, [load]);

  async function grant(e: React.FormEvent) {
    e.preventDefault();
    const ok = await run("Grant access", () =>
      fetch(api(provider, `/workspaces/${encodeURIComponent(workspace.id)}/members`), {
        method: "POST",
        headers: { "Content-Type": "application/json", ...authHeaders() },
        body: JSON.stringify({ userId, role }),
      }),
    );
    if (ok) {
      setUserId("");
      load();
    }
  }

  async function removeMember(uid: string) {
    const ok = await run("Remove member", () =>
      fetch(api(provider, `/workspaces/${encodeURIComponent(workspace.id)}/members/${encodeURIComponent(uid)}`), { method: "DELETE", headers: authHeaders() }),
    );
    if (ok) load();
  }

  return (
    <div className="mt-3 space-y-3 rounded-lg border border-border bg-muted/30 p-3">
      {access === null && <p className="text-xs text-muted-foreground">Loading members…</p>}
      {access && access.length === 0 && <p className="text-xs text-muted-foreground">No members yet.</p>}
      {access && access.length > 0 && (
        <ul className="space-y-1.5">
          {access.map((a) => (
            <li key={a.userId} className="flex items-center justify-between gap-3 text-sm">
              <span className="min-w-0 truncate">
                {a.email || a.userId}
                <Badge className="ml-2">{roleLabel(a.workspaceRole)}</Badge>
                {a.inherited && <span className="ml-2 text-xs text-muted-foreground">inherited from {roleLabel(a.orgRole || "org role")}</span>}
              </span>
              {!a.inherited && (
                <button type="button" onClick={() => removeMember(a.userId)} disabled={busy} className="text-xs font-medium text-danger hover:underline">
                  Remove
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
      <form onSubmit={grant} className="flex flex-wrap items-end gap-2 border-t border-border pt-3">
        <label className="flex flex-1 flex-col gap-1">
          <span className="text-xs font-medium text-muted-foreground">User</span>
          <select
            className={inputCls}
            required
            value={userId}
            onChange={(e) => {
              const next = e.target.value;
              setUserId(next);
              // Snap the role into the matrix-allowed set for the chosen user.
              const allowed = assignableWorkspaceRoles(providerType, users.find((u) => u.id === next)?.role ?? "");
              if (!allowed.includes(role)) setRole(allowed[0]);
            }}
          >
            <option value="" disabled>
              Select a user…
            </option>
            {grantable.map((u) => (
              <option key={u.id} value={u.id}>
                {u.email} ({roleLabel(u.role)})
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-1">
          <span className="text-xs font-medium text-muted-foreground">Role</span>
          <select className={inputCls} value={role} onChange={(e) => setRole(e.target.value)} disabled={!userId}>
            {roleOptions.map((r) => (
              <option key={r} value={r}>{roleLabel(r)}</option>
            ))}
          </select>
        </label>
        <button type="submit" disabled={busy || !userId} className={button({ size: "sm" })}>
          {busy ? <Loader2 className="size-4 animate-spin" /> : <UserPlus className="size-4" />}
          Grant
        </button>
      </form>
      {selectedUser?.role === "billing" && (
        <p className="text-xs text-muted-foreground">
          {selectedUser.email} has the <span className="font-medium">billing</span> org role - they already see billing on
          every workspace and the provider only allows promoting them to <span className="font-medium">workspace admin</span>.
          To make them a regular developer/user, change their org role first.
        </p>
      )}
      {grantable.length === 0 && (
        <p className="text-xs text-muted-foreground">No grantable users - invite one, or org admins already have inherited access.</p>
      )}
    </div>
  );
}

// --- Shared modal chrome ---

function Modal({ title, onClose, children }: { title: string; onClose: () => void; children: React.ReactNode }) {
  return (
    <div className="fixed inset-0 z-[70] flex items-center justify-center p-4" role="dialog" aria-modal="true">
      <div className="absolute inset-0 bg-black/50" onClick={onClose} />
      <div className="relative w-full max-w-md rounded-xl border border-border bg-card p-5 shadow-xl">
        <h2 className="mb-4 text-base font-semibold text-foreground">{title}</h2>
        {children}
      </div>
    </div>
  );
}

function ModalActions({ busy, onCancel, submitLabel }: { busy: boolean; onCancel: () => void; submitLabel: string }) {
  return (
    <div className="flex items-center justify-end gap-2 pt-1">
      <button type="button" onClick={onCancel} disabled={busy} className={button({ variant: "outline", size: "sm" })}>
        Cancel
      </button>
      <button type="submit" disabled={busy} className={cn(button({ size: "sm" }), busy && "opacity-70")}>
        {busy && <Loader2 className="size-4 animate-spin" />}
        {submitLabel}
      </button>
    </div>
  );
}

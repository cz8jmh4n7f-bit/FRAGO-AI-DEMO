"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { createPortal } from "react-dom";
import { CheckCircle2, Loader2, Plus, ShieldCheck, Trash2, UserPlus, XCircle } from "lucide-react";
import type { MCPGrant, MCPServer } from "@/lib/types";
import { authHeaders } from "@/lib/client-auth";
import { button } from "@/components/ui/button";
import { useConfirm } from "@/components/ui/confirm";
import { useToast } from "@/components/ui/toast";
import { cn } from "@/lib/utils";

const API = "/bff";
const inputCls =
  "h-9 w-full rounded-lg border border-input bg-card px-3 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

const RISK_TIERS = ["low", "medium", "high", "critical"];

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

export function RegisterMCPServerButton() {
  const router = useRouter();
  const { toast } = useToast();
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [name, setName] = useState("");
  const [transport, setTransport] = useState("stdio");
  const [endpoint, setEndpoint] = useState("");
  const [risk, setRisk] = useState("medium");
  const [tools, setTools] = useState("");

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      const res = await fetch(`${API}/api/v1/ai/mcp/servers`, {
        method: "POST",
        headers: { "Content-Type": "application/json", ...authHeaders() },
        body: JSON.stringify({
          name,
          transport,
          endpoint,
          riskTier: risk,
          allowedTools: tools.split(",").map((t) => t.trim()).filter(Boolean),
        }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        toast({ variant: "error", title: "Register failed", description: data.error ?? `Request failed (${res.status})` });
        return;
      }
      toast({ variant: "success", title: `MCP server “${name}” registered` });
      setOpen(false);
      setName("");
      setEndpoint("");
      setTools("");
      router.refresh();
    } catch (err) {
      toast({ variant: "error", title: "Register failed", description: String(err) });
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <button type="button" onClick={() => setOpen(true)} className={button({ size: "sm" })}>
        <Plus className="size-4" />
        Register MCP server
      </button>
      {open &&
        typeof document !== "undefined" &&
        createPortal(
          <Modal title="Register MCP server" onClose={() => !busy && setOpen(false)}>
            <form onSubmit={submit} className="space-y-4">
              <p className="text-xs text-muted-foreground">
                Add an approved MCP server to the governed catalog. Teams must be granted access, and the authorize
                check enforces the tool allow-list at connect time.
              </p>
              <label className="flex flex-col gap-1.5">
                <span className="text-xs font-medium text-muted-foreground">Name</span>
                <input className={inputCls} required value={name} onChange={(e) => setName(e.target.value)} placeholder="github-mcp" />
              </label>
              <div className="grid grid-cols-2 gap-2">
                <label className="flex flex-col gap-1.5">
                  <span className="text-xs font-medium text-muted-foreground">Transport</span>
                  <select className={inputCls} value={transport} onChange={(e) => setTransport(e.target.value)}>
                    <option value="stdio">stdio</option>
                    <option value="http">http</option>
                    <option value="sse">sse</option>
                  </select>
                </label>
                <label className="flex flex-col gap-1.5">
                  <span className="text-xs font-medium text-muted-foreground">Risk tier</span>
                  <select className={inputCls} value={risk} onChange={(e) => setRisk(e.target.value)}>
                    {RISK_TIERS.map((t) => (
                      <option key={t} value={t}>{t}</option>
                    ))}
                  </select>
                </label>
              </div>
              <label className="flex flex-col gap-1.5">
                <span className="text-xs font-medium text-muted-foreground">Endpoint / command</span>
                <input className={inputCls} value={endpoint} onChange={(e) => setEndpoint(e.target.value)} placeholder="npx -y @modelcontextprotocol/server-github" />
              </label>
              <label className="flex flex-col gap-1.5">
                <span className="text-xs font-medium text-muted-foreground">Allowed tools (comma-separated, empty = all)</span>
                <input className={inputCls} value={tools} onChange={(e) => setTools(e.target.value)} placeholder="search_repositories, get_file_contents" />
              </label>
              <div className="flex items-center justify-end gap-2 pt-1">
                <button type="button" onClick={() => setOpen(false)} disabled={busy} className={button({ variant: "outline", size: "sm" })}>Cancel</button>
                <button type="submit" disabled={busy} className={cn(button({ size: "sm" }), busy && "opacity-70")}>
                  {busy && <Loader2 className="size-4 animate-spin" />} Register
                </button>
              </div>
            </form>
          </Modal>,
          document.body,
        )}
    </>
  );
}

export function MCPServerActions({ server }: { server: MCPServer }) {
  const router = useRouter();
  const { toast } = useToast();
  const { prompt } = useConfirm();
  const [busy, setBusy] = useState(false);

  async function del() {
    const typed = await prompt({
      title: `Delete MCP server “${server.name}”?`,
      message: "Removes the server and all its grants from the governed catalog. Type the name to confirm.",
      requireValue: server.name,
      confirmLabel: "Delete",
      danger: true,
    });
    if (typed == null) return;
    setBusy(true);
    try {
      const res = await fetch(`${API}/api/v1/ai/mcp/servers/${encodeURIComponent(server.name)}`, { method: "DELETE", headers: authHeaders() });
      if (!res.ok) {
        const d = await res.json().catch(() => ({}));
        toast({ variant: "error", title: "Delete failed", description: d.error ?? `(${res.status})` });
        return;
      }
      toast({ variant: "success", title: `“${server.name}” deleted` });
      router.refresh();
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex justify-end gap-2">
      <GrantMCPButton server={server.name} />
      <button type="button" onClick={del} disabled={busy} className={cn(button({ variant: "outline", size: "sm" }), "text-danger")}>
        {busy ? <Loader2 className="size-4 animate-spin" /> : <Trash2 className="size-4" />} Delete
      </button>
    </div>
  );
}

function GrantMCPButton({ server }: { server: string }) {
  const router = useRouter();
  const { toast } = useToast();
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [owner, setOwner] = useState("");

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      const res = await fetch(`${API}/api/v1/ai/mcp/grants`, {
        method: "POST",
        headers: { "Content-Type": "application/json", ...authHeaders() },
        body: JSON.stringify({ server, owner }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        toast({ variant: "error", title: "Grant failed", description: data.error ?? `(${res.status})` });
        return;
      }
      toast({ variant: "success", title: `Granted ${owner} access to ${server}` });
      setOpen(false);
      setOwner("");
      router.refresh();
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <button type="button" onClick={() => setOpen(true)} className={button({ variant: "outline", size: "sm" })}>
        <UserPlus className="size-4" /> Grant
      </button>
      {open &&
        typeof document !== "undefined" &&
        createPortal(
          <Modal title={`Grant access · ${server}`} onClose={() => !busy && setOpen(false)}>
            <form onSubmit={submit} className="space-y-4">
              <label className="flex flex-col gap-1.5">
                <span className="text-xs font-medium text-muted-foreground">Team / owner</span>
                <input className={inputCls} required value={owner} onChange={(e) => setOwner(e.target.value)} placeholder="agent-team or you@company.com" />
              </label>
              <div className="flex items-center justify-end gap-2">
                <button type="button" onClick={() => setOpen(false)} disabled={busy} className={button({ variant: "outline", size: "sm" })}>Cancel</button>
                <button type="submit" disabled={busy} className={cn(button({ size: "sm" }), busy && "opacity-70")}>
                  {busy && <Loader2 className="size-4 animate-spin" />} Grant
                </button>
              </div>
            </form>
          </Modal>,
          document.body,
        )}
    </>
  );
}

// MCPGrantActions is status-aware: a 'requested' grant shows Approve/Reject
// (operator decision on a self-service request); an 'active' grant shows Revoke;
// terminal states show the status text.
export function MCPGrantActions({ grant }: { grant: MCPGrant }) {
  const router = useRouter();
  const { toast } = useToast();
  const [busy, setBusy] = useState(false);

  async function act(action: "approve" | "reject" | "revoke", label: string) {
    setBusy(true);
    try {
      const res = await fetch(`${API}/api/v1/ai/mcp/grants/${grant.id}/${action}`, {
        method: "POST",
        headers: { "Content-Type": "application/json", ...authHeaders() },
        body: JSON.stringify({ by: "web" }),
      });
      if (!res.ok) {
        const d = await res.json().catch(() => ({}));
        toast({ variant: "error", title: `${label} failed`, description: d.error ?? `(${res.status})` });
        return;
      }
      toast({ variant: "success", title: `${label} done` });
      router.refresh();
    } finally {
      setBusy(false);
    }
  }

  if (grant.status === "requested") {
    return (
      <div className="flex justify-end gap-3">
        <button type="button" onClick={() => act("approve", "Approve")} disabled={busy} className="text-xs font-medium text-success hover:underline">Approve</button>
        <button type="button" onClick={() => act("reject", "Reject")} disabled={busy} className="text-xs font-medium text-danger hover:underline">Reject</button>
      </div>
    );
  }
  if (grant.status === "active") {
    return (
      <button type="button" onClick={() => act("revoke", "Revoke")} disabled={busy} className="text-xs font-medium text-danger hover:underline">
        {busy ? "…" : "Revoke"}
      </button>
    );
  }
  return <span className="text-xs text-muted-foreground">{grant.status}</span>;
}

// RequestMCPAccessButton is the self-service path (viewer): pick a server + owner
// and submit an access request that an operator approves.
export function RequestMCPAccessButton({ servers }: { servers: MCPServer[] }) {
  const router = useRouter();
  const { toast } = useToast();
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [server, setServer] = useState(servers[0]?.name ?? "");
  const [owner, setOwner] = useState("");

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      const res = await fetch(`${API}/api/v1/ai/mcp/requests`, {
        method: "POST",
        headers: { "Content-Type": "application/json", ...authHeaders() },
        body: JSON.stringify({ server, owner, requester: owner }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        toast({ variant: "error", title: "Request failed", description: data.error ?? `(${res.status})` });
        return;
      }
      toast({ variant: "success", title: "Access requested - awaiting approval" });
      setOpen(false);
      setOwner("");
      router.refresh();
    } finally {
      setBusy(false);
    }
  }

  if (servers.length === 0) return null;
  return (
    <>
      <button type="button" onClick={() => setOpen(true)} className={button({ variant: "outline", size: "sm" })}>
        <UserPlus className="size-4" /> Request access
      </button>
      {open &&
        typeof document !== "undefined" &&
        createPortal(
          <Modal title="Request MCP access" onClose={() => !busy && setOpen(false)}>
            <form onSubmit={submit} className="space-y-4">
              <p className="text-xs text-muted-foreground">Creates a pending request an operator approves. It does not authorize until approved.</p>
              <label className="flex flex-col gap-1.5">
                <span className="text-xs font-medium text-muted-foreground">Server</span>
                <select className={inputCls} value={server} onChange={(e) => setServer(e.target.value)}>
                  {servers.map((s) => (<option key={s.id} value={s.name}>{s.name}</option>))}
                </select>
              </label>
              <label className="flex flex-col gap-1.5">
                <span className="text-xs font-medium text-muted-foreground">Team / owner</span>
                <input className={inputCls} required value={owner} onChange={(e) => setOwner(e.target.value)} placeholder="agent-team or you@company.com" />
              </label>
              <div className="flex items-center justify-end gap-2">
                <button type="button" onClick={() => setOpen(false)} disabled={busy} className={button({ variant: "outline", size: "sm" })}>Cancel</button>
                <button type="submit" disabled={busy} className={cn(button({ size: "sm" }), busy && "opacity-70")}>{busy && <Loader2 className="size-4 animate-spin" />} Request</button>
              </div>
            </form>
          </Modal>,
          document.body,
        )}
    </>
  );
}

// AuthorizeTester is the live demo of the enforcement endpoint - exactly what an
// agent runtime calls before connecting to an MCP server.
export function AuthorizeTester({ servers }: { servers: MCPServer[] }) {
  const [server, setServer] = useState(servers[0]?.name ?? "");
  const [owner, setOwner] = useState("");
  const [tool, setTool] = useState("");
  const [result, setResult] = useState<{ allowed: boolean; reason: string } | null>(null);
  const [busy, setBusy] = useState(false);

  async function check(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setResult(null);
    try {
      const qs = new URLSearchParams({ server, owner, tool });
      const res = await fetch(`${API}/api/v1/ai/mcp/authorize?${qs}`, { headers: authHeaders() });
      const data = await res.json().catch(() => ({ allowed: false, reason: `(${res.status})` }));
      setResult({ allowed: !!data.allowed, reason: data.reason ?? "" });
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={check} className="space-y-3">
      <div className="flex items-center gap-2 text-sm font-semibold">
        <ShieldCheck className="size-4 text-primary" />
        Authorize check <span className="font-normal text-muted-foreground">(what an agent runtime calls before connecting)</span>
      </div>
      <div className="grid gap-2 sm:grid-cols-[1fr_1fr_1fr_auto]">
        <select className={inputCls} value={server} onChange={(e) => setServer(e.target.value)}>
          {servers.map((s) => (
            <option key={s.id} value={s.name}>{s.name}</option>
          ))}
        </select>
        <input className={inputCls} value={owner} onChange={(e) => setOwner(e.target.value)} placeholder="owner / team" required />
        <input className={inputCls} value={tool} onChange={(e) => setTool(e.target.value)} placeholder="tool (optional)" />
        <button type="submit" disabled={busy || !server} className={button({ size: "sm" })}>
          {busy ? <Loader2 className="size-4 animate-spin" /> : "Check"}
        </button>
      </div>
      {result && (
        <div
          className={cn(
            "flex items-center gap-2 rounded-lg border px-3 py-2 text-sm",
            result.allowed ? "border-success/30 bg-success/10 text-success" : "border-danger/30 bg-danger/10 text-danger",
          )}
        >
          {result.allowed ? <CheckCircle2 className="size-4" /> : <XCircle className="size-4" />}
          <span className="font-medium">{result.allowed ? "ALLOWED" : "DENIED"}</span>
          <span className="text-muted-foreground">— {result.reason}</span>
        </div>
      )}
    </form>
  );
}

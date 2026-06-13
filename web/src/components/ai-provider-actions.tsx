"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { createPortal } from "react-dom";
import { Activity, Loader2, Pencil, Plus, RefreshCw, Trash2 } from "lucide-react";
import type { AIProvider } from "@/lib/types";
import { authHeaders } from "@/lib/client-auth";
import { button } from "@/components/ui/button";
import { useConfirm } from "@/components/ui/confirm";
import { useToast } from "@/components/ui/toast";
import { cn } from "@/lib/utils";

const API = "/bff";
const inputCls =
  "h-9 w-full rounded-lg border border-input bg-card px-3 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

type ProviderType = "mock_ai" | "openai" | "anthropic" | "litellm";

const providerDefaults: Record<ProviderType, { name: string; secretRef: string; scopes: string[] }> = {
  mock_ai: { name: "mock-ai-extra", secretRef: "", scopes: ["catalog:read", "access:govern"] },
  openai: { name: "openai-main", secretRef: "", scopes: ["models:read", "access:govern"] },
  anthropic: { name: "anthropic-main", secretRef: "", scopes: ["models:read", "access:govern"] },
  litellm: { name: "litellm-proxy", secretRef: "", scopes: ["keys:mint", "access:govern"] },
};

export function AddAIProviderButton() {
  const router = useRouter();
  const { toast } = useToast();
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [providerType, setProviderType] = useState<ProviderType>("openai");
  const [name, setName] = useState(providerDefaults.openai.name);
  const [secretRef, setSecretRef] = useState(providerDefaults.openai.secretRef);
  const [baseUrl, setBaseUrl] = useState("");
  const [anthropicVersion, setAnthropicVersion] = useState("2023-06-01");

  function changeProviderType(next: ProviderType) {
    setProviderType(next);
    setName(providerDefaults[next].name);
    setSecretRef(providerDefaults[next].secretRef);
    setBaseUrl("");
    setAnthropicVersion("2023-06-01");
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      const config: Record<string, string | boolean> = { governance_only: true };
      if (baseUrl.trim()) config.base_url = baseUrl.trim();
      if (providerType === "anthropic" && anthropicVersion.trim()) config.anthropic_version = anthropicVersion.trim();

      const res = await fetch(`${API}/api/v1/ai/providers`, {
        method: "POST",
        headers: { "Content-Type": "application/json", ...authHeaders() },
        body: JSON.stringify({
          name,
          type: providerType,
          config,
          secretRef: secretRef.trim(),
          scopes: providerDefaults[providerType].scopes,
        }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        toast({ variant: "error", title: "Add failed", description: data.error ?? `Request failed (${res.status})` });
        return;
      }
      toast({ variant: "success", title: `AI provider “${name}” registered` });
      setOpen(false);
      router.refresh();
    } catch (err) {
      toast({ variant: "error", title: "Add failed", description: String(err) });
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <button type="button" onClick={() => setOpen(true)} className={button({ size: "md" })}>
        <Plus className="size-4" />
        Add AI provider
      </button>
      {open &&
        typeof document !== "undefined" &&
        createPortal(
          <div className="fixed inset-0 z-[70] flex items-center justify-center p-4" role="dialog" aria-modal="true">
            <div className="absolute inset-0 bg-black/50" onClick={() => !busy && setOpen(false)} />
            <form onSubmit={submit} className="relative w-full max-w-lg space-y-4 rounded-xl border border-border bg-card p-5 shadow-xl">
              <div>
                <h2 className="text-base font-semibold text-foreground">Add AI provider</h2>
                <p className="mt-1 text-xs text-muted-foreground">
                  OpenAI and Anthropic validate credentials, then create governance catalog services. Access provisioning stays manual in MVP.
                </p>
              </div>
              <label className="flex flex-col gap-1.5">
                <span className="text-xs font-medium text-muted-foreground">Provider type</span>
                <select className={inputCls} value={providerType} onChange={(e) => changeProviderType(e.target.value as ProviderType)}>
                  <option value="openai">OpenAI / ChatGPT</option>
                  <option value="anthropic">Anthropic / Claude Code</option>
                  <option value="litellm">LiteLLM proxy (virtual keys)</option>
                  <option value="mock_ai">MockAI</option>
                </select>
              </label>
              <label className="flex flex-col gap-1.5">
                <span className="text-xs font-medium text-muted-foreground">Name</span>
                <input className={inputCls} value={name} onChange={(e) => setName(e.target.value)} required />
              </label>
              <label className="flex flex-col gap-1.5">
                <span className="text-xs font-medium text-muted-foreground">Secret reference (optional)</span>
                <input
                  className={inputCls}
                  value={secretRef}
                  onChange={(e) => setSecretRef(e.target.value)}
                  placeholder="leave blank to use the env key"
                />
                <span className="text-xs text-muted-foreground">
                  {providerType === "litellm"
                    ? "Enter a KV path (OpenBao/Vault) holding the LiteLLM proxy master_key - OPORD mints scoped virtual keys with it. Set the Base URL to your proxy."
                    : "Leave blank to use the OPENAI_API_KEY / ANTHROPIC_API_KEY env var. For a secret store (OpenBao/Vault), enter a KV path holding api_key / openai_api_key / anthropic_api_key / token."}
                </span>
              </label>
              <label className="flex flex-col gap-1.5">
                <span className="text-xs font-medium text-muted-foreground">Base URL</span>
                <input
                  className={inputCls}
                  value={baseUrl}
                  onChange={(e) => setBaseUrl(e.target.value)}
                  placeholder={providerType === "anthropic" ? "https://api.anthropic.com" : providerType === "litellm" ? "http://litellm:4000" : "https://api.openai.com"}
                />
              </label>
              {providerType === "anthropic" && (
                <label className="flex flex-col gap-1.5">
                  <span className="text-xs font-medium text-muted-foreground">Anthropic API version</span>
                  <input className={inputCls} value={anthropicVersion} onChange={(e) => setAnthropicVersion(e.target.value)} />
                </label>
              )}
              <div className="flex items-center justify-end gap-2">
                <button type="button" onClick={() => setOpen(false)} disabled={busy} className={button({ variant: "outline", size: "sm" })}>
                  Cancel
                </button>
                <button type="submit" disabled={busy} className={cn(button({ size: "sm" }), busy && "opacity-70")}>
                  {busy && <Loader2 className="size-4 animate-spin" />}
                  Add
                </button>
              </div>
            </form>
          </div>,
          document.body,
        )}
    </>
  );
}

export function AIProviderActions({ provider }: { provider: AIProvider }) {
  const router = useRouter();
  const { toast } = useToast();
  const { prompt } = useConfirm();
  const [busyAction, setBusyAction] = useState<"check" | "sync" | "models" | "delete" | null>(null);

  async function del() {
    const typed = await prompt({
      title: `Delete AI provider “${provider.name}”?`,
      message:
        "Removes the provider, its catalog services, credential references, and terminal access history. Active access blocks deletion. Type the provider name to confirm.",
      requireValue: provider.name,
      confirmLabel: "Delete",
      danger: true,
    });
    if (typed == null) return;
    setBusyAction("delete");
    try {
      const res = await fetch(`${API}/api/v1/ai/providers/${encodeURIComponent(provider.name)}`, {
        method: "DELETE",
        headers: authHeaders(),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        toast({ variant: "error", title: "Delete failed", description: data.error ?? `Request failed (${res.status})` });
        return;
      }
      toast({ variant: "success", title: `AI provider “${provider.name}” deleted` });
      router.refresh();
    } catch (err) {
      toast({ variant: "error", title: "Delete failed", description: String(err) });
    } finally {
      setBusyAction(null);
    }
  }

  async function check() {
    setBusyAction("check");
    try {
      const res = await fetch(`${API}/api/v1/ai/providers/${encodeURIComponent(provider.name)}/check`, {
        method: "POST",
        headers: authHeaders(),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        toast({ variant: "error", title: "Check failed", description: data.error ?? `Request failed (${res.status})` });
        return;
      }
      toast({ variant: "success", title: `“${provider.name}” checked`, description: `${provider.type} credentials are valid.` });
      router.refresh();
    } catch (err) {
      toast({ variant: "error", title: "Check failed", description: String(err) });
    } finally {
      setBusyAction(null);
    }
  }

  async function sync() {
    setBusyAction("sync");
    try {
      const res = await fetch(`${API}/api/v1/ai/providers/${encodeURIComponent(provider.name)}/sync`, {
        method: "POST",
        headers: authHeaders(),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        toast({ variant: "error", title: "Sync failed", description: data.error ?? `Request failed (${res.status})` });
        return;
      }
      toast({ variant: "success", title: `“${provider.name}” synced`, description: "Catalog services are up to date." });
      router.refresh();
    } catch (err) {
      toast({ variant: "error", title: "Sync failed", description: String(err) });
    } finally {
      setBusyAction(null);
    }
  }

  async function syncModels() {
    setBusyAction("models");
    try {
      const res = await fetch(`${API}/api/v1/ai/providers/${encodeURIComponent(provider.name)}/sync-models`, {
        method: "POST",
        headers: authHeaders(),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        toast({ variant: "error", title: "Model sync failed", description: data.error ?? `Request failed (${res.status})` });
        return;
      }
      toast({ variant: "success", title: `“${provider.name}” models synced`, description: "Model catalog is up to date." });
      router.refresh();
    } catch (err) {
      toast({ variant: "error", title: "Model sync failed", description: String(err) });
    } finally {
      setBusyAction(null);
    }
  }

  return (
    <div className="flex justify-end gap-2">
      <button
        type="button"
        onClick={sync}
        disabled={busyAction !== null}
        className={cn(button({ variant: "outline", size: "sm" }), busyAction && "opacity-70")}
      >
        {busyAction === "sync" ? <Loader2 className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
        Sync
      </button>
      <button
        type="button"
        onClick={syncModels}
        disabled={busyAction !== null}
        className={cn(button({ variant: "outline", size: "sm" }), busyAction && "opacity-70")}
      >
        {busyAction === "models" ? <Loader2 className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
        Models
      </button>
      <button
        type="button"
        onClick={check}
        disabled={busyAction !== null}
        className={cn(button({ variant: "outline", size: "sm" }), busyAction && "opacity-70")}
      >
        {busyAction === "check" ? <Loader2 className="size-4 animate-spin" /> : <Activity className="size-4" />}
        Check
      </button>
      <EditAIProviderButton provider={provider} disabled={busyAction !== null} />
      <button
        type="button"
        onClick={del}
        disabled={busyAction !== null}
        className={cn(button({ variant: "outline", size: "sm" }), "text-danger", busyAction && "opacity-70")}
      >
        {busyAction === "delete" ? <Loader2 className="size-4 animate-spin" /> : <Trash2 className="size-4" />}
        Delete
      </button>
    </div>
  );
}

// EditAIProviderButton edits the mutable provider settings: status, base_url, and
// the credential reference. Submitting a new secret_ref records a NEW credential
// row (rotation - the resolver reads the latest); "switch to env key" records an
// empty ref so the provider falls back to OPENAI_API_KEY / ANTHROPIC_API_KEY.
function EditAIProviderButton({ provider, disabled }: { provider: AIProvider; disabled: boolean }) {
  const router = useRouter();
  const { toast } = useToast();
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const currentBaseUrl = typeof provider.config?.base_url === "string" ? provider.config.base_url : "";
  const [baseUrl, setBaseUrl] = useState(currentBaseUrl);
  const [status, setStatus] = useState(provider.status);
  const [secretRef, setSecretRef] = useState("");
  const [clearSecret, setClearSecret] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    const body: { config?: Record<string, string | null>; status?: string; secretRef?: string } = {};
    if (baseUrl.trim() !== currentBaseUrl) body.config = { base_url: baseUrl.trim() === "" ? null : baseUrl.trim() };
    if (status !== provider.status) body.status = status;
    if (clearSecret) body.secretRef = "";
    else if (secretRef.trim() !== "") body.secretRef = secretRef.trim();
    if (Object.keys(body).length === 0) {
      toast({ variant: "info", title: "Nothing to update" });
      setOpen(false);
      return;
    }
    setBusy(true);
    try {
      const res = await fetch(`${API}/api/v1/ai/providers/${encodeURIComponent(provider.name)}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json", ...authHeaders() },
        body: JSON.stringify(body),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        toast({ variant: "error", title: "Update failed", description: data.error ?? `Request failed (${res.status})` });
        return;
      }
      toast({ variant: "success", title: `AI provider “${provider.name}” updated` });
      setOpen(false);
      router.refresh();
    } catch (err) {
      toast({ variant: "error", title: "Update failed", description: String(err) });
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        disabled={disabled}
        className={button({ variant: "outline", size: "sm" })}
      >
        <Pencil className="size-4" />
        Edit
      </button>
      {open &&
        typeof document !== "undefined" &&
        createPortal(
          <div className="fixed inset-0 z-[70] flex items-center justify-center p-4" role="dialog" aria-modal="true">
            <div className="absolute inset-0 bg-black/50" onClick={() => !busy && setOpen(false)} />
            <form onSubmit={submit} className="relative w-full max-w-md space-y-4 rounded-xl border border-border bg-card p-5 shadow-xl">
              <div>
                <h2 className="text-base font-semibold text-foreground">Edit {provider.name}</h2>
                <p className="mt-1 text-xs text-muted-foreground">{provider.type} · name and type are immutable</p>
              </div>
              <label className="flex flex-col gap-1.5">
                <span className="text-xs font-medium text-muted-foreground">Status</span>
                <select className={inputCls} value={status} onChange={(e) => setStatus(e.target.value)}>
                  <option value="active">active</option>
                  <option value="disabled">disabled</option>
                </select>
              </label>
              <label className="flex flex-col gap-1.5">
                <span className="text-xs font-medium text-muted-foreground">Base URL</span>
                <input
                  className={inputCls}
                  value={baseUrl}
                  onChange={(e) => setBaseUrl(e.target.value)}
                  placeholder={provider.type === "anthropic" ? "https://api.anthropic.com" : "https://api.openai.com"}
                />
                <span className="text-xs text-muted-foreground">Leave empty to use the provider default.</span>
              </label>
              <label className="flex flex-col gap-1.5">
                <span className="text-xs font-medium text-muted-foreground">Rotate secret reference</span>
                <input
                  className={inputCls}
                  value={secretRef}
                  onChange={(e) => setSecretRef(e.target.value)}
                  placeholder="empty = keep current"
                  disabled={clearSecret}
                />
              </label>
              <label className="flex items-center gap-2 text-xs text-muted-foreground">
                <input type="checkbox" checked={clearSecret} onChange={(e) => setClearSecret(e.target.checked)} />
                Switch to the env key ({provider.type === "anthropic" ? "ANTHROPIC_API_KEY" : "OPENAI_API_KEY"})
              </label>
              <div className="flex items-center justify-end gap-2">
                <button type="button" onClick={() => setOpen(false)} disabled={busy} className={button({ variant: "outline", size: "sm" })}>
                  Cancel
                </button>
                <button type="submit" disabled={busy} className={cn(button({ size: "sm" }), busy && "opacity-70")}>
                  {busy && <Loader2 className="size-4 animate-spin" />}
                  Save
                </button>
              </div>
            </form>
          </div>,
          document.body,
        )}
    </>
  );
}

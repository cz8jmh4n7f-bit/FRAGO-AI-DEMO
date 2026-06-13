"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { createPortal } from "react-dom";
import { Loader2, Send } from "lucide-react";
import type { AIService } from "@/lib/types";
import { authHeaders } from "@/lib/client-auth";
import { button } from "@/components/ui/button";
import { useToast } from "@/components/ui/toast";
import { cn } from "@/lib/utils";

const API = "/bff";
const inputCls =
  "h-9 w-full rounded-lg border border-input bg-card px-3 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

export function AIRequestButton({ service }: { service: AIService }) {
  const router = useRouter();
  const { toast } = useToast();
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [name, setName] = useState(`${service.slug}-request`);
  const [requester, setRequester] = useState("");
  const [owner, setOwner] = useState("");
  const [workspace, setWorkspace] = useState("default");
  const [justification, setJustification] = useState("");
  // LiteLLM virtual-key scoping (only shown for that service): the minted key is
  // restricted to these models + budget.
  const isLiteLLMKey = service.slug === "litellm-virtual-key";
  const [models, setModels] = useState("");
  const [maxBudget, setMaxBudget] = useState("");

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      const metadata: Record<string, unknown> = {};
      if (isLiteLLMKey) {
        const list = models.split(",").map((m) => m.trim()).filter(Boolean);
        if (list.length) metadata.models = list;
        if (maxBudget.trim()) metadata.max_budget = Number(maxBudget);
      }
      const res = await fetch(`${API}/api/v1/ai/requests`, {
        method: "POST",
        headers: { "Content-Type": "application/json", ...authHeaders() },
        body: JSON.stringify({
          name,
          requester,
          serviceId: service.id,
          serviceSlug: service.slug,
          owner,
          workspace,
          justification,
          ...(Object.keys(metadata).length ? { metadata } : {}),
        }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        toast({ variant: "error", title: "Request failed", description: data.error ?? `Request failed (${res.status})` });
        return;
      }
      toast({ variant: "success", title: `AI request “${data.name}” submitted` });
      setOpen(false);
      router.push("/ai/requests");
      router.refresh();
    } catch (err) {
      toast({ variant: "error", title: "Request failed", description: String(err) });
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <button type="button" onClick={() => setOpen(true)} className={button({ size: "sm" })}>
        <Send className="size-4" />
        Request
      </button>
      {open &&
        typeof document !== "undefined" &&
        createPortal(
          <div className="fixed inset-0 z-[70] flex items-center justify-center p-4" role="dialog" aria-modal="true">
            <div className="absolute inset-0 bg-black/50" onClick={() => !busy && setOpen(false)} />
            <form onSubmit={submit} className="relative w-full max-w-md space-y-4 rounded-xl border border-border bg-card p-5 shadow-xl">
              <div>
                <h2 className="text-base font-semibold text-foreground">Request {service.name}</h2>
                <p className="mt-1 text-xs text-muted-foreground">{service.providerName} · {service.category}</p>
              </div>
              <label className="flex flex-col gap-1.5">
                <span className="text-xs font-medium text-muted-foreground">Request name</span>
                <input className={inputCls} value={name} onChange={(e) => setName(e.target.value)} required />
              </label>
              <label className="flex flex-col gap-1.5">
                <span className="text-xs font-medium text-muted-foreground">Requester</span>
                <input className={inputCls} value={requester} onChange={(e) => setRequester(e.target.value)} placeholder="you@example.com" />
              </label>
              <label className="flex flex-col gap-1.5">
                <span className="text-xs font-medium text-muted-foreground">Owner</span>
                <input className={inputCls} value={owner} onChange={(e) => setOwner(e.target.value)} placeholder="team or owner email" />
              </label>
              <label className="flex flex-col gap-1.5">
                <span className="text-xs font-medium text-muted-foreground">Workspace</span>
                <input className={inputCls} value={workspace} onChange={(e) => setWorkspace(e.target.value)} />
              </label>
              <label className="flex flex-col gap-1.5">
                <span className="text-xs font-medium text-muted-foreground">Justification</span>
                <textarea
                  className="min-h-24 rounded-lg border border-input bg-card px-3 py-2 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  value={justification}
                  onChange={(e) => setJustification(e.target.value)}
                />
              </label>
              {isLiteLLMKey && (
                <div className="grid grid-cols-2 gap-2 rounded-lg border border-border bg-muted/30 p-3">
                  <label className="col-span-2 text-xs font-medium text-muted-foreground">Scope the minted key (LiteLLM enforces these at runtime)</label>
                  <label className="flex flex-col gap-1">
                    <span className="text-xs text-muted-foreground">Models (comma-separated, empty = all)</span>
                    <input className={inputCls} value={models} onChange={(e) => setModels(e.target.value)} placeholder="gpt-4o, mock-gpt" />
                  </label>
                  <label className="flex flex-col gap-1">
                    <span className="text-xs text-muted-foreground">Max budget (USD, empty = none)</span>
                    <input className={inputCls} type="number" min="0" step="0.01" value={maxBudget} onChange={(e) => setMaxBudget(e.target.value)} placeholder="50" />
                  </label>
                </div>
              )}
              <div className="flex items-center justify-end gap-2">
                <button type="button" onClick={() => setOpen(false)} className={button({ variant: "outline", size: "sm" })} disabled={busy}>
                  Cancel
                </button>
                <button type="submit" className={cn(button({ size: "sm" }), busy && "opacity-70")} disabled={busy}>
                  {busy && <Loader2 className="size-4 animate-spin" />}
                  Submit
                </button>
              </div>
            </form>
          </div>,
          document.body,
        )}
    </>
  );
}

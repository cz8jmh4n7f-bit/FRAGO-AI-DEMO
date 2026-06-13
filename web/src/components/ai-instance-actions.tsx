"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Loader2, Ban, KeyRound } from "lucide-react";
import { authHeaders } from "@/lib/client-auth";
import { button } from "@/components/ui/button";
import { useConfirm } from "@/components/ui/confirm";
import { useToast } from "@/components/ui/toast";
import { cn } from "@/lib/utils";

const API = "/bff";

export function AIInstanceActions({ id, status }: { id: string; status: string }) {
  const router = useRouter();
  const { confirm } = useConfirm();
  const { toast } = useToast();
  const [busy, setBusy] = useState(false);
  const [revealed, setRevealed] = useState<string | null>(null);

  if (!["active", "suspended"].includes(status)) {
    return <span className="text-xs text-muted-foreground">-</span>;
  }

  // RevealKey fetches a provider-minted credential (e.g. a LiteLLM virtual key)
  // from the secret store on demand. Gracefully no-ops for instances without one.
  async function revealKey() {
    setBusy(true);
    try {
      const res = await fetch(`${API}/api/v1/ai/instances/${encodeURIComponent(id)}/secret`, { headers: authHeaders() });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        toast({ variant: data.error?.includes("no stored") ? "info" : "error", title: "No key to reveal", description: data.error ?? `(${res.status})` });
        return;
      }
      setRevealed(data.key);
      try {
        await navigator.clipboard.writeText(data.key);
        toast({ variant: "success", title: "Key revealed + copied to clipboard" });
      } catch {
        toast({ variant: "success", title: "Key revealed" });
      }
    } catch (err) {
      toast({ variant: "error", title: "Reveal failed", description: String(err) });
    } finally {
      setBusy(false);
    }
  }

  async function revoke() {
    const ok = await confirm({
      title: "Revoke AI access?",
      message: "The mock provider will mark this access instance as revoked.",
      confirmLabel: "Revoke",
      danger: true,
    });
    if (!ok) return;
    setBusy(true);
    try {
      const res = await fetch(`${API}/api/v1/ai/instances/${encodeURIComponent(id)}/revoke`, {
        method: "POST",
        headers: { "Content-Type": "application/json", ...authHeaders() },
        body: JSON.stringify({ by: "web" }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        toast({ variant: "error", title: "Revoke failed", description: data.error ?? `Request failed (${res.status})` });
        return;
      }
      toast({ variant: "success", title: "AI access revoked" });
      router.refresh();
    } catch (err) {
      toast({ variant: "error", title: "Revoke failed", description: String(err) });
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex items-center justify-end gap-2">
      {revealed && <code className="max-w-40 truncate rounded bg-muted px-2 py-1 font-mono text-xs" title={revealed}>{revealed}</code>}
      <button type="button" onClick={revealKey} disabled={busy} className={button({ variant: "outline", size: "sm" })}>
        <KeyRound className="size-4" />
        Reveal key
      </button>
      <button type="button" onClick={revoke} disabled={busy} className={cn(button({ variant: "danger", size: "sm" }), busy && "opacity-70")}>
        {busy ? <Loader2 className="size-4 animate-spin" /> : <Ban className="size-4" />}
        Revoke
      </button>
    </div>
  );
}

"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { CheckCircle2, Loader2 } from "lucide-react";
import { authHeaders } from "@/lib/client-auth";
import { button } from "@/components/ui/button";
import { useConfirm } from "@/components/ui/confirm";
import { useToast } from "@/components/ui/toast";

const API = "/bff";

// RecertifyButton re-justifies an AI access grant for another window (the access
// review attestation), extending its expiry by the chosen number of days.
export function RecertifyButton({ id }: { id: string }) {
  const router = useRouter();
  const { toast } = useToast();
  const { prompt } = useConfirm();
  const [busy, setBusy] = useState(false);

  async function recertify() {
    const days = await prompt({
      title: "Recertify access",
      message: "Confirm this access is still needed and extend it. Days to extend:",
      defaultValue: "90",
      confirmLabel: "Recertify",
    });
    if (days == null) return;
    const n = Number(days);
    if (!Number.isFinite(n) || n <= 0) {
      toast({ variant: "error", title: "Enter a positive number of days" });
      return;
    }
    setBusy(true);
    try {
      const res = await fetch(`${API}/api/v1/ai/instances/${encodeURIComponent(id)}/recertify`, {
        method: "POST",
        headers: { "Content-Type": "application/json", ...authHeaders() },
        body: JSON.stringify({ extendDays: n, by: "web" }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        toast({ variant: "error", title: "Recertify failed", description: data.error ?? `(${res.status})` });
        return;
      }
      toast({ variant: "success", title: `Recertified for ${n} days` });
      router.refresh();
    } finally {
      setBusy(false);
    }
  }

  return (
    <button type="button" onClick={recertify} disabled={busy} className={button({ variant: "outline", size: "sm" })}>
      {busy ? <Loader2 className="size-4 animate-spin" /> : <CheckCircle2 className="size-4" />}
      Recertify
    </button>
  );
}
